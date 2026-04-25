package handlers

import (
	"github.com/gofiber/fiber/v2"
	"github.com/netscope/hub-api/clickhouse"
	"github.com/netscope/hub-api/util"
)

type ThreatHandler struct {
	CH *clickhouse.Client
}

// Summary handles GET /api/v1/threats
// Returns top threat-scored destination IPs seen in the requested window.
func (h *ThreatHandler) Summary(c *fiber.Ctx) error {
	if h.CH == nil {
		return c.Status(503).JSON(fiber.Map{"error": "ClickHouse not available"})
	}
	window := c.Query("window", "24h")
	wc := windowToInterval(window)
	limit := c.QueryInt("limit", 50)

	// Top threat IPs with aggregated details
	rows, err := h.CH.Query(c.Context(), `
        SELECT
            dst_ip,
            max(threat_score)                       AS score,
            any(threat_level)                       AS level,
            any(country_code)                       AS country_code,
            any(country_name)                       AS country_name,
            any(as_org)                             AS as_org,
            count()                                 AS flow_count,
            max(ts)                                 AS last_seen,
            groupUniqArray(10)(process_name)        AS processes
        FROM flows
        WHERE threat_score > 0
          AND ts >= now() - INTERVAL `+wc+`
        GROUP BY dst_ip
        ORDER BY score DESC, flow_count DESC
        LIMIT ?`, uint64(limit))
	if err != nil {
		return util.InternalError(c, err)
	}
	defer rows.Close()

	type ThreatIP struct {
		DstIP       string   `json:"dst_ip"`
		ThreatScore uint8    `json:"threat_score"`
		ThreatLevel string   `json:"threat_level"`
		CountryCode string   `json:"country_code"`
		CountryName string   `json:"country_name"`
		ASOrg       string   `json:"as_org"`
		FlowCount   uint64   `json:"flow_count"`
		LastSeen    string   `json:"last_seen"`
		Processes   []string `json:"processes"`
	}

	items := make([]ThreatIP, 0)
	for rows.Next() {
		var t ThreatIP
		var processes []string
		if err := rows.Scan(
			&t.DstIP, &t.ThreatScore, &t.ThreatLevel,
			&t.CountryCode, &t.CountryName, &t.ASOrg,
			&t.FlowCount, &t.LastSeen, &processes,
		); err != nil {
			continue
		}
		// Filter empty process names
		for _, p := range processes {
			if p != "" {
				t.Processes = append(t.Processes, p)
			}
		}
		if t.Processes == nil {
			t.Processes = []string{}
		}
		items = append(items, t)
	}

	// Summary counts
	var high, medium, low int
	for _, t := range items {
		switch t.ThreatLevel {
		case "high":
			high++
		case "medium":
			medium++
		case "low":
			low++
		}
	}

	return c.JSON(fiber.Map{
		"threats": items,
		"summary": fiber.Map{
			"total":  len(items),
			"high":   high,
			"medium": medium,
			"low":    low,
		},
		"window": window,
	})
}
