package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/klyzar/hub-api/alerting"
	"github.com/klyzar/hub-api/baseline"
	"github.com/klyzar/hub-api/clickhouse"
	"github.com/klyzar/hub-api/cloud"
	"github.com/klyzar/hub-api/config"
	"github.com/klyzar/hub-api/enterprise/compliance"
	"github.com/klyzar/hub-api/enterprise/incidents"
	"github.com/klyzar/hub-api/enterprise/license"
	"github.com/klyzar/hub-api/enterprise/scim"
	"github.com/klyzar/hub-api/enterprise/sigma"
	"github.com/klyzar/hub-api/enterprise/sinks"
	"github.com/klyzar/hub-api/enterprise/sso"
	"github.com/klyzar/hub-api/enterprise/storage"
	"github.com/klyzar/hub-api/geoip"
	"github.com/klyzar/hub-api/handlers"
	"github.com/klyzar/hub-api/kafka"
	nsmetrics "github.com/klyzar/hub-api/metrics"
	"github.com/klyzar/hub-api/middleware"
	"github.com/klyzar/hub-api/models"
	"github.com/klyzar/hub-api/pubsub"
	"github.com/klyzar/hub-api/sessions"
	"github.com/klyzar/hub-api/threat"
)

// sigmaDispatcherAdapter adapts incidents.Dispatcher to the sigma.Dispatcher
// interface without creating an import cycle between the two packages.
type sigmaDispatcherAdapter struct {
	d *incidents.Dispatcher
}

func (a sigmaDispatcherAdapter) Dispatch(ctx context.Context, ev sigma.DispatchEvent) {
	a.d.Dispatch(ctx, incidents.SigmaMatchEvent{
		RuleID:    ev.RuleID,
		RuleTitle: ev.RuleTitle,
		Severity:  ev.Severity,
		SrcIP:     ev.SrcIP,
		DstIP:     ev.DstIP,
		FiredAt:   ev.FiredAt,
	})
}

func main() {
	// Structured JSON logging to stdout
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	cfg := config.Load()

	// ── ClickHouse ────────────────────────────────────────────────────────────
	var chClient *clickhouse.Client
	var chWriter *clickhouse.Writer

	for attempt := 1; attempt <= 6; attempt++ {
		slog.Info("connecting to ClickHouse", "attempt", attempt)
		var err error
		chClient, err = clickhouse.New(cfg.ClickHouseDSN)
		if err == nil {
			pingCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			err = chClient.Ping(pingCtx)
			cancel()
		}
		if err != nil {
			slog.Warn("ClickHouse not ready", "err", err)
			chClient = nil
			if attempt < 6 {
				time.Sleep(time.Duration(attempt) * 2 * time.Second)
			}
			continue
		}
		slog.Info("ClickHouse connected")
		break
	}

	if chClient != nil {
		if err := runMigrations(chClient); err != nil {
			slog.Error("schema migration failed", "err", err)
			os.Exit(1)
		}
		chWriter = clickhouse.NewWriter(chClient)
		defer chWriter.Stop()
	} else {
		slog.Warn("ClickHouse unavailable — flows will not be persisted to disk")
	}

	// ── Kafka / Redpanda ──────────────────────────────────────────────────────
	var producer *kafka.Producer
	var consumer *kafka.Consumer

	if prod, err := kafka.NewProducer(cfg.KafkaBrokers, cfg.KafkaTopic); err != nil {
		slog.Warn("Kafka producer unavailable (will write directly to ClickHouse)", "err", err)
	} else {
		producer = prod
		slog.Info("Kafka producer connected", "brokers", cfg.KafkaBrokers)
		defer producer.Close()
	}

	if cons, err := kafka.NewConsumer(cfg.KafkaBrokers, cfg.KafkaTopic, cfg.KafkaGroupID); err != nil {
		slog.Warn("Kafka consumer unavailable", "err", err)
	} else {
		consumer = cons
		slog.Info("Kafka consumer connected")
		defer consumer.Close()
	}

	// Kafka → ClickHouse bridge goroutine
	consCtx, consCancel := context.WithCancel(context.Background())
	defer consCancel()

	// flowH is declared here so the Kafka goroutine can reference it.
	// It is fully initialised below, before any requests are served.
	var flowH *handlers.FlowHandler

	if consumer != nil && chWriter != nil {
		go func() {
			if err := consumer.Consume(consCtx, func(flow models.Flow) {
				chWriter.Write(flow)
				if flowH != nil {
					flowH.BroadcastFlow(flow)
				}
			}); err != nil && err != context.Canceled {
				slog.Error("Kafka consumer exited", "err", err)
			}
		}()
	}

	// ── Geo-IP + Threat scoring ───────────────────────────────────────────────
	geoReader := geoip.New(cfg.GeoIPCityDB, cfg.GeoIPAsnDB)
	defer geoReader.Close()

	threatScorer := threat.New()
	if cfg.AbuseIPDBKey != "" {
		threatScorer.SetAbuseIPDBKey(cfg.AbuseIPDBKey)
	}
	if cfg.ThreatBlocklist != "" {
		if err := threatScorer.LoadBlocklist(cfg.ThreatBlocklist); err != nil {
			slog.Warn("threat blocklist load failed", "path", cfg.ThreatBlocklist, "err", err)
		}
	}

	// ── Enterprise license ────────────────────────────────────────────────────
	lic := license.Parse(cfg.EnterpriseLicenseKey, cfg.EnterpriseLicenseSigningKey)
	slog.Info("enterprise license loaded",
		"plan", lic.Plan,
		"valid", lic.Valid,
		"agent_quota", lic.AgentQuota,
	)

	// ── Session store (in-memory, survives until hub restart) ─────────────────
	sessionStore := sessions.NewStore()

	// ── SSE broadcast hub ─────────────────────────────────────────────────────
	flowHub := pubsub.NewInMemoryHub()

	// ── Alert evaluator ───────────────────────────────────────────────────────
	var evaluator *alerting.Evaluator
	if chClient != nil {
		evaluator = alerting.NewEvaluator(chClient, 60*time.Second)
		evaluator.SMTP = alerting.SMTPConfig{
			Host:     cfg.SMTPHost,
			Port:     cfg.SMTPPort,
			User:     cfg.SMTPUser,
			Password: cfg.SMTPPassword,
			From:     cfg.SMTPFrom,
			OrgName:  cfg.OrgName,
			AppURL:   cfg.AppURL,
		}
		evaluator.Start()
		defer evaluator.Stop()
		slog.Info("alert evaluator started")

		if cfg.ReportEmail != "" && cfg.SMTPHost != "" {
			reporter := alerting.NewReporter(chClient, alerting.ReportConfig{
				SMTP: alerting.SMTPConfig{
					Host:     cfg.SMTPHost,
					Port:     cfg.SMTPPort,
					User:     cfg.SMTPUser,
					Password: cfg.SMTPPassword,
					From:     cfg.SMTPFrom,
					OrgName:  cfg.OrgName,
					AppURL:   cfg.AppURL,
				},
				Email:    cfg.ReportEmail,
				Schedule: cfg.ReportSchedule,
			})
			reporter.Start()
			defer reporter.Stop()
			slog.Info("report scheduler started",
				"schedule", cfg.ReportSchedule,
				"recipient", cfg.ReportEmail)
		}
	}

	// ── Fiber ─────────────────────────────────────────────────────────────────
	app := fiber.New(fiber.Config{
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 60 * time.Second,
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			code := fiber.StatusInternalServerError
			if e, ok := err.(*fiber.Error); ok {
				code = e.Code
			}
			return c.Status(code).JSON(fiber.Map{"error": err.Error()})
		},
	})

	app.Use(recover.New())
	app.Use(logger.New(logger.Config{
		Format: "${time} ${method} ${path} ${status} ${latency}\n",
	}))
	app.Use(cors.New(cors.Config{
		AllowOrigins: cfg.AllowedOrigins,
		AllowHeaders: "Origin, Content-Type, Accept, X-Api-Key",
		AllowMethods: "GET,POST,PATCH,DELETE,OPTIONS",
	}))

	// ── Security response headers ─────────────────────────────────────────────
	app.Use(func(c *fiber.Ctx) error {
		c.Set("X-Content-Type-Options", "nosniff")
		c.Set("X-Frame-Options", "DENY")
		c.Set("X-XSS-Protection", "1; mode=block")
		c.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		c.Set("Permissions-Policy", "geolocation=(), microphone=(), camera=()")
		return c.Next()
	})

	// Count every request
	app.Use(func(c *fiber.Ctx) error {
		nsmetrics.APIRequestsTotal.Add(1)
		return c.Next()
	})

	// Public endpoints
	app.Get("/health", func(c *fiber.Ctx) error {
		status := "ok"
		if chClient == nil {
			status = "degraded"
		}
		return c.JSON(fiber.Map{"status": status, "version": version})
	})
	app.Get("/metrics", func(c *fiber.Ctx) error {
		// Optional bearer-token protection.  Set METRICS_TOKEN to require
		// "Authorization: Bearer <token>" on Prometheus scrape jobs.
		if cfg.MetricsToken != "" {
			auth := c.Get("Authorization")
			if auth != "Bearer "+cfg.MetricsToken {
				return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
			}
		}
		c.Set("Content-Type", "text/plain; version=0.0.4")
		return c.SendString(nsmetrics.Text())
	})

	// Protected API routes — TokenAuth checks bootstrap key OR api_tokens table
	auth     := middleware.TokenAuth(cfg.APIKey, chClient)
	auditLog := middleware.AuditLog(chClient)
	// Ingest gets a generous limit (agents post many flows); general API is tighter.
	ingestLimit := middleware.RateLimit(50_000, time.Minute)
	apiLimit    := middleware.RateLimit(2_000, time.Minute)

	v1 := app.Group("/api/v1", auth, auditLog)

	flowH = &handlers.FlowHandler{CH: chClient, Writer: chWriter, Producer: producer, CertsCH: chClient, GeoIP: geoReader, Threat: threatScorer, Hub: flowHub}
	agentH     := &handlers.AgentHandler{CH: chClient}
	metricsH   := &handlers.MetricsHandler{CH: chClient}
	statsH     := &handlers.StatsHandler{CH: chClient}
	alertH     := &handlers.AlertHandler{CH: chClient, Evaluator: evaluator}
	policyH    := &handlers.PolicyHandler{CH: chClient}
	threatH    := &handlers.ThreatHandler{CH: chClient}
	servicesH  := &handlers.ServicesHandler{CH: chClient}
	analyticsH := &handlers.AnalyticsHandler{CH: chClient}
	otelH      := &handlers.OtelHandler{CH: chClient}
	enrollH    := &handlers.EnrollmentHandler{CH: chClient, Cfg: cfg}
	certH      := &handlers.CertHandler{CH: chClient}
	tokenH     := &handlers.TokenHandler{CH: chClient}
	complianceH := &handlers.ComplianceHandler{CH: chClient}
	auditH     := &handlers.AuditHandler{CH: chClient, License: lic}

	// ── Public (no auth) ──────────────────────────────────────────────────────
	app.Post("/api/v1/agents/enroll", apiLimit, enrollH.Enroll)
	app.Get("/install",              apiLimit, enrollH.InstallScript)

	v1.Post("/ingest",                    ingestLimit,                         flowH.Ingest)
	v1.Get("/flows",                      apiLimit,                            flowH.Query)
	v1.Get("/flows/stream",               apiLimit,                            flowH.Stream)
	v1.Get("/stats",                      apiLimit,                            statsH.Stats)
	v1.Get("/agents",                     apiLimit,                            agentH.List)
	v1.Post("/agents/register",           apiLimit, middleware.RequireAdmin(), agentH.Register)
	v1.Post("/agents/heartbeat",          apiLimit,                            agentH.Heartbeat)
	v1.Get("/alerts",                     apiLimit,                            alertH.ListRules)
	v1.Post("/alerts",                    apiLimit, middleware.RequireAdmin(), alertH.CreateRule)
	v1.Patch("/alerts/:id",               apiLimit, middleware.RequireAdmin(), alertH.UpdateRule)
	v1.Delete("/alerts/:id",              apiLimit, middleware.RequireAdmin(), alertH.DeleteRule)
	v1.Get("/alerts/events",              apiLimit,                            alertH.ListEvents)
	// Phase 5
	v1.Get("/services/graph",             apiLimit,                            servicesH.Graph)
	v1.Get("/analytics/endpoints",        apiLimit,                            analyticsH.Endpoints)
	v1.Get("/otel/traces",                apiLimit,                            otelH.ExportTraces)
	// Phase 6
	v1.Get("/enrollment-tokens",          apiLimit, middleware.RequireAdmin(), enrollH.ListTokens)
	v1.Post("/enrollment-tokens",         apiLimit, middleware.RequireAdmin(), enrollH.CreateToken)
	v1.Delete("/enrollment-tokens/:id",   apiLimit, middleware.RequireAdmin(), enrollH.RevokeToken)
	v1.Get("/certs",                      apiLimit,                            certH.List)
	v1.Get("/tokens",                     apiLimit, middleware.RequireAdmin(), tokenH.List)
	v1.Post("/tokens",                    apiLimit, middleware.RequireAdmin(), tokenH.Create)
	v1.Delete("/tokens/:id",              apiLimit, middleware.RequireAdmin(), tokenH.Revoke)
	// Phase 7 — compliance
	v1.Get("/compliance/summary",         apiLimit, complianceH.Summary)
	v1.Get("/compliance/connections",     apiLimit, complianceH.Connections)
	v1.Get("/compliance/tls",             apiLimit, complianceH.TLSAudit)
	v1.Get("/compliance/top-talkers",     apiLimit, complianceH.TopTalkers)
	v1.Get("/compliance/external",        apiLimit, complianceH.ExternalConnections)
	// Phase 8 — geo enrichment
	v1.Get("/compliance/geo",             apiLimit, complianceH.GeoSummary)
	// Phase 9 — audit log
	v1.Get("/audit",                      apiLimit, middleware.RequireAdmin(), auditH.List)
	// Phase 10 — metrics timeseries
	v1.Get("/metrics/timeseries",         apiLimit,                            metricsH.Timeseries)
	v1.Get("/metrics/protocols",          apiLimit,                            metricsH.ProtocolBreakdown)
	// Phase 11 — process policies
	v1.Get("/policies",                   apiLimit,                            policyH.List)
	v1.Post("/policies",                  apiLimit, middleware.RequireAdmin(), policyH.Create)
	v1.Patch("/policies/:id",             apiLimit, middleware.RequireAdmin(), policyH.Update)
	v1.Delete("/policies/:id",            apiLimit, middleware.RequireAdmin(), policyH.Delete)
	v1.Get("/policies/violations",        apiLimit,                            policyH.ListViolations)
	// Phase 11 — threat intel
	v1.Get("/threats",                    apiLimit,                            threatH.Summary)
	// Phase 11 — alert test delivery
	v1.Post("/alerts/:id/test",           apiLimit, middleware.RequireAdmin(), alertH.TestDelivery)
	// Phase 11 — agent flow count
	v1.Get("/agents/stats",               apiLimit,                            agentH.Stats)
	v1.Get("/agents/:id/perf",            apiLimit,                            agentH.PerfHistory)
	replayH    := &handlers.ReplayHandler{CH: chClient}
	inventoryH := &handlers.InventoryHandler{CH: chClient}
	v1.Get("/replay",                     apiLimit,                            replayH.Timeline)
	// v0.7 V3 — Passive API Inventory
	v1.Get("/inventory/endpoints",        apiLimit,                            inventoryH.Endpoints)
	// v0.7 V4 — Alert resolve
	v1.Post("/alerts/:id/resolve",        apiLimit, middleware.RequireAdmin(), alertH.Resolve)

	// ── Phase 12 — Enterprise: org, members, teams, SSO config, license ──────
	// Seed the initial local admin account if ADMIN_EMAIL + ADMIN_PASSWORD are set.
	if chClient != nil && cfg.AdminEmail != "" && cfg.AdminPassword != "" {
		if err := seedAdmin(chClient, cfg.AdminEmail, cfg.AdminPassword); err != nil {
			slog.Warn("admin seed failed", "err", err)
		}
	}

	smtpCfg := alerting.SMTPConfig{
		Host:     cfg.SMTPHost,
		Port:     cfg.SMTPPort,
		User:     cfg.SMTPUser,
		Password: cfg.SMTPPassword,
		From:     cfg.SMTPFrom,
		OrgName:  cfg.OrgName,
		AppURL:   cfg.AppURL,
	}

	enterpriseH := &handlers.EnterpriseHandler{
		CH:          chClient,
		License:     lic,
		Sessions:    sessionStore,
		SMTP:        smtpCfg,
		FrontendURL: cfg.FrontendURL,
	}
	authH    := &handlers.AuthHandler{
		CH:                 chClient,
		Sessions:           sessionStore,
		FrontendURL:        cfg.FrontendURL,
		AppURL:             cfg.AppURL,
		DemoEnabled:        cfg.DemoEnabled,
		GoogleClientID:     cfg.GoogleClientID,
		GoogleClientSecret: cfg.GoogleClientSecret,
		SecureCookie:       cfg.Production,
	}
	inviteH  := &handlers.InviteHandler{CH: chClient, Sessions: sessionStore, SMTP: smtpCfg, FrontendURL: cfg.FrontendURL, SecureCookie: cfg.Production}
	scimH    := &scim.Handler{CH: chClient, License: lic, BearerToken: cfg.SCIMBearerToken}

	// ── SIEM sink dispatcher ──────────────────────────────────────────────────
	sinksDispatcher := sinks.New(chClient)
	if chClient != nil {
		sinksDispatcher.Start()
		defer sinksDispatcher.Stop()
	}
	integrationsH := &handlers.IntegrationsHandler{
		CH:      chClient,
		License: lic,
		Sinks:   sinksDispatcher,
	}

	// ── Sigma detection engine ────────────────────────────────────────────────
	sigmaEngine := sigma.New(chClient)
	if chClient != nil {
		sigmaEngine.Start()
		defer sigmaEngine.Stop()
	}
	sigmaH := &handlers.SigmaHandler{CH: chClient, License: lic, Engine: sigmaEngine}
	savedQueriesH := &handlers.SavedQueryHandler{CH: chClient, License: lic}

	// ── Long-term storage exporter (S3/GCS) ───────────────────────────────────
	storageExporter := storage.New(chClient)
	if chClient != nil && lic.Plan == "enterprise" {
		storageExporter.Start()
		defer storageExporter.Stop()
	}
	storageH := &handlers.StorageHandler{CH: chClient, License: lic}

	// ── v0.6: AI Security Copilot ────────────────────────────────────────────
	copilotH := &handlers.CopilotHandler{CH: chClient, AnthropicKey: cfg.AnthropicKey}
	if cfg.AnthropicKey != "" {
		slog.Info("AI Copilot enabled")
	} else {
		slog.Info("AI Copilot disabled — set ANTHROPIC_API_KEY to enable")
	}

	// ── v0.6: Behavioral Baseline + Anomaly Detection ────────────────────────
	baselineEngine := baseline.New(chClient)
	if chClient != nil {
		baselineEngine.Start()
		defer baselineEngine.Stop()
		slog.Info("baseline engine started")
	}

	// ── v0.5: Cloud VPC Flow Log Ingestion ────────────────────────────────────
	cloudIngester := cloud.New(chClient, lic)
	cloudIngester.Start()
	defer cloudIngester.Stop()
	cloudH := &handlers.CloudSourceHandler{CH: chClient, License: lic}

	// ── v0.5: Multi-Cluster Fleet Overview ────────────────────────────────────
	fleetH := &handlers.FleetHandler{CH: chClient}

	// ── v0.5: Compliance Reports (Enterprise) ─────────────────────────────────
	complianceScheduler := compliance.New(chClient, lic, nil) // nil = no SMTP (use API download)
	complianceScheduler.Start()
	defer complianceScheduler.Stop()
	complianceReportH := &handlers.ComplianceReportHandler{CH: chClient, License: lic}
	anomalyH          := &handlers.AnomalyHandler{CH: chClient}

	// ── v0.5: Incident Workflow (Enterprise) ──────────────────────────────────
	incidentDispatcher := incidents.New(chClient, lic)
	// Adapt incidents.Dispatcher to sigma.Dispatcher interface (no import cycle).
	sigmaEngine.SetDispatcher(sigmaDispatcherAdapter{d: incidentDispatcher})
	incidentH := &handlers.IncidentHandler{CH: chClient, License: lic}

	oidcH    := sso.NewOIDCHandler(chClient, sessionStore, lic,
		cfg.AppURL, cfg.FrontendURL, cfg.SSOClientSecret, cfg.Production)
	samlH    := sso.NewSAMLHandler(chClient, sessionStore, lic,
		cfg.AppURL, cfg.FrontendURL, cfg.Production)

	// ── Public auth endpoints (no API key required) ───────────────────────────
	app.Get( "/api/v1/enterprise/auth/me",                   apiLimit, authH.Me)
	app.Post("/api/v1/enterprise/auth/logout",                apiLimit, authH.Logout)
	app.Post("/api/v1/enterprise/auth/login",                 apiLimit, authH.LocalLogin)
	app.Put( "/api/v1/enterprise/auth/password",              apiLimit, authH.SetPassword)
	// Demo + first-run setup — no auth required, public endpoints.
	app.Post("/api/v1/auth/demo",                             apiLimit, authH.DemoLogin)
	app.Get( "/api/v1/auth/setup",                            apiLimit, authH.SetupStatus)
	app.Post("/api/v1/auth/setup",                            apiLimit, authH.SetupAdmin)
	// Google OAuth2 sign-in (enabled when GOOGLE_CLIENT_ID is set)
	app.Get("/api/v1/auth/google/initiate",                   apiLimit, authH.GoogleInitiate)
	app.Get("/api/v1/auth/google/callback",                   apiLimit, authH.GoogleCallback)
	app.Post("/api/v1/enterprise/auth/invite/accept",         apiLimit, inviteH.AcceptInvite)
	app.Post("/api/v1/enterprise/auth/forgot-password",       apiLimit, inviteH.ForgotPassword)
	app.Post("/api/v1/enterprise/auth/reset-password",        apiLimit, inviteH.ResetPassword)
	// OIDC SSO
	app.Get("/api/v1/enterprise/auth/oidc/initiate",          apiLimit, oidcH.Initiate)
	app.Get("/api/v1/enterprise/auth/oidc/callback",          apiLimit, oidcH.Callback)
	// SAML 2.0 SSO
	if samlH != nil {
		app.Get( "/api/v1/enterprise/auth/saml/initiate",     apiLimit, samlH.Initiate)
		app.Post("/api/v1/enterprise/auth/saml/callback",     apiLimit, samlH.Callback)
		app.Get( "/saml/metadata",                            samlH.Metadata)
	}

	// ── Enterprise data routes (session cookie OR API key, with RBAC) ─────────
	entAuth   := middleware.EnterpriseAuth(cfg.APIKey, chClient, sessionStore)
	entAdmin  := middleware.RequireAdminOrAbove()
	entOwner  := middleware.RequireOwner()
	demoGuard := middleware.DemoGuard()

	// demoGuard sits between entAuth and auditLog: sessions marked IsDemo=true
	// receive HTTP 403 on any non-safe method (POST/PUT/PATCH/DELETE).
	ent := app.Group("/api/v1/enterprise", entAuth, demoGuard, auditLog)

	ent.Get( "/org",                         apiLimit,                enterpriseH.GetOrg)
	ent.Put( "/org",                         apiLimit, entAdmin,      enterpriseH.UpdateOrg)
	ent.Get( "/members",                     apiLimit,                enterpriseH.ListMembers)
	ent.Post("/members",                     apiLimit, entAdmin,       enterpriseH.InviteMember)
	ent.Patch("/members/:id/role",           apiLimit, entAdmin,       enterpriseH.UpdateMemberRole)
	ent.Delete("/members/:id",               apiLimit, entAdmin,       enterpriseH.RemoveMember)
	ent.Get( "/teams",                       apiLimit,                enterpriseH.ListTeams)
	ent.Post("/teams",                       apiLimit, entAdmin,       enterpriseH.CreateTeam)
	ent.Delete("/teams/:id",                 apiLimit, entAdmin,       enterpriseH.DeleteTeam)
	ent.Get( "/teams/:id/members",           apiLimit,                enterpriseH.ListTeamMembers)
	ent.Post("/teams/:id/members",           apiLimit, entAdmin,       enterpriseH.AddTeamMember)
	ent.Delete("/teams/:id/members/:uid",    apiLimit, entAdmin,       enterpriseH.RemoveTeamMember)
	ent.Get( "/sso/config",                  apiLimit,                enterpriseH.GetSSOConfig)
	ent.Put( "/sso/config",                  apiLimit, entAdmin,       enterpriseH.UpdateSSOConfig)
	ent.Get( "/license",                     apiLimit, entOwner,       enterpriseH.GetLicense)

	// Integrations (SIEM sinks)
	ent.Get(   "/integrations",              apiLimit,           integrationsH.List)
	ent.Put(   "/integrations/:type",        apiLimit, entAdmin, integrationsH.Upsert)
	ent.Delete("/integrations/:type",        apiLimit, entAdmin, integrationsH.Delete)
	ent.Post(  "/integrations/:type/test",   apiLimit, entAdmin, integrationsH.Test)

	// Audit export (authenticated — any session user)
	ent.Get("/audit/export",                 apiLimit,           auditH.Export)

	// Sigma detection rules (Community: read-only built-ins; Enterprise: full CRUD)
	ent.Get(   "/sigma/rules",               apiLimit,           sigmaH.ListRules)
	ent.Post(  "/sigma/rules",               apiLimit, entAdmin, sigmaH.CreateRule)
	ent.Patch( "/sigma/rules/:id",           apiLimit, entAdmin, sigmaH.UpdateRule)
	ent.Delete("/sigma/rules/:id",           apiLimit, entAdmin, sigmaH.DeleteRule)
	ent.Get(   "/sigma/matches",             apiLimit,           sigmaH.ListMatches)

	// Saved flow queries (Community: max 10; Enterprise: unlimited)
	// Reads are viewer-safe; writes require admin to prevent unprivileged query injection.
	v1.Get(   "/saved-queries",              apiLimit,                            savedQueriesH.List)
	v1.Post(  "/saved-queries",              apiLimit, middleware.RequireAdmin(), savedQueriesH.Create)
	v1.Patch( "/saved-queries/:id",          apiLimit, middleware.RequireAdmin(), savedQueriesH.Update)
	v1.Delete("/saved-queries/:id",          apiLimit, middleware.RequireAdmin(), savedQueriesH.Delete)

	// Long-term storage export config (Enterprise)
	ent.Get(   "/storage/config",            apiLimit,           storageH.GetConfig)
	ent.Put(   "/storage/config",            apiLimit, entAdmin, storageH.UpsertConfig)
	ent.Delete("/storage/config",            apiLimit, entAdmin, storageH.DeleteConfig)
	ent.Get(   "/storage/exports",           apiLimit,           storageH.ListExports)

	// ── v0.5: Cloud VPC Flow Sources (Community: AWS; Enterprise: GCP + Azure)
	v1.Get(   "/cloud/sources",              apiLimit,                            cloudH.List)
	v1.Post(  "/cloud/sources",              apiLimit, middleware.RequireAdmin(), cloudH.Create)
	v1.Patch( "/cloud/sources/:id",          apiLimit, entAdmin,                 cloudH.Update)
	v1.Delete("/cloud/sources/:id",          apiLimit, entAdmin,                 cloudH.Delete)
	v1.Get(   "/cloud/sources/:id/log",      apiLimit,           cloudH.PullLog)

	// ── v0.6: Behavioral baseline + anomaly detection (Community)
	v1.Get("/anomalies",                     apiLimit,           anomalyH.List)
	v1.Get("/anomalies/stats",               apiLimit,           anomalyH.Stats)
	v1.Get("/baseline",                      apiLimit,           anomalyH.GetBaseline)

	// ── v0.6: AI Security Copilot (Community — requires ANTHROPIC_API_KEY)
	v1.Post("/copilot/chat",   apiLimit, copilotH.Chat)
	// ── v0.7 V2: Natural Language Flow Search
	v1.Post("/copilot/search", apiLimit, copilotH.Search)

	// ── G4: Custom Dashboard Builder
	dashH := &handlers.DashboardHandler{CH: chClient}
	v1.Get(   "/dashboards",          apiLimit,                            dashH.List)
	v1.Post(  "/dashboards",          apiLimit, middleware.RequireAdmin(), dashH.Create)
	v1.Get(   "/dashboards/:id",      apiLimit,                            dashH.Get)
	v1.Put(   "/dashboards/:id",      apiLimit, middleware.RequireAdmin(), dashH.Update)
	v1.Delete("/dashboards/:id",      apiLimit, middleware.RequireAdmin(), dashH.Delete)
	v1.Get(   "/flows/top-talkers",   apiLimit,                            dashH.TopTalkers)

	// ── v0.5: Multi-Cluster Fleet Overview (Community)
	v1.Get("/fleet/clusters",                apiLimit,           fleetH.Clusters)
	v1.Get("/fleet/search",                  apiLimit,           fleetH.Search)
	v1.Get("/agents/:id/config",             apiLimit,           fleetH.GetAgentConfig)
	v1.Post("/agents/:id/config",            apiLimit, entAdmin, fleetH.PushAgentConfig)
	v1.Post("/agents/:id/config/ack",        apiLimit,           fleetH.AckAgentConfig)
	v1.Get("/agents/:id/sampling",           apiLimit,           fleetH.GetSamplingMode)
	v1.Post("/agents/:id/sampling",          apiLimit, entAdmin, fleetH.SetSamplingMode)

	// ── v0.5: Compliance Reports (Enterprise)
	ent.Get(  "/compliance/reports",                   apiLimit,           complianceReportH.List)
	ent.Post( "/compliance/reports",                   apiLimit, entAdmin, complianceReportH.Create)
	ent.Patch("/compliance/reports/:id",               apiLimit, entAdmin, complianceReportH.Update)
	ent.Delete("/compliance/reports/:id",              apiLimit, entAdmin, complianceReportH.Delete)
	ent.Post( "/compliance/reports/:id/run",           apiLimit, entAdmin, complianceReportH.Run)
	ent.Get(  "/compliance/reports/:id/history",       apiLimit,           complianceReportH.History)
	ent.Get(  "/compliance/reports/:id/preview",       apiLimit,           complianceReportH.Preview)

	// ── v0.5: Incident Workflow (Enterprise)
	ent.Get(  "/incidents",                            apiLimit,           incidentH.List)
	ent.Post( "/incidents",                            apiLimit,           incidentH.CreateManual)
	ent.Get(  "/incidents/:id",                        apiLimit,           incidentH.Get)
	ent.Post( "/incidents/:id/ack",                    apiLimit,           incidentH.Ack)
	ent.Post( "/incidents/:id/resolve",                apiLimit,           incidentH.Resolve)
	ent.Post( "/incidents/:id/notes",                  apiLimit,           incidentH.AddNote)
	ent.Get(  "/incident-config",                      apiLimit,           incidentH.ListWorkflowConfigs)
	ent.Put(  "/incident-config/:type",                apiLimit, entAdmin, incidentH.UpsertWorkflowConfig)
	ent.Delete("/incident-config/:type",               apiLimit, entAdmin, incidentH.DeleteWorkflowConfig)
	ent.Post( "/incident-config/:type/test",           apiLimit, entAdmin, incidentH.TestWorkflowConfig)

	// SCIM 2.0 — separate Bearer token auth (set SCIM_BEARER_TOKEN env var)
	scimGroup := app.Group("/scim/v2", scimH.BearerAuth)
	scimGroup.Get("/ServiceProviderConfig",  scimH.ServiceProviderConfig)
	scimGroup.Get("/Users",                  scimH.ListUsers)
	scimGroup.Post("/Users",                 scimH.CreateUser)
	scimGroup.Get("/Users/:id",              scimH.GetUser)
	scimGroup.Put("/Users/:id",              scimH.ReplaceUser)
	scimGroup.Patch("/Users/:id",            scimH.PatchUser)
	scimGroup.Delete("/Users/:id",           scimH.DeleteUser)

	// ── Graceful shutdown ─────────────────────────────────────────────────────
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		slog.Info("Nexor Hub API starting", "port", cfg.Port)
		if err := app.Listen(":" + cfg.Port); err != nil {
			slog.Error("server error", "err", err)
		}
	}()

	<-quit
	slog.Info("shutting down gracefully…")
	consCancel()
	if err := app.ShutdownWithTimeout(10 * time.Second); err != nil {
		slog.Error("shutdown error", "err", err)
	}
	slog.Info("bye")
}

// runMigrations creates the ClickHouse tables if they do not already exist.
// The actual DDL lives in clickhouse.Migrate so integration tests can apply
// the same schema against a throwaway ClickHouse container.
func runMigrations(ch *clickhouse.Client) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return clickhouse.Migrate(ctx, ch)
}

// seedAdmin creates the initial local admin account when ADMIN_EMAIL and
// ADMIN_PASSWORD are set and no account with that email already exists.
// This runs once at startup; the env vars can be removed afterwards.
func seedAdmin(ch *clickhouse.Client, email, plainPassword string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Check if the user already exists.
	rows, err := ch.Query(ctx,
		`SELECT user_id FROM org_members
		 WHERE org_id = 'default' AND email = ?
		 ORDER BY last_seen DESC LIMIT 1`, email)
	if err != nil {
		return err
	}
	var existingID string
	if rows.Next() {
		_ = rows.Scan(&existingID)
	}
	rows.Close()

	if existingID != "" {
		slog.Info("admin user already exists — skipping seed", "email", email)
		return nil
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(plainPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	userID := uuid.NewString()
	now := time.Now()

	if err := ch.Exec(ctx,
		`INSERT INTO org_members
		 (user_id, org_id, email, display_name, role,
		  sso_provider, sso_subject, is_active, created_at, last_seen, version)
		 VALUES (?, 'default', ?, 'Admin', 'owner', 'local', '', 1, ?, ?, ?)`,
		userID, email, now, now, now.UnixMilli(),
	); err != nil {
		return err
	}

	if err := ch.Exec(ctx,
		`INSERT INTO local_credentials (user_id, org_id, password_hash, updated_at, version)
		 VALUES (?, 'default', ?, ?, ?)`,
		userID, string(hash), now, now.UnixMilli(),
	); err != nil {
		return err
	}

	slog.Info("seeded initial admin user", "email", email, "user_id", userID)
	return nil
}

// version is set at build time via:
//   go build -ldflags="-X main.version=v0.7.0"
// Defaults to "dev" for local `go run`.
var version = "dev"
