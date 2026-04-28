// Package storage implements long-term cold storage export for NetScope flows.
//
// On a configurable schedule (default: hourly) the exporter reads a one-hour
// slice of flows from ClickHouse and writes it to S3-compatible object storage
// (AWS S3, GCS with the interoperability API, MinIO, Cloudflare R2 …) using
// ClickHouse's native S3 table function.
//
// Only SELECT + INSERT queries are issued. The exporter never deletes rows from
// the hot ClickHouse store — retention is governed by the flows table's TTL.
//
// # Enterprise gate
//
// Callers must hold license.FeatureAuditExport (reused for storage export) to
// start the exporter. Community instances show the UI but get an upgrade prompt.
package storage

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/netscope/hub-api/clickhouse"
)

// Config holds the storage destination settings loaded from the hub's
// storage_config ClickHouse table.
type Config struct {
	Provider  string // "s3" | "gcs" | "minio" | "r2"
	Enabled   bool
	Bucket    string
	Region    string
	Endpoint  string // custom endpoint override (MinIO / R2 / GCS)
	AccessKey string
	SecretKey string
	Prefix    string // object key prefix, e.g. "netscope/flows"
	Schedule  string // "hourly" | "daily"
}

// ExportResult summarises one export run.
type ExportResult struct {
	Window    string    `json:"window"`   // "2026-04-27T05:00:00Z"
	ObjectKey string    `json:"key"`      // full S3 object path
	RowCount  uint64    `json:"rows"`
	ExportedAt time.Time `json:"exported_at"`
	Error     string    `json:"error,omitempty"`
}

// Exporter runs scheduled flow exports to S3-compatible storage.
type Exporter struct {
	ch       *clickhouse.Client
	interval time.Duration

	mu     sync.Mutex
	ticker *time.Ticker
	done   chan struct{}
}

// New returns a new Exporter. Call Start() to begin the schedule.
func New(ch *clickhouse.Client) *Exporter {
	return &Exporter{
		ch:       ch,
		interval: time.Hour,
		done:     make(chan struct{}),
	}
}

// Start launches the background export goroutine.
func (e *Exporter) Start() {
	if e.ch == nil {
		return
	}
	e.mu.Lock()
	e.ticker = time.NewTicker(e.interval)
	e.mu.Unlock()

	go func() {
		slog.Info("storage exporter started", "interval", e.interval)
		// Wait until the next clean hour boundary before first run.
		now := time.Now().UTC()
		next := now.Truncate(time.Hour).Add(time.Hour)
		select {
		case <-time.After(time.Until(next)):
		case <-e.done:
			return
		}
		e.export()
		for {
			select {
			case <-e.ticker.C:
				e.export()
			case <-e.done:
				return
			}
		}
	}()
}

// Stop shuts down the exporter.
func (e *Exporter) Stop() {
	e.mu.Lock()
	if e.ticker != nil {
		e.ticker.Stop()
	}
	e.mu.Unlock()
	close(e.done)
}

// export loads the active config and runs one export window.
func (e *Exporter) export() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	cfg, err := e.loadConfig(ctx)
	if err != nil {
		slog.Warn("storage: failed to load config", "err", err)
		return
	}
	if !cfg.Enabled || cfg.Bucket == "" {
		return
	}

	// Export the previous complete hour.
	end := time.Now().UTC().Truncate(time.Hour)
	start := end.Add(-time.Hour)

	res := e.exportWindow(ctx, cfg, start, end)
	if res.Error != "" {
		slog.Warn("storage: export failed", "window", res.Window, "err", res.Error)
	} else {
		slog.Info("storage: export complete",
			"window", res.Window, "key", res.ObjectKey, "rows", res.RowCount)
	}

	// Record the result in ClickHouse for the audit log.
	_ = e.ch.Exec(ctx,
		`INSERT INTO storage_exports (window, object_key, row_count, exported_at, error)
		 VALUES (?, ?, ?, ?, ?)`,
		res.Window, res.ObjectKey, res.RowCount, res.ExportedAt, res.Error,
	)
	// Update last_export timestamp in config.
	_ = e.ch.Exec(ctx,
		`INSERT INTO storage_config
		 (provider, enabled, bucket, region, endpoint, access_key, secret_key, prefix, schedule, last_export, updated_at, version)
		 SELECT provider, enabled, bucket, region, endpoint, access_key, secret_key, prefix, schedule, ?, now64(),
		        toUInt64(toUnixTimestamp64Milli(now64()))
		 FROM storage_config
		 WHERE provider = ? ORDER BY version DESC LIMIT 1`,
		end, cfg.Provider,
	)
}

// exportWindow exports flows for [start, end) to S3.
func (e *Exporter) exportWindow(ctx context.Context, cfg Config, start, end time.Time) ExportResult {
	res := ExportResult{
		Window:     start.UTC().Format(time.RFC3339),
		ExportedAt: time.Now().UTC(),
	}

	key := fmt.Sprintf("%s/%s.ndjson.gz", cfg.Prefix, start.UTC().Format("2006/01/02/15"))
	res.ObjectKey = fmt.Sprintf("s3://%s/%s", cfg.Bucket, key)

	// Build the S3 endpoint URL. GCS uses storage.googleapis.com.
	endpoint := cfg.Endpoint
	if endpoint == "" {
		switch cfg.Provider {
		case "gcs":
			endpoint = "https://storage.googleapis.com"
		case "r2":
			endpoint = fmt.Sprintf("https://%s.r2.cloudflarestorage.com", cfg.AccessKey[:8])
		default:
			if cfg.Region != "" {
				endpoint = fmt.Sprintf("https://s3.%s.amazonaws.com", cfg.Region)
			} else {
				endpoint = "https://s3.amazonaws.com"
			}
		}
	}

	s3URL := fmt.Sprintf("%s/%s/%s", endpoint, cfg.Bucket, key)

	// Count rows first so we can record it.
	countRows, err := e.ch.Query(ctx,
		`SELECT count() FROM flows WHERE ts >= ? AND ts < ?`, start, end)
	if err != nil {
		res.Error = fmt.Sprintf("count: %v", err)
		return res
	}
	if countRows.Next() {
		_ = countRows.Scan(&res.RowCount)
	}
	countRows.Close()

	if res.RowCount == 0 {
		res.ObjectKey = ""
		return res
	}

	// Use ClickHouse's native S3 table function for the export.
	// This runs entirely inside ClickHouse — no data traverses the hub process.
	exportSQL := fmt.Sprintf(`
		INSERT INTO FUNCTION s3('%s', '%s', '%s', 'JSONEachRow')
		SELECT id, agent_id, hostname, ts, protocol,
		       src_ip, src_port, dst_ip, dst_port,
		       bytes_in, bytes_out, duration_ms, info,
		       process_name, pid, pod_name, k8s_namespace,
		       country_code, country_name, as_org,
		       threat_score, threat_level, trace_id
		FROM flows
		WHERE ts >= toDateTime64('%s', 3) AND ts < toDateTime64('%s', 3)
		SETTINGS s3_truncate_on_insert = 1`,
		s3URL, cfg.AccessKey, cfg.SecretKey,
		start.UTC().Format("2006-01-02 15:04:05"),
		end.UTC().Format("2006-01-02 15:04:05"),
	)

	if err := e.ch.Exec(ctx, exportSQL); err != nil {
		res.Error = fmt.Sprintf("export: %v", err)
	}
	return res
}

// loadConfig reads the active storage configuration from ClickHouse.
func (e *Exporter) loadConfig(ctx context.Context) (Config, error) {
	rows, err := e.ch.Query(ctx,
		`SELECT provider, enabled, bucket, region, endpoint,
		        access_key, secret_key, prefix, schedule
		 FROM storage_config
		 WHERE enabled = 1
		 ORDER BY version DESC LIMIT 1`)
	if err != nil {
		return Config{}, err
	}
	defer rows.Close()

	var cfg Config
	var enabledU uint8
	if rows.Next() {
		if err := rows.Scan(
			&cfg.Provider, &enabledU, &cfg.Bucket, &cfg.Region, &cfg.Endpoint,
			&cfg.AccessKey, &cfg.SecretKey, &cfg.Prefix, &cfg.Schedule,
		); err != nil {
			return Config{}, err
		}
		cfg.Enabled = enabledU == 1
	}
	return cfg, nil
}
