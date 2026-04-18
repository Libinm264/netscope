package handlers

import (
	"log/slog"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/netscope/hub-api/clickhouse"
	"github.com/netscope/hub-api/models"
	"github.com/netscope/hub-api/util"
)

// AgentHandler provides list and register endpoints for agent management.
type AgentHandler struct {
	CH *clickhouse.Client
}

// List handles GET /api/v1/agents.
func (h *AgentHandler) List(c *fiber.Ctx) error {
	if h.CH == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": "ClickHouse is not available",
		})
	}

	rows, err := h.CH.Query(c.Context(),
		`SELECT agent_id, hostname, version, interface, last_seen, registered_at
		 FROM agents
		 ORDER BY last_seen DESC`)
	if err != nil {
		return util.InternalError(c, err)
	}
	defer rows.Close()

	agents := make([]models.Agent, 0)
	for rows.Next() {
		var a models.Agent
		if err := rows.Scan(
			&a.AgentID, &a.Hostname, &a.Version,
			&a.Interface, &a.LastSeen, &a.RegisteredAt,
		); err != nil {
			slog.Warn("agents: scan row", "err", err)
			continue
		}
		agents = append(agents, a)
	}

	return c.JSON(fiber.Map{"agents": agents})
}

// Register handles POST /api/v1/agents/register.
func (h *AgentHandler) Register(c *fiber.Ctx) error {
	var req models.RegisterRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "invalid request body: " + err.Error(),
		})
	}
	if req.AgentID == "" || req.Hostname == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "agent_id and hostname are required",
		})
	}

	if h.CH == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": "ClickHouse is not available",
		})
	}

	now := time.Now().UTC()
	if err := h.CH.Exec(c.Context(),
		`INSERT INTO agents (agent_id, hostname, version, interface, last_seen, registered_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		req.AgentID, req.Hostname, req.Version, req.Interface, now, now,
	); err != nil {
		return util.InternalError(c, err)
	}

	slog.Info("agent registered", "agent_id", req.AgentID, "hostname", req.Hostname)
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"agent_id":      req.AgentID,
		"registered_at": now,
	})
}

// Heartbeat handles POST /api/v1/agents/heartbeat.
// Called periodically by agents to update their last_seen timestamp.
func (h *AgentHandler) Heartbeat(c *fiber.Ctx) error {
	var req struct {
		AgentID   string `json:"agent_id"`
		Hostname  string `json:"hostname"`
		Version   string `json:"version"`
		Interface string `json:"interface"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid body: " + err.Error()})
	}
	if req.AgentID == "" {
		return c.Status(400).JSON(fiber.Map{"error": "agent_id is required"})
	}
	if h.CH == nil {
		return c.Status(503).JSON(fiber.Map{"error": "ClickHouse not available"})
	}

	now := time.Now().UTC()
	// ReplacingMergeTree on last_seen: inserting a new row with the same agent_id
	// effectively updates last_seen after the next OPTIMIZE.
	if err := h.CH.Exec(c.Context(),
		`INSERT INTO agents (agent_id, hostname, version, interface, last_seen, registered_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		req.AgentID, req.Hostname, req.Version, req.Interface, now, now,
	); err != nil {
		return util.InternalError(c, err)
	}

	return c.JSON(fiber.Map{"ok": true, "ts": now})
}
