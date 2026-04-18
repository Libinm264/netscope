package handlers

import (
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/netscope/hub-api/clickhouse"
	"github.com/netscope/hub-api/util"
)

// MetricsHandler serves time-series data for dashboard charts.
type MetricsHandler struct {
	CH *clickhouse.Client
}

// TimeseriesPoint is one minute-bucket of flow activity.
type TimeseriesPoint struct {
	Ts       time.Time `json:"ts"`
	Count    uint64    `json:"count"`
	BytesIn  uint64    `json:"bytes_in"`
	BytesOut uint64    `json:"bytes_out"`
}

// Timeseries handles GET /api/v1/metrics/timeseries?hours=N
// Returns per-minute flow counts for the last N hours (default 1, max 24).
func (h *MetricsHandler) Timeseries(c *fiber.Ctx) error {
	if h.CH == nil {
		return c.Status(503).JSON(fiber.Map{"error": "ClickHouse not available"})
	}

	hours := c.QueryInt("hours", 1)
	if hours < 1 {
		hours = 1
	}
	if hours > 24 {
		hours = 24
	}

	since := time.Now().UTC().Add(-time.Duration(hours) * time.Hour)

	rows, err := h.CH.Query(c.Context(),
		`SELECT toStartOfMinute(ts) AS minute,
		        count()             AS cnt,
		        sum(bytes_in)       AS bin,
		        sum(bytes_out)      AS bout
		 FROM flows
		 WHERE ts >= ?
		 GROUP BY minute
		 ORDER BY minute ASC`,
		since,
	)
	if err != nil {
		return util.InternalError(c, err)
	}
	defer rows.Close()

	points := make([]TimeseriesPoint, 0, hours*60)
	for rows.Next() {
		var p TimeseriesPoint
		if err := rows.Scan(&p.Ts, &p.Count, &p.BytesIn, &p.BytesOut); err != nil {
			continue
		}
		points = append(points, p)
	}

	return c.JSON(fiber.Map{
		"points": points,
		"hours":  hours,
	})
}

// ProtocolBreakdown handles GET /api/v1/metrics/protocols?hours=N
// Returns per-protocol flow counts for the last N hours.
func (h *MetricsHandler) ProtocolBreakdown(c *fiber.Ctx) error {
	if h.CH == nil {
		return c.Status(503).JSON(fiber.Map{"error": "ClickHouse not available"})
	}

	hours := c.QueryInt("hours", 1)
	if hours < 1 {
		hours = 1
	}
	if hours > 168 {
		hours = 168
	}

	since := time.Now().UTC().Add(-time.Duration(hours) * time.Hour)

	rows, err := h.CH.Query(c.Context(),
		`SELECT protocol, count() AS cnt
		 FROM flows
		 WHERE ts >= ?
		 GROUP BY protocol
		 ORDER BY cnt DESC
		 LIMIT 20`,
		since,
	)
	if err != nil {
		return util.InternalError(c, err)
	}
	defer rows.Close()

	type ProtocolCount struct {
		Protocol string `json:"protocol"`
		Count    uint64 `json:"count"`
	}
	results := make([]ProtocolCount, 0, 20)
	for rows.Next() {
		var p ProtocolCount
		if err := rows.Scan(&p.Protocol, &p.Count); err != nil {
			continue
		}
		results = append(results, p)
	}

	return c.JSON(fiber.Map{"protocols": results, "hours": hours})
}
