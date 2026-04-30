package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/netscope/hub-api/clickhouse"
	"github.com/netscope/hub-api/util"
)

// DashboardHandler manages custom dashboards and widget data endpoints.
type DashboardHandler struct {
	CH *clickhouse.Client
}

type dashboardMeta struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Widgets     json.RawMessage `json:"widgets"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

// ── List ──────────────────────────────────────────────────────────────────────

// List handles GET /api/v1/dashboards
func (h *DashboardHandler) List(c *fiber.Ctx) error {
	if h.CH == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "database unavailable"})
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// FINAL is acceptable here — dashboards is tiny (≪1000 rows).
	rows, err := h.CH.Query(ctx,
		`SELECT id, name, description, widgets, created_at, updated_at
		 FROM dashboards FINAL
		 WHERE is_deleted = 0
		 ORDER BY updated_at DESC`)
	if err != nil {
		return util.InternalError(c, err)
	}
	defer rows.Close()

	result := make([]dashboardMeta, 0)
	for rows.Next() {
		var d dashboardMeta
		var ws string
		if err := rows.Scan(&d.ID, &d.Name, &d.Description, &ws, &d.CreatedAt, &d.UpdatedAt); err != nil {
			continue
		}
		if ws == "" {
			ws = "[]"
		}
		d.Widgets = json.RawMessage(ws)
		result = append(result, d)
	}
	return c.JSON(fiber.Map{"dashboards": result})
}

// ── Get ───────────────────────────────────────────────────────────────────────

// Get handles GET /api/v1/dashboards/:id
func (h *DashboardHandler) Get(c *fiber.Ctx) error {
	if h.CH == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "database unavailable"})
	}
	id := c.Params("id")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := h.CH.Query(ctx,
		`SELECT id, name, description, widgets, is_deleted, created_at, updated_at
		 FROM dashboards
		 WHERE id = ?
		 ORDER BY version DESC LIMIT 1`, id)
	if err != nil {
		return util.InternalError(c, err)
	}
	defer rows.Close()

	if !rows.Next() {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "dashboard not found"})
	}
	var d dashboardMeta
	var ws string
	var isDeleted uint8
	if err := rows.Scan(&d.ID, &d.Name, &d.Description, &ws, &isDeleted, &d.CreatedAt, &d.UpdatedAt); err != nil {
		return util.InternalError(c, err)
	}
	if isDeleted == 1 {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "dashboard not found"})
	}
	if ws == "" {
		ws = "[]"
	}
	d.Widgets = json.RawMessage(ws)
	return c.JSON(d)
}

// ── Create ────────────────────────────────────────────────────────────────────

type createDashboardRequest struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Widgets     json.RawMessage `json:"widgets"`
}

// Create handles POST /api/v1/dashboards
func (h *DashboardHandler) Create(c *fiber.Ctx) error {
	if h.CH == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "database unavailable"})
	}
	var req createDashboardRequest
	if err := c.BodyParser(&req); err != nil || req.Name == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "name is required"})
	}
	widgets := req.Widgets
	if len(widgets) == 0 {
		widgets = json.RawMessage("[]")
	}

	id := uuid.NewString()
	now := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := h.CH.Exec(ctx,
		`INSERT INTO dashboards (id, name, description, widgets, is_deleted, created_at, updated_at, version)
		 VALUES (?, ?, ?, ?, 0, ?, ?, ?)`,
		id, req.Name, req.Description, string(widgets), now, now, now.UnixMilli(),
	); err != nil {
		return util.InternalError(c, err)
	}

	slog.Info("dashboard created", "id", id, "name", req.Name)
	return c.Status(fiber.StatusCreated).JSON(dashboardMeta{
		ID: id, Name: req.Name, Description: req.Description,
		Widgets: widgets, CreatedAt: now, UpdatedAt: now,
	})
}

// ── Update ────────────────────────────────────────────────────────────────────

type updateDashboardRequest struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Widgets     json.RawMessage `json:"widgets"`
}

// Update handles PUT /api/v1/dashboards/:id
func (h *DashboardHandler) Update(c *fiber.Ctx) error {
	if h.CH == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "database unavailable"})
	}
	id := c.Params("id")
	var req updateDashboardRequest
	if err := c.BodyParser(&req); err != nil || req.Name == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "name is required"})
	}
	widgets := req.Widgets
	if len(widgets) == 0 {
		widgets = json.RawMessage("[]")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Fetch original created_at so we preserve it.
	rows, err := h.CH.Query(ctx,
		`SELECT created_at, is_deleted FROM dashboards WHERE id = ? ORDER BY version DESC LIMIT 1`, id)
	if err != nil {
		return util.InternalError(c, err)
	}
	var createdAt time.Time
	var isDeleted uint8
	if rows.Next() {
		_ = rows.Scan(&createdAt, &isDeleted)
	}
	rows.Close()
	if createdAt.IsZero() || isDeleted == 1 {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "dashboard not found"})
	}

	now := time.Now()
	if err := h.CH.Exec(ctx,
		`INSERT INTO dashboards (id, name, description, widgets, is_deleted, created_at, updated_at, version)
		 VALUES (?, ?, ?, ?, 0, ?, ?, ?)`,
		id, req.Name, req.Description, string(widgets), createdAt, now, now.UnixMilli(),
	); err != nil {
		return util.InternalError(c, err)
	}

	return c.JSON(dashboardMeta{
		ID: id, Name: req.Name, Description: req.Description,
		Widgets: widgets, CreatedAt: createdAt, UpdatedAt: now,
	})
}

// ── Delete ────────────────────────────────────────────────────────────────────

// Delete handles DELETE /api/v1/dashboards/:id
// Uses soft-delete (is_deleted=1) compatible with ReplacingMergeTree.
func (h *DashboardHandler) Delete(c *fiber.Ctx) error {
	if h.CH == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "database unavailable"})
	}
	id := c.Params("id")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	now := time.Now()
	// Copy latest row with is_deleted=1 + bumped version.
	if err := h.CH.Exec(ctx,
		`INSERT INTO dashboards (id, name, description, widgets, is_deleted, created_at, updated_at, version)
		 SELECT id, name, description, widgets, 1, created_at, ?, ?
		 FROM dashboards WHERE id = ? ORDER BY version DESC LIMIT 1`,
		now, now.UnixMilli(), id,
	); err != nil {
		return util.InternalError(c, err)
	}

	slog.Info("dashboard deleted", "id", id)
	return c.JSON(fiber.Map{"deleted": true})
}

// ── Widget data: Top Talkers ──────────────────────────────────────────────────

// TopTalkers handles GET /api/v1/flows/top-talkers
// Query params: window (1h|6h|24h|7d), by (flows|bytes), limit (1-50)
func (h *DashboardHandler) TopTalkers(c *fiber.Ctx) error {
	if h.CH == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "database unavailable"})
	}

	window := c.Query("window", "1h")
	by := c.Query("by", "flows")
	limit := c.QueryInt("limit", 10)
	if limit < 1 || limit > 50 {
		limit = 10
	}

	// Both interval and orderBy are derived from a closed set — no injection risk.
	var interval string
	switch window {
	case "6h":
		interval = "6 HOUR"
	case "24h":
		interval = "24 HOUR"
	case "7d":
		interval = "7 DAY"
	default:
		window = "1h"
		interval = "1 HOUR"
	}

	var orderBy string
	if by == "bytes" {
		orderBy = "total_bytes"
	} else {
		by = "flows"
		orderBy = "flow_count"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	q := fmt.Sprintf(
		`SELECT src_ip,
		        count()                      AS flow_count,
		        sum(bytes_in + bytes_out)    AS total_bytes
		 FROM flows
		 WHERE ts > now() - INTERVAL %s
		 GROUP BY src_ip
		 ORDER BY %s DESC
		 LIMIT ?`, interval, orderBy)

	rows, err := h.CH.Query(ctx, q, limit)
	if err != nil {
		return util.InternalError(c, err)
	}
	defer rows.Close()

	type talker struct {
		SrcIP      string `json:"src_ip"`
		FlowCount  uint64 `json:"flow_count"`
		TotalBytes uint64 `json:"total_bytes"`
	}
	result := make([]talker, 0, limit)
	for rows.Next() {
		var t talker
		if err := rows.Scan(&t.SrcIP, &t.FlowCount, &t.TotalBytes); err != nil {
			continue
		}
		result = append(result, t)
	}
	return c.JSON(fiber.Map{"talkers": result, "window": window, "by": by})
}
