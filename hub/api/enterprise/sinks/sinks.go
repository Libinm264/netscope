// Enterprise Edition — see hub/enterprise/LICENSE (BSL-1.1)
//
// Package sinks implements the SIEM/log-sink dispatcher for NetScope Hub.
//
// Supported sinks:
//   - Splunk HEC   (HTTP Event Collector)
//   - Elastic/ECS  (Elasticsearch bulk API)
//   - Datadog Logs (HTTP intake v2)
//   - Grafana Loki (push API)
//
// The Dispatcher polls ClickHouse audit_events every 30 s and fans out
// batches of up to 500 events to all enabled sinks.  last_shipped is
// persisted per-sink in the integrations_config table so position is
// retained across hub restarts.
package sinks

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	chclient "github.com/netscope/hub-api/clickhouse"
)

// ── Sink types ────────────────────────────────────────────────────────────────

type SinkType string

const (
	SinkTypeSplunk  SinkType = "splunk"
	SinkTypeElastic SinkType = "elastic"
	SinkTypeDatadog SinkType = "datadog"
	SinkTypeLoki    SinkType = "loki"
)

// ValidSinkTypes is the set of recognised sink type names.
var ValidSinkTypes = map[SinkType]bool{
	SinkTypeSplunk:  true,
	SinkTypeElastic: true,
	SinkTypeDatadog: true,
	SinkTypeLoki:    true,
}

// SinkConfig is one row from integrations_config.
type SinkConfig struct {
	Type        SinkType       `json:"type"`
	Enabled     bool           `json:"enabled"`
	Config      map[string]any `json:"config"`
	LastShipped time.Time      `json:"last_shipped"`
	UpdatedAt   time.Time      `json:"updated_at"`
}

// AuditRow is a minimal copy of an audit_events row for fan-out.
type AuditRow struct {
	ID        string
	TokenID   string
	Role      string
	Method    string
	Path      string
	Status    uint16
	ClientIP  string
	LatencyMs uint32
	Ts        time.Time
}

// ── Dispatcher ────────────────────────────────────────────────────────────────

// Dispatcher polls audit_events and ships batches to configured SIEM sinks.
type Dispatcher struct {
	ch     *chclient.Client
	client *http.Client

	mu   sync.Mutex
	stop chan struct{}
}

// New creates a Dispatcher backed by the given ClickHouse client.
func New(ch *chclient.Client) *Dispatcher {
	return &Dispatcher{
		ch:     ch,
		client: &http.Client{Timeout: 15 * time.Second},
		stop:   make(chan struct{}),
	}
}

// Start launches the background dispatch loop.
func (d *Dispatcher) Start() {
	go d.loop()
	slog.Info("sinks: dispatcher started")
}

// Stop shuts down the background loop.
func (d *Dispatcher) Stop() {
	close(d.stop)
}

func (d *Dispatcher) loop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-d.stop:
			return
		case <-ticker.C:
			d.dispatch()
		}
	}
}

func (d *Dispatcher) dispatch() {
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	configs, err := d.loadConfigs(ctx)
	if err != nil {
		slog.Warn("sinks: load configs", "err", err)
		return
	}

	for _, cfg := range configs {
		if !cfg.Enabled {
			continue
		}
		events, err := d.fetchNewEvents(ctx, cfg.LastShipped)
		if err != nil {
			slog.Warn("sinks: fetch events", "sink", cfg.Type, "err", err)
			continue
		}
		if len(events) == 0 {
			continue
		}
		if err := d.sendToSink(ctx, cfg, events); err != nil {
			slog.Warn("sinks: send failed", "sink", cfg.Type, "err", err)
			continue
		}
		maxTs := events[len(events)-1].Ts
		if err := d.updateLastShipped(ctx, cfg, maxTs); err != nil {
			slog.Warn("sinks: update last_shipped", "sink", cfg.Type, "err", err)
		}
		slog.Info("sinks: shipped", "sink", cfg.Type, "count", len(events))
	}
}

// ── DB helpers ────────────────────────────────────────────────────────────────

func (d *Dispatcher) loadConfigs(ctx context.Context) ([]SinkConfig, error) {
	rows, err := d.ch.Query(ctx,
		`SELECT sink_type, enabled, config, last_shipped, updated_at
		 FROM integrations_config FINAL
		 ORDER BY sink_type`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []SinkConfig
	for rows.Next() {
		var (
			sinkType    string
			enabled     uint8
			configJSON  string
			lastShipped time.Time
			updatedAt   time.Time
		)
		if err := rows.Scan(&sinkType, &enabled, &configJSON, &lastShipped, &updatedAt); err != nil {
			continue
		}
		var cfg map[string]any
		_ = json.Unmarshal([]byte(configJSON), &cfg)
		if cfg == nil {
			cfg = map[string]any{}
		}
		out = append(out, SinkConfig{
			Type:        SinkType(sinkType),
			Enabled:     enabled == 1,
			Config:      cfg,
			LastShipped: lastShipped,
			UpdatedAt:   updatedAt,
		})
	}
	return out, nil
}

func (d *Dispatcher) fetchNewEvents(ctx context.Context, after time.Time) ([]AuditRow, error) {
	rows, err := d.ch.Query(ctx,
		`SELECT id, token_id, role, method, path, status, client_ip, latency_ms, ts
		 FROM audit_events
		 WHERE ts > ?
		 ORDER BY ts ASC
		 LIMIT 500`,
		after)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []AuditRow
	for rows.Next() {
		var e AuditRow
		if err := rows.Scan(
			&e.ID, &e.TokenID, &e.Role, &e.Method, &e.Path,
			&e.Status, &e.ClientIP, &e.LatencyMs, &e.Ts,
		); err != nil {
			continue
		}
		out = append(out, e)
	}
	return out, nil
}

func (d *Dispatcher) updateLastShipped(ctx context.Context, cfg SinkConfig, ts time.Time) error {
	cfgJSON, _ := json.Marshal(cfg.Config)
	enabled := uint8(0)
	if cfg.Enabled {
		enabled = 1
	}
	return d.ch.Exec(ctx,
		`INSERT INTO integrations_config
		 (sink_type, enabled, config, last_shipped, updated_at, version)
		 VALUES (?, ?, ?, ?, now64(), toUInt64(toUnixTimestamp64Milli(now64())))`,
		string(cfg.Type), enabled, string(cfgJSON), ts,
	)
}

// ── Fan-out ───────────────────────────────────────────────────────────────────

func (d *Dispatcher) sendToSink(ctx context.Context, cfg SinkConfig, events []AuditRow) error {
	switch cfg.Type {
	case SinkTypeSplunk:
		return d.sendSplunk(ctx, cfg.Config, events)
	case SinkTypeElastic:
		return d.sendElastic(ctx, cfg.Config, events)
	case SinkTypeDatadog:
		return d.sendDatadog(ctx, cfg.Config, events)
	case SinkTypeLoki:
		return d.sendLoki(ctx, cfg.Config, events)
	default:
		return fmt.Errorf("unknown sink type: %s", cfg.Type)
	}
}

// ── Splunk HEC ────────────────────────────────────────────────────────────────

func (d *Dispatcher) sendSplunk(ctx context.Context, cfg map[string]any, events []AuditRow) error {
	url, _ := cfg["url"].(string)
	token, _ := cfg["token"].(string)
	index, _ := cfg["index"].(string)
	if url == "" || token == "" {
		return fmt.Errorf("splunk: url and token are required")
	}

	var buf bytes.Buffer
	for _, e := range events {
		ev := map[string]any{
			"time":       float64(e.Ts.UnixMilli()) / 1000.0,
			"source":     "netscope",
			"sourcetype": "netscope:audit",
			"host":       "hub-api",
			"event": map[string]any{
				"id":         e.ID,
				"token_id":   e.TokenID,
				"role":       e.Role,
				"method":     e.Method,
				"path":       e.Path,
				"status":     e.Status,
				"client_ip":  e.ClientIP,
				"latency_ms": e.LatencyMs,
			},
		}
		if index != "" {
			ev["index"] = index
		}
		line, _ := json.Marshal(ev)
		buf.Write(line)
		buf.WriteByte('\n')
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url+"/services/collector/event", &buf)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Splunk "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := d.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("splunk returned %d", resp.StatusCode)
	}
	return nil
}

// ── Elasticsearch / ECS ───────────────────────────────────────────────────────

func (d *Dispatcher) sendElastic(ctx context.Context, cfg map[string]any, events []AuditRow) error {
	url, _ := cfg["url"].(string)
	apiKey, _ := cfg["api_key"].(string)
	index, _ := cfg["index"].(string)
	if url == "" {
		return fmt.Errorf("elastic: url is required")
	}
	if index == "" {
		index = "netscope-audit"
	}

	var buf bytes.Buffer
	for _, e := range events {
		meta, _ := json.Marshal(map[string]any{"index": map[string]string{"_index": index}})
		buf.Write(meta)
		buf.WriteByte('\n')

		outcome := "success"
		if e.Status >= 400 {
			outcome = "failure"
		}
		doc := map[string]any{
			"@timestamp": e.Ts.UTC().Format(time.RFC3339Nano),
			"event": map[string]any{
				"action":   e.Method + " " + e.Path,
				"category": []string{"web"},
				"type":     []string{"access"},
				"outcome":  outcome,
				"duration": int64(e.LatencyMs) * 1_000_000,
			},
			"source": map[string]string{"ip": e.ClientIP},
			"http": map[string]any{
				"request":  map[string]string{"method": e.Method, "target": e.Path},
				"response": map[string]any{"status_code": e.Status},
			},
			"netscope": map[string]any{
				"id": e.ID, "token_id": e.TokenID,
				"role": e.Role, "latency_ms": e.LatencyMs,
			},
		}
		docBytes, _ := json.Marshal(doc)
		buf.Write(docBytes)
		buf.WriteByte('\n')
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url+"/_bulk", &buf)
	if err != nil {
		return err
	}
	if apiKey != "" {
		req.Header.Set("Authorization", "ApiKey "+apiKey)
	}
	req.Header.Set("Content-Type", "application/x-ndjson")

	resp, err := d.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("elastic returned %d", resp.StatusCode)
	}
	return nil
}

// ── Datadog Logs ──────────────────────────────────────────────────────────────

func (d *Dispatcher) sendDatadog(ctx context.Context, cfg map[string]any, events []AuditRow) error {
	apiKey, _ := cfg["api_key"].(string)
	site, _ := cfg["site"].(string)
	if apiKey == "" {
		return fmt.Errorf("datadog: api_key is required")
	}
	if site == "" {
		site = "datadoghq.com"
	}

	logs := make([]map[string]any, 0, len(events))
	for _, e := range events {
		logs = append(logs, map[string]any{
			"ddsource":  "netscope",
			"ddtags":    fmt.Sprintf("role:%s,method:%s,status:%d", e.Role, e.Method, e.Status),
			"service":   "hub-api",
			"timestamp": e.Ts.UTC().Format(time.RFC3339),
			"message":   fmt.Sprintf("%s %s → %d (%dms) [%s]", e.Method, e.Path, e.Status, e.LatencyMs, e.Role),
			"attributes": map[string]any{
				"id": e.ID, "token_id": e.TokenID, "role": e.Role,
				"method": e.Method, "path": e.Path, "status": e.Status,
				"client_ip": e.ClientIP, "latency_ms": e.LatencyMs,
			},
		})
	}

	body, _ := json.Marshal(logs)
	req, err := http.NewRequestWithContext(ctx, "POST",
		fmt.Sprintf("https://http-intake.logs.%s/api/v2/logs", site),
		bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("DD-API-KEY", apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := d.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("datadog returned %d", resp.StatusCode)
	}
	return nil
}

// ── Grafana Loki ──────────────────────────────────────────────────────────────

func (d *Dispatcher) sendLoki(ctx context.Context, cfg map[string]any, events []AuditRow) error {
	lokiURL, _ := cfg["url"].(string)
	tenantID, _ := cfg["tenant_id"].(string)
	username, _ := cfg["username"].(string)
	password, _ := cfg["password"].(string)
	if lokiURL == "" {
		return fmt.Errorf("loki: url is required")
	}

	values := make([][]string, 0, len(events))
	for _, e := range events {
		ns := fmt.Sprintf("%d", e.Ts.UnixNano())
		line := fmt.Sprintf(`method=%s path=%q status=%d role=%s client_ip=%s latency_ms=%d token_id=%s`,
			e.Method, e.Path, e.Status, e.Role, e.ClientIP, e.LatencyMs, e.TokenID)
		values = append(values, []string{ns, line})
	}

	payload := map[string]any{
		"streams": []map[string]any{{
			"stream": map[string]string{"app": "netscope", "component": "hub-api"},
			"values": values,
		}},
	}

	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, "POST", lokiURL+"/loki/api/v1/push", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if tenantID != "" {
		req.Header.Set("X-Scope-OrgID", tenantID)
	}
	if username != "" {
		req.SetBasicAuth(username, password)
	}

	resp, err := d.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("loki returned %d", resp.StatusCode)
	}
	return nil
}

// ── Test connectivity ─────────────────────────────────────────────────────────

// TestResult is returned by TestSink.
type TestResult struct {
	OK        bool   `json:"ok"`
	LatencyMs int64  `json:"latency_ms,omitempty"`
	Error     string `json:"error,omitempty"`
}

// TestSink verifies connectivity for the given sink type and config.
// It does NOT ship real events — it only performs a lightweight health check.
func (d *Dispatcher) TestSink(sinkType SinkType, cfg map[string]any) TestResult {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	start := time.Now()
	var err error

	switch sinkType {
	case SinkTypeSplunk:
		url, _ := cfg["url"].(string)
		token, _ := cfg["token"].(string)
		req, _ := http.NewRequestWithContext(ctx, "GET", url+"/services/collector/health", nil)
		if req != nil {
			req.Header.Set("Authorization", "Splunk "+token)
			var resp *http.Response
			resp, err = d.client.Do(req)
			if err == nil {
				resp.Body.Close()
				if resp.StatusCode >= 400 {
					err = fmt.Errorf("HTTP %d", resp.StatusCode)
				}
			}
		}

	case SinkTypeElastic:
		url, _ := cfg["url"].(string)
		apiKey, _ := cfg["api_key"].(string)
		req, _ := http.NewRequestWithContext(ctx, "GET", url+"/", nil)
		if req != nil {
			if apiKey != "" {
				req.Header.Set("Authorization", "ApiKey "+apiKey)
			}
			var resp *http.Response
			resp, err = d.client.Do(req)
			if err == nil {
				resp.Body.Close()
				if resp.StatusCode >= 400 {
					err = fmt.Errorf("HTTP %d", resp.StatusCode)
				}
			}
		}

	case SinkTypeDatadog:
		apiKey, _ := cfg["api_key"].(string)
		site, _ := cfg["site"].(string)
		if site == "" {
			site = "datadoghq.com"
		}
		req, _ := http.NewRequestWithContext(ctx, "GET",
			fmt.Sprintf("https://api.%s/api/v1/validate", site), nil)
		if req != nil {
			req.Header.Set("DD-API-KEY", apiKey)
			var resp *http.Response
			resp, err = d.client.Do(req)
			if err == nil {
				resp.Body.Close()
				if resp.StatusCode >= 400 {
					err = fmt.Errorf("HTTP %d", resp.StatusCode)
				}
			}
		}

	case SinkTypeLoki:
		lokiURL, _ := cfg["url"].(string)
		req, _ := http.NewRequestWithContext(ctx, "GET", lokiURL+"/ready", nil)
		if req != nil {
			username, _ := cfg["username"].(string)
			password, _ := cfg["password"].(string)
			if username != "" {
				req.SetBasicAuth(username, password)
			}
			var resp *http.Response
			resp, err = d.client.Do(req)
			if err == nil {
				resp.Body.Close()
				if resp.StatusCode >= 400 {
					err = fmt.Errorf("HTTP %d", resp.StatusCode)
				}
			}
		}

	default:
		err = fmt.Errorf("unknown sink type: %s", sinkType)
	}

	latency := time.Since(start).Milliseconds()
	if err != nil {
		msg := err.Error()
		// Trim noisy URL prefix from http errors.
		if idx := strings.LastIndex(msg, ": "); idx >= 0 {
			msg = msg[idx+2:]
		}
		return TestResult{OK: false, Error: msg}
	}
	return TestResult{OK: true, LatencyMs: latency}
}
