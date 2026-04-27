// Package sigma implements a detection rule engine that evaluates ClickHouse queries
// against flow data on a configurable schedule and records matches.
//
// # Rule format
//
// Rules are stored in the sigma_rules ClickHouse table.  The detection logic is a
// ClickHouse SQL SELECT that returns rows when a threat is detected.  Each returned
// row is serialised to JSON and recorded as a sigma_match.
//
// Example query that detects port scanning (>50 distinct destination ports from one
// source IP in the last 5 minutes):
//
//	SELECT src_ip, count(DISTINCT dst_port) AS port_count
//	FROM flows
//	WHERE ts > now() - INTERVAL 5 MINUTE
//	GROUP BY src_ip
//	HAVING port_count > 50
//
// # Tiers
//
// Community: 5 read-only built-in rules (cannot be edited or deleted).
// Enterprise: unlimited custom rules via the CRUD API.
package sigma

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
)

// Rule represents one detection rule stored in sigma_rules.
type Rule struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Severity    string    `json:"severity"` // low | medium | high | critical
	Tags        []string  `json:"tags"`
	Query       string    `json:"query"`
	Enabled     bool      `json:"enabled"`
	Builtin     bool      `json:"builtin"` // true = Community read-only rule
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Match is a single rule firing event.
type Match struct {
	ID        string    `json:"id"`
	RuleID    string    `json:"rule_id"`
	RuleTitle string    `json:"rule_title"`
	Severity  string    `json:"severity"`
	MatchData string    `json:"match_data"` // JSON from first result row
	FiredAt   time.Time `json:"fired_at"`
}

// Engine polls sigma_rules and executes each enabled rule's query on a schedule.
type Engine struct {
	ch       *clickhouse.Client
	interval time.Duration

	mu      sync.Mutex
	ticker  *time.Ticker
	done    chan struct{}
}

// New returns a new Engine.  It does not start the background goroutine.
func New(ch *clickhouse.Client) *Engine {
	return &Engine{
		ch:       ch,
		interval: 5 * time.Minute,
		done:     make(chan struct{}),
	}
}

// Start launches the background evaluation goroutine.
func (e *Engine) Start() {
	if e.ch == nil {
		slog.Warn("sigma: ClickHouse unavailable — rule evaluation disabled")
		return
	}
	e.mu.Lock()
	e.ticker = time.NewTicker(e.interval)
	e.mu.Unlock()

	go func() {
		slog.Info("sigma engine started", "interval", e.interval)
		// Run immediately on startup to populate matches quickly.
		e.evaluate()
		for {
			select {
			case <-e.ticker.C:
				e.evaluate()
			case <-e.done:
				return
			}
		}
	}()
}

// Stop shuts down the background goroutine.
func (e *Engine) Stop() {
	e.mu.Lock()
	if e.ticker != nil {
		e.ticker.Stop()
	}
	e.mu.Unlock()
	close(e.done)
	slog.Info("sigma engine stopped")
}

// evaluate loads all enabled rules and runs each one.
func (e *Engine) evaluate() {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()

	rules, err := e.loadRules(ctx)
	if err != nil {
		slog.Warn("sigma: failed to load rules", "err", err)
		return
	}

	for _, r := range rules {
		if err := e.runRule(ctx, r); err != nil {
			slog.Warn("sigma: rule evaluation failed", "rule", r.ID, "title", r.Title, "err", err)
		}
	}
}

// loadRules fetches all enabled rules from ClickHouse.
func (e *Engine) loadRules(ctx context.Context) ([]Rule, error) {
	rows, err := e.ch.Query(ctx,
		`SELECT id, title, description, severity, tags, query, enabled, builtin, created_at, updated_at
		 FROM sigma_rules FINAL
		 WHERE enabled = 1
		 ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rules []Rule
	for rows.Next() {
		var r Rule
		var tagsJSON string
		var enabledU, builtinU uint8
		if err := rows.Scan(
			&r.ID, &r.Title, &r.Description, &r.Severity,
			&tagsJSON, &r.Query,
			&enabledU, &builtinU,
			&r.CreatedAt, &r.UpdatedAt,
		); err != nil {
			slog.Warn("sigma: scan rule row", "err", err)
			continue
		}
		r.Enabled = enabledU == 1
		r.Builtin = builtinU == 1
		_ = json.Unmarshal([]byte(tagsJSON), &r.Tags)
		rules = append(rules, r)
	}
	return rules, nil
}

// runRule executes a single rule's ClickHouse query and records any matches.
func (e *Engine) runRule(ctx context.Context, r Rule) error {
	// Basic safety: only allow SELECT queries.
	trimmed := strings.TrimSpace(strings.ToUpper(r.Query))
	if !strings.HasPrefix(trimmed, "SELECT") {
		slog.Warn("sigma: non-SELECT query rejected", "rule", r.ID)
		return fmt.Errorf("rule %s: only SELECT queries are allowed", r.ID)
	}

	rows, err := e.ch.Query(ctx, r.Query)
	if err != nil {
		return fmt.Errorf("run query: %w", err)
	}
	defer rows.Close()

	cols := rows.Columns()
	now := time.Now()
	matchCount := 0

	for rows.Next() {
		// Scan into interface{} slice then marshal to JSON.
		vals := make([]interface{}, len(cols))
		ptrs := make([]interface{}, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			continue
		}
		row := make(map[string]interface{}, len(cols))
		for i, col := range cols {
			row[col] = vals[i]
		}
		data, _ := json.Marshal(row)

		matchID := uuid.NewString()
		if err := e.ch.Exec(ctx,
			`INSERT INTO sigma_matches (id, rule_id, rule_title, severity, match_data, fired_at)
			 VALUES (?, ?, ?, ?, ?, ?)`,
			matchID, r.ID, r.Title, r.Severity, string(data), now,
		); err != nil {
			slog.Warn("sigma: insert match failed", "rule", r.ID, "err", err)
		}
		matchCount++
	}

	if matchCount > 0 {
		slog.Info("sigma rule fired", "rule", r.Title, "severity", r.Severity, "matches", matchCount)
	}
	return nil
}

// BuiltinRules returns the 5 built-in detection rules that are seeded for all plans.
// These are read-only on the Community plan.
func BuiltinRules() []Rule {
	return []Rule{
		{
			ID:          "builtin-001",
			Title:       "Port Scan Detection",
			Description: "Detects hosts probing more than 50 unique destination ports within a 5-minute window — a strong indicator of reconnaissance activity.",
			Severity:    "high",
			Tags:        []string{"recon", "portscan", "attack.discovery"},
			Enabled:     true,
			Builtin:     true,
			Query: `SELECT src_ip, count(DISTINCT dst_port) AS port_count, min(ts) AS first_seen
FROM flows
WHERE ts > now() - INTERVAL 5 MINUTE
  AND protocol IN ('TCP', 'UDP')
GROUP BY src_ip
HAVING port_count > 50`,
		},
		{
			ID:          "builtin-002",
			Title:       "DNS Tunneling Indicator",
			Description: "Identifies unusually long DNS query names (>60 chars) that may indicate DNS-based data exfiltration or C2 communication.",
			Severity:    "medium",
			Tags:        []string{"dns", "exfiltration", "attack.c2"},
			Enabled:     true,
			Builtin:     true,
			Query: `SELECT src_ip, hostname, dns_query, ts
FROM flows
WHERE ts > now() - INTERVAL 10 MINUTE
  AND protocol = 'DNS'
  AND length(dns_query) > 60`,
		},
		{
			ID:          "builtin-003",
			Title:       "Cleartext Credential Submission",
			Description: "Detects HTTP POST requests to common authentication paths over plain HTTP (not HTTPS), risking credential exposure.",
			Severity:    "high",
			Tags:        []string{"credentials", "http", "attack.credential_access"},
			Enabled:     true,
			Builtin:     true,
			Query: `SELECT src_ip, dst_ip, dst_port, http_path, hostname, ts
FROM flows
WHERE ts > now() - INTERVAL 15 MINUTE
  AND protocol = 'HTTP'
  AND http_method = 'POST'
  AND (http_path LIKE '%/login%' OR http_path LIKE '%/auth%' OR http_path LIKE '%/signin%' OR http_path LIKE '%/password%')
  AND dst_port != 443`,
		},
		{
			ID:          "builtin-004",
			Title:       "Unexpected Outbound High Port",
			Description: "Flags connections to destination ports above 49151 (ephemeral range) that may indicate beaconing, reverse shells, or non-standard services.",
			Severity:    "medium",
			Tags:        []string{"c2", "beaconing", "attack.command_and_control"},
			Enabled:     true,
			Builtin:     true,
			Query: `SELECT src_ip, dst_ip, dst_port, protocol, bytes_out, hostname, ts
FROM flows
WHERE ts > now() - INTERVAL 5 MINUTE
  AND dst_port > 49151
  AND protocol = 'TCP'
  AND bytes_out > 10000`,
		},
		{
			ID:          "builtin-005",
			Title:       "Privileged Process Network Activity",
			Description: "Detects shell or interpreter processes (bash, sh, python, powershell) making outbound network connections — a common sign of post-exploitation.",
			Severity:    "critical",
			Tags:        []string{"process", "shell", "attack.execution", "attack.lateral_movement"},
			Enabled:     true,
			Builtin:     true,
			Query: `SELECT process_name, pid, src_ip, dst_ip, dst_port, hostname, ts
FROM flows
WHERE ts > now() - INTERVAL 5 MINUTE
  AND protocol = 'TCP'
  AND process_name IN ('bash', 'sh', 'zsh', 'python', 'python3', 'powershell', 'pwsh', 'cmd', 'perl', 'ruby')
  AND dst_port NOT IN (22, 80, 443)`,
		},
	}
}
