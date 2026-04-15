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

	"github.com/netscope/hub-api/clickhouse"
	"github.com/netscope/hub-api/config"
	"github.com/netscope/hub-api/handlers"
	"github.com/netscope/hub-api/kafka"
	"github.com/netscope/hub-api/middleware"
	"github.com/netscope/hub-api/models"
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
		AllowOrigins: "*",
		AllowHeaders: "Origin, Content-Type, Accept, X-Api-Key",
		AllowMethods: "GET,POST,OPTIONS",
	}))

	// Public health endpoint
	app.Get("/health", func(c *fiber.Ctx) error {
		status := "ok"
		if chClient == nil {
			status = "degraded"
		}
		return c.JSON(fiber.Map{"status": status, "version": "0.1.0"})
	})

	// Protected API routes
	v1 := app.Group("/api/v1", middleware.APIKeyAuth(cfg.APIKey))

	flowH := &handlers.FlowHandler{
		CH:       chClient,
		Writer:   chWriter,
		Producer: producer,
	}
	agentH := &handlers.AgentHandler{CH: chClient}
	statsH := &handlers.StatsHandler{CH: chClient}

	v1.Post("/ingest", flowH.Ingest)
	v1.Get("/flows", flowH.Query)
	v1.Get("/flows/stream", flowH.Stream)
	v1.Get("/stats", statsH.Stats)
	v1.Get("/agents", agentH.List)
	v1.Post("/agents/register", agentH.Register)

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
	}

	for _, q := range ddl {
		if err := ch.Exec(ctx, q); err != nil {
			return err
		}
	}

	slog.Info("schema migrations complete")
	return nil
}
