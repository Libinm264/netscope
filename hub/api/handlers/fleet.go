package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/netscope/hub-api/clickhouse"
	"github.com/netscope/hub-api/models"
	"github.com/netscope/hub-api/util"
)

// FleetHandler provides multi-cluster fleet visibility and remote config push.
type FleetHandler struct {
	CH *clickhouse.Client
}

// Clusters returns a per-cluster health grid aggregated from the agents table.
//
// GET /api/v1/fleet/clusters
func (h *FleetHandler) Clusters(c *fiber.Ctx) error {
	if h.CH == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "ClickHouse unavailable"})
	}
	ctx, cancel := context.WithTimeout(c.Context(), 10*time.Second)
	defer cancel()

	rows, err := h.CH.Query(ctx, `
		SELECT
			cluster,
			count()                                           AS agent_count,
			countIf(last_seen > now() - INTERVAL 5 MINUTE)   AS online_count,
			groupUniqArray(version)                           AS versions,
			sum(flow_count_1h)                                AS flows_1h
		FROM (
			SELECT
				a.cluster,
				a.version,
				a.last_seen,
				countIf(f.ts >= now() - INTERVAL 1 HOUR) AS flow_count_1h
			FROM (
				SELECT agent_id, hostname, version, cluster, last_seen
				FROM agents FINAL
			) a
			LEFT JOIN flows f ON f.agent_id = a.agent_id
			GROUP BY a.cluster, a.version, a.last_seen, a.agent_id
		)
		GROUP BY cluster
		ORDER BY cluster`)
	if err != nil {
		return util.InternalError(c, err)
	}
	defer rows.Close()

	summaries := make([]models.ClusterSummary, 0)
	for rows.Next() {
		var s models.ClusterSummary
		var versionsRaw string
		if err := rows.Scan(&s.Cluster, &s.AgentCount, &s.OnlineCount, &versionsRaw, &s.Flows1h); err != nil {
			continue
		}
		// versionsRaw comes back as a ClickHouse Array string: ['v0.4','v0.5']
		versionsRaw = strings.Trim(versionsRaw, "[]'")
		for _, v := range strings.Split(versionsRaw, ",") {
			v = strings.Trim(strings.TrimSpace(v), "'")
			if v != "" {
				s.Versions = append(s.Versions, v)
			}
		}
		if s.Cluster == "" {
			s.Cluster = "default"
		}
		summaries = append(summaries, s)
	}
	return c.JSON(fiber.Map{"clusters": summaries})
}

// Search executes a cross-cluster flow search — identical to the flows query but
// pre-filtered by cluster label.
//
// GET /api/v1/fleet/search
func (h *FleetHandler) Search(c *fiber.Ctx) error {
	if h.CH == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "ClickHouse unavailable"})
	}

	cluster := c.Query("cluster")
	srcIP := c.Query("src_ip")
	dstIP := c.Query("dst_ip")
	protocol := c.Query("protocol")
	limit := c.QueryInt("limit", 200)
	if limit > 1000 {
		limit = 1000
	}

	where := []string{"1=1"}
	args := []any{}

	if cluster != "" {
		// Join through agents to get cluster filter
		where = append(where, "f.agent_id IN (SELECT agent_id FROM agents FINAL WHERE cluster = ?)")
		args = append(args, cluster)
	}
	if srcIP != "" {
		where = append(where, "f.src_ip = ?")
		args = append(args, srcIP)
	}
	if dstIP != "" {
		where = append(where, "f.dst_ip = ?")
		args = append(args, dstIP)
	}
	if protocol != "" {
		where = append(where, "f.protocol = ?")
		args = append(args, strings.ToUpper(protocol))
	}

	q := fmt.Sprintf(`
		SELECT f.id, f.agent_id, f.hostname, f.ts, f.protocol,
		       f.src_ip, f.src_port, f.dst_ip, f.dst_port,
		       f.bytes_in, f.bytes_out, f.duration_ms,
		       f.process_name, f.cluster, a.cluster AS agent_cluster
		FROM flows f
		LEFT JOIN (SELECT agent_id, cluster FROM agents FINAL) a ON a.agent_id = f.agent_id
		WHERE %s
		ORDER BY f.ts DESC
		LIMIT %d`, strings.Join(where, " AND "), limit)

	ctx, cancel := context.WithTimeout(c.Context(), 15*time.Second)
	defer cancel()

	rows, err := h.CH.Query(ctx, q, args...)
	if err != nil {
		return util.InternalError(c, err)
	}
	defer rows.Close()

	type flowRow struct {
		ID          string    `json:"id"`
		AgentID     string    `json:"agent_id"`
		Hostname    string    `json:"hostname"`
		TS          time.Time `json:"ts"`
		Protocol    string    `json:"protocol"`
		SrcIP       string    `json:"src_ip"`
		SrcPort     uint16    `json:"src_port"`
		DstIP       string    `json:"dst_ip"`
		DstPort     uint16    `json:"dst_port"`
		BytesIn     uint64    `json:"bytes_in"`
		BytesOut    uint64    `json:"bytes_out"`
		DurationMs  uint32    `json:"duration_ms"`
		ProcessName string    `json:"process_name,omitempty"`
		Cluster     string    `json:"cluster,omitempty"`
	}

	flows := make([]flowRow, 0, limit)
	for rows.Next() {
		var r flowRow
		var clusterF, agentCluster string
		if err := rows.Scan(
			&r.ID, &r.AgentID, &r.Hostname, &r.TS, &r.Protocol,
			&r.SrcIP, &r.SrcPort, &r.DstIP, &r.DstPort,
			&r.BytesIn, &r.BytesOut, &r.DurationMs,
			&r.ProcessName, &clusterF, &agentCluster,
		); err != nil {
			continue
		}
		r.Cluster = agentCluster
		if r.Cluster == "" {
			r.Cluster = clusterF
		}
		flows = append(flows, r)
	}
	return c.JSON(fiber.Map{"flows": flows, "count": len(flows)})
}

// GetAgentConfig returns the pending (un-acked) config for an agent, if any.
//
// GET /api/v1/agents/:id/config
func (h *FleetHandler) GetAgentConfig(c *fiber.Ctx) error {
	agentID := c.Params("id")
	if agentID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "agent id required"})
	}
	if h.CH == nil {
		return c.JSON(fiber.Map{"config": nil})
	}
	ctx, cancel := context.WithTimeout(c.Context(), 5*time.Second)
	defer cancel()

	rows, err := h.CH.Query(ctx,
		`SELECT config, pushed_at, ack_at, version
		 FROM agent_configs FINAL
		 WHERE agent_id = ?
		 LIMIT 1`, agentID)
	if err != nil {
		return util.InternalError(c, err)
	}
	defer rows.Close()

	if !rows.Next() {
		return c.JSON(fiber.Map{"config": nil})
	}
	var rec models.AgentConfigRecord
	var ackAt time.Time
	rec.AgentID = agentID
	if err := rows.Scan(&rec.Config, &rec.PushedAt, &ackAt, &rec.Version); err != nil {
		return util.InternalError(c, err)
	}
	if ackAt.Year() > 2000 {
		rec.AckAt = ackAt
	}
	// If ack_at >= pushed_at the agent has already applied this config — no
	// pending update to deliver.
	if !rec.AckAt.IsZero() && !rec.AckAt.Before(rec.PushedAt) {
		return c.JSON(fiber.Map{"config": nil})
	}
	return c.JSON(fiber.Map{"config": rec})
}

// PushAgentConfig writes a new config for the agent to pick up on next heartbeat.
//
// POST /api/v1/agents/:id/config   (admin only)
func (h *FleetHandler) PushAgentConfig(c *fiber.Ctx) error {
	agentID := c.Params("id")
	if agentID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "agent id required"})
	}
	if h.CH == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "ClickHouse unavailable"})
	}

	var body models.AgentConfigPush
	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid body"})
	}
	if len(body.Config) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "config must not be empty"})
	}

	cfgBytes, err := json.Marshal(body.Config)
	if err != nil {
		return util.InternalError(c, err)
	}

	ctx, cancel := context.WithTimeout(c.Context(), 5*time.Second)
	defer cancel()

	now := time.Now().UTC()
	if err := h.CH.Exec(ctx,
		`INSERT INTO agent_configs (agent_id, config, pushed_at, ack_at, version)
		 VALUES (?, ?, ?, toDateTime64(0,3), ?)`,
		agentID, string(cfgBytes), now, now.UnixMilli(),
	); err != nil {
		return util.InternalError(c, err)
	}
	return c.JSON(fiber.Map{"ok": true, "pushed_at": now})
}

// AckAgentConfig records that an agent has applied the pushed config.
//
// POST /api/v1/agents/:id/config/ack
func (h *FleetHandler) AckAgentConfig(c *fiber.Ctx) error {
	agentID := c.Params("id")
	if agentID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "agent id required"})
	}
	if h.CH == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "ClickHouse unavailable"})
	}

	var body models.ConfigAck
	if err := c.BodyParser(&body); err != nil || body.AgentID == "" {
		body.AgentID = agentID
	}

	ctx, cancel := context.WithTimeout(c.Context(), 5*time.Second)
	defer cancel()

	// Read current version to preserve it in the upsert.
	rows, _ := h.CH.Query(ctx, `SELECT version FROM agent_configs FINAL WHERE agent_id = ? LIMIT 1`, agentID)
	var ver uint64
	if rows != nil {
		if rows.Next() {
			_ = rows.Scan(&ver)
		}
		rows.Close()
	}

	now := time.Now().UTC()
	if err := h.CH.Exec(ctx,
		`INSERT INTO agent_configs (agent_id, config, pushed_at, ack_at, version)
		 SELECT config, pushed_at, ?, ?
		 FROM agent_configs FINAL
		 WHERE agent_id = ?`,
		now, ver+1, agentID,
	); err != nil {
		return util.InternalError(c, err)
	}

	// Also update config_version on the agents table.
	_ = h.CH.Exec(ctx,
		`INSERT INTO agents (agent_id, hostname, version, interface, last_seen, registered_at,
		  os, capture_mode, ebpf_enabled, cluster, config_version)
		 SELECT agent_id, hostname, version, interface, last_seen, registered_at,
		        os, capture_mode, ebpf_enabled, cluster, ?
		 FROM agents FINAL WHERE agent_id = ?`,
		body.ConfigVersion, agentID,
	)

	_ = uuid.New() // ensure import used
	return c.JSON(fiber.Map{"ok": true})
}
