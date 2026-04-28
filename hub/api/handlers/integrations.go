package handlers

import (
	"context"
	"encoding/json"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/netscope/hub-api/clickhouse"
	"github.com/netscope/hub-api/enterprise/license"
	"github.com/netscope/hub-api/enterprise/sinks"
)

// IntegrationsHandler manages SIEM sink configurations.
type IntegrationsHandler struct {
	CH      *clickhouse.Client
	License *license.License
	Sinks   *sinks.Dispatcher
}

// integrationRow is the shape of a row returned to the client.
type integrationRow struct {
	Type        string         `json:"type"`
	Enabled     bool           `json:"enabled"`
	Config      map[string]any `json:"config"`
	LastShipped string         `json:"last_shipped,omitempty"`
	UpdatedAt   string         `json:"updated_at,omitempty"`
}

// List handles GET /api/v1/enterprise/integrations
//
// Returns all configured SIEM sinks.  Missing sinks are absent from the
// response (not pre-populated with defaults) so the UI can distinguish
// "not yet configured" from "configured but disabled".
func (h *IntegrationsHandler) List(c *fiber.Ctx) error {
	if h.CH == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "ClickHouse unavailable"})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := h.CH.Query(ctx,
		`SELECT sink_type, enabled, config, last_shipped, updated_at
		 FROM integrations_config
		 ORDER BY sink_type`)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	defer rows.Close()

	out := make([]integrationRow, 0)
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
		// Redact secrets from the response — never return tokens/keys to the browser.
		redactSecrets(cfg)

		row := integrationRow{
			Type:      sinkType,
			Enabled:   enabled == 1,
			Config:    cfg,
			UpdatedAt: updatedAt.UTC().Format(time.RFC3339),
		}
		if !lastShipped.IsZero() && lastShipped.Year() > 1970 {
			row.LastShipped = lastShipped.UTC().Format(time.RFC3339)
		}
		out = append(out, row)
	}

	return c.JSON(fiber.Map{"integrations": out})
}

// Upsert handles PUT /api/v1/enterprise/integrations/:type
//
// Creates or replaces the configuration for a sink.  The request body is:
//
//	{ "enabled": true, "config": { ... sink-specific fields ... } }
//
// last_shipped is preserved when re-configuring an existing sink so the
// dispatcher does not re-ship already-delivered events.
func (h *IntegrationsHandler) Upsert(c *fiber.Ctx) error {
	if h.CH == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "ClickHouse unavailable"})
	}
	if !h.License.HasFeature(license.FeatureAuditExport) {
		return c.Status(fiber.StatusPaymentRequired).JSON(fiber.Map{
			"error": "SIEM integrations require the Team or Enterprise plan",
		})
	}

	sinkType := sinks.SinkType(c.Params("type"))
	if !sinks.ValidSinkTypes[sinkType] {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "unknown sink type — valid values: splunk, elastic, datadog, loki",
		})
	}

	var body struct {
		Enabled bool           `json:"enabled"`
		Config  map[string]any `json:"config"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid JSON body"})
	}
	if body.Config == nil {
		body.Config = map[string]any{}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Preserve last_shipped so we don't re-send already-delivered events.
	var lastShipped time.Time
	rows, err := h.CH.Query(ctx,
		`SELECT last_shipped FROM integrations_config WHERE sink_type = ? ORDER BY updated_at DESC LIMIT 1`,
		string(sinkType))
	if err == nil {
		if rows.Next() {
			_ = rows.Scan(&lastShipped)
		}
		rows.Close()
	}

	cfgJSON, _ := json.Marshal(body.Config)
	enabled := uint8(0)
	if body.Enabled {
		enabled = 1
	}

	if err := h.CH.Exec(ctx,
		`INSERT INTO integrations_config
		 (sink_type, enabled, config, last_shipped, updated_at, version)
		 VALUES (?, ?, ?, ?, now64(), toUInt64(toUnixTimestamp64Milli(now64())))`,
		string(sinkType), enabled, string(cfgJSON), lastShipped,
	); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"ok": true})
}

// Delete handles DELETE /api/v1/enterprise/integrations/:type
//
// Disables the sink and wipes its configuration.
func (h *IntegrationsHandler) Delete(c *fiber.Ctx) error {
	if h.CH == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "ClickHouse unavailable"})
	}

	sinkType := sinks.SinkType(c.Params("type"))
	if !sinks.ValidSinkTypes[sinkType] {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "unknown sink type",
		})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := h.CH.Exec(ctx,
		`INSERT INTO integrations_config
		 (sink_type, enabled, config, last_shipped, updated_at, version)
		 VALUES (?, 0, '{}', toDateTime64(0, 3), now64(), toUInt64(toUnixTimestamp64Milli(now64())))`,
		string(sinkType),
	); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"ok": true})
}

// Test handles POST /api/v1/enterprise/integrations/:type/test
//
// Verifies connectivity for the given sink using the config provided in the
// request body.  No real events are shipped; this is a lightweight health check.
//
//	Request:  { "config": { ... sink-specific fields ... } }
//	Response: { "ok": true, "latency_ms": 42 }
//	       or { "ok": false, "error": "connection refused" }
func (h *IntegrationsHandler) Test(c *fiber.Ctx) error {
	sinkType := sinks.SinkType(c.Params("type"))
	if !sinks.ValidSinkTypes[sinkType] {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "unknown sink type",
		})
	}
	if !h.License.HasFeature(license.FeatureAuditExport) {
		return c.Status(fiber.StatusPaymentRequired).JSON(fiber.Map{
			"error": "SIEM integrations require the Team or Enterprise plan",
		})
	}

	var body struct {
		Config map[string]any `json:"config"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid JSON body"})
	}

	result := h.Sinks.TestSink(sinkType, body.Config)
	return c.JSON(result)
}

// ── helpers ───────────────────────────────────────────────────────────────────

// redactSecrets replaces sensitive field values with "***" so they are not
// returned to the browser via the List endpoint.  Only the presence of the
// field is shown, not its value.
func redactSecrets(cfg map[string]any) {
	secretKeys := []string{"token", "api_key", "password", "secret"}
	for _, k := range secretKeys {
		if _, ok := cfg[k]; ok {
			cfg[k] = "***"
		}
	}
}
