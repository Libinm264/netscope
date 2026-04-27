package handlers

import (
	"log/slog"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/netscope/hub-api/clickhouse"
	"github.com/netscope/hub-api/models"
	"github.com/netscope/hub-api/util"
)

type AgentHandler struct {
	CH *clickhouse.Client
}

func (h *AgentHandler) List(c *fiber.Ctx) error {
	if h.CH == nil {
		return c.Status(503).JSON(fiber.Map{"error": "ClickHouse not available"})
	}
	rows, err := h.CH.Query(c.Context(), `
        SELECT
            a.agent_id, a.hostname, a.version, a.interface,
            a.last_seen, a.registered_at,
            a.os, a.capture_mode, a.ebpf_enabled, a.cluster,
            countIf(f.ts >= now() - INTERVAL 1 HOUR) AS flow_count_1h
        FROM (
            SELECT agent_id, hostname, version, interface, last_seen, registered_at,
                   os, capture_mode, ebpf_enabled, cluster
            FROM agents FINAL
        ) a
        LEFT JOIN flows f ON f.agent_id = a.agent_id
        GROUP BY a.agent_id, a.hostname, a.version, a.interface,
                 a.last_seen, a.registered_at, a.os, a.capture_mode, a.ebpf_enabled, a.cluster
        ORDER BY a.last_seen DESC`)
	if err != nil {
		return util.InternalError(c, err)
	}
	defer rows.Close()

	agents := make([]models.Agent, 0)
	for rows.Next() {
		var a models.Agent
		var ebpfInt uint8
		if err := rows.Scan(
			&a.AgentID, &a.Hostname, &a.Version, &a.Interface,
			&a.LastSeen, &a.RegisteredAt,
			&a.OS, &a.CaptureMode, &ebpfInt, &a.Cluster, &a.FlowCount1h,
		); err != nil {
			slog.Warn("agents list: scan", "err", err)
			continue
		}
		a.EbpfEnabled = ebpfInt == 1
		agents = append(agents, a)
	}
	return c.JSON(fiber.Map{"agents": agents})
}

func (h *AgentHandler) Register(c *fiber.Ctx) error {
	var req models.RegisterRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid body: " + err.Error()})
	}
	if req.AgentID == "" || req.Hostname == "" {
		return c.Status(400).JSON(fiber.Map{"error": "agent_id and hostname are required"})
	}
	if h.CH == nil {
		return c.Status(503).JSON(fiber.Map{"error": "ClickHouse not available"})
	}
	now := time.Now().UTC()
	ebpfInt := uint8(0)
	if req.EbpfEnabled {
		ebpfInt = 1
	}
	if err := h.CH.Exec(c.Context(),
		`INSERT INTO agents (agent_id, hostname, version, interface, last_seen, registered_at, os, capture_mode, ebpf_enabled, cluster)
         VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		req.AgentID, req.Hostname, req.Version, req.Interface, now, now, req.OS, req.CaptureMode, ebpfInt, req.Cluster,
	); err != nil {
		return util.InternalError(c, err)
	}
	slog.Info("agent registered", "agent_id", req.AgentID)
	return c.Status(201).JSON(fiber.Map{"agent_id": req.AgentID, "registered_at": now})
}

func (h *AgentHandler) Heartbeat(c *fiber.Ctx) error {
	var req struct {
		AgentID     string `json:"agent_id"`
		Hostname    string `json:"hostname"`
		Version     string `json:"version"`
		Interface   string `json:"interface"`
		OS          string `json:"os"`
		CaptureMode string `json:"capture_mode"`
		EbpfEnabled bool   `json:"ebpf_enabled"`
		Cluster     string `json:"cluster,omitempty"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid body"})
	}
	if req.AgentID == "" {
		return c.Status(400).JSON(fiber.Map{"error": "agent_id is required"})
	}
	if h.CH == nil {
		return c.Status(503).JSON(fiber.Map{"error": "ClickHouse not available"})
	}
	now := time.Now().UTC()
	ebpfInt := uint8(0)
	if req.EbpfEnabled {
		ebpfInt = 1
	}
	if err := h.CH.Exec(c.Context(),
		`INSERT INTO agents (agent_id, hostname, version, interface, last_seen, registered_at, os, capture_mode, ebpf_enabled, cluster)
         VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		req.AgentID, req.Hostname, req.Version, req.Interface, now, now, req.OS, req.CaptureMode, ebpfInt, req.Cluster,
	); err != nil {
		return util.InternalError(c, err)
	}
	return c.JSON(fiber.Map{"ok": true, "ts": now})
}

func (h *AgentHandler) Stats(c *fiber.Ctx) error {
	if h.CH == nil {
		return c.Status(503).JSON(fiber.Map{"error": "ClickHouse not available"})
	}
	rows, err := h.CH.Query(c.Context(), `
        SELECT agent_id, count() AS flow_count
        FROM flows
        WHERE ts >= now() - INTERVAL 1 HOUR
        GROUP BY agent_id`)
	if err != nil {
		return util.InternalError(c, err)
	}
	defer rows.Close()
	stats := make(map[string]int64)
	for rows.Next() {
		var id string
		var cnt int64
		if err := rows.Scan(&id, &cnt); err != nil {
			continue
		}
		stats[id] = cnt
	}
	return c.JSON(fiber.Map{"stats": stats})
}
