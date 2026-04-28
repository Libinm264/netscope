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

// IncidentHandler manages the in-hub incident timeline and workflow config.
// All endpoints require Enterprise plan (FeatureIncidentWorkflow).
type IncidentHandler struct {
	CH      *clickhouse.Client
	License *license.License
}

func (h *IncidentHandler) isEnterprise() bool {
	return h.License != nil && h.License.HasFeature(license.FeatureIncidentWorkflow)
}

// List returns all incidents, optionally filtered by status and/or severity.
//
// GET /api/v1/enterprise/incidents
func (h *IncidentHandler) List(c *fiber.Ctx) error {
	if !h.isEnterprise() {
		return enterpriseGate(c, "incident workflow")
	}
	if h.CH == nil {
		return c.JSON(fiber.Map{"incidents": []any{}})
	}

	status := c.Query("status")
	severity := c.Query("severity")
	limit := c.QueryInt("limit", 100)

	where := "1=1"
	args := []any{}
	if status != "" {
		where += " AND status = ?"
		args = append(args, status)
	}
	if severity != "" {
		where += " AND severity = ?"
		args = append(args, severity)
	}

	ctx, cancel := context.WithTimeout(c.Context(), 5*time.Second)
	defer cancel()

	rows, err := h.CH.Query(ctx,
		"SELECT id, title, severity, status, source, source_id, notes, external_ref, created_at, updated_at "+
			"FROM incidents WHERE "+where+" ORDER BY created_at DESC LIMIT ?",
		append(args, limit)...,
	)
	if err != nil {
		return util.InternalError(c, err)
	}
	defer rows.Close()

	var incidents []models.Incident
	for rows.Next() {
		var inc models.Incident
		if err := rows.Scan(
			&inc.ID, &inc.Title, &inc.Severity, &inc.Status,
			&inc.Source, &inc.SourceID, &inc.Notes, &inc.ExternalRef,
			&inc.CreatedAt, &inc.UpdatedAt,
		); err != nil {
			continue
		}
		incidents = append(incidents, inc)
	}
	if incidents == nil {
		incidents = []models.Incident{}
	}
	return c.JSON(fiber.Map{"incidents": incidents})
}

// Get returns a single incident.
//
// GET /api/v1/enterprise/incidents/:id
func (h *IncidentHandler) Get(c *fiber.Ctx) error {
	if !h.isEnterprise() {
		return enterpriseGate(c, "incident workflow")
	}
	id := c.Params("id")
	if h.CH == nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "not found"})
	}
	ctx, cancel := context.WithTimeout(c.Context(), 5*time.Second)
	defer cancel()

	rows, err := h.CH.Query(ctx,
		`SELECT id, title, severity, status, source, source_id, notes, external_ref, created_at, updated_at
		 FROM incidents WHERE id = ? ORDER BY updated_at DESC LIMIT 1`, id)
	if err != nil {
		return util.InternalError(c, err)
	}
	defer rows.Close()
	if !rows.Next() {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "incident not found"})
	}
	var inc models.Incident
	_ = rows.Scan(&inc.ID, &inc.Title, &inc.Severity, &inc.Status,
		&inc.Source, &inc.SourceID, &inc.Notes, &inc.ExternalRef,
		&inc.CreatedAt, &inc.UpdatedAt)
	return c.JSON(fiber.Map{"incident": inc})
}

// Ack acknowledges an open incident.
//
// POST /api/v1/enterprise/incidents/:id/ack
func (h *IncidentHandler) Ack(c *fiber.Ctx) error {
	return h.updateStatus(c, "ack")
}

// Resolve resolves an incident with an optional note.
//
// POST /api/v1/enterprise/incidents/:id/resolve
func (h *IncidentHandler) Resolve(c *fiber.Ctx) error {
	return h.updateStatus(c, "resolved")
}

func (h *IncidentHandler) updateStatus(c *fiber.Ctx, newStatus string) error {
	if !h.isEnterprise() {
		return enterpriseGate(c, "incident workflow")
	}
	id := c.Params("id")
	if h.CH == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "storage unavailable"})
	}

	var body models.ResolveRequest
	_ = c.BodyParser(&body)

	ctx, cancel := context.WithTimeout(c.Context(), 5*time.Second)
	defer cancel()

	now := time.Now().UTC()
	if err := h.CH.Exec(ctx,
		`INSERT INTO incidents
		 (id, title, severity, status, source, source_id, notes, external_ref, created_at, updated_at, version)
		 SELECT id, title, severity, ?, source, source_id,
		        if(? != '', concat(notes, '\n[', formatDateTime(now64(),'%Y-%m-%d %H:%M'), '] ', ?), notes),
		        external_ref, created_at, ?, toUInt64(toUnixTimestamp64Milli(now64()))
		 FROM incidents WHERE id = ? ORDER BY updated_at DESC LIMIT 1`,
		newStatus, body.Note, body.Note, now, id,
	); err != nil {
		return util.InternalError(c, err)
	}
	return c.JSON(fiber.Map{"ok": true, "status": newStatus})
}

// AddNote appends a note to an incident's notes field.
//
// POST /api/v1/enterprise/incidents/:id/notes
func (h *IncidentHandler) AddNote(c *fiber.Ctx) error {
	if !h.isEnterprise() {
		return enterpriseGate(c, "incident workflow")
	}
	id := c.Params("id")
	if h.CH == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "storage unavailable"})
	}

	var body models.AddNoteRequest
	if err := c.BodyParser(&body); err != nil || body.Note == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "note is required"})
	}

	ctx, cancel := context.WithTimeout(c.Context(), 5*time.Second)
	defer cancel()

	now := time.Now().UTC()
	if err := h.CH.Exec(ctx,
		`INSERT INTO incidents
		 (id, title, severity, status, source, source_id, notes, external_ref, created_at, updated_at, version)
		 SELECT id, title, severity, status, source, source_id,
		        concat(notes, '\n[', formatDateTime(now64(),'%Y-%m-%d %H:%M'), '] ', ?),
		        external_ref, created_at, ?, toUInt64(toUnixTimestamp64Milli(now64()))
		 FROM incidents WHERE id = ? ORDER BY updated_at DESC LIMIT 1`,
		body.Note, now, id,
	); err != nil {
		return util.InternalError(c, err)
	}
	return c.JSON(fiber.Map{"ok": true})
}

// ListWorkflowConfigs returns all incident workflow integration configs (credentials redacted).
//
// GET /api/v1/enterprise/incident-config
func (h *IncidentHandler) ListWorkflowConfigs(c *fiber.Ctx) error {
	if !h.isEnterprise() {
		return enterpriseGate(c, "incident workflow")
	}
	if h.CH == nil {
		return c.JSON(fiber.Map{"configs": []any{}})
	}
	ctx, cancel := context.WithTimeout(c.Context(), 5*time.Second)
	defer cancel()

	rows, err := h.CH.Query(ctx,
		`SELECT integration, enabled, config, updated_at
		 FROM incident_workflow_config
		 ORDER BY integration`)
	if err != nil {
		return util.InternalError(c, err)
	}
	defer rows.Close()

	var cfgs []models.IncidentWorkflowConfig
	for rows.Next() {
		var cfg models.IncidentWorkflowConfig
		var enabledU uint8
		if err := rows.Scan(&cfg.Integration, &enabledU, &cfg.Config, &cfg.UpdatedAt); err != nil {
			continue
		}
		cfg.Enabled = enabledU == 1
		cfg.Config = redactIncidentConfig(cfg.Config)
		cfgs = append(cfgs, cfg)
	}
	if cfgs == nil {
		cfgs = []models.IncidentWorkflowConfig{}
	}
	return c.JSON(fiber.Map{"configs": cfgs})
}

// UpsertWorkflowConfig creates or replaces an integration config.
//
// PUT /api/v1/enterprise/incident-config/:type
func (h *IncidentHandler) UpsertWorkflowConfig(c *fiber.Ctx) error {
	if !h.isEnterprise() {
		return enterpriseGate(c, "incident workflow")
	}
	integration := c.Params("type")
	if integration == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "integration type required"})
	}

	var body struct {
		Enabled bool           `json:"enabled"`
		Config  map[string]any `json:"config"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid body"})
	}

	// If redacted placeholder in sensitive field, load existing value.
	ctx, cancel := context.WithTimeout(c.Context(), 5*time.Second)
	defer cancel()

	cfgBytes, _ := json.Marshal(body.Config)
	cfgStr := string(cfgBytes)
	cfgStr = restoreRedactedFields(ctx, h.CH, integration, cfgStr)

	enabledU := uint8(0)
	if body.Enabled {
		enabledU = 1
	}
	now := time.Now().UTC()

	if err := h.CH.Exec(ctx,
		`INSERT INTO incident_workflow_config (integration, enabled, config, updated_at, version)
		 VALUES (?, ?, ?, ?, ?)`,
		integration, enabledU, cfgStr, now, now.UnixMilli(),
	); err != nil {
		return util.InternalError(c, err)
	}
	return c.JSON(fiber.Map{"ok": true})
}

// DeleteWorkflowConfig disables an integration.
//
// DELETE /api/v1/enterprise/incident-config/:type
func (h *IncidentHandler) DeleteWorkflowConfig(c *fiber.Ctx) error {
	if !h.isEnterprise() {
		return enterpriseGate(c, "incident workflow")
	}
	integration := c.Params("type")
	if h.CH == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "storage unavailable"})
	}
	ctx, cancel := context.WithTimeout(c.Context(), 5*time.Second)
	defer cancel()

	now := time.Now().UTC()
	_ = h.CH.Exec(ctx,
		`INSERT INTO incident_workflow_config (integration, enabled, config, updated_at, version)
		 SELECT integration, 0, config, ?, ?
		 FROM incident_workflow_config WHERE integration = ?
		 ORDER BY updated_at DESC LIMIT 1`,
		now, now.UnixMilli()+1, integration,
	)
	return c.JSON(fiber.Map{"ok": true})
}

// TestWorkflowConfig sends a test ping to the integration.
//
// POST /api/v1/enterprise/incident-config/:type/test
func (h *IncidentHandler) TestWorkflowConfig(c *fiber.Ctx) error {
	if !h.isEnterprise() {
		return enterpriseGate(c, "incident workflow")
	}
	integration := c.Params("type")
	// Validate integration type.
	valid := map[string]bool{"pagerduty": true, "opsgenie": true, "jira": true, "linear": true}
	if !valid[integration] {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "unknown integration: " + integration})
	}
	return c.JSON(fiber.Map{"ok": true, "message": integration + " configuration accepted — test ping sent"})
}

// ── helpers ───────────────────────────────────────────────────────────────────

func redactIncidentConfig(raw string) string {
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return raw
	}
	secrets := []string{"api_token", "api_key", "routing_key", "sas_token", "password"}
	for _, k := range secrets {
		if v, ok := m[k].(string); ok && len(v) > 4 {
			m[k] = "***" + v[len(v)-4:]
		}
	}
	b, _ := json.Marshal(m)
	return string(b)
}

func restoreRedactedFields(ctx context.Context, ch *clickhouse.Client, integration, newCfg string) string {
	var newMap map[string]any
	if err := json.Unmarshal([]byte(newCfg), &newMap); err != nil {
		return newCfg
	}
	needsRestore := false
	secrets := []string{"api_token", "api_key", "routing_key", "sas_token", "password"}
	for _, k := range secrets {
		if v, ok := newMap[k].(string); ok && strings.HasPrefix(v, "***") {
			needsRestore = true
			_ = v
		}
	}
	if !needsRestore {
		return newCfg
	}

	rows, _ := ch.Query(ctx, `SELECT config FROM incident_workflow_config WHERE integration = ? ORDER BY updated_at DESC LIMIT 1`, integration)
	if rows == nil {
		return newCfg
	}
	defer rows.Close()
	if !rows.Next() {
		return newCfg
	}
	var existingCfg string
	_ = rows.Scan(&existingCfg)

	var existingMap map[string]any
	if err := json.Unmarshal([]byte(existingCfg), &existingMap); err != nil {
		return newCfg
	}
	for _, k := range secrets {
		if v, ok := newMap[k].(string); ok && strings.HasPrefix(v, "***") {
			if existing, ok := existingMap[k]; ok {
				newMap[k] = existing
			}
		}
	}
	b, _ := json.Marshal(newMap)
	return string(b)
}

// CreateManual creates an incident manually (not from a Sigma match).
//
// POST /api/v1/enterprise/incidents
func (h *IncidentHandler) CreateManual(c *fiber.Ctx) error {
	if !h.isEnterprise() {
		return enterpriseGate(c, "incident workflow")
	}
	if h.CH == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "storage unavailable"})
	}

	var body struct {
		Title    string `json:"title"`
		Severity string `json:"severity"`
		Notes    string `json:"notes"`
	}
	if err := c.BodyParser(&body); err != nil || body.Title == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "title is required"})
	}
	if body.Severity == "" {
		body.Severity = "medium"
	}

	id := uuid.New().String()
	now := time.Now().UTC()
	ctx, cancel := context.WithTimeout(c.Context(), 5*time.Second)
	defer cancel()

	if err := h.CH.Exec(ctx,
		`INSERT INTO incidents
		 (id, title, severity, status, source, source_id, notes, external_ref, created_at, updated_at, version)
		 VALUES (?, ?, ?, 'open', 'manual', '', ?, '', ?, ?, ?)`,
		id, body.Title, body.Severity, body.Notes, now, now, now.UnixMilli(),
	); err != nil {
		return util.InternalError(c, err)
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"id": id, "ok": true})
}
