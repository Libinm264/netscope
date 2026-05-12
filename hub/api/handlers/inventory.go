package handlers

import (
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/klyzar/hub-api/clickhouse"
	"github.com/klyzar/hub-api/util"
)

// InventoryHandler serves the passive API inventory endpoints.
type InventoryHandler struct {
	CH *clickhouse.Client
}

// EndpointEntry is one auto-discovered API endpoint.
type EndpointEntry struct {
	Hostname   string    `json:"hostname"`
	Protocol   string    `json:"protocol"`
	Method     string    `json:"method"`
	Path       string    `json:"path"`
	CallCount  uint64    `json:"call_count"`
	ErrorCount uint64    `json:"error_count"`
	ErrorRate  float64   `json:"error_rate"`
	P95Ms      float64   `json:"p95_ms"`
	AvgMs      float64   `json:"avg_ms"`
	AgentCount uint64    `json:"agent_count"`
	FirstSeen  time.Time `json:"first_seen"`
	LastSeen   time.Time `json:"last_seen"`
	IsNew      bool      `json:"is_new"` // first_seen within last 24 h
}

// Endpoints handles GET /api/v1/inventory/endpoints
//
// Query params:
//
//	window  — 1h | 6h | 24h | 7d (default 24h)
//	hostname — filter to a single agent hostname
//	search   — substring match on path
func (h *InventoryHandler) Endpoints(c *fiber.Ctx) error {
	if h.CH == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": "ClickHouse is not available",
		})
	}

	window := c.Query("window", "24h")
	hostname := c.Query("hostname", "")
	search := c.Query("search", "")

	interval := windowToInterval(window)

	// Build optional WHERE clauses.
	extraWhere := ""
	if hostname != "" {
		extraWhere += fmt.Sprintf(" AND hostname = '%s'", sanitise(hostname))
	}
	if search != "" {
		extraWhere += fmt.Sprintf(" AND http_path LIKE '%%%s%%'", sanitise(search))
	}

	rows, err := h.CH.Query(c.Context(), fmt.Sprintf(`
		SELECT
			hostname,
			protocol,
			http_method,
			http_path,
			count()                                                    AS call_count,
			countIf(http_status >= 400)                                AS error_count,
			if(count() > 0,
			   toFloat64(countIf(http_status >= 400)) / count() * 100.0,
			   0.0)                                                     AS error_rate,
			quantile(0.95)(duration_ms)                                AS p95_ms,
			avg(duration_ms)                                           AS avg_ms,
			uniq(agent_id)                                             AS agent_count,
			min(ts)                                                    AS first_seen,
			max(ts)                                                    AS last_seen
		FROM flows
		WHERE protocol IN ('HTTP', 'HTTPS', 'HTTP2', 'GRPC')
		  AND ts >= now() - INTERVAL %s
		  AND http_method != ''
		  AND http_path   != ''
		  %s
		GROUP BY hostname, protocol, http_method, http_path
		ORDER BY call_count DESC
		LIMIT 500
	`, interval, extraWhere))
	if err != nil {
		return util.InternalError(c, err)
	}
	defer rows.Close()

	now := time.Now().UTC()
	cutoff24h := now.Add(-24 * time.Hour)

	entries := make([]EndpointEntry, 0, 64)
	for rows.Next() {
		var e EndpointEntry
		if err := rows.Scan(
			&e.Hostname, &e.Protocol, &e.Method, &e.Path,
			&e.CallCount, &e.ErrorCount, &e.ErrorRate,
			&e.P95Ms, &e.AvgMs, &e.AgentCount,
			&e.FirstSeen, &e.LastSeen,
		); err != nil {
			continue
		}
		e.IsNew = e.FirstSeen.After(cutoff24h)
		entries = append(entries, e)
	}

	return c.JSON(fiber.Map{
		"endpoints": entries,
		"window":    window,
		"total":     len(entries),
	})
}

// sanitise strips single-quotes to prevent SQL injection in LIKE/= clauses.
// ClickHouse parameterised queries are not supported for all expression types,
// so we sanitise manually for hostname/search inputs.
func sanitise(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		if s[i] != '\'' && s[i] != '\\' {
			out = append(out, s[i])
		}
	}
	return string(out)
}
