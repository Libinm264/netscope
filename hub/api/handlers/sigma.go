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
	"github.com/netscope/hub-api/enterprise/sigma"
	"github.com/netscope/hub-api/util"
)

// SigmaHandler provides HTTP endpoints for Sigma detection rules and matches.
//
// Community plan: read-only access to the 5 built-in rules; cannot create/edit.
// Enterprise plan: full CRUD for custom rules; built-in rules still read-only.
type SigmaHandler struct {
	CH      *clickhouse.Client
	License *license.License
	Engine  *sigma.Engine
}

// ListRules returns all rules (built-in + custom).
func (h *SigmaHandler) ListRules(c *fiber.Ctx) error {
	if h.CH == nil {
		return c.JSON(fiber.Map{"rules": []any{}})
	}
	ctx := c.Context()
	rows, err := h.CH.Query(ctx,
		`SELECT id, title, description, severity, tags, query, enabled, builtin, created_at, updated_at
		 FROM sigma_rules
		 ORDER BY builtin DESC, created_at`)
	if err != nil {
		return util.InternalError(c, err)
	}
	defer rows.Close()

	type ruleRow struct {
		sigma.Rule
	}
	rules := make([]sigma.Rule, 0, 16)
	for rows.Next() {
		var r sigma.Rule
		var tagsJSON string
		var enabledU, builtinU uint8
		if err := rows.Scan(
			&r.ID, &r.Title, &r.Description, &r.Severity,
			&tagsJSON, &r.Query,
			&enabledU, &builtinU,
			&r.CreatedAt, &r.UpdatedAt,
		); err != nil {
			continue
		}
		r.Enabled = enabledU == 1
		r.Builtin = builtinU == 1
		_ = json.Unmarshal([]byte(tagsJSON), &r.Tags)
		rules = append(rules, r)
	}

	plan := h.planStr()
	return c.JSON(fiber.Map{
		"rules": rules,
		"plan":  plan,
	})
}

// CreateRule creates a new custom rule (Enterprise only).
func (h *SigmaHandler) CreateRule(c *fiber.Ctx) error {
	if h.CH == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "storage unavailable"})
	}
	if h.planStr() != "enterprise" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error":        "custom Sigma rules require Enterprise plan",
			"upgrade_hint": "Upgrade to Enterprise to create custom detection rules.",
		})
	}

	var body sigma.Rule
	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid body"})
	}
	if body.Title == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "title is required"})
	}
	if body.Query == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "query is required"})
	}
	// Only SELECT queries allowed.
	if !strings.HasPrefix(strings.TrimSpace(strings.ToUpper(body.Query)), "SELECT") {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "only SELECT queries are allowed"})
	}
	if body.Severity == "" {
		body.Severity = "medium"
	}

	tagsJSON, _ := json.Marshal(body.Tags)
	body.ID = uuid.NewString()
	now := time.Now()

	ctx := c.Context()
	if err := h.CH.Exec(ctx,
		`INSERT INTO sigma_rules
		 (id, title, description, severity, tags, query, enabled, builtin, created_at, updated_at, version)
		 VALUES (?, ?, ?, ?, ?, ?, ?, 0, ?, ?, ?)`,
		body.ID, body.Title, body.Description, body.Severity,
		string(tagsJSON), body.Query,
		boolToUint8(body.Enabled),
		now, now, now.UnixMilli(),
	); err != nil {
		return util.InternalError(c, err)
	}

	body.Builtin = false
	body.CreatedAt = now
	body.UpdatedAt = now
	return c.Status(fiber.StatusCreated).JSON(body)
}

// UpdateRule patches a custom rule's fields (Enterprise only; built-in rules immutable).
func (h *SigmaHandler) UpdateRule(c *fiber.Ctx) error {
	if h.CH == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "storage unavailable"})
	}
	if h.planStr() != "enterprise" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error":        "editing Sigma rules requires Enterprise plan",
			"upgrade_hint": "Upgrade to Enterprise to manage custom detection rules.",
		})
	}
	id := c.Params("id")

	// Load existing rule.
	ctx := c.Context()
	existing, err := h.loadRule(ctx, id)
	if err != nil || existing.ID == "" {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "rule not found"})
	}
	if existing.Builtin {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "built-in rules cannot be modified"})
	}

	var body sigma.Rule
	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid body"})
	}

	if body.Title != "" {
		existing.Title = body.Title
	}
	if body.Description != "" {
		existing.Description = body.Description
	}
	if body.Query != "" {
		if !strings.HasPrefix(strings.TrimSpace(strings.ToUpper(body.Query)), "SELECT") {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "only SELECT queries are allowed"})
		}
		existing.Query = body.Query
	}
	if body.Severity != "" {
		existing.Severity = body.Severity
	}
	if len(body.Tags) > 0 {
		existing.Tags = body.Tags
	}
	// Allow toggling enabled state.
	existing.Enabled = body.Enabled

	tagsJSON, _ := json.Marshal(existing.Tags)
	now := time.Now()

	if err := h.CH.Exec(ctx,
		`INSERT INTO sigma_rules
		 (id, title, description, severity, tags, query, enabled, builtin, created_at, updated_at, version)
		 VALUES (?, ?, ?, ?, ?, ?, ?, 0, ?, ?, ?)`,
		existing.ID, existing.Title, existing.Description, existing.Severity,
		string(tagsJSON), existing.Query,
		boolToUint8(existing.Enabled),
		existing.CreatedAt, now, now.UnixMilli(),
	); err != nil {
		return util.InternalError(c, err)
	}

	return c.JSON(fiber.Map{"ok": true})
}

// DeleteRule removes a custom rule (Enterprise only; built-in rules immutable).
func (h *SigmaHandler) DeleteRule(c *fiber.Ctx) error {
	if h.CH == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "storage unavailable"})
	}
	if h.planStr() != "enterprise" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error":        "deleting Sigma rules requires Enterprise plan",
			"upgrade_hint": "Upgrade to Enterprise to manage detection rules.",
		})
	}
	id := c.Params("id")

	ctx := c.Context()
	existing, err := h.loadRule(ctx, id)
	if err != nil || existing.ID == "" {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "rule not found"})
	}
	if existing.Builtin {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "built-in rules cannot be deleted"})
	}

	// Soft-delete: set enabled=0 and mark deleted via a sentinel version.
	now := time.Now()
	if err := h.CH.Exec(ctx,
		`INSERT INTO sigma_rules
		 (id, title, description, severity, tags, query, enabled, builtin, created_at, updated_at, version)
		 VALUES (?, ?, ?, ?, ?, ?, 0, 0, ?, ?, ?)`,
		existing.ID, existing.Title, existing.Description, existing.Severity,
		"[]", existing.Query,
		existing.CreatedAt, now, now.UnixMilli()+1,
	); err != nil {
		return util.InternalError(c, err)
	}
	// Hard-purge via ALTER TABLE DELETE for cleanliness (async, best-effort).
	go func() {
		delCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = h.CH.Exec(delCtx,
			`ALTER TABLE sigma_rules DELETE WHERE id = ?`, id)
	}()

	return c.JSON(fiber.Map{"ok": true})
}

// ListMatches returns recent sigma_matches with optional filters.
func (h *SigmaHandler) ListMatches(c *fiber.Ctx) error {
	if h.CH == nil {
		return c.JSON(fiber.Map{"matches": []any{}})
	}
	ruleID   := c.Query("rule_id")
	severity := c.Query("severity")
	limit    := c.QueryInt("limit", 100)
	if limit > 500 {
		limit = 500
	}

	where := "1=1"
	args := make([]interface{}, 0, 3)
	if ruleID != "" {
		where += " AND rule_id = ?"
		args = append(args, ruleID)
	}
	if severity != "" {
		where += " AND severity = ?"
		args = append(args, severity)
	}
	args = append(args, uint64(limit))

	ctx := c.Context()
	rows, err := h.CH.Query(ctx,
		"SELECT id, rule_id, rule_title, severity, match_data, fired_at "+
			"FROM sigma_matches WHERE "+where+" ORDER BY fired_at DESC LIMIT ?",
		args...)
	if err != nil {
		return util.InternalError(c, err)
	}
	defer rows.Close()

	matches := make([]sigma.Match, 0, limit)
	for rows.Next() {
		var m sigma.Match
		if err := rows.Scan(
			&m.ID, &m.RuleID, &m.RuleTitle, &m.Severity, &m.MatchData, &m.FiredAt,
		); err != nil {
			continue
		}
		matches = append(matches, m)
	}

	return c.JSON(fiber.Map{"matches": matches})
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func (h *SigmaHandler) planStr() string {
	if h.License != nil {
		return h.License.Plan
	}
	return "community"
}

func (h *SigmaHandler) loadRule(ctx context.Context, id string) (sigma.Rule, error) {
	rows, err := h.CH.Query(ctx,
		`SELECT id, title, description, severity, tags, query, enabled, builtin, created_at, updated_at
		 FROM sigma_rules
		 WHERE id = ? LIMIT 1`, id)
	if err != nil {
		return sigma.Rule{}, err
	}
	defer rows.Close()
	var r sigma.Rule
	if rows.Next() {
		var tagsJSON string
		var enabledU, builtinU uint8
		_ = rows.Scan(&r.ID, &r.Title, &r.Description, &r.Severity,
			&tagsJSON, &r.Query, &enabledU, &builtinU, &r.CreatedAt, &r.UpdatedAt)
		r.Enabled = enabledU == 1
		r.Builtin = builtinU == 1
		_ = json.Unmarshal([]byte(tagsJSON), &r.Tags)
	}
	return r, nil
}

func boolToUint8(b bool) uint8 {
	if b {
		return 1
	}
	return 0
}
