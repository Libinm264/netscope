package handlers

import (
	"github.com/gofiber/fiber/v2"

	"github.com/netscope/hub-api/clickhouse"
	"github.com/netscope/hub-api/util"
)

// AuditHandler serves audit log query endpoints.
type AuditHandler struct {
	CH *clickhouse.Client
}

// AuditEvent is the shape returned by the query endpoints.
type AuditEvent struct {
	ID        string `json:"id"`
	TokenID   string `json:"token_id"`
	Role      string `json:"role"`
	Method    string `json:"method"`
	Path      string `json:"path"`
	Status    uint16 `json:"status"`
	ClientIP  string `json:"client_ip"`
	LatencyMs uint32 `json:"latency_ms"`
	Ts        string `json:"ts"`
}

// List handles GET /api/v1/audit
// Query params:
//
//	limit  – max rows (default 100, max 1000)
//	token  – filter by token_id (optional)
//	status – filter by HTTP status code (optional)
func (h *AuditHandler) List(c *fiber.Ctx) error {
	if h.CH == nil {
		return c.Status(503).JSON(fiber.Map{"error": "ClickHouse unavailable"})
	}

	limit := c.QueryInt("limit", 100)
	if limit > 1000 {
		limit = 1000
	}

	query := `SELECT id, token_id, role, method, path, status, client_ip, latency_ms,
	                  formatDateTime(ts, '%Y-%m-%dT%H:%i:%SZ') AS ts
	           FROM audit_events`

	args := make([]any, 0, 3)
	where := ""

	if tok := c.Query("token"); tok != "" {
		where += " WHERE token_id = ?"
		args = append(args, tok)
	}
	if st := c.QueryInt("status", 0); st != 0 {
		if where == "" {
			where += " WHERE status = ?"
		} else {
			where += " AND status = ?"
		}
		args = append(args, uint16(st))
	}

	query += where + " ORDER BY ts DESC LIMIT ?"
	args = append(args, uint64(limit))

	rows, err := h.CH.Query(c.Context(), query, args...)
	if err != nil {
		return util.InternalError(c, err)
	}
	defer rows.Close()

	events := make([]AuditEvent, 0)
	for rows.Next() {
		var e AuditEvent
		if err := rows.Scan(
			&e.ID, &e.TokenID, &e.Role, &e.Method, &e.Path,
			&e.Status, &e.ClientIP, &e.LatencyMs, &e.Ts,
		); err != nil {
			continue
		}
		events = append(events, e)
	}
	return c.JSON(fiber.Map{"events": events, "count": len(events)})
}
