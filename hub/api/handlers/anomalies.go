package handlers

import (
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/netscope/hub-api/clickhouse"
	"github.com/netscope/hub-api/util"
)

// AnomalyHandler serves behavioral baseline and anomaly detection data.
type AnomalyHandler struct {
	CH *clickhouse.Client
}

// AnomalyEvent is a single detected anomaly returned by the API.
type AnomalyEvent struct {
	ID          string    `json:"id"`
	AgentID     string    `json:"agent_id"`
	Hostname    string    `json:"hostname"`
	Protocol    string    `json:"protocol"`
	AnomalyType string    `json:"anomaly_type"` // "spike" | "drop"
	ZScore      float64   `json:"z_score"`
	Observed    float64   `json:"observed"`
	Expected    float64   `json:"expected"`
	Description string    `json:"description"`
	Severity    string    `json:"severity"` // "low" | "medium" | "high"
	DetectedAt  time.Time `json:"detected_at"`
}

// BaselineEntry is one per-(agent, protocol, hour_of_week) row.
type BaselineEntry struct {
	AgentID      string  `json:"agent_id"`
	Protocol     string  `json:"protocol"`
	HourOfWeek   uint8   `json:"hour_of_week"`
	FlowMean     float64 `json:"flow_count_mean"`
	FlowStd      float64 `json:"flow_count_std"`
	BytesInMean  float64 `json:"bytes_in_mean"`
	BytesOutMean float64 `json:"bytes_out_mean"`
	SampleCount  uint32  `json:"sample_count"`
}

// List handles GET /api/v1/anomalies
// Query params:
//
//	hours=24        look-back window (default 24, max 168)
//	agent_id=xxx    filter by agent
//	severity=high   filter by severity (low/medium/high)
//	limit=100       max rows (default 100, max 500)
func (h *AnomalyHandler) List(c *fiber.Ctx) error {
	if h.CH == nil {
		return c.Status(503).JSON(fiber.Map{"error": "ClickHouse not available"})
	}

	hours := c.QueryInt("hours", 24)
	if hours < 1 {
		hours = 1
	}
	if hours > 168 {
		hours = 168
	}

	limit := c.QueryInt("limit", 100)
	if limit < 1 {
		limit = 1
	}
	if limit > 500 {
		limit = 500
	}

	since    := time.Now().UTC().Add(-time.Duration(hours) * time.Hour)
	agentID  := c.Query("agent_id")
	severity := c.Query("severity")

	// Build WHERE clause with optional filters.
	where := "WHERE detected_at >= ?"
	args  := []any{since}

	if agentID != "" {
		where += " AND agent_id = ?"
		args = append(args, agentID)
	}
	if severity != "" {
		where += " AND severity = ?"
		args = append(args, severity)
	}

	args = append(args, limit)

	rows, err := h.CH.Query(c.Context(),
		`SELECT toString(id), agent_id, hostname, protocol,
		        anomaly_type, z_score, observed, expected,
		        description, severity, detected_at
		 FROM anomaly_events
		 `+where+`
		 ORDER BY detected_at DESC
		 LIMIT ?`,
		args...,
	)
	if err != nil {
		return util.InternalError(c, err)
	}
	defer rows.Close()

	events := make([]AnomalyEvent, 0, 64)
	for rows.Next() {
		var e AnomalyEvent
		if err := rows.Scan(
			&e.ID, &e.AgentID, &e.Hostname, &e.Protocol,
			&e.AnomalyType, &e.ZScore, &e.Observed, &e.Expected,
			&e.Description, &e.Severity, &e.DetectedAt,
		); err != nil {
			continue
		}
		events = append(events, e)
	}

	return c.JSON(fiber.Map{
		"events": events,
		"hours":  hours,
		"total":  len(events),
	})
}

// Stats handles GET /api/v1/anomalies/stats
// Returns summary counts for the dashboard widget.
func (h *AnomalyHandler) Stats(c *fiber.Ctx) error {
	if h.CH == nil {
		return c.Status(503).JSON(fiber.Map{"error": "ClickHouse not available"})
	}

	since24h := time.Now().UTC().Add(-24 * time.Hour)

	rows, err := h.CH.Query(c.Context(), `
		SELECT
			severity,
			count() AS cnt
		FROM anomaly_events
		WHERE detected_at >= ?
		GROUP BY severity`,
		since24h,
	)
	if err != nil {
		return util.InternalError(c, err)
	}
	defer rows.Close()

	counts := map[string]uint64{"low": 0, "medium": 0, "high": 0}
	for rows.Next() {
		var sev string
		var cnt uint64
		if err := rows.Scan(&sev, &cnt); err != nil {
			continue
		}
		counts[sev] = cnt
	}

	total := counts["low"] + counts["medium"] + counts["high"]

	// Last anomaly timestamp
	tsRows, err := h.CH.Query(c.Context(), `
		SELECT detected_at
		FROM anomaly_events
		ORDER BY detected_at DESC
		LIMIT 1`)
	var lastSeen *time.Time
	if err == nil {
		defer tsRows.Close()
		if tsRows.Next() {
			var ts time.Time
			if tsRows.Scan(&ts) == nil {
				lastSeen = &ts
			}
		}
	}

	return c.JSON(fiber.Map{
		"total_24h":   total,
		"high":        counts["high"],
		"medium":      counts["medium"],
		"low":         counts["low"],
		"last_seen":   lastSeen,
	})
}

// GetBaseline handles GET /api/v1/baseline?agent_id=xxx&protocol=TCP
// Returns stored baseline rows so the frontend can render the envelope chart.
func (h *AnomalyHandler) GetBaseline(c *fiber.Ctx) error {
	if h.CH == nil {
		return c.Status(503).JSON(fiber.Map{"error": "ClickHouse not available"})
	}

	agentID  := c.Query("agent_id")
	protocol := c.Query("protocol")

	where := "WHERE 1=1"
	args  := []any{}

	if agentID != "" {
		where += " AND agent_id = ?"
		args = append(args, agentID)
	}
	if protocol != "" {
		where += " AND protocol = ?"
		args = append(args, protocol)
	}

	rows, err := h.CH.Query(c.Context(),
		`SELECT agent_id, protocol, hour_of_week,
		        flow_count_mean, flow_count_std,
		        bytes_in_mean, bytes_out_mean, sample_count
		 FROM traffic_baselines
		 FINAL
		 `+where+`
		 ORDER BY agent_id, protocol, hour_of_week`,
		args...,
	)
	if err != nil {
		return util.InternalError(c, err)
	}
	defer rows.Close()

	entries := make([]BaselineEntry, 0, 168)
	for rows.Next() {
		var e BaselineEntry
		if err := rows.Scan(
			&e.AgentID, &e.Protocol, &e.HourOfWeek,
			&e.FlowMean, &e.FlowStd,
			&e.BytesInMean, &e.BytesOutMean, &e.SampleCount,
		); err != nil {
			continue
		}
		entries = append(entries, e)
	}

	return c.JSON(fiber.Map{
		"baseline": entries,
		"total":    len(entries),
	})
}
