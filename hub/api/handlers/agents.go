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
	// Use argMax / max aggregation instead of FINAL to avoid missing freshly-inserted
	// rows before the ReplacingMergeTree background merge runs on ClickHouse 24.x.
	//
	// Flow counts are pre-aggregated in a subquery so that the right side of the
	// JOIN is a tiny per-agent summary rather than the full flows table.  Joining
	// the raw flows table directly causes ClickHouse to build a large intermediate
	// result (one row per flow × agent) before the GROUP BY can reduce it, which
	// can hit the default max_memory_usage limit and silently return empty results.
	rows, err := h.CH.Query(c.Context(), `
        SELECT
            a.agent_id,
            argMax(a.hostname,     a.last_seen) AS hostname,
            argMax(a.version,      a.last_seen) AS version,
            argMax(a.interface,    a.last_seen) AS interface,
            max(a.last_seen)                    AS last_seen,
            min(a.registered_at)               AS registered_at,
            argMax(a.os,           a.last_seen) AS os,
            argMax(a.capture_mode, a.last_seen) AS capture_mode,
            argMax(a.ebpf_enabled, a.last_seen) AS ebpf_enabled,
            argMax(a.cluster,      a.last_seen) AS cluster,
            coalesce(any(fc.flow_count_1h), 0)  AS flow_count_1h
        FROM agents AS a
        LEFT JOIN (
            SELECT agent_id, count() AS flow_count_1h
            FROM flows
            WHERE ts >= now() - INTERVAL 1 HOUR
            GROUP BY agent_id
        ) AS fc ON fc.agent_id = a.agent_id
        GROUP BY a.agent_id
        ORDER BY max(a.last_seen) DESC`)
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
		AgentID        string  `json:"agent_id"`
		Hostname       string  `json:"hostname"`
		Version        string  `json:"version"`
		Interface      string  `json:"interface"`
		OS             string  `json:"os"`
		CaptureMode    string  `json:"capture_mode"`
		EbpfEnabled    bool    `json:"ebpf_enabled"`
		Cluster        string  `json:"cluster,omitempty"`
		// Performance telemetry (v0.7+) — optional: older agents omit these.
		CpuPct         float64 `json:"cpu_pct"`
		MemMB          uint64  `json:"mem_mb"`
		PacketsDropped uint64  `json:"packets_dropped"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid body"})
	}
	if req.AgentID == "" {
		return c.Status(400).JSON(fiber.Map{"error": "agent_id is required"})
	}

	// Whitelist-validate enum fields to prevent junk data from cluttering the DB.
	validCaptureMode := map[string]bool{"": true, "pcap": true, "ebpf": true}
	if !validCaptureMode[req.CaptureMode] {
		return c.Status(400).JSON(fiber.Map{"error": "invalid capture_mode"})
	}
	validOS := map[string]bool{"": true, "linux": true, "darwin": true, "windows": true}
	if !validOS[req.OS] {
		return c.Status(400).JSON(fiber.Map{"error": "invalid os"})
	}
	const maxStrLen = 256
	if len(req.Hostname) > maxStrLen || len(req.Cluster) > maxStrLen ||
		len(req.Interface) > maxStrLen || len(req.Version) > 64 {
		return c.Status(400).JSON(fiber.Map{"error": "field value too long"})
	}
	// Clamp CPU to a sane range — guard against instrumentation bugs.
	if req.CpuPct < 0 || req.CpuPct > 100 {
		req.CpuPct = 0
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

	// Persist perf sample if the agent sent telemetry.
	if req.CpuPct > 0 || req.MemMB > 0 {
		_ = h.CH.Exec(c.Context(),
			`INSERT INTO agent_perf (agent_id, ts, cpu_pct, mem_mb, packets_dropped)
             VALUES (?, ?, ?, ?, ?)`,
			req.AgentID, now, req.CpuPct, req.MemMB, req.PacketsDropped,
		)
	}

	return c.JSON(fiber.Map{"ok": true, "ts": now})
}

// PerfHistory returns the last N performance samples for a specific agent.
//
// GET /api/v1/agents/:id/perf?limit=60
func (h *AgentHandler) PerfHistory(c *fiber.Ctx) error {
	agentID := c.Params("id")
	if agentID == "" {
		return c.Status(400).JSON(fiber.Map{"error": "agent_id required"})
	}
	limit := c.QueryInt("limit", 60)
	if limit < 1 || limit > 1440 {
		limit = 60
	}
	if h.CH == nil {
		return c.JSON(fiber.Map{"samples": []struct{}{}})
	}

	rows, err := h.CH.Query(c.Context(),
		`SELECT ts, cpu_pct, mem_mb, packets_dropped
         FROM agent_perf
         WHERE agent_id = ?
         ORDER BY ts DESC
         LIMIT ?`, agentID, limit)
	if err != nil {
		return util.InternalError(c, err)
	}
	defer rows.Close()

	type sample struct {
		TS             time.Time `json:"ts"`
		CpuPct         float64   `json:"cpu_pct"`
		MemMB          uint64    `json:"mem_mb"`
		PacketsDropped uint64    `json:"packets_dropped"`
	}
	samples := make([]sample, 0, limit)
	for rows.Next() {
		var s sample
		if err := rows.Scan(&s.TS, &s.CpuPct, &s.MemMB, &s.PacketsDropped); err != nil {
			continue
		}
		samples = append(samples, s)
	}
	return c.JSON(fiber.Map{"agent_id": agentID, "samples": samples})
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
