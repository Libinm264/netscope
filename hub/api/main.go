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

	"github.com/netscope/hub-api/alerting"
	"github.com/netscope/hub-api/clickhouse"
	"github.com/netscope/hub-api/config"
	"github.com/netscope/hub-api/geoip"
	"github.com/netscope/hub-api/handlers"
	"github.com/netscope/hub-api/kafka"
	nsmetrics "github.com/netscope/hub-api/metrics"
	"github.com/netscope/hub-api/middleware"
	"github.com/netscope/hub-api/models"
	"github.com/netscope/hub-api/threat"
)

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

	if cons, err := kafka.NewConsumer(cfg.KafkaBrokers, cfg.KafkaTopic, "netscope-ch-writer"); err != nil {
		slog.Warn("Kafka consumer unavailable", "err", err)
	} else {
		consumer = cons
		slog.Info("Kafka consumer connected")
		defer consumer.Close()
	}

	// Kafka → ClickHouse bridge goroutine
	consCtx, consCancel := context.WithCancel(context.Background())
	defer consCancel()

	if consumer != nil && chWriter != nil {
		go func() {
			if err := consumer.Consume(consCtx, func(flow models.Flow) {
				chWriter.Write(flow)
				handlers.BroadcastFlow(flow)
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

	// ── Alert evaluator ───────────────────────────────────────────────────────
	var evaluator *alerting.Evaluator
	if chClient != nil {
		evaluator = alerting.NewEvaluator(chClient, 60*time.Second)
		evaluator.Start()
		defer evaluator.Stop()
		slog.Info("alert evaluator started")
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
	auth := middleware.TokenAuth(cfg.APIKey, chClient)
	// Ingest gets a generous limit (agents post many flows); general API is tighter.
	ingestLimit := middleware.RateLimit(50_000, time.Minute)
	apiLimit    := middleware.RateLimit(2_000, time.Minute)

	v1 := app.Group("/api/v1", auth)

	flowH      := &handlers.FlowHandler{CH: chClient, Writer: chWriter, Producer: producer, CertsCH: chClient, GeoIP: geoReader, Threat: threatScorer}
	agentH     := &handlers.AgentHandler{CH: chClient}
	statsH     := &handlers.StatsHandler{CH: chClient}
	alertH     := &handlers.AlertHandler{CH: chClient}
	servicesH  := &handlers.ServicesHandler{CH: chClient}
	analyticsH := &handlers.AnalyticsHandler{CH: chClient}
	otelH      := &handlers.OtelHandler{CH: chClient}
	enrollH    := &handlers.EnrollmentHandler{CH: chClient, Cfg: cfg}
	certH      := &handlers.CertHandler{CH: chClient}
	tokenH     := &handlers.TokenHandler{CH: chClient}
	complianceH := &handlers.ComplianceHandler{CH: chClient}

	// ── Public (no auth) ──────────────────────────────────────────────────────
	app.Post("/api/v1/agents/enroll", apiLimit, enrollH.Enroll)
	app.Get("/install", enrollH.InstallScript)

	v1.Post("/ingest",                    ingestLimit,                         flowH.Ingest)
	v1.Get("/flows",                      apiLimit,                            flowH.Query)
	v1.Get("/flows/stream",                                                    flowH.Stream)
	v1.Get("/stats",                      apiLimit,                            statsH.Stats)
	v1.Get("/agents",                     apiLimit,                            agentH.List)
	v1.Post("/agents/register",           apiLimit, middleware.RequireAdmin(), agentH.Register)
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
		TTL fired_at + INTERVAL 30 DAY`,

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
		TTL ts + INTERVAL 90 DAY
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
			revoked    UInt8   DEFAULT 0
		) ENGINE = ReplacingMergeTree(created_at)
		ORDER BY id`,

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
	}

	for _, q := range ddl {
		if err := ch.Exec(ctx, q); err != nil {
			return err
		}
	}

	slog.Info("schema migrations complete")
	return nil
}
