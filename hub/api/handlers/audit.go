package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/netscope/hub-api/clickhouse"
	"github.com/netscope/hub-api/enterprise/license"
	"github.com/netscope/hub-api/util"
)

// AuditHandler serves audit log query endpoints.
type AuditHandler struct {
	CH      *clickhouse.Client
	License *license.License
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

// Export handles GET /api/v1/enterprise/audit/export
//
// Downloads audit events as a file attachment.
//
// Query params:
//
//	format – "json" (default), "cef", "leef"
//	from   – RFC3339 start time (default: 24 h ago)
//	to     – RFC3339 end time   (default: now)
//	limit  – max rows           (default 10000, max 50000)
func (h *AuditHandler) Export(c *fiber.Ctx) error {
	if h.CH == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "ClickHouse unavailable"})
	}
	if h.License != nil && !h.License.HasFeature(license.FeatureAuditExport) {
		return c.Status(fiber.StatusPaymentRequired).JSON(fiber.Map{
			"error": "audit export requires the Team or Enterprise plan",
		})
	}

	format := strings.ToLower(c.Query("format", "json"))
	if format != "json" && format != "cef" && format != "leef" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "format must be json, cef, or leef",
		})
	}

	now := time.Now().UTC()
	fromTime := now.Add(-24 * time.Hour)
	toTime := now

	if raw := c.Query("from"); raw != "" {
		if t, err := time.Parse(time.RFC3339, raw); err == nil {
			fromTime = t
		}
	}
	if raw := c.Query("to"); raw != "" {
		if t, err := time.Parse(time.RFC3339, raw); err == nil {
			toTime = t
		}
	}

	limit := c.QueryInt("limit", 10000)
	if limit < 1 || limit > 50000 {
		limit = 10000
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rows, err := h.CH.Query(ctx,
		`SELECT id, token_id, role, method, path, status, client_ip, latency_ms, ts
		 FROM audit_events
		 WHERE ts >= ? AND ts <= ?
		 ORDER BY ts ASC
		 LIMIT ?`,
		fromTime, toTime, uint64(limit))
	if err != nil {
		return util.InternalError(c, err)
	}
	defer rows.Close()

	type exportRow struct {
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

	var events []exportRow
	for rows.Next() {
		var e exportRow
		if err := rows.Scan(
			&e.ID, &e.TokenID, &e.Role, &e.Method, &e.Path,
			&e.Status, &e.ClientIP, &e.LatencyMs, &e.Ts,
		); err != nil {
			continue
		}
		events = append(events, e)
	}

	// ── Format and stream ───────────────────────────────────────────────────

	var buf bytes.Buffer
	var contentType, ext string

	switch format {
	case "cef":
		contentType = "text/plain; charset=utf-8"
		ext = "cef"
		for _, e := range events {
			sev := cefSeverity(e.Status)
			line := fmt.Sprintf(
				"CEF:0|NetScope|HubAPI|1.0|%s|%s %s|%d|"+
					"rt=%d src=%s requestMethod=%s request=%s outcome=%d "+
					"act=%s dproc=%s deviceCustomNumber1=%d deviceCustomNumber1Label=latency_ms\n",
				e.ID, e.Method, e.Path, sev,
				e.Ts.UnixMilli(), e.ClientIP, e.Method, e.Path, e.Status,
				e.Role, e.TokenID, e.LatencyMs,
			)
			buf.WriteString(line)
		}

	case "leef":
		contentType = "text/plain; charset=utf-8"
		ext = "leef"
		for _, e := range events {
			line := fmt.Sprintf(
				"LEEF:2.0|NetScope|HubAPI|1.0|%s %s\t"+
					"devTime=%s\tdevTimeFormat=yyyy-MM-dd'T'HH:mm:ss.SSS'Z'\t"+
					"src=%s\trequestMethod=%s\trequest=%s\tstatusCode=%d\t"+
					"role=%s\ttokenId=%s\tlatencyMs=%d\n",
				e.Method, e.Path,
				e.Ts.UTC().Format("2006-01-02T15:04:05.000Z"),
				e.ClientIP, e.Method, e.Path, e.Status,
				e.Role, e.TokenID, e.LatencyMs,
			)
			buf.WriteString(line)
		}

	default: // json
		contentType = "application/json"
		ext = "json"
		type jsonRow struct {
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
		rows2 := make([]jsonRow, 0, len(events))
		for _, e := range events {
			rows2 = append(rows2, jsonRow{
				ID: e.ID, TokenID: e.TokenID, Role: e.Role,
				Method: e.Method, Path: e.Path, Status: e.Status,
				ClientIP: e.ClientIP, LatencyMs: e.LatencyMs,
				Ts: e.Ts.UTC().Format(time.RFC3339Nano),
			})
		}
		b, _ := json.Marshal(rows2)
		buf.Write(b)
	}

	filename := fmt.Sprintf("netscope-audit-%s.%s",
		now.Format("20060102-150405"), ext)

	c.Set("Content-Type", contentType)
	c.Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	return c.Send(buf.Bytes())
}

func cefSeverity(status uint16) int {
	switch {
	case status >= 500:
		return 7
	case status >= 400:
		return 5
	default:
		return 3
	}
}
