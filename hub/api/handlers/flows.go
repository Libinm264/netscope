package handlers

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/netscope/hub-api/clickhouse"
	"github.com/netscope/hub-api/kafka"
	"github.com/netscope/hub-api/models"
)

// ── SSE broadcast hub ────────────────────────────────────────────────────────

type sseHub struct {
	mu   sync.RWMutex
	subs map[string]chan []byte
}

var globalHub = &sseHub{subs: make(map[string]chan []byte)}

func (h *sseHub) subscribe(id string) chan []byte {
	ch := make(chan []byte, 64)
	h.mu.Lock()
	h.subs[id] = ch
	h.mu.Unlock()
	return ch
}

func (h *sseHub) unsubscribe(id string) {
	h.mu.Lock()
	if ch, ok := h.subs[id]; ok {
		close(ch)
		delete(h.subs, id)
	}
	h.mu.Unlock()
}

func (h *sseHub) broadcast(data []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, ch := range h.subs {
		select {
		case ch <- data:
		default:
			// Slow consumer — skip rather than block
		}
	}
}

// BroadcastFlow serialises a flow and fans it out to all active SSE clients.
// Called by both the ingest handler and the Kafka consumer goroutine.
func BroadcastFlow(flow models.Flow) {
	data, err := json.Marshal(flow)
	if err != nil {
		slog.Warn("broadcast: marshal failed", "err", err)
		return
	}
	globalHub.broadcast(data)
}

// ── FlowHandler ──────────────────────────────────────────────────────────────

// FlowHandler groups the three flow-related HTTP handlers.
type FlowHandler struct {
	CH       *clickhouse.Client
	Writer   *clickhouse.Writer
	Producer *kafka.Producer
}

// Ingest handles POST /api/v1/ingest.
// Agents batch-post decoded flows here. Each flow is:
//  1. Written to ClickHouse via the async Writer (or Kafka → CH if Kafka is up)
//  2. Fanned out to all connected SSE clients immediately
func (h *FlowHandler) Ingest(c *fiber.Ctx) error {
	var req models.IngestRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "invalid request body: " + err.Error(),
		})
	}

	var errs []string
	for i := range req.Flows {
		f := &req.Flows[i]

		// Back-fill agent metadata from the envelope if the flow doesn't carry it.
		if f.AgentID == "" {
			f.AgentID = req.AgentID
		}
		if f.Hostname == "" {
			f.Hostname = req.Hostname
		}
		if f.Timestamp.IsZero() {
			f.Timestamp = time.Now().UTC()
		}

		// Persist —————————————————————————————————————————————————————————
		if h.Producer != nil {
			// Kafka path: produce → consumer goroutine writes to ClickHouse
			if err := h.Producer.Publish(c.Context(), *f); err != nil {
				slog.Warn("kafka publish failed, falling back to direct write", "err", err)
				if h.Writer != nil {
					h.Writer.Write(*f)
				}
			}
		} else if h.Writer != nil {
			// Direct path when Kafka is unavailable
			h.Writer.Write(*f)
		}

		// SSE fan-out (always, regardless of persistence status)
		BroadcastFlow(*f)
	}

	return c.JSON(models.IngestResponse{
		Received: len(req.Flows),
		Errors:   errs,
	})
}

// Query handles GET /api/v1/flows.
// Supported query params: protocol, src_ip, dst_ip, hostname, limit, offset.
func (h *FlowHandler) Query(c *fiber.Ctx) error {
	if h.CH == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": "ClickHouse is not available",
		})
	}

	protocol := c.Query("protocol")
	srcIP    := c.Query("src_ip")
	dstIP    := c.Query("dst_ip")
	hostname := c.Query("hostname")
	limit    := c.QueryInt("limit", 100)
	offset   := c.QueryInt("offset", 0)
	if limit > 1000 {
		limit = 1000
	}

	// Build dynamic WHERE clause
	where := "1=1"
	filterArgs := make([]interface{}, 0, 4)
	if protocol != "" {
		where += " AND protocol = ?"
		filterArgs = append(filterArgs, protocol)
	}
	if srcIP != "" {
		where += " AND src_ip = ?"
		filterArgs = append(filterArgs, srcIP)
	}
	if dstIP != "" {
		where += " AND dst_ip = ?"
		filterArgs = append(filterArgs, dstIP)
	}
	if hostname != "" {
		where += " AND hostname = ?"
		filterArgs = append(filterArgs, hostname)
	}

	ctx := c.Context()

	// Total count
	countArgs := append(append([]interface{}{}, filterArgs...), nil)[:len(filterArgs)]
	countRows, err := h.CH.Query(ctx,
		fmt.Sprintf("SELECT count() FROM flows WHERE %s", where),
		countArgs...)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	var total uint64
	if countRows.Next() {
		_ = countRows.Scan(&total)
	}
	countRows.Close()

	// Data rows
	dataArgs := append(filterArgs, uint64(limit), uint64(offset))
	rows, err := h.CH.Query(ctx, fmt.Sprintf(
		`SELECT id, agent_id, hostname, ts, protocol,
		        src_ip, src_port, dst_ip, dst_port,
		        bytes_in, bytes_out, duration_ms, info,
		        http_method, http_path, http_status,
		        dns_query, dns_type
		 FROM flows
		 WHERE %s
		 ORDER BY ts DESC
		 LIMIT ? OFFSET ?`, where),
		dataArgs...)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	defer rows.Close()

	flows := make([]models.Flow, 0, limit)
	for rows.Next() {
		var (
			f          models.Flow
			httpMethod string
			httpPath   string
			httpStatus uint16
			dnsQuery   string
			dnsType    string
		)
		if err := rows.Scan(
			&f.ID, &f.AgentID, &f.Hostname, &f.Timestamp, &f.Protocol,
			&f.SrcIP, &f.SrcPort, &f.DstIP, &f.DstPort,
			&f.BytesIn, &f.BytesOut, &f.DurationMs, &f.Info,
			&httpMethod, &httpPath, &httpStatus,
			&dnsQuery, &dnsType,
		); err != nil {
			slog.Warn("scan flow row", "err", err)
			continue
		}
		if httpMethod != "" {
			f.HTTP = &models.HttpFlow{
				Method: httpMethod,
				Path:   httpPath,
				Status: int(httpStatus),
			}
		}
		if dnsQuery != "" {
			f.DNS = &models.DnsFlow{
				QueryName: dnsQuery,
				QueryType: dnsType,
			}
		}
		flows = append(flows, f)
	}

	return c.JSON(fiber.Map{
		"flows": flows,
		"total": total,
	})
}

// Stream handles GET /api/v1/flows/stream using Server-Sent Events.
// Clients receive a real-time stream of flows as they are ingested.
func (h *FlowHandler) Stream(c *fiber.Ctx) error {
	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")
	c.Set("Access-Control-Allow-Origin", "*")
	c.Set("X-Accel-Buffering", "no")

	subID := fmt.Sprintf("sse-%d", time.Now().UnixNano())
	ch := globalHub.subscribe(subID)

	c.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
		defer globalHub.unsubscribe(subID)

		ticker := time.NewTicker(20 * time.Second)
		defer ticker.Stop()

		// Send a connected confirmation immediately
		_, _ = fmt.Fprintf(w, ": connected\n\n")
		_ = w.Flush()

		for {
			select {
			case data, ok := <-ch:
				if !ok {
					return
				}
				if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
					return
				}
				if err := w.Flush(); err != nil {
					return
				}

			case <-ticker.C:
				// Keep-alive ping so proxies don't close idle connections
				if _, err := fmt.Fprintf(w, ": ping\n\n"); err != nil {
					return
				}
				if err := w.Flush(); err != nil {
					return
				}
			}
		}
	})

	return nil
}
