package handlers

import (
	"log/slog"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/netscope/hub-api/clickhouse"
	"github.com/netscope/hub-api/models"
	"github.com/netscope/hub-api/util"
)

// AlertHandler manages alert rule CRUD and recent event queries.
type AlertHandler struct {
	CH *clickhouse.Client
}

// ListRules handles GET /api/v1/alerts.
func (h *AlertHandler) ListRules(c *fiber.Ctx) error {
	if h.CH == nil {
		return c.Status(503).JSON(fiber.Map{"error": "ClickHouse not available"})
	}

	rows, err := h.CH.Query(c.Context(),
		`SELECT id, name, metric, condition, threshold, window_minutes,
		        integration_type, webhook_url, enabled, cooldown_minutes, created_at
		 FROM alert_rules ORDER BY created_at DESC`)
	if err != nil {
		return util.InternalError(c, err)
	}
	defer rows.Close()

	rules := make([]models.AlertRule, 0)
	for rows.Next() {
		var r models.AlertRule
		var enabledInt uint8
		if err := rows.Scan(
			&r.ID, &r.Name, &r.Metric, &r.Condition,
			&r.Threshold, &r.WindowMinutes,
			&r.IntegrationType, &r.WebhookURL, &enabledInt, &r.CooldownMinutes, &r.CreatedAt,
		); err != nil {
			slog.Warn("alerts: scan rule", "err", err)
			continue
		}
		r.Enabled = enabledInt == 1
		rules = append(rules, r)
	}
	return c.JSON(fiber.Map{"rules": rules})
}

// CreateRule handles POST /api/v1/alerts.
func (h *AlertHandler) CreateRule(c *fiber.Ctx) error {
	var req models.CreateAlertRuleRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid body: " + err.Error()})
	}

	// Validation
	validMetrics := map[string]bool{
		"flows_per_minute":      true,
		"http_error_rate":       true,
		"dns_nxdomain_rate":     true,
		"anomaly_flow_rate":     true,
		"anomaly_http_latency":  true,
	}
	if !validMetrics[req.Metric] {
		return c.Status(400).JSON(fiber.Map{
			"error": "unknown metric: " + req.Metric,
		})
	}
	validIntegrations := map[string]bool{
		"":           true, // default → webhook
		"webhook":    true,
		"slack":      true,
		"pagerduty":  true,
		"opsgenie":   true,
		"teams":      true,
	}
	if !validIntegrations[req.IntegrationType] {
		return c.Status(400).JSON(fiber.Map{
			"error": "integration_type must be one of: webhook, slack, pagerduty, opsgenie, teams",
		})
	}
	if req.IntegrationType == "" {
		req.IntegrationType = "webhook"
	}
	if req.Condition != "gt" && req.Condition != "lt" {
		return c.Status(400).JSON(fiber.Map{"error": "condition must be 'gt' or 'lt'"})
	}
	if req.Name == "" {
		return c.Status(400).JSON(fiber.Map{"error": "name is required"})
	}
	if req.WindowMinutes == 0 {
		req.WindowMinutes = 5
	}
	if req.CooldownMinutes == 0 {
		req.CooldownMinutes = 15
	}
	// Validate webhook URL to prevent SSRF attacks
	if req.WebhookURL != "" {
		if err := util.ValidateWebhookURL(req.WebhookURL); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
	}

	if h.CH == nil {
		return c.Status(503).JSON(fiber.Map{"error": "ClickHouse not available"})
	}

	id := uuid.New().String()
	now := time.Now().UTC()
	if err := h.CH.Exec(c.Context(),
		`INSERT INTO alert_rules
		 (id, name, metric, condition, threshold, window_minutes,
		  integration_type, webhook_url, enabled, cooldown_minutes, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, 1, ?, ?)`,
		id, req.Name, req.Metric, req.Condition, req.Threshold,
		req.WindowMinutes, req.IntegrationType, req.WebhookURL, req.CooldownMinutes, now,
	); err != nil {
		return util.InternalError(c, err)
	}

	slog.Info("alert rule created", "id", id, "name", req.Name)
	return c.Status(201).JSON(fiber.Map{"id": id, "created_at": now})
}

// UpdateRule handles PATCH /api/v1/alerts/:id (toggle enabled / update webhook).
func (h *AlertHandler) UpdateRule(c *fiber.Ctx) error {
	id := c.Params("id")
	if id == "" {
		return c.Status(400).JSON(fiber.Map{"error": "id is required"})
	}

	var body struct {
		Enabled    *bool   `json:"enabled"`
		WebhookURL *string `json:"webhook_url"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid body"})
	}

	if h.CH == nil {
		return c.Status(503).JSON(fiber.Map{"error": "ClickHouse not available"})
	}

	if body.Enabled != nil {
		var v uint8
		if *body.Enabled {
			v = 1
		}
		if err := h.CH.Exec(c.Context(),
			`ALTER TABLE alert_rules UPDATE enabled = ? WHERE id = ?`, v, id,
		); err != nil {
			return util.InternalError(c, err)
		}
	}
	if body.WebhookURL != nil {
		// Re-validate on update — prevents SSRF via webhook URL change
		if *body.WebhookURL != "" {
			if err := util.ValidateWebhookURL(*body.WebhookURL); err != nil {
				return c.Status(400).JSON(fiber.Map{"error": err.Error()})
			}
		}
		if err := h.CH.Exec(c.Context(),
			`ALTER TABLE alert_rules UPDATE webhook_url = ? WHERE id = ?`, *body.WebhookURL, id,
		); err != nil {
			return util.InternalError(c, err)
		}
	}

	return c.JSON(fiber.Map{"ok": true})
}

// DeleteRule handles DELETE /api/v1/alerts/:id.
func (h *AlertHandler) DeleteRule(c *fiber.Ctx) error {
	id := c.Params("id")
	if id == "" {
		return c.Status(400).JSON(fiber.Map{"error": "id is required"})
	}
	if h.CH == nil {
		return c.Status(503).JSON(fiber.Map{"error": "ClickHouse not available"})
	}
	if err := h.CH.Exec(c.Context(),
		`ALTER TABLE alert_rules DELETE WHERE id = ?`, id,
	); err != nil {
		return util.InternalError(c, err)
	}
	return c.JSON(fiber.Map{"ok": true})
}

// ListEvents handles GET /api/v1/alerts/events.
func (h *AlertHandler) ListEvents(c *fiber.Ctx) error {
	if h.CH == nil {
		return c.Status(503).JSON(fiber.Map{"error": "ClickHouse not available"})
	}
	limit := c.QueryInt("limit", 50)
	rows, err := h.CH.Query(c.Context(),
		`SELECT id, rule_id, rule_name, metric, value, threshold, fired_at, delivered
		 FROM alert_events ORDER BY fired_at DESC LIMIT ?`, uint64(limit))
	if err != nil {
		return util.InternalError(c, err)
	}
	defer rows.Close()

	events := make([]models.AlertEvent, 0)
	for rows.Next() {
		var e models.AlertEvent
		var deliveredInt uint8
		if err := rows.Scan(
			&e.ID, &e.RuleID, &e.RuleName, &e.Metric,
			&e.Value, &e.Threshold, &e.FiredAt, &deliveredInt,
		); err != nil {
			continue
		}
		e.Delivered = deliveredInt == 1
		events = append(events, e)
	}
	return c.JSON(fiber.Map{"events": events})
}
