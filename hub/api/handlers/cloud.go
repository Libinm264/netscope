package handlers

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/netscope/hub-api/clickhouse"
	"github.com/netscope/hub-api/enterprise/license"
	"github.com/netscope/hub-api/models"
	"github.com/netscope/hub-api/util"
)

// CloudSourceHandler manages cloud VPC flow log pull sources.
//
// Tier: AWS sources are Community. GCP + Azure require Enterprise.
type CloudSourceHandler struct {
	CH      *clickhouse.Client
	License *license.License
}

// List returns all configured cloud flow sources.
//
// GET /api/v1/cloud/sources
func (h *CloudSourceHandler) List(c *fiber.Ctx) error {
	if h.CH == nil {
		return c.JSON(fiber.Map{"sources": []any{}})
	}
	ctx, cancel := context.WithTimeout(c.Context(), 5*time.Second)
	defer cancel()

	rows, err := h.CH.Query(ctx,
		`SELECT id, provider, name, config, enabled, last_pulled, error_msg, created_at
		 FROM cloud_flow_sources
		 ORDER BY created_at DESC`)
	if err != nil {
		return util.InternalError(c, err)
	}
	defer rows.Close()

	var sources []models.CloudSource
	for rows.Next() {
		var s models.CloudSource
		var enabledU uint8
		if err := rows.Scan(
			&s.ID, &s.Provider, &s.Name, &s.Config, &enabledU,
			&s.LastPulled, &s.ErrorMsg, &s.CreatedAt,
		); err != nil {
			continue
		}
		s.Enabled = enabledU == 1
		// Redact secret fields in config JSON.
		s.Config = redactCloudConfig(s.Config)
		sources = append(sources, s)
	}
	if sources == nil {
		sources = []models.CloudSource{}
	}
	return c.JSON(fiber.Map{"sources": sources})
}

// Create adds a new cloud flow source.
//
// POST /api/v1/cloud/sources
func (h *CloudSourceHandler) Create(c *fiber.Ctx) error {
	var body struct {
		Provider string         `json:"provider"`
		Name     string         `json:"name"`
		Config   map[string]any `json:"config"`
		Enabled  bool           `json:"enabled"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid body"})
	}
	if body.Provider == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "provider is required"})
	}
	body.Provider = strings.ToLower(body.Provider)

	// Enterprise gate for GCP + Azure.
	if err := h.checkProviderTier(c, body.Provider); err != nil {
		return err
	}

	if h.CH == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "storage unavailable"})
	}

	cfgBytes, _ := json.Marshal(body.Config)
	id := uuid.New().String()
	now := time.Now().UTC()
	enabledU := uint8(0)
	if body.Enabled {
		enabledU = 1
	}

	ctx, cancel := context.WithTimeout(c.Context(), 5*time.Second)
	defer cancel()

	if err := h.CH.Exec(ctx,
		`INSERT INTO cloud_flow_sources
		 (id, provider, name, config, enabled, last_pulled, error_msg, created_at, version)
		 VALUES (?, ?, ?, ?, ?, toDateTime64(0,3), '', ?, ?)`,
		id, body.Provider, body.Name, string(cfgBytes), enabledU, now, now.UnixMilli(),
	); err != nil {
		return util.InternalError(c, err)
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"id": id, "ok": true})
}

// Update modifies an existing cloud flow source.
//
// PATCH /api/v1/cloud/sources/:id
func (h *CloudSourceHandler) Update(c *fiber.Ctx) error {
	id := c.Params("id")
	if id == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "id required"})
	}
	if h.CH == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "storage unavailable"})
	}

	var body struct {
		Name    *string        `json:"name"`
		Config  map[string]any `json:"config"`
		Enabled *bool          `json:"enabled"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid body"})
	}

	ctx, cancel := context.WithTimeout(c.Context(), 5*time.Second)
	defer cancel()

	// Load current record first.
	rows, err := h.CH.Query(ctx,
		`SELECT id, provider, name, config, enabled, last_pulled, error_msg, created_at
		 FROM cloud_flow_sources WHERE id = ?
		 ORDER BY created_at DESC LIMIT 1`, id)
	if err != nil {
		return util.InternalError(c, err)
	}
	defer rows.Close()
	if !rows.Next() {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "source not found"})
	}
	var cur models.CloudSource
	var enabledU uint8
	_ = rows.Scan(&cur.ID, &cur.Provider, &cur.Name, &cur.Config, &enabledU,
		&cur.LastPulled, &cur.ErrorMsg, &cur.CreatedAt)
	cur.Enabled = enabledU == 1
	rows.Close()

	if body.Name != nil {
		cur.Name = *body.Name
	}
	if body.Enabled != nil {
		cur.Enabled = *body.Enabled
	}
	if body.Config != nil {
		cfgBytes, _ := json.Marshal(body.Config)
		cur.Config = string(cfgBytes)
	}

	enabledUNew := uint8(0)
	if cur.Enabled {
		enabledUNew = 1
	}
	now := time.Now().UTC()

	if err := h.CH.Exec(ctx,
		`INSERT INTO cloud_flow_sources
		 (id, provider, name, config, enabled, last_pulled, error_msg, created_at, version)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		cur.ID, cur.Provider, cur.Name, cur.Config, enabledUNew,
		cur.LastPulled, cur.ErrorMsg, cur.CreatedAt, now.UnixMilli(),
	); err != nil {
		return util.InternalError(c, err)
	}
	return c.JSON(fiber.Map{"ok": true})
}

// Delete removes a cloud flow source.
//
// DELETE /api/v1/cloud/sources/:id
func (h *CloudSourceHandler) Delete(c *fiber.Ctx) error {
	id := c.Params("id")
	if id == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "id required"})
	}
	if h.CH == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "storage unavailable"})
	}
	ctx, cancel := context.WithTimeout(c.Context(), 5*time.Second)
	defer cancel()

	// Soft-delete: write a disabled row with empty config.
	now := time.Now().UTC()
	_ = h.CH.Exec(ctx,
		`INSERT INTO cloud_flow_sources
		 (id, provider, name, config, enabled, last_pulled, error_msg, created_at, version)
		 SELECT id, provider, name, config, 0, last_pulled, error_msg, created_at, ?
		 FROM cloud_flow_sources WHERE id = ?
		 ORDER BY created_at DESC LIMIT 1`,
		now.UnixMilli()+1, id,
	)
	return c.JSON(fiber.Map{"ok": true})
}

// PullLog returns the last 50 pull run results for a source.
//
// GET /api/v1/cloud/sources/:id/log
func (h *CloudSourceHandler) PullLog(c *fiber.Ctx) error {
	id := c.Params("id")
	if h.CH == nil {
		return c.JSON(fiber.Map{"log": []any{}})
	}
	ctx, cancel := context.WithTimeout(c.Context(), 5*time.Second)
	defer cancel()

	rows, err := h.CH.Query(ctx,
		`SELECT id, source_id, provider, rows_ingested, pulled_at, duration_ms, error
		 FROM cloud_flow_pull_log
		 WHERE source_id = ?
		 ORDER BY pulled_at DESC LIMIT 50`, id)
	if err != nil {
		return util.InternalError(c, err)
	}
	defer rows.Close()

	var log []models.CloudPullResult
	for rows.Next() {
		var r models.CloudPullResult
		if err := rows.Scan(
			&r.ID, &r.SourceID, &r.Provider, &r.RowsIngested,
			&r.PulledAt, &r.DurationMs, &r.Error,
		); err != nil {
			continue
		}
		log = append(log, r)
	}
	if log == nil {
		log = []models.CloudPullResult{}
	}
	return c.JSON(fiber.Map{"log": log})
}

// ── helpers ───────────────────────────────────────────────────────────────────

func (h *CloudSourceHandler) checkProviderTier(c *fiber.Ctx, provider string) error {
	if h.License == nil {
		return nil
	}
	switch provider {
	case "gcp":
		if !h.License.HasFeature(license.FeatureCloudIngestGCP) {
			return c.Status(fiber.StatusPaymentRequired).JSON(fiber.Map{
				"error":        "GCP VPC flow ingestion requires Enterprise plan",
				"upgrade_hint": "Upgrade to Enterprise to ingest GCP Pub/Sub VPC flow logs.",
			})
		}
	case "azure":
		if !h.License.HasFeature(license.FeatureCloudIngestAzure) {
			return c.Status(fiber.StatusPaymentRequired).JSON(fiber.Map{
				"error":        "Azure NSG flow ingestion requires Enterprise plan",
				"upgrade_hint": "Upgrade to Enterprise to ingest Azure NSG flow logs.",
			})
		}
	}
	return nil
}

// redactCloudConfig masks sensitive credential fields in the stored config JSON.
func redactCloudConfig(raw string) string {
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return raw
	}
	secretKeys := []string{"secret_access_key", "credentials_json", "sas_token", "connection_string"}
	for _, k := range secretKeys {
		if v, ok := m[k].(string); ok && len(v) > 4 {
			m[k] = "***" + v[len(v)-4:]
		}
	}
	b, _ := json.Marshal(m)
	return string(b)
}
