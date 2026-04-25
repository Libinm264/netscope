package handlers

import (
	"log/slog"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/netscope/hub-api/clickhouse"
	"github.com/netscope/hub-api/models"
	"github.com/netscope/hub-api/util"
)

type PolicyHandler struct {
	CH *clickhouse.Client
}

func (h *PolicyHandler) List(c *fiber.Ctx) error {
	if h.CH == nil {
		return c.Status(503).JSON(fiber.Map{"error": "ClickHouse not available"})
	}
	rows, err := h.CH.Query(c.Context(),
		`SELECT id, name, process_name, action, dst_ip_cidr, dst_port, description, enabled, created_at
         FROM process_policies ORDER BY created_at DESC`)
	if err != nil {
		return util.InternalError(c, err)
	}
	defer rows.Close()
	policies := make([]models.ProcessPolicy, 0)
	for rows.Next() {
		var p models.ProcessPolicy
		var enabledInt uint8
		if err := rows.Scan(
			&p.ID, &p.Name, &p.ProcessName, &p.Action,
			&p.DstIPCIDR, &p.DstPort, &p.Description, &enabledInt, &p.CreatedAt,
		); err != nil {
			slog.Warn("policies list: scan", "err", err)
			continue
		}
		p.Enabled = enabledInt == 1
		policies = append(policies, p)
	}
	return c.JSON(fiber.Map{"policies": policies})
}

func (h *PolicyHandler) Create(c *fiber.Ctx) error {
	var req models.CreatePolicyRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid body"})
	}
	if req.Name == "" || req.ProcessName == "" {
		return c.Status(400).JSON(fiber.Map{"error": "name and process_name are required"})
	}
	if req.Action != "alert" && req.Action != "deny" {
		return c.Status(400).JSON(fiber.Map{"error": "action must be 'alert' or 'deny'"})
	}
	if h.CH == nil {
		return c.Status(503).JSON(fiber.Map{"error": "ClickHouse not available"})
	}
	id := uuid.New().String()
	now := time.Now().UTC()
	if err := h.CH.Exec(c.Context(),
		`INSERT INTO process_policies (id, name, process_name, action, dst_ip_cidr, dst_port, description, enabled, created_at)
         VALUES (?, ?, ?, ?, ?, ?, ?, 1, ?)`,
		id, req.Name, req.ProcessName, req.Action, req.DstIPCIDR, req.DstPort, req.Description, now,
	); err != nil {
		return util.InternalError(c, err)
	}
	slog.Info("policy created", "id", id, "name", req.Name)
	return c.Status(201).JSON(fiber.Map{"id": id, "created_at": now})
}

func (h *PolicyHandler) Update(c *fiber.Ctx) error {
	id := c.Params("id")
	if id == "" {
		return c.Status(400).JSON(fiber.Map{"error": "id required"})
	}
	var body struct {
		Enabled *bool   `json:"enabled"`
		Action  *string `json:"action"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid body"})
	}
	if h.CH == nil {
		return c.Status(503).JSON(fiber.Map{"error": "ClickHouse not available"})
	}
	if body.Enabled != nil {
		v := uint8(0)
		if *body.Enabled {
			v = 1
		}
		if err := h.CH.Exec(c.Context(),
			`ALTER TABLE process_policies UPDATE enabled = ? WHERE id = ?`, v, id,
		); err != nil {
			return util.InternalError(c, err)
		}
	}
	if body.Action != nil {
		if *body.Action != "alert" && *body.Action != "deny" {
			return c.Status(400).JSON(fiber.Map{"error": "action must be 'alert' or 'deny'"})
		}
		if err := h.CH.Exec(c.Context(),
			`ALTER TABLE process_policies UPDATE action = ? WHERE id = ?`, *body.Action, id,
		); err != nil {
			return util.InternalError(c, err)
		}
	}
	return c.JSON(fiber.Map{"ok": true})
}

func (h *PolicyHandler) Delete(c *fiber.Ctx) error {
	id := c.Params("id")
	if id == "" {
		return c.Status(400).JSON(fiber.Map{"error": "id required"})
	}
	if h.CH == nil {
		return c.Status(503).JSON(fiber.Map{"error": "ClickHouse not available"})
	}
	if err := h.CH.Exec(c.Context(),
		`ALTER TABLE process_policies DELETE WHERE id = ?`, id,
	); err != nil {
		return util.InternalError(c, err)
	}
	return c.JSON(fiber.Map{"ok": true})
}

func (h *PolicyHandler) ListViolations(c *fiber.Ctx) error {
	if h.CH == nil {
		return c.Status(503).JSON(fiber.Map{"error": "ClickHouse not available"})
	}
	window := c.Query("window", "24h")
	windowClause := windowToInterval(window)
	limit := c.QueryInt("limit", 100)

	rows, err := h.CH.Query(c.Context(),
		`SELECT id, policy_id, policy_name, process_name, pid,
                src_ip, dst_ip, dst_port, protocol, agent_id, hostname, violated_at
         FROM policy_violations
         WHERE violated_at >= now() - INTERVAL `+windowClause+`
         ORDER BY violated_at DESC LIMIT ?`, uint64(limit))
	if err != nil {
		return util.InternalError(c, err)
	}
	defer rows.Close()
	violations := make([]models.PolicyViolation, 0)
	for rows.Next() {
		var v models.PolicyViolation
		if err := rows.Scan(
			&v.ID, &v.PolicyID, &v.PolicyName, &v.ProcessName, &v.PID,
			&v.SrcIP, &v.DstIP, &v.DstPort, &v.Protocol, &v.AgentID, &v.Hostname, &v.ViolatedAt,
		); err != nil {
			continue
		}
		violations = append(violations, v)
	}
	return c.JSON(fiber.Map{"violations": violations, "window": window})
}

