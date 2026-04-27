package handlers

import (
	"fmt"

	"github.com/gofiber/fiber/v2"

	"github.com/netscope/hub-api/clickhouse"
	"github.com/netscope/hub-api/models"
	"github.com/netscope/hub-api/util"
)

// ServicesHandler provides the service dependency graph endpoint.
type ServicesHandler struct {
	CH *clickhouse.Client
}

// Graph handles GET /api/v1/services/graph.
// It aggregates flows by src_ip → dst_ip pair over the requested time window
// and returns a graph of nodes (IPs) and weighted edges (protocols, counts, latencies).
func (h *ServicesHandler) Graph(c *fiber.Ctx) error {
	if h.CH == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": "ClickHouse is not available",
		})
	}

	window := c.Query("window", "1h")
	interval := windowToInterval(window)

	// Aggregate flows into edges
	rows, err := h.CH.Query(c.Context(), fmt.Sprintf(`
		SELECT
			src_ip,
			dst_ip,
			protocol,
			count()                      AS flow_count,
			avg(duration_ms)             AS avg_latency_ms,
			sum(bytes_in + bytes_out)    AS bytes_total
		FROM flows
		WHERE ts >= now() - INTERVAL %s
		GROUP BY src_ip, dst_ip, protocol
		ORDER BY flow_count DESC
		LIMIT 300
	`, interval))
	if err != nil {
		return util.InternalError(c, err)
	}
	defer rows.Close()

	nodeMap := make(map[string]*models.ServiceNode)
	edges := make([]models.ServiceEdge, 0, 64)

	for rows.Next() {
		var edge models.ServiceEdge
		if err := rows.Scan(
			&edge.Source, &edge.Target, &edge.Protocol,
			&edge.Count, &edge.AvgLatencyMs, &edge.BytesTotal,
		); err != nil {
			continue
		}
		edges = append(edges, edge)

		for _, ip := range []string{edge.Source, edge.Target} {
			if ip == "" {
				continue
			}
			if _, ok := nodeMap[ip]; !ok {
				nodeMap[ip] = &models.ServiceNode{ID: ip, IP: ip}
			}
			nodeMap[ip].FlowCount += edge.Count
		}
	}

	// Enrich nodes with agent hostnames (known hosts)
	agentRows, err := h.CH.Query(c.Context(),
		`SELECT agent_id, argMax(hostname, last_seen) AS hostname FROM agents GROUP BY agent_id ORDER BY max(last_seen) DESC`)
	if err == nil {
		defer agentRows.Close()
		for agentRows.Next() {
			var agentID, hostname string
			if err := agentRows.Scan(&agentID, &hostname); err != nil {
				continue
			}
			// Match on agent_id (UUID format) or hostname (may equal an observed IP)
			for _, node := range nodeMap {
				if node.IP == agentID || node.IP == hostname {
					node.IsKnown = true
					node.Hostname = hostname
				}
			}
		}
	}

	nodes := make([]models.ServiceNode, 0, len(nodeMap))
	for _, n := range nodeMap {
		nodes = append(nodes, *n)
	}

	return c.JSON(models.ServiceGraph{
		Nodes:  nodes,
		Edges:  edges,
		Window: window,
	})
}

// windowToInterval converts a UI window string to a ClickHouse INTERVAL expression.
func windowToInterval(w string) string {
	switch w {
	case "15m":
		return "15 MINUTE"
	case "1h":
		return "1 HOUR"
	case "6h":
		return "6 HOUR"
	case "7d":
		return "7 DAY"
	case "30d":
		return "30 DAY"
	default: // "24h"
		return "24 HOUR"
	}
}
