package handlers

import (
	"log/slog"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/klyzar/hub-api/clickhouse"
	"github.com/klyzar/hub-api/util"
)

type ReplayHandler struct {
	CH *clickhouse.Client
}

// Timeline returns all flows for a given agent in a ±window_mins window
// around the specified timestamp. Used by the Incident Replay UI.
//
// GET /api/v1/replay?agent_id=&around=<RFC3339>&window_mins=5
func (h *ReplayHandler) Timeline(c *fiber.Ctx) error {
	agentID := c.Query("agent_id")
	if agentID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "agent_id required"})
	}
	aroundStr := c.Query("around")
	if aroundStr == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "around (RFC3339) required"})
	}

	// Accept both RFC3339 and RFC3339Nano
	var around time.Time
	var err error
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		around, err = time.Parse(layout, aroundStr)
		if err == nil {
			break
		}
	}
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "around must be RFC3339 (e.g. 2026-05-05T12:00:00Z)"})
	}

	windowMins := c.QueryInt("window_mins", 5)
	if windowMins < 1 {
		windowMins = 1
	}
	if windowMins > 30 {
		windowMins = 30
	}

	from := around.Add(-time.Duration(windowMins) * time.Minute).UTC()
	to   := around.Add( time.Duration(windowMins) * time.Minute).UTC()

	if h.CH == nil {
		return c.JSON(fiber.Map{
			"agent_id":    agentID,
			"hostname":    "",
			"around":      around,
			"window_mins": windowMins,
			"from":        from,
			"to":          to,
			"flows":       []struct{}{},
			"total":       0,
		})
	}

	rows, err := h.CH.Query(c.Context(), `
		SELECT
			id, agent_id, hostname, ts,
			protocol, src_ip, src_port, dst_ip, dst_port,
			bytes_in, bytes_out, duration_ms, info
		FROM flows
		WHERE agent_id = ?
		  AND ts >= ? AND ts <= ?
		ORDER BY ts ASC
		LIMIT 500`,
		agentID, from, to)
	if err != nil {
		return util.InternalError(c, err)
	}
	defer rows.Close()

	type ReplayFlow struct {
		ID         string    `json:"id"`
		AgentID    string    `json:"agent_id"`
		Hostname   string    `json:"hostname"`
		Timestamp  time.Time `json:"timestamp"`
		Protocol   string    `json:"protocol"`
		SrcIP      string    `json:"src_ip"`
		SrcPort    uint16    `json:"src_port"`
		DstIP      string    `json:"dst_ip"`
		DstPort    uint16    `json:"dst_port"`
		BytesIn    uint64    `json:"bytes_in"`
		BytesOut   uint64    `json:"bytes_out"`
		DurationMS uint32    `json:"duration_ms"`
		Info       string    `json:"info"`
	}

	flows := make([]ReplayFlow, 0, 64)
	for rows.Next() {
		var f ReplayFlow
		if err := rows.Scan(
			&f.ID, &f.AgentID, &f.Hostname, &f.Timestamp,
			&f.Protocol, &f.SrcIP, &f.SrcPort, &f.DstIP, &f.DstPort,
			&f.BytesIn, &f.BytesOut, &f.DurationMS, &f.Info,
		); err != nil {
			slog.Warn("replay: scan", "err", err)
			continue
		}
		flows = append(flows, f)
	}

	hostname := ""
	if len(flows) > 0 {
		hostname = flows[0].Hostname
	}

	return c.JSON(fiber.Map{
		"agent_id":    agentID,
		"hostname":    hostname,
		"around":      around,
		"window_mins": windowMins,
		"from":        from,
		"to":          to,
		"flows":       flows,
		"total":       len(flows),
	})
}
