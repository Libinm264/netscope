package clickhouse

import (
	"context"
	"log/slog"
	"time"

	"github.com/netscope/hub-api/models"
)

const (
	batchSize    = 500
	flushTimeout = time.Second
)

// Writer receives flows on a channel and batch-inserts them into ClickHouse.
// It also upserts agent records as flows arrive.
type Writer struct {
	client  *Client
	ch      chan models.Flow
	stopCh  chan struct{}
	doneCh  chan struct{}
}

// NewWriter creates a Writer and starts its background flush goroutine.
func NewWriter(client *Client) *Writer {
	w := &Writer{
		client: client,
		ch:     make(chan models.Flow, batchSize*4),
		stopCh: make(chan struct{}),
		doneCh: make(chan struct{}),
	}
	go w.run()
	return w
}

// Write enqueues a flow for asynchronous batch insertion. It is non-blocking;
// if the internal buffer is full the flow is dropped and a warning is logged.
func (w *Writer) Write(flow models.Flow) {
	select {
	case w.ch <- flow:
	default:
		slog.Warn("clickhouse writer buffer full, dropping flow", "id", flow.ID)
	}
}

// Stop signals the background goroutine to flush remaining flows and exit.
func (w *Writer) Stop() {
	close(w.stopCh)
	<-w.doneCh
}

// run is the background goroutine that accumulates flows and flushes them.
func (w *Writer) run() {
	defer close(w.doneCh)

	ticker := time.NewTicker(flushTimeout)
	defer ticker.Stop()

	batch := make([]models.Flow, 0, batchSize)

	flush := func() {
		if len(batch) == 0 {
			return
		}
		if err := w.insertFlows(batch); err != nil {
			slog.Error("clickhouse: batch insert failed", "err", err, "count", len(batch))
		} else {
			slog.Debug("clickhouse: flushed flows", "count", len(batch))
		}
		batch = batch[:0]
	}

	for {
		select {
		case flow := <-w.ch:
			batch = append(batch, flow)
			if len(batch) >= batchSize {
				flush()
			}

		case <-ticker.C:
			flush()

		case <-w.stopCh:
			// Drain remaining items from the channel before exiting.
			for {
				select {
				case flow := <-w.ch:
					batch = append(batch, flow)
				default:
					flush()
					return
				}
			}
		}
	}
}

// insertFlows performs a batch INSERT into the flows table and upserts each
// unique agent into the agents table.
func (w *Writer) insertFlows(flows []models.Flow) error {
	ctx := context.Background()

	b, err := w.client.PrepareBatch(ctx,
		`INSERT INTO flows
		 (id, agent_id, hostname, ts, protocol, src_ip, src_port, dst_ip, dst_port,
		  bytes_in, bytes_out, duration_ms, info,
		  http_method, http_path, http_status,
		  dns_query, dns_type,
		  country_code, country_name, as_org, threat_score, threat_level)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
	)
	if err != nil {
		return err
	}

	// Track unique agents so we upsert each at most once per batch.
	seenAgents := make(map[string]models.Flow)

	for _, f := range flows {
		httpMethod, httpPath := "", ""
		var httpStatus uint16
		dnsQuery, dnsType := "", ""

		if f.HTTP != nil {
			httpMethod = f.HTTP.Method
			httpPath = f.HTTP.Path
			httpStatus = uint16(f.HTTP.Status) //nolint:gosec
		}
		if f.DNS != nil {
			dnsQuery = f.DNS.QueryName
			dnsType = f.DNS.QueryType
		}

		if err := b.Append(
			f.ID,
			f.AgentID,
			f.Hostname,
			f.Timestamp,
			f.Protocol,
			f.SrcIP,
			f.SrcPort,
			f.DstIP,
			f.DstPort,
			f.BytesIn,
			f.BytesOut,
			f.DurationMs,
			f.Info,
			httpMethod,
			httpPath,
			httpStatus,
			dnsQuery,
			dnsType,
			f.CountryCode,
			f.CountryName,
			f.ASOrg,
			f.ThreatScore,
			f.ThreatLevel,
		); err != nil {
			slog.Warn("clickhouse: append row failed", "err", err, "id", f.ID)
			continue
		}

		if _, ok := seenAgents[f.AgentID]; !ok {
			seenAgents[f.AgentID] = f
		}
	}

	if err := b.Send(); err != nil {
		return err
	}

	// Upsert agents — fire-and-forget; failures are non-fatal.
	for agentID, f := range seenAgents {
		if err := w.client.Exec(ctx,
			`INSERT INTO agents (agent_id, hostname, last_seen)
			 VALUES (?, ?, ?)`,
			agentID, f.Hostname, f.Timestamp,
		); err != nil {
			slog.Warn("clickhouse: agent upsert failed",
				"agent_id", agentID, "err", err)
		}
	}

	return nil
}
