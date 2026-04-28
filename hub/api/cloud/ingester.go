package cloud

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/netscope/hub-api/clickhouse"
	"github.com/netscope/hub-api/enterprise/license"
)

// Ingester polls all enabled cloud flow sources on a configurable interval
// (default: 5 minutes) and writes normalised flows to ClickHouse.
type Ingester struct {
	ch       *clickhouse.Client
	lic      *license.License
	interval time.Duration

	mu     sync.Mutex
	ticker *time.Ticker
	done   chan struct{}
}

// New returns a new Ingester. Call Start() to begin scheduled pulls.
func New(ch *clickhouse.Client, lic *license.License) *Ingester {
	return &Ingester{
		ch:       ch,
		lic:      lic,
		interval: 5 * time.Minute,
		done:     make(chan struct{}),
	}
}

// Start launches the background pull goroutine.
func (ing *Ingester) Start() {
	if ing.ch == nil {
		return
	}
	ing.mu.Lock()
	ing.ticker = time.NewTicker(ing.interval)
	ing.mu.Unlock()

	go func() {
		slog.Info("cloud ingester started", "interval", ing.interval)
		ing.pullAll()
		for {
			select {
			case <-ing.ticker.C:
				ing.pullAll()
			case <-ing.done:
				return
			}
		}
	}()
}

// Stop gracefully shuts down the ingester.
func (ing *Ingester) Stop() {
	ing.mu.Lock()
	if ing.ticker != nil {
		ing.ticker.Stop()
	}
	ing.mu.Unlock()
	close(ing.done)
}

// pullAll loads enabled sources from the DB and pulls each one.
func (ing *Ingester) pullAll() {
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Minute)
	defer cancel()

	sources, err := ing.loadSources(ctx)
	if err != nil {
		slog.Warn("cloud: load sources", "err", err)
		return
	}
	for _, src := range sources {
		ing.pullSource(ctx, src)
	}
}

type cloudSource struct {
	ID         string
	Provider   string
	Name       string
	Config     string
	LastPulled time.Time
}

func (ing *Ingester) loadSources(ctx context.Context) ([]cloudSource, error) {
	rows, err := ing.ch.Query(ctx,
		`SELECT id, provider, name, config, last_pulled
		 FROM cloud_flow_sources
		 WHERE enabled = 1`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sources []cloudSource
	for rows.Next() {
		var s cloudSource
		if err := rows.Scan(&s.ID, &s.Provider, &s.Name, &s.Config, &s.LastPulled); err != nil {
			continue
		}
		sources = append(sources, s)
	}
	return sources, nil
}

// pullSource runs one pull cycle for a single source.
func (ing *Ingester) pullSource(ctx context.Context, src cloudSource) {
	start := time.Now()
	result := struct {
		rows uint64
		err  string
	}{}

	flows, pullErr := ing.doPull(ctx, src)
	if pullErr != nil {
		result.err = pullErr.Error()
		slog.Warn("cloud: pull error", "source", src.Name, "provider", src.Provider, "err", pullErr)
	} else {
		result.rows = uint64(len(flows))
		if result.rows > 0 {
			if err := ing.writeFlows(ctx, flows); err != nil {
				result.err = fmt.Sprintf("write: %v", err)
				slog.Warn("cloud: write error", "source", src.Name, "err", err)
			}
		}
		slog.Info("cloud: pull complete", "source", src.Name, "rows", result.rows)
	}

	durationMs := uint32(time.Since(start).Milliseconds())

	// Record pull log entry.
	_ = ing.ch.Exec(ctx,
		`INSERT INTO cloud_flow_pull_log
		 (id, source_id, provider, rows_ingested, pulled_at, duration_ms, error)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		uuid.New().String(), src.ID, src.Provider,
		result.rows, time.Now().UTC(), durationMs, result.err,
	)

	// Update last_pulled + error on the source record.
	now := time.Now().UTC()
	_ = ing.ch.Exec(ctx,
		`INSERT INTO cloud_flow_sources
		 (id, provider, name, config, enabled, last_pulled, error_msg, created_at, version)
		 SELECT id, provider, name, config, enabled, ?, ?, created_at,
		        toUInt64(toUnixTimestamp64Milli(now64()))
		 FROM cloud_flow_sources
		 WHERE id = ? ORDER BY created_at DESC LIMIT 1`,
		now, result.err, src.ID,
	)
}

// doPull dispatches to the correct cloud-provider puller after license checks.
func (ing *Ingester) doPull(ctx context.Context, src cloudSource) ([]*ParsedFlow, error) {
	switch strings.ToLower(src.Provider) {
	case "aws":
		puller, err := newAWSPuller(src.Config, src.Name)
		if err != nil {
			return nil, err
		}
		return puller.pull(ctx, src.ID, src.LastPulled)

	case "gcp":
		if !ing.lic.HasFeature(license.FeatureCloudIngestGCP) {
			return nil, fmt.Errorf("GCP VPC flow ingestion requires Enterprise license")
		}
		puller, err := newGCPPuller(src.Config, src.Name)
		if err != nil {
			return nil, err
		}
		return puller.pull(ctx, src.ID, src.LastPulled)

	case "azure":
		if !ing.lic.HasFeature(license.FeatureCloudIngestAzure) {
			return nil, fmt.Errorf("Azure NSG flow ingestion requires Enterprise license")
		}
		puller, err := newAzurePuller(src.Config, src.Name)
		if err != nil {
			return nil, err
		}
		return puller.pull(ctx, src.ID, src.LastPulled)

	default:
		return nil, fmt.Errorf("unknown provider: %s", src.Provider)
	}
}

// writeFlows batch-inserts parsed flows into ClickHouse.
func (ing *Ingester) writeFlows(ctx context.Context, flows []*ParsedFlow) error {
	const batchSize = 500
	for i := 0; i < len(flows); i += batchSize {
		end := i + batchSize
		if end > len(flows) {
			end = len(flows)
		}
		batch := flows[i:end]

		// Build a single INSERT with multiple rows.
		sb := strings.Builder{}
		sb.WriteString(`INSERT INTO flows
			(id, agent_id, hostname, ts, protocol,
			 src_ip, src_port, dst_ip, dst_port,
			 bytes_in, bytes_out, duration_ms,
			 country_code, country_name, as_org,
			 threat_score, threat_level, source)
			VALUES `)

		args := make([]any, 0, len(batch)*18)
		for j, f := range batch {
			if j > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString("(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)")
			args = append(args,
				f.ID, f.AgentID, f.Hostname, f.TS, f.Protocol,
				f.SrcIP, f.SrcPort, f.DstIP, f.DstPort,
				f.BytesIn, f.BytesOut, f.DurationMs,
				f.CountryCode, f.CountryName, f.AsOrg,
				f.ThreatScore, f.ThreatLevel, f.Source,
			)
		}
		if err := ing.ch.Exec(ctx, sb.String(), args...); err != nil {
			return err
		}
	}
	return nil
}

// ── JSON helper used across pull tests ───────────────────────────────────────

func marshalConfig(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
