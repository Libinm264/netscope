package alerting

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	nsmetrics "github.com/netscope/hub-api/metrics"
	"github.com/netscope/hub-api/models"

	chclient "github.com/netscope/hub-api/clickhouse"
)

// Evaluator runs all enabled alert rules on a fixed schedule and fires
// webhooks when thresholds are breached.
type Evaluator struct {
	ch       *chclient.Client
	interval time.Duration
	stopCh   chan struct{}
	SMTP     SMTPConfig
}

// NewEvaluator creates an Evaluator that checks rules every interval.
func NewEvaluator(ch *chclient.Client, interval time.Duration) *Evaluator {
	return &Evaluator{
		ch:       ch,
		interval: interval,
		stopCh:   make(chan struct{}),
	}
}

// Start launches the evaluation loop in a background goroutine.
func (e *Evaluator) Start() {
	go e.run()
}

// Stop signals the evaluation loop to exit.
func (e *Evaluator) Stop() {
	close(e.stopCh)
}

func (e *Evaluator) run() {
	ticker := time.NewTicker(e.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			e.evaluateAll()
		case <-e.stopCh:
			return
		}
	}
}

func (e *Evaluator) evaluateAll() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rows, err := e.ch.Query(ctx,
		`SELECT id, name, metric, condition, threshold, window_minutes,
		        integration_type, webhook_url, webhook_secret, email_to, cooldown_minutes
		 FROM alert_rules
		 WHERE enabled = 1`)
	if err != nil {
		slog.Warn("alert evaluator: fetch rules", "err", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var r models.AlertRule
		if err := rows.Scan(
			&r.ID, &r.Name, &r.Metric, &r.Condition,
			&r.Threshold, &r.WindowMinutes,
			&r.IntegrationType, &r.WebhookURL, &r.WebhookSecret, &r.EmailTo, &r.CooldownMinutes,
		); err != nil {
			slog.Warn("alert evaluator: scan rule", "err", err)
			continue
		}
		r.Enabled = true
		e.evaluate(ctx, r)
	}
}

func (e *Evaluator) evaluate(ctx context.Context, rule models.AlertRule) {
	value, err := e.computeMetric(ctx, rule.Metric, rule.WindowMinutes)
	if err != nil {
		slog.Warn("alert evaluator: compute metric",
			"rule", rule.Name, "metric", rule.Metric, "err", err)
		return
	}

	if !matchesCondition(value, rule.Condition, rule.Threshold) {
		return
	}

	// Check cooldown: skip if already fired within CooldownMinutes
	if e.inCooldown(ctx, rule.ID, rule.CooldownMinutes) {
		return
	}

	slog.Info("alert firing",
		"rule", rule.Name, "metric", rule.Metric,
		"value", value, "threshold", rule.Threshold)

	payload := models.WebhookPayload{
		AlertID:   uuid.New().String(),
		RuleName:  rule.Name,
		Metric:    rule.Metric,
		Value:     value,
		Threshold: rule.Threshold,
		Condition: rule.Condition,
		FiredAt:   time.Now().UTC(),
		Message:   BuildMessage(rule, value),
	}

	delivered := FireAlert(ctx, rule, payload, e.SMTP)

	if delivered {
		nsmetrics.AlertsFiredTotal.Add(1)
	}

	// Record the event regardless of delivery status
	e.recordEvent(ctx, rule, value, payload.AlertID, delivered)
}

// computeMetric evaluates a named metric over the given look-back window.
func (e *Evaluator) computeMetric(ctx context.Context, metric string, windowMinutes uint32) (float64, error) {
	window := fmt.Sprintf("%d", windowMinutes)

	switch metric {
	case "flows_per_minute":
		rows, err := e.ch.Query(ctx,
			fmt.Sprintf(`SELECT count() / %s FROM flows
			             WHERE ts > now() - INTERVAL %s MINUTE`, window, window))
		if err != nil {
			return 0, err
		}
		defer rows.Close()
		var v float64
		if rows.Next() {
			if scanErr := rows.Scan(&v); scanErr != nil {
				return 0, scanErr
			}
		}
		return v, nil

	case "http_error_rate":
		rows, err := e.ch.Query(ctx,
			fmt.Sprintf(`SELECT
			               countIf(http_status >= 400) * 100.0 / greatest(count(), 1)
			             FROM flows
			             WHERE ts > now() - INTERVAL %s MINUTE
			               AND protocol = 'HTTP'`, window))
		if err != nil {
			return 0, err
		}
		defer rows.Close()
		var v float64
		if rows.Next() {
			if scanErr := rows.Scan(&v); scanErr != nil {
				return 0, scanErr
			}
		}
		return v, nil

	case "dns_nxdomain_rate":
		rows, err := e.ch.Query(ctx,
			fmt.Sprintf(`SELECT
			               countIf(dns_type = 'NXDOMAIN') * 100.0 / greatest(count(), 1)
			             FROM flows
			             WHERE ts > now() - INTERVAL %s MINUTE
			               AND protocol = 'DNS'`, window))
		if err != nil {
			return 0, err
		}
		defer rows.Close()
		var v float64
		if rows.Next() {
			if scanErr := rows.Scan(&v); scanErr != nil {
				return 0, scanErr
			}
		}
		return v, nil

	// ── Anomaly metrics ──────────────────────────────────────────────────────
	// For anomaly metrics, threshold = sigma multiplier (e.g. 2.0).
	// computeMetric returns the deviation in sigmas; alert fires when > threshold.

	case "anomaly_flow_rate":
		// Compute 7-day per-hour baseline (same hour-of-day, all days)
		baseRows, err := e.ch.Query(ctx, `
			SELECT avg(cnt) AS baseline_avg, stddevPop(cnt) AS baseline_std
			FROM (
				SELECT toStartOfHour(ts) AS h, count() AS cnt
				FROM flows
				WHERE ts >= now() - INTERVAL 7 DAY
				  AND ts  < now() - INTERVAL 1 HOUR
				GROUP BY h
			)`)
		if err != nil {
			return 0, err
		}
		var bAvg, bStd float64
		if baseRows.Next() {
			if scanErr := baseRows.Scan(&bAvg, &bStd); scanErr != nil {
				baseRows.Close()
				return 0, scanErr
			}
		}
		baseRows.Close()

		if bStd < 1 {
			return 0, nil // not enough data to establish baseline
		}

		// Current rate: extrapolate last-5-min count to per-hour
		currRows, err := e.ch.Query(ctx,
			`SELECT count() * 12.0 FROM flows WHERE ts >= now() - INTERVAL 5 MINUTE`)
		if err != nil {
			return 0, err
		}
		var curr float64
		if currRows.Next() {
			if scanErr := currRows.Scan(&curr); scanErr != nil {
				currRows.Close()
				return 0, scanErr
			}
		}
		currRows.Close()

		return (curr - bAvg) / bStd, nil // return sigma deviation

	case "anomaly_http_latency":
		baseRows, err := e.ch.Query(ctx, `
			SELECT avg(avg_lat) AS baseline_avg, stddevPop(avg_lat) AS baseline_std
			FROM (
				SELECT toStartOfHour(ts) AS h, avg(duration_ms) AS avg_lat
				FROM flows
				WHERE ts >= now() - INTERVAL 7 DAY
				  AND ts  < now() - INTERVAL 1 HOUR
				  AND protocol IN ('HTTP', 'HTTPS')
				  AND http_method != ''
				GROUP BY h
			)`)
		if err != nil {
			return 0, err
		}
		var bAvg, bStd float64
		if baseRows.Next() {
			if scanErr := baseRows.Scan(&bAvg, &bStd); scanErr != nil {
				baseRows.Close()
				return 0, scanErr
			}
		}
		baseRows.Close()

		if bStd < 1 {
			return 0, nil
		}

		currRows, err := e.ch.Query(ctx,
			`SELECT avg(duration_ms) FROM flows
			 WHERE ts >= now() - INTERVAL 5 MINUTE
			   AND protocol IN ('HTTP', 'HTTPS')
			   AND http_method != ''`)
		if err != nil {
			return 0, err
		}
		var curr float64
		if currRows.Next() {
			if scanErr := currRows.Scan(&curr); scanErr != nil {
				currRows.Close()
				return 0, scanErr
			}
		}
		currRows.Close()

		return (curr - bAvg) / bStd, nil

	default:
		return 0, fmt.Errorf("unknown metric: %s", metric)
	}
}

func (e *Evaluator) inCooldown(ctx context.Context, ruleID string, cooldownMinutes uint32) bool {
	rows, err := e.ch.Query(ctx,
		fmt.Sprintf(`SELECT count() FROM alert_events
		             WHERE rule_id = ?
		               AND fired_at > now() - INTERVAL %d MINUTE`, cooldownMinutes),
		ruleID)
	if err != nil {
		return false
	}
	defer rows.Close()
	var cnt uint64
	if rows.Next() {
		if err := rows.Scan(&cnt); err != nil {
			slog.Warn("alert evaluator: scan cooldown count", "err", err)
			return false
		}
	}
	return cnt > 0
}

func (e *Evaluator) recordEvent(ctx context.Context, rule models.AlertRule, value float64, eventID string, delivered bool) {
	var deliveredInt uint8
	if delivered {
		deliveredInt = 1
	}
	if err := e.ch.Exec(ctx,
		`INSERT INTO alert_events (id, rule_id, rule_name, metric, value, threshold, fired_at, delivered)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		eventID, rule.ID, rule.Name, rule.Metric,
		value, rule.Threshold, time.Now().UTC(), deliveredInt,
	); err != nil {
		slog.Warn("alert evaluator: record event", "err", err)
	}
}

func matchesCondition(value float64, condition string, threshold float64) bool {
	switch condition {
	case "gt":
		return value > threshold
	case "lt":
		return value < threshold
	default:
		return false
	}
}
