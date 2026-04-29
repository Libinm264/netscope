// Package baseline computes a 7-day rolling traffic baseline per agent+protocol
// and detects Z-score anomalies against it.
//
// Two goroutines:
//
//   Engine.Start()
//     └─ hourly ticker  → recomputeBaselines()   writes traffic_baselines
//     └─ 5-min ticker   → detectAnomalies()       writes anomaly_events
//
// The baseline is: for each (agent_id, protocol, hour_of_week) bucket we store
// the mean and population stddev of hourly flow counts over the last 7 days.
// hour_of_week ∈ [0, 167]  where 0 = Monday 00:00 … 167 = Sunday 23:00.
package baseline

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"time"

	"github.com/netscope/hub-api/clickhouse"
)

const (
	// recomputeInterval is how often we rebuild the baseline from flows history.
	recomputeInterval = 1 * time.Hour

	// detectInterval is how often we run the Z-score detector.
	detectInterval = 5 * time.Minute

	// zScoreThreshold is the minimum |Z| to emit an anomaly.
	zScoreThreshold = 3.0

	// minSamples is the minimum number of historical hourly buckets required
	// before we trust the baseline enough to emit anomalies.
	minSamples = 3

	// minMeanFloor prevents noise on extremely quiet agents.
	minMeanFloor = 5.0
)

// Engine manages the baseline computation and anomaly detection loops.
type Engine struct {
	ch     *clickhouse.Client
	cancel context.CancelFunc
}

// New creates an Engine. Call Start() to begin background processing.
func New(ch *clickhouse.Client) *Engine {
	return &Engine{ch: ch}
}

// Start launches the background goroutines. Call Stop() to shut them down.
func (e *Engine) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	e.cancel = cancel

	go e.loop(ctx)
	slog.Info("baseline engine started")
}

// Stop signals both goroutines to exit and waits for them.
func (e *Engine) Stop() {
	if e.cancel != nil {
		e.cancel()
	}
	slog.Info("baseline engine stopped")
}

func (e *Engine) loop(ctx context.Context) {
	// Run once immediately on startup, then on each ticker.
	e.recomputeBaselines(ctx)
	e.detectAnomalies(ctx)

	recomputeTicker := time.NewTicker(recomputeInterval)
	detectTicker    := time.NewTicker(detectInterval)
	defer recomputeTicker.Stop()
	defer detectTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-recomputeTicker.C:
			e.recomputeBaselines(ctx)
		case <-detectTicker.C:
			e.detectAnomalies(ctx)
		}
	}
}

// recomputeBaselines rebuilds traffic_baselines from the last 7 days of flows.
// Uses ClickHouse's avg() + stddevPop() to compute per (agent, protocol, hour_of_week).
func (e *Engine) recomputeBaselines(ctx context.Context) {
	start := time.Now()
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	// Step 1: aggregate hourly flow counts per (agent, protocol, hour_of_week).
	// Step 2: compute mean + stddev across all matching hours in the last 7 days.
	const query = `
		SELECT
			agent_id,
			protocol,
			toUInt8((toDayOfWeek(hour_bucket) - 1) * 24 + toHour(hour_bucket)) AS hour_of_week,
			toFloat64(avg(hourly_count))      AS flow_mean,
			toFloat64(stddevPop(hourly_count)) AS flow_std,
			toFloat64(avg(bytes_in_sum))       AS bytes_in_mean,
			toFloat64(avg(bytes_out_sum))      AS bytes_out_mean,
			count()                            AS sample_count
		FROM (
			SELECT
				agent_id,
				protocol,
				toStartOfHour(ts) AS hour_bucket,
				count()           AS hourly_count,
				sum(bytes_in)     AS bytes_in_sum,
				sum(bytes_out)    AS bytes_out_sum
			FROM flows
			WHERE ts >= now() - INTERVAL 7 DAY
			  AND agent_id != ''
			GROUP BY agent_id, protocol, hour_bucket
		)
		GROUP BY agent_id, protocol, hour_of_week
		HAVING sample_count >= 1
	`

	rows, err := e.ch.Query(ctx, query)
	if err != nil {
		slog.Error("baseline recompute query failed", "err", err)
		return
	}
	defer rows.Close()

	type row struct {
		AgentID      string
		Protocol     string
		HourOfWeek   uint8
		FlowMean     float64
		FlowStd      float64
		BytesInMean  float64
		BytesOutMean float64
		SampleCount  uint32
	}

	var records []row
	for rows.Next() {
		var r row
		if err := rows.Scan(
			&r.AgentID, &r.Protocol, &r.HourOfWeek,
			&r.FlowMean, &r.FlowStd,
			&r.BytesInMean, &r.BytesOutMean,
			&r.SampleCount,
		); err != nil {
			continue
		}
		records = append(records, r)
	}
	if err := rows.Err(); err != nil {
		slog.Error("baseline recompute scan failed", "err", err)
		return
	}

	if len(records) == 0 {
		slog.Debug("baseline recompute: no data yet")
		return
	}

	now := time.Now().UnixMilli()
	for _, r := range records {
		if err := e.ch.Exec(ctx, `
			INSERT INTO traffic_baselines
			  (agent_id, protocol, hour_of_week,
			   flow_count_mean, flow_count_std,
			   bytes_in_mean, bytes_out_mean,
			   sample_count, computed_at, version)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, now64(), ?)`,
			r.AgentID, r.Protocol, r.HourOfWeek,
			r.FlowMean, r.FlowStd,
			r.BytesInMean, r.BytesOutMean,
			r.SampleCount, now,
		); err != nil {
			slog.Error("baseline upsert failed", "agent", r.AgentID, "err", err)
		}
	}

	slog.Info("baseline recomputed",
		"records", len(records),
		"duration", time.Since(start).Round(time.Millisecond),
	)
}

// detectAnomalies compares the last 5-minute window's flow rate against the
// stored baseline and emits anomaly_events for any Z-score outliers.
func (e *Engine) detectAnomalies(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// Current time → hour_of_week for baseline lookup.
	now         := time.Now().UTC()
	// ClickHouse toDayOfWeek: Mon=1 … Sun=7
	dow         := int(now.Weekday()) // Go: Sun=0, Mon=1…Sat=6
	if dow == 0 { dow = 7 }           // Sunday → 7 to match ClickHouse
	hourOfWeek  := uint8((dow-1)*24 + now.Hour())

	// Count flows in the last 5 minutes per (agent_id, protocol).
	const obsQuery = `
		SELECT agent_id, hostname, protocol, count() AS flow_count
		FROM flows
		WHERE ts >= now() - INTERVAL 5 MINUTE
		  AND agent_id != ''
		GROUP BY agent_id, hostname, protocol
	`
	obsRows, err := e.ch.Query(ctx, obsQuery)
	if err != nil {
		slog.Error("anomaly detection: observation query failed", "err", err)
		return
	}
	defer obsRows.Close()

	type obs struct {
		AgentID   string
		Hostname  string
		Protocol  string
		FlowCount uint64
	}
	var observations []obs
	for obsRows.Next() {
		var o obs
		if err := obsRows.Scan(&o.AgentID, &o.Hostname, &o.Protocol, &o.FlowCount); err != nil {
			continue
		}
		observations = append(observations, o)
	}
	obsRows.Close()

	if len(observations) == 0 {
		return
	}

	// Fetch baselines for this hour_of_week in one query.
	baselineRows, err := e.ch.Query(ctx, `
		SELECT agent_id, protocol,
		       flow_count_mean, flow_count_std, sample_count
		FROM traffic_baselines
		FINAL
		WHERE hour_of_week = ?
		  AND sample_count >= ?`,
		hourOfWeek, minSamples,
	)
	if err != nil {
		slog.Error("anomaly detection: baseline fetch failed", "err", err)
		return
	}
	defer baselineRows.Close()

	type baselineKey struct{ AgentID, Protocol string }
	type baselineVal struct {
		Mean        float64
		Std         float64
		SampleCount uint32
	}
	baselines := make(map[baselineKey]baselineVal)
	for baselineRows.Next() {
		var agentID, protocol string
		var mean, std float64
		var samples uint32
		if err := baselineRows.Scan(&agentID, &protocol, &mean, &std, &samples); err != nil {
			continue
		}
		baselines[baselineKey{agentID, protocol}] = baselineVal{mean, std, samples}
	}
	baselineRows.Close()

	// Compare each observation against its baseline.
	var detected int
	for _, o := range observations {
		bl, ok := baselines[baselineKey{o.AgentID, o.Protocol}]
		if !ok {
			continue // No baseline yet — skip
		}

		// The observation is a 5-min count; scale to hourly rate for comparison.
		observed := float64(o.FlowCount) * 12.0

		// Adaptive std floor: prevents false positives on quiet agents.
		stdFloor := math.Max(bl.Std, math.Max(bl.Mean*0.1, minMeanFloor))
		zScore   := (observed - bl.Mean) / stdFloor

		if math.Abs(zScore) < zScoreThreshold {
			continue
		}

		severity := zSeverity(zScore)
		anomalyType := "spike"
		if zScore < 0 {
			anomalyType = "drop"
		}

		desc := fmt.Sprintf(
			"%s: %s traffic on agent %s is %.1fx %s than usual "+
				"(observed: %.0f flows/hr, expected: ~%.0f, σ=%.1f)",
			capitalize(anomalyType),
			o.Protocol,
			o.Hostname,
			math.Abs(observed/math.Max(bl.Mean, 1)),
			map[string]string{"spike": "higher", "drop": "lower"}[anomalyType],
			observed, bl.Mean, zScore,
		)

		if err := e.ch.Exec(ctx, `
			INSERT INTO anomaly_events
			  (agent_id, hostname, protocol,
			   anomaly_type, z_score, observed, expected,
			   description, severity, detected_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, now64())`,
			o.AgentID, o.Hostname, o.Protocol,
			anomalyType, zScore, observed, bl.Mean,
			desc, severity,
		); err != nil {
			slog.Error("anomaly insert failed", "err", err)
			continue
		}
		detected++
		slog.Info("anomaly detected",
			"agent", o.Hostname,
			"protocol", o.Protocol,
			"z_score", fmt.Sprintf("%.2f", zScore),
			"severity", severity,
		)
	}

	if detected > 0 {
		slog.Info("anomaly detection pass complete", "anomalies", detected)
	}
}

func zSeverity(z float64) string {
	abs := math.Abs(z)
	switch {
	case abs >= 6:
		return "high"
	case abs >= 4:
		return "medium"
	default:
		return "low"
	}
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return string(s[0]-32) + s[1:]
}
