package handlers

import (
	"log/slog"

	"github.com/gofiber/fiber/v2"

	"github.com/netscope/hub-api/clickhouse"
)

// StatsHandler serves the dashboard summary endpoint.
type StatsHandler struct {
	CH *clickhouse.Client
}

type protocolStat struct {
	Protocol string `json:"protocol"`
	Count    uint64 `json:"count"`
}

type talkerStat struct {
	IP    string `json:"ip"`
	Flows uint64 `json:"flows"`
}

type statsResponse struct {
	TotalFlows     uint64         `json:"total_flows"`
	FlowsPerMinute float64        `json:"flows_per_minute"`
	TopProtocols   []protocolStat `json:"top_protocols"`
	TopTalkers     []talkerStat   `json:"top_talkers"`
	ActiveAgents   uint64         `json:"active_agents"`
}

// Stats handles GET /api/v1/stats.
func (h *StatsHandler) Stats(c *fiber.Ctx) error {
	if h.CH == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": "ClickHouse is not available",
		})
	}

	ctx := c.Context()
	resp := statsResponse{
		TopProtocols: []protocolStat{},
		TopTalkers:   []talkerStat{},
	}

	// Total flows ─────────────────────────────────────────────────────────────
	if rows, err := h.CH.Query(ctx, "SELECT count() FROM flows"); err == nil {
		defer rows.Close()
		if rows.Next() {
			_ = rows.Scan(&resp.TotalFlows)
		}
	} else {
		slog.Warn("stats: total_flows query", "err", err)
	}

	// Flows in the last 60 seconds → flows_per_minute ─────────────────────────
	if rows, err := h.CH.Query(ctx,
		"SELECT count() FROM flows WHERE ts > now() - INTERVAL 60 SECOND",
	); err == nil {
		defer rows.Close()
		var cnt uint64
		if rows.Next() {
			_ = rows.Scan(&cnt)
		}
		resp.FlowsPerMinute = float64(cnt)
	} else {
		slog.Warn("stats: flows_per_minute query", "err", err)
	}

	// Top 5 protocols ─────────────────────────────────────────────────────────
	if rows, err := h.CH.Query(ctx,
		`SELECT protocol, count() AS cnt
		 FROM flows
		 GROUP BY protocol
		 ORDER BY cnt DESC
		 LIMIT 5`,
	); err == nil {
		defer rows.Close()
		for rows.Next() {
			var ps protocolStat
			if err := rows.Scan(&ps.Protocol, &ps.Count); err == nil {
				resp.TopProtocols = append(resp.TopProtocols, ps)
			}
		}
	} else {
		slog.Warn("stats: top_protocols query", "err", err)
	}

	// Top 5 source IPs ────────────────────────────────────────────────────────
	if rows, err := h.CH.Query(ctx,
		`SELECT src_ip, count() AS cnt
		 FROM flows
		 GROUP BY src_ip
		 ORDER BY cnt DESC
		 LIMIT 5`,
	); err == nil {
		defer rows.Close()
		for rows.Next() {
			var ts talkerStat
			if err := rows.Scan(&ts.IP, &ts.Flows); err == nil {
				resp.TopTalkers = append(resp.TopTalkers, ts)
			}
		}
	} else {
		slog.Warn("stats: top_talkers query", "err", err)
	}

	// Active agents (distinct agent_ids seen within the last 5 minutes).
	// Use a subquery that groups by agent_id before counting so that multiple
	// raw rows for the same agent (pre-merge on ReplacingMergeTree) are not
	// double-counted, keeping this consistent with the Agents-page list query.
	if rows, err := h.CH.Query(ctx,
		`SELECT count() FROM (
			SELECT agent_id
			FROM agents
			WHERE last_seen > now() - INTERVAL 5 MINUTE
			GROUP BY agent_id
		)`,
	); err == nil {
		defer rows.Close()
		if rows.Next() {
			_ = rows.Scan(&resp.ActiveAgents)
		}
	} else {
		slog.Warn("stats: active_agents query", "err", err)
	}

	return c.JSON(resp)
}
