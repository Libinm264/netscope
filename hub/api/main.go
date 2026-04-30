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

	"github.com/netscope/hub-api/alerting"
	"github.com/netscope/hub-api/baseline"
	"github.com/netscope/hub-api/clickhouse"
	"github.com/netscope/hub-api/cloud"
	"github.com/netscope/hub-api/config"
	"github.com/netscope/hub-api/enterprise/compliance"
	"github.com/netscope/hub-api/enterprise/incidents"
	"github.com/netscope/hub-api/enterprise/license"
	"github.com/netscope/hub-api/enterprise/scim"
	"github.com/netscope/hub-api/enterprise/sigma"
	"github.com/netscope/hub-api/enterprise/sinks"
	"github.com/netscope/hub-api/enterprise/sso"
	"github.com/netscope/hub-api/enterprise/storage"
	"github.com/netscope/hub-api/geoip"
	"github.com/netscope/hub-api/handlers"
	"github.com/netscope/hub-api/kafka"
	nsmetrics "github.com/netscope/hub-api/metrics"
	"github.com/netscope/hub-api/middleware"
	"github.com/netscope/hub-api/models"
	"github.com/netscope/hub-api/pubsub"
	"github.com/netscope/hub-api/sessions"
	"github.com/netscope/hub-api/threat"
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
		return c.JSON(fiber.Map{"status": status, "version": "0.1.0"})
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
	v1.Post("/copilot/chat",                 apiLimit,           copilotH.Chat)

	// ── v0.5: Multi-Cluster Fleet Overview (Community)
	v1.Get("/fleet/clusters",                apiLimit,           fleetH.Clusters)
	v1.Get("/fleet/search",                  apiLimit,           fleetH.Search)
	v1.Get("/agents/:id/config",             apiLimit,           fleetH.GetAgentConfig)
	v1.Post("/agents/:id/config",            apiLimit, entAdmin, fleetH.PushAgentConfig)
	v1.Post("/agents/:id/config/ack",        apiLimit,           fleetH.AckAgentConfig)

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
		slog.Info("NetScope Hub API starting", "port", cfg.Port)
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
func runMigrations(ch *clickhouse.Client) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	ddl := []string{
		`CREATE TABLE IF NOT EXISTS alert_rules (
			id               UUID    DEFAULT generateUUIDv4(),
			name             String,
			metric           LowCardinality(String),
			condition        LowCardinality(String),
			threshold        Float64,
			window_minutes   UInt32  DEFAULT 5,
			webhook_url      String  DEFAULT '',
			enabled          UInt8   DEFAULT 1,
			cooldown_minutes UInt32  DEFAULT 15,
			created_at       DateTime64(3, 'UTC') DEFAULT now64()
		) ENGINE = MergeTree()
		ORDER BY (created_at, id)`,

		`CREATE TABLE IF NOT EXISTS alert_events (
			id        UUID DEFAULT generateUUIDv4(),
			rule_id   String,
			rule_name String,
			metric    String,
			value     Float64,
			threshold Float64,
			fired_at  DateTime64(3, 'UTC'),
			delivered UInt8 DEFAULT 0
		) ENGINE = MergeTree()
		PARTITION BY toYYYYMM(fired_at)
		ORDER BY fired_at
		TTL toDateTime(fired_at) + INTERVAL 30 DAY`,

		`CREATE TABLE IF NOT EXISTS flows (
			id          UUID              DEFAULT generateUUIDv4(),
			agent_id    String,
			hostname    LowCardinality(String),
			ts          DateTime64(3, 'UTC'),
			protocol    LowCardinality(String),
			src_ip      String,
			src_port    UInt16,
			dst_ip      String,
			dst_port    UInt16,
			bytes_in    UInt64            DEFAULT 0,
			bytes_out   UInt64            DEFAULT 0,
			duration_ms UInt32            DEFAULT 0,
			info        String            DEFAULT '',
			http_method LowCardinality(String) DEFAULT '',
			http_path   String            DEFAULT '',
			http_status UInt16            DEFAULT 0,
			dns_query   String            DEFAULT '',
			dns_type    LowCardinality(String) DEFAULT ''
		) ENGINE = MergeTree()
		PARTITION BY toYYYYMM(ts)
		ORDER BY (ts, agent_id, protocol)
		TTL toDateTime(ts) + INTERVAL 90 DAY
		SETTINGS index_granularity = 8192`,

		`CREATE TABLE IF NOT EXISTS agents (
			agent_id      String,
			hostname      LowCardinality(String),
			version       String            DEFAULT '',
			interface     String            DEFAULT '',
			last_seen     DateTime64(3, 'UTC'),
			registered_at DateTime64(3, 'UTC') DEFAULT now64()
		) ENGINE = ReplacingMergeTree(last_seen)
		ORDER BY agent_id`,

		// Phase 6: enrollment tokens
		`CREATE TABLE IF NOT EXISTS enrollment_tokens (
			id         String,
			name       String,
			token      String,
			created_at DateTime64(3, 'UTC') DEFAULT now64(),
			expires_at DateTime64(3, 'UTC'),
			used_count UInt32  DEFAULT 0,
			max_uses   UInt32  DEFAULT 0,
			revoked    UInt8   DEFAULT 0
		) ENGINE = ReplacingMergeTree(created_at)
		ORDER BY id`,

		// v0.7: add max_uses cap to existing deployments
		`ALTER TABLE enrollment_tokens ADD COLUMN IF NOT EXISTS max_uses UInt32 DEFAULT 0`,

		// Phase 6: TLS certificate fleet
		`CREATE TABLE IF NOT EXISTS tls_certs (
			fingerprint String,
			cn          String,
			issuer      String  DEFAULT '',
			expiry      String  DEFAULT '',
			expired     UInt8   DEFAULT 0,
			sans        String  DEFAULT '',
			agent_id    String  DEFAULT '',
			hostname    LowCardinality(String) DEFAULT '',
			src_ip      String  DEFAULT '',
			dst_ip      String  DEFAULT '',
			first_seen  DateTime64(3, 'UTC') DEFAULT now64(),
			last_seen   DateTime64(3, 'UTC') DEFAULT now64()
		) ENGINE = ReplacingMergeTree(last_seen)
		ORDER BY fingerprint`,

		// Phase 7: add integration_type column to alert_rules (idempotent)
		`ALTER TABLE alert_rules ADD COLUMN IF NOT EXISTS
		 integration_type LowCardinality(String) DEFAULT 'webhook'`,

		// Phase 8: geo + threat enrichment columns on flows (idempotent)
		`ALTER TABLE flows ADD COLUMN IF NOT EXISTS country_code LowCardinality(String) DEFAULT ''`,
		`ALTER TABLE flows ADD COLUMN IF NOT EXISTS country_name LowCardinality(String) DEFAULT ''`,
		`ALTER TABLE flows ADD COLUMN IF NOT EXISTS as_org       LowCardinality(String) DEFAULT ''`,
		`ALTER TABLE flows ADD COLUMN IF NOT EXISTS threat_score UInt8 DEFAULT 0`,
		`ALTER TABLE flows ADD COLUMN IF NOT EXISTS threat_level LowCardinality(String) DEFAULT ''`,

		// Phase 9: eBPF process attribution columns on flows (idempotent)
		`ALTER TABLE flows ADD COLUMN IF NOT EXISTS process_name LowCardinality(String) DEFAULT ''`,
		`ALTER TABLE flows ADD COLUMN IF NOT EXISTS pid          UInt32 DEFAULT 0`,

		// Phase 6: API tokens (RBAC)
		`CREATE TABLE IF NOT EXISTS api_tokens (
			id         String,
			name       String,
			role       LowCardinality(String) DEFAULT 'viewer',
			token      String,
			created_at DateTime64(3, 'UTC') DEFAULT now64(),
			last_used  DateTime64(3, 'UTC') DEFAULT now64(),
			revoked    UInt8 DEFAULT 0
		) ENGINE = ReplacingMergeTree(last_used)
		ORDER BY id`,

		// Phase 10: new alert_rules columns (idempotent)
		`ALTER TABLE alert_rules ADD COLUMN IF NOT EXISTS webhook_secret String DEFAULT ''`,
		`ALTER TABLE alert_rules ADD COLUMN IF NOT EXISTS email_to String DEFAULT ''`,

		// Phase 11a: agent fleet enrichment (idempotent ALTER TABLE)
		`ALTER TABLE agents ADD COLUMN IF NOT EXISTS os LowCardinality(String) DEFAULT ''`,
		`ALTER TABLE agents ADD COLUMN IF NOT EXISTS capture_mode LowCardinality(String) DEFAULT 'pcap'`,
		`ALTER TABLE agents ADD COLUMN IF NOT EXISTS ebpf_enabled UInt8 DEFAULT 0`,

		// Phase 11b: K8s pod enrichment on flows (idempotent)
		`ALTER TABLE flows ADD COLUMN IF NOT EXISTS pod_name LowCardinality(String) DEFAULT ''`,
		`ALTER TABLE flows ADD COLUMN IF NOT EXISTS k8s_namespace LowCardinality(String) DEFAULT ''`,

		// Phase 11c: Process policies table
		`CREATE TABLE IF NOT EXISTS process_policies (
			id          UUID DEFAULT generateUUIDv4(),
			name        String,
			process_name String,
			action      LowCardinality(String) DEFAULT 'alert',
			dst_ip_cidr String DEFAULT '',
			dst_port    UInt16 DEFAULT 0,
			description String DEFAULT '',
			enabled     UInt8  DEFAULT 1,
			created_at  DateTime64(3, 'UTC') DEFAULT now64()
		) ENGINE = MergeTree() ORDER BY (created_at, id)`,

		// Phase 12a: Multi-tenant organisations table
		`CREATE TABLE IF NOT EXISTS organisations (
			org_id         String,
			name           String,
			slug           LowCardinality(String),
			agent_quota    Int32    DEFAULT 10,
			retention_days Int32    DEFAULT 90,
			plan           LowCardinality(String) DEFAULT 'community',
			created_at     DateTime64(3, 'UTC') DEFAULT now64(),
			version        UInt64   DEFAULT 1
		) ENGINE = ReplacingMergeTree(version)
		ORDER BY org_id`,

		// Phase 12b: Org members (identity mapping, no credentials stored)
		`CREATE TABLE IF NOT EXISTS org_members (
			user_id      String,
			org_id       LowCardinality(String) DEFAULT 'default',
			email        String,
			display_name String  DEFAULT '',
			role         LowCardinality(String) DEFAULT 'viewer',
			sso_provider LowCardinality(String) DEFAULT '',
			sso_subject  String  DEFAULT '',
			is_active    UInt8   DEFAULT 1,
			created_at   DateTime64(3, 'UTC') DEFAULT now64(),
			last_seen    DateTime64(3, 'UTC') DEFAULT now64(),
			version      UInt64  DEFAULT 1
		) ENGINE = ReplacingMergeTree(version)
		ORDER BY (org_id, user_id)`,

		// Phase 12c: Teams
		`CREATE TABLE IF NOT EXISTS teams (
			team_id     String,
			org_id      LowCardinality(String) DEFAULT 'default',
			name        String,
			description String  DEFAULT '',
			created_at  DateTime64(3, 'UTC') DEFAULT now64(),
			version     UInt64  DEFAULT 1
		) ENGINE = ReplacingMergeTree(version)
		ORDER BY (org_id, team_id)`,

		// Phase 12d: Team membership
		`CREATE TABLE IF NOT EXISTS team_members (
			team_id  String,
			user_id  String,
			org_id   LowCardinality(String) DEFAULT 'default',
			added_at DateTime64(3, 'UTC') DEFAULT now64(),
			version  UInt64 DEFAULT 1
		) ENGINE = ReplacingMergeTree(version)
		ORDER BY (team_id, user_id)`,

		// Phase 12e: SSO provider configurations (no secrets)
		`CREATE TABLE IF NOT EXISTS sso_configs (
			org_id      LowCardinality(String) DEFAULT 'default',
			provider    LowCardinality(String),
			enabled     UInt8   DEFAULT 0,
			entity_id   String  DEFAULT '',
			sso_url     String  DEFAULT '',
			certificate String  DEFAULT '',
			issuer_url  String  DEFAULT '',
			client_id   String  DEFAULT '',
			updated_at  DateTime64(3, 'UTC') DEFAULT now64(),
			version     UInt64  DEFAULT 1
		) ENGINE = ReplacingMergeTree(version)
		ORDER BY (org_id, provider)`,

		// Phase 12f: seed default organisation (idempotent via ReplacingMergeTree)
		`INSERT INTO organisations (org_id, name, slug, agent_quota, retention_days, plan)
		 SELECT 'default', 'Default Organisation', 'default', 10, 90, 'community'
		 WHERE NOT EXISTS (
		   SELECT 1 FROM organisations WHERE org_id = 'default'
		 )`,

		// Phase 12g: Local credentials (bcrypt password hashes for email/password login)
		`CREATE TABLE IF NOT EXISTS local_credentials (
			user_id       String,
			org_id        LowCardinality(String) DEFAULT 'default',
			password_hash String,
			updated_at    DateTime64(3, 'UTC') DEFAULT now64(),
			version       UInt64 DEFAULT 1
		) ENGINE = ReplacingMergeTree(version)
		ORDER BY (org_id, user_id)`,

		// Phase 12h: Invite tokens (single-use, 7-day TTL)
		`CREATE TABLE IF NOT EXISTS invite_tokens (
			token      String,
			user_id    String,
			email      String,
			expires_at DateTime64(3, 'UTC'),
			used       UInt8 DEFAULT 0,
			version    UInt64 DEFAULT 1
		) ENGINE = ReplacingMergeTree(version)
		ORDER BY token`,

		// Phase 12i: Password reset tokens (single-use, 1-hour TTL)
		`CREATE TABLE IF NOT EXISTS password_reset_tokens (
			token      String,
			user_id    String,
			email      String,
			expires_at DateTime64(3, 'UTC'),
			used       UInt8 DEFAULT 0,
			version    UInt64 DEFAULT 1
		) ENGINE = ReplacingMergeTree(version)
		ORDER BY token`,

		// Phase 11d: Policy violations log
		`CREATE TABLE IF NOT EXISTS policy_violations (
			id          UUID DEFAULT generateUUIDv4(),
			policy_id   String,
			policy_name String,
			process_name String DEFAULT '',
			pid         UInt32 DEFAULT 0,
			src_ip      String DEFAULT '',
			dst_ip      String DEFAULT '',
			dst_port    UInt16 DEFAULT 0,
			protocol    LowCardinality(String) DEFAULT '',
			agent_id    String DEFAULT '',
			hostname    LowCardinality(String) DEFAULT '',
			violated_at DateTime64(3, 'UTC') DEFAULT now64()
		) ENGINE = MergeTree()
		PARTITION BY toYYYYMM(violated_at)
		ORDER BY (violated_at, policy_id)
		TTL toDateTime(violated_at) + INTERVAL 30 DAY`,

		// Phase 9: audit log — every authenticated API call
		`CREATE TABLE IF NOT EXISTS audit_events (
			id         String,
			token_id   String            DEFAULT '',
			role       LowCardinality(String) DEFAULT '',
			method     LowCardinality(String),
			path       String,
			status     UInt16,
			client_ip  String            DEFAULT '',
			latency_ms UInt32            DEFAULT 0,
			ts         DateTime64(3, 'UTC') DEFAULT now64()
		) ENGINE = MergeTree()
		PARTITION BY toYYYYMM(ts)
		ORDER BY (ts, token_id)
		TTL toDateTime(ts) + INTERVAL 90 DAY`,

		// Phase 13: SIEM sink configurations
		`CREATE TABLE IF NOT EXISTS integrations_config (
			sink_type    LowCardinality(String),
			enabled      UInt8            DEFAULT 0,
			config       String           DEFAULT '{}',
			last_shipped DateTime64(3, 'UTC') DEFAULT toDateTime64(0, 3),
			updated_at   DateTime64(3, 'UTC') DEFAULT now64(),
			version      UInt64           DEFAULT 1
		) ENGINE = ReplacingMergeTree(version)
		ORDER BY sink_type`,

		// Phase 14: OTel trace correlation — trace_id on flows (idempotent)
		`ALTER TABLE flows ADD COLUMN IF NOT EXISTS trace_id String DEFAULT ''`,

		// Phase 15: Saved flow queries (Community: 10 max; Enterprise: unlimited)
		`CREATE TABLE IF NOT EXISTS saved_queries (
			id          String,
			name        String,
			description String  DEFAULT '',
			filters     String  DEFAULT '{}',
			deleted     UInt8   DEFAULT 0,
			created_at  DateTime64(3, 'UTC') DEFAULT now64(),
			updated_at  DateTime64(3, 'UTC') DEFAULT now64(),
			version     UInt64  DEFAULT 1
		) ENGINE = ReplacingMergeTree(version)
		ORDER BY id`,

		// Phase 16: Sigma detection rules
		`CREATE TABLE IF NOT EXISTS sigma_rules (
			id          String,
			title       String,
			description String  DEFAULT '',
			severity    LowCardinality(String) DEFAULT 'medium',
			tags        String  DEFAULT '[]',
			query       String  DEFAULT '',
			enabled     UInt8   DEFAULT 1,
			builtin     UInt8   DEFAULT 0,
			created_at  DateTime64(3, 'UTC') DEFAULT now64(),
			updated_at  DateTime64(3, 'UTC') DEFAULT now64(),
			version     UInt64  DEFAULT 1
		) ENGINE = ReplacingMergeTree(version)
		ORDER BY id`,

		// Phase 17: Sigma match events
		`CREATE TABLE IF NOT EXISTS sigma_matches (
			id         UUID    DEFAULT generateUUIDv4(),
			rule_id    String,
			rule_title String,
			severity   LowCardinality(String) DEFAULT 'medium',
			match_data String  DEFAULT '{}',
			fired_at   DateTime64(3, 'UTC') DEFAULT now64()
		) ENGINE = MergeTree()
		PARTITION BY toYYYYMM(fired_at)
		ORDER BY (fired_at, rule_id)
		TTL toDateTime(fired_at) + INTERVAL 30 DAY`,

		// Phase 18: OTel backend URL on organisations (idempotent)
		`ALTER TABLE organisations ADD COLUMN IF NOT EXISTS otel_backend_url String DEFAULT ''`,

		// Phase 19: Cluster label on agents (idempotent)
		`ALTER TABLE agents ADD COLUMN IF NOT EXISTS cluster LowCardinality(String) DEFAULT ''`,

		// Phase 20: Long-term storage config (Enterprise S3/GCS export)
		`CREATE TABLE IF NOT EXISTS storage_config (
			provider    LowCardinality(String) DEFAULT 's3',
			enabled     UInt8            DEFAULT 0,
			bucket      String           DEFAULT '',
			region      String           DEFAULT '',
			endpoint    String           DEFAULT '',
			access_key  String           DEFAULT '',
			secret_key  String           DEFAULT '',
			prefix      String           DEFAULT 'netscope/flows',
			schedule    LowCardinality(String) DEFAULT 'hourly',
			last_export DateTime64(3, 'UTC') DEFAULT toDateTime64(0, 3),
			updated_at  DateTime64(3, 'UTC') DEFAULT now64(),
			version     UInt64           DEFAULT 1
		) ENGINE = ReplacingMergeTree(version)
		ORDER BY provider`,

		// Phase 21: Storage export audit log
		`CREATE TABLE IF NOT EXISTS storage_exports (
			id          UUID    DEFAULT generateUUIDv4(),
			window      String,
			object_key  String  DEFAULT '',
			row_count   UInt64  DEFAULT 0,
			exported_at DateTime64(3, 'UTC') DEFAULT now64(),
			error       String  DEFAULT ''
		) ENGINE = MergeTree()
		PARTITION BY toYYYYMM(exported_at)
		ORDER BY exported_at
		TTL toDateTime(exported_at) + INTERVAL 90 DAY`,

		// Phase 17b: Seed the 5 built-in Community detection rules (idempotent)
		`INSERT INTO sigma_rules (id, title, description, severity, tags, query, enabled, builtin, created_at, updated_at, version)
		 SELECT 'builtin-001', 'Port Scan Detection',
		   'Detects hosts probing more than 50 unique destination ports within a 5-minute window.',
		   'high', '["recon","portscan","attack.discovery"]',
		   'SELECT src_ip, count(DISTINCT dst_port) AS port_count, min(ts) AS first_seen FROM flows WHERE ts > now() - INTERVAL 5 MINUTE AND protocol IN (''TCP'',''UDP'') GROUP BY src_ip HAVING port_count > 50',
		   1, 1, now64(), now64(), 1
		 WHERE NOT EXISTS (SELECT 1 FROM sigma_rules WHERE id = 'builtin-001')`,

		`INSERT INTO sigma_rules (id, title, description, severity, tags, query, enabled, builtin, created_at, updated_at, version)
		 SELECT 'builtin-002', 'DNS Tunneling Indicator',
		   'Identifies unusually long DNS query names (>60 chars) indicating DNS-based exfiltration.',
		   'medium', '["dns","exfiltration","attack.c2"]',
		   'SELECT src_ip, hostname, dns_query, ts FROM flows WHERE ts > now() - INTERVAL 10 MINUTE AND protocol = ''DNS'' AND length(dns_query) > 60',
		   1, 1, now64(), now64(), 1
		 WHERE NOT EXISTS (SELECT 1 FROM sigma_rules WHERE id = 'builtin-002')`,

		`INSERT INTO sigma_rules (id, title, description, severity, tags, query, enabled, builtin, created_at, updated_at, version)
		 SELECT 'builtin-003', 'Cleartext Credential Submission',
		   'Detects HTTP POST to authentication paths over plain HTTP risking credential exposure.',
		   'high', '["credentials","http","attack.credential_access"]',
		   'SELECT src_ip, dst_ip, dst_port, http_path, hostname, ts FROM flows WHERE ts > now() - INTERVAL 15 MINUTE AND protocol = ''HTTP'' AND http_method = ''POST'' AND (http_path LIKE ''%/login%'' OR http_path LIKE ''%/auth%'' OR http_path LIKE ''%/signin%'') AND dst_port != 443',
		   1, 1, now64(), now64(), 1
		 WHERE NOT EXISTS (SELECT 1 FROM sigma_rules WHERE id = 'builtin-003')`,

		`INSERT INTO sigma_rules (id, title, description, severity, tags, query, enabled, builtin, created_at, updated_at, version)
		 SELECT 'builtin-004', 'Unexpected Outbound High Port',
		   'Flags large outbound connections to ephemeral ports that may indicate beaconing or reverse shells.',
		   'medium', '["c2","beaconing","attack.command_and_control"]',
		   'SELECT src_ip, dst_ip, dst_port, protocol, bytes_out, hostname, ts FROM flows WHERE ts > now() - INTERVAL 5 MINUTE AND dst_port > 49151 AND protocol = ''TCP'' AND bytes_out > 10000',
		   1, 1, now64(), now64(), 1
		 WHERE NOT EXISTS (SELECT 1 FROM sigma_rules WHERE id = 'builtin-004')`,

		`INSERT INTO sigma_rules (id, title, description, severity, tags, query, enabled, builtin, created_at, updated_at, version)
		 SELECT 'builtin-005', 'Privileged Process Network Activity',
		   'Detects shell or interpreter processes making unexpected outbound connections post-exploitation indicator.',
		   'critical', '["process","shell","attack.execution","attack.lateral_movement"]',
		   'SELECT process_name, pid, src_ip, dst_ip, dst_port, hostname, ts FROM flows WHERE ts > now() - INTERVAL 5 MINUTE AND protocol = ''TCP'' AND process_name IN (''bash'',''sh'',''zsh'',''python'',''python3'',''powershell'',''pwsh'',''cmd'',''perl'',''ruby'') AND dst_port NOT IN (22, 80, 443)',
		   1, 1, now64(), now64(), 1
		 WHERE NOT EXISTS (SELECT 1 FROM sigma_rules WHERE id = 'builtin-005')`,

		// ── v0.5 Fleet Intelligence ───────────────────────────────────────────

		// Phase 22: Cloud flow sources (VPC Flow Logs config per cloud account)
		`CREATE TABLE IF NOT EXISTS cloud_flow_sources (
			id          String,
			provider    LowCardinality(String) DEFAULT 'aws',
			name        String                 DEFAULT '',
			config      String                 DEFAULT '{}',
			enabled     UInt8                  DEFAULT 1,
			last_pulled DateTime64(3,'UTC')    DEFAULT toDateTime64(0,3),
			error_msg   String                 DEFAULT '',
			created_at  DateTime64(3,'UTC')    DEFAULT now64(),
			version     UInt64                 DEFAULT 1
		) ENGINE = ReplacingMergeTree(version)
		ORDER BY id`,

		// Phase 23: Cloud pull audit log
		`CREATE TABLE IF NOT EXISTS cloud_flow_pull_log (
			id            UUID    DEFAULT generateUUIDv4(),
			source_id     String,
			provider      LowCardinality(String) DEFAULT 'aws',
			rows_ingested UInt64  DEFAULT 0,
			pulled_at     DateTime64(3,'UTC') DEFAULT now64(),
			duration_ms   UInt32  DEFAULT 0,
			error         String  DEFAULT ''
		) ENGINE = MergeTree()
		PARTITION BY toYYYYMM(pulled_at)
		ORDER BY pulled_at
		TTL toDateTime(pulled_at) + INTERVAL 30 DAY`,

		// Phase 24: Source label on flows (agent vs cloud-pull)
		`ALTER TABLE flows ADD COLUMN IF NOT EXISTS source LowCardinality(String) DEFAULT 'agent'`,

		// Phase 25: Agent remote-config push table
		`CREATE TABLE IF NOT EXISTS agent_configs (
			agent_id   String,
			config     String              DEFAULT '{}',
			pushed_at  DateTime64(3,'UTC') DEFAULT now64(),
			ack_at     DateTime64(3,'UTC') DEFAULT toDateTime64(0,3),
			version    UInt64              DEFAULT 1
		) ENGINE = ReplacingMergeTree(version)
		ORDER BY agent_id`,

		// Phase 26: Config version on agents (agent reports running config)
		`ALTER TABLE agents ADD COLUMN IF NOT EXISTS config_version String DEFAULT ''`,

		// Phase 27: Compliance report schedules (Enterprise)
		`CREATE TABLE IF NOT EXISTS compliance_report_schedules (
			id         String,
			name       String                 DEFAULT '',
			framework  LowCardinality(String) DEFAULT 'soc2',
			format     LowCardinality(String) DEFAULT 'pdf',
			schedule   LowCardinality(String) DEFAULT 'weekly',
			recipients String                 DEFAULT '[]',
			enabled    UInt8                  DEFAULT 1,
			last_sent  DateTime64(3,'UTC')    DEFAULT toDateTime64(0,3),
			created_at DateTime64(3,'UTC')    DEFAULT now64(),
			version    UInt64                 DEFAULT 1
		) ENGINE = ReplacingMergeTree(version)
		ORDER BY id`,

		// Phase 28: Compliance report run history
		`CREATE TABLE IF NOT EXISTS compliance_report_runs (
			id          UUID    DEFAULT generateUUIDv4(),
			schedule_id String,
			framework   LowCardinality(String) DEFAULT 'soc2',
			format      LowCardinality(String) DEFAULT 'pdf',
			recipients  String  DEFAULT '[]',
			rows        UInt64  DEFAULT 0,
			sent_at     DateTime64(3,'UTC') DEFAULT now64(),
			error       String  DEFAULT ''
		) ENGINE = MergeTree()
		PARTITION BY toYYYYMM(sent_at)
		ORDER BY sent_at
		TTL toDateTime(sent_at) + INTERVAL 90 DAY`,

		// Phase 29: In-hub incident timeline (Enterprise)
		`CREATE TABLE IF NOT EXISTS incidents (
			id           String,
			title        String                 DEFAULT '',
			severity     LowCardinality(String) DEFAULT 'medium',
			status       LowCardinality(String) DEFAULT 'open',
			source       LowCardinality(String) DEFAULT 'sigma',
			source_id    String                 DEFAULT '',
			notes        String                 DEFAULT '',
			external_ref String                 DEFAULT '',
			created_at   DateTime64(3,'UTC')    DEFAULT now64(),
			updated_at   DateTime64(3,'UTC')    DEFAULT now64(),
			version      UInt64                 DEFAULT 1
		) ENGINE = ReplacingMergeTree(version)
		ORDER BY (created_at, id)`,

		// Phase 31 (v0.6): Traffic baselines — 7-day rolling mean/stddev
		`CREATE TABLE IF NOT EXISTS traffic_baselines (
			agent_id        String,
			protocol        LowCardinality(String),
			hour_of_week    UInt8,
			flow_count_mean  Float64 DEFAULT 0,
			flow_count_std   Float64 DEFAULT 0,
			bytes_in_mean    Float64 DEFAULT 0,
			bytes_out_mean   Float64 DEFAULT 0,
			sample_count     UInt32  DEFAULT 0,
			computed_at      DateTime64(3, 'UTC') DEFAULT now64(),
			version          UInt64  DEFAULT 1
		) ENGINE = ReplacingMergeTree(version)
		ORDER BY (agent_id, protocol, hour_of_week)`,

		// Phase 32 (v0.6): Anomaly events — Z-score outliers
		`CREATE TABLE IF NOT EXISTS anomaly_events (
			id           UUID    DEFAULT generateUUIDv4(),
			agent_id     String,
			hostname     LowCardinality(String) DEFAULT '',
			protocol     LowCardinality(String),
			anomaly_type LowCardinality(String) DEFAULT 'spike',
			z_score      Float64 DEFAULT 0,
			observed     Float64 DEFAULT 0,
			expected     Float64 DEFAULT 0,
			description  String  DEFAULT '',
			severity     LowCardinality(String) DEFAULT 'low',
			detected_at  DateTime64(3, 'UTC') DEFAULT now64()
		) ENGINE = MergeTree()
		PARTITION BY toYYYYMM(detected_at)
		ORDER BY (detected_at, agent_id)
		TTL toDateTime(detected_at) + INTERVAL 30 DAY`,

		// Phase 30: Incident workflow config (per-integration credentials)
		`CREATE TABLE IF NOT EXISTS incident_workflow_config (
			integration LowCardinality(String) DEFAULT 'pagerduty',
			enabled     UInt8                  DEFAULT 0,
			config      String                 DEFAULT '{}',
			updated_at  DateTime64(3,'UTC')    DEFAULT now64(),
			version     UInt64                 DEFAULT 1
		) ENGINE = ReplacingMergeTree(version)
		ORDER BY integration`,
	}

	for _, q := range ddl {
		if err := ch.Exec(ctx, q); err != nil {
			return err
		}
	}

	slog.Info("schema migrations complete")
	return nil
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
