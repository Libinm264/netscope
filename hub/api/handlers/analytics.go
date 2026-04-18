package handlers

import (
	"fmt"

	"github.com/gofiber/fiber/v2"

	"github.com/netscope/hub-api/clickhouse"
	"github.com/netscope/hub-api/models"
	"github.com/netscope/hub-api/util"
)

// AnalyticsHandler provides HTTP endpoint analytics.
type AnalyticsHandler struct {
	CH *clickhouse.Client
}

// Endpoints handles GET /api/v1/analytics/endpoints.
// Returns per-endpoint latency histograms (p50/p95/p99) and error rates
// aggregated from HTTP/HTTPS flows over the requested time window.
func (h *AnalyticsHandler) Endpoints(c *fiber.Ctx) error {
	if h.CH == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": "ClickHouse is not available",
		})
	}

	window := c.Query("window", "1h")
	interval := windowToInterval(window)

	rows, err := h.CH.Query(c.Context(), fmt.Sprintf(`
		SELECT
			http_method,
			http_path,
			count()                                                  AS total,
			countIf(http_status >= 200 AND http_status < 400)        AS success_count,
			countIf(http_status >= 400)                              AS error_count,
			if(count() > 0,
			   toFloat64(countIf(http_status >= 400)) / count() * 100.0,
			   0.0)                                                   AS error_rate,
			avg(duration_ms)                                         AS avg_latency,
			quantile(0.50)(duration_ms)                              AS p50,
			quantile(0.95)(duration_ms)                              AS p95,
			quantile(0.99)(duration_ms)                              AS p99
		FROM flows
		WHERE protocol IN ('HTTP', 'HTTPS')
		  AND ts >= now() - INTERVAL %s
		  AND http_method != ''
		  AND http_path   != ''
		GROUP BY http_method, http_path
		ORDER BY total DESC
		LIMIT 100
	`, interval))
	if err != nil {
		return util.InternalError(c, err)
	}
	defer rows.Close()

	endpoints := make([]models.EndpointStat, 0)
	for rows.Next() {
		var e models.EndpointStat
		if err := rows.Scan(
			&e.Method, &e.Path,
			&e.Count, &e.SuccessCount, &e.ErrorCount, &e.ErrorRate,
			&e.AvgLatencyMs, &e.P50Ms, &e.P95Ms, &e.P99Ms,
		); err != nil {
			continue
		}
		endpoints = append(endpoints, e)
	}

	return c.JSON(models.EndpointStatsResponse{
		Endpoints: endpoints,
		Window:    window,
		Total:     len(endpoints),
	})
}
