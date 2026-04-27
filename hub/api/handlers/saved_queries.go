package handlers

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/netscope/hub-api/clickhouse"
	"github.com/netscope/hub-api/enterprise/license"
	"github.com/netscope/hub-api/util"
)

// SavedQueryHandler implements CRUD for saved flow queries.
// Community plan: max 10 queries.  Enterprise: unlimited.
type SavedQueryHandler struct {
	CH      *clickhouse.Client
	License *license.License
}

// communityQueryLimit is the maximum number of saved queries on the Community plan.
const communityQueryLimit = 10

type savedQuery struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Filters     string    `json:"filters"` // JSON blob: {protocol,src_ip,dst_ip,hostname,trace_id,from,to}
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// List returns all saved queries ordered by created_at desc.
func (h *SavedQueryHandler) List(c *fiber.Ctx) error {
	if h.CH == nil {
		return c.JSON(fiber.Map{"queries": []any{}})
	}
	ctx := c.Context()
	rows, err := h.CH.Query(ctx,
		`SELECT id, name, description, filters, created_at, updated_at
		 FROM saved_queries FINAL
		 WHERE deleted = 0
		 ORDER BY created_at DESC`)
	if err != nil {
		return util.InternalError(c, err)
	}
	defer rows.Close()

	queries := make([]savedQuery, 0, 32)
	for rows.Next() {
		var q savedQuery
		if err := rows.Scan(&q.ID, &q.Name, &q.Description, &q.Filters,
			&q.CreatedAt, &q.UpdatedAt); err != nil {
			continue
		}
		queries = append(queries, q)
	}

	plan := "community"
	if h.License != nil {
		plan = h.License.Plan
	}

	return c.JSON(fiber.Map{
		"queries":    queries,
		"count":      len(queries),
		"limit":      communityLimitForPlan(plan),
		"plan":       plan,
	})
}

// Create inserts a new saved query.
func (h *SavedQueryHandler) Create(c *fiber.Ctx) error {
	if h.CH == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "storage unavailable"})
	}

	var body struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Filters     string `json:"filters"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid body"})
	}
	if body.Name == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "name is required"})
	}
	if body.Filters == "" {
		body.Filters = "{}"
	}

	plan := "community"
	if h.License != nil {
		plan = h.License.Plan
	}

	// Enforce Community quota
	lim := communityLimitForPlan(plan)
	if lim > 0 {
		ctx := c.Context()
		countRows, err := h.CH.Query(ctx,
			`SELECT count() FROM saved_queries FINAL WHERE deleted = 0`)
		if err != nil {
			return util.InternalError(c, err)
		}
		var count uint64
		if countRows.Next() {
			_ = countRows.Scan(&count)
		}
		countRows.Close()

		if count >= uint64(lim) {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"error": "saved query limit reached",
				"limit": lim,
				"plan":  plan,
				"upgrade_hint": "Upgrade to Enterprise for unlimited saved queries.",
			})
		}
	}

	id := uuid.NewString()
	now := time.Now()
	ctx := c.Context()

	if err := h.CH.Exec(ctx,
		`INSERT INTO saved_queries
		 (id, name, description, filters, deleted, created_at, updated_at, version)
		 VALUES (?, ?, ?, ?, 0, ?, ?, ?)`,
		id, body.Name, body.Description, body.Filters,
		now, now, now.UnixMilli(),
	); err != nil {
		return util.InternalError(c, err)
	}

	return c.Status(fiber.StatusCreated).JSON(savedQuery{
		ID:          id,
		Name:        body.Name,
		Description: body.Description,
		Filters:     body.Filters,
		CreatedAt:   now,
		UpdatedAt:   now,
	})
}

// Update replaces a saved query's name/description/filters.
func (h *SavedQueryHandler) Update(c *fiber.Ctx) error {
	if h.CH == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "storage unavailable"})
	}
	id := c.Params("id")

	var body struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Filters     string `json:"filters"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid body"})
	}

	// Fetch existing to merge
	ctx := c.Context()
	rows, err := h.CH.Query(ctx,
		`SELECT name, description, filters FROM saved_queries FINAL
		 WHERE id = ? AND deleted = 0 LIMIT 1`, id)
	if err != nil {
		return util.InternalError(c, err)
	}
	var existing savedQuery
	if rows.Next() {
		_ = rows.Scan(&existing.Name, &existing.Description, &existing.Filters)
	}
	rows.Close()

	if existing.Name == "" && body.Name == "" {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "not found"})
	}

	if body.Name != "" {
		existing.Name = body.Name
	}
	if body.Description != "" {
		existing.Description = body.Description
	}
	if body.Filters != "" {
		existing.Filters = body.Filters
	}

	now := time.Now()
	if err := h.CH.Exec(ctx,
		`INSERT INTO saved_queries
		 (id, name, description, filters, deleted, created_at, updated_at, version)
		 VALUES (?, ?, ?, ?, 0, now64(), ?, ?)`,
		id, existing.Name, existing.Description, existing.Filters,
		now, now.UnixMilli(),
	); err != nil {
		return util.InternalError(c, err)
	}

	return c.JSON(fiber.Map{"ok": true})
}

// Delete soft-deletes a saved query by writing a tombstone row.
func (h *SavedQueryHandler) Delete(c *fiber.Ctx) error {
	if h.CH == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "storage unavailable"})
	}
	id := c.Params("id")
	ctx := c.Context()

	now := time.Now()
	if err := h.CH.Exec(ctx,
		`INSERT INTO saved_queries
		 (id, name, description, filters, deleted, created_at, updated_at, version)
		 VALUES (?, '', '', '{}', 1, now64(), ?, ?)`,
		id, now, now.UnixMilli(),
	); err != nil {
		return util.InternalError(c, err)
	}

	return c.JSON(fiber.Map{"ok": true})
}

// communityLimitForPlan returns 0 (unlimited) for Enterprise, communityQueryLimit otherwise.
func communityLimitForPlan(plan string) int {
	if plan == "enterprise" {
		return 0
	}
	return communityQueryLimit
}

