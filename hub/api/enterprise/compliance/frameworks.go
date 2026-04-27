// Package compliance implements SOC 2, PCI-DSS, and HIPAA compliance report
// generation for NetScope Enterprise.
//
// Each framework is defined as a bundle of named ClickHouse queries that
// produce the evidence rows included in the report. The renderer in renderer.go
// iterates the bundle and formats the results as PDF or CSV.
package compliance

import (
	"context"
	"fmt"
	"time"

	"github.com/netscope/hub-api/clickhouse"
)

// Framework names.
const (
	FrameworkSOC2   = "soc2"
	FrameworkPCI    = "pci-dss"
	FrameworkHIPAA  = "hipaa"
)

// CheckResult is the result of one compliance check query.
type CheckResult struct {
	CheckName   string     `json:"check"`
	Description string     `json:"description"`
	Status      string     `json:"status"` // "pass" | "warn" | "fail" | "info"
	RowCount    int        `json:"row_count"`
	Rows        [][]string `json:"rows,omitempty"` // column values as strings
	Columns     []string   `json:"columns"`
}

// ReportData is the full set of check results for one framework.
type ReportData struct {
	Framework   string        `json:"framework"`
	GeneratedAt time.Time     `json:"generated_at"`
	Period      string        `json:"period"`   // "last 30 days" etc.
	Checks      []CheckResult `json:"checks"`
}

// RunFramework executes all checks for the named framework and returns a
// ReportData ready for rendering.
func RunFramework(ctx context.Context, ch *clickhouse.Client, framework string, since time.Time) (*ReportData, error) {
	var bundle []checkDef

	switch framework {
	case FrameworkSOC2:
		bundle = soc2Checks
	case FrameworkPCI:
		bundle = pciChecks
	case FrameworkHIPAA:
		bundle = hipaaChecks
	default:
		return nil, fmt.Errorf("unknown framework: %s", framework)
	}

	data := &ReportData{
		Framework:   framework,
		GeneratedAt: time.Now().UTC(),
		Period:      fmt.Sprintf("since %s", since.Format("2006-01-02")),
	}

	for _, check := range bundle {
		result, err := runCheck(ctx, ch, check, since)
		if err != nil {
			result = &CheckResult{
				CheckName:   check.name,
				Description: check.description,
				Status:      "fail",
				RowCount:    0,
			}
		}
		data.Checks = append(data.Checks, *result)
	}
	return data, nil
}

// ── check definition ─────────────────────────────────────────────────────────

type checkDef struct {
	name        string
	description string
	// query returns evidence rows. It must accept (since DateTime64) as $1.
	// A zero result set means "pass"; non-zero is "warn" or "fail" depending on
	// the failOnRows flag.
	query      string
	columns    []string
	failOnRows bool   // if true, having rows means FAIL; otherwise WARN
	passOnRows bool   // if true, having rows means PASS (e.g. "active agents found")
	infoOnly   bool   // result is always "info" regardless of row count
}

func runCheck(ctx context.Context, ch *clickhouse.Client, def checkDef, since time.Time) (*CheckResult, error) {
	rows, err := ch.Query(ctx, def.query, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cols := rows.Columns()
	var resultRows [][]string
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			continue
		}
		row := make([]string, len(cols))
		for i, v := range vals {
			row[i] = fmt.Sprintf("%v", v)
		}
		resultRows = append(resultRows, row)
	}

	status := "pass"
	switch {
	case def.infoOnly:
		status = "info"
	case def.failOnRows && len(resultRows) > 0:
		status = "fail"
	case def.passOnRows && len(resultRows) == 0:
		status = "warn" // expected to have rows — none found is a problem
	case !def.failOnRows && !def.passOnRows && len(resultRows) > 0:
		status = "warn"
	}

	return &CheckResult{
		CheckName:   def.name,
		Description: def.description,
		Status:      status,
		RowCount:    len(resultRows),
		Rows:        resultRows,
		Columns:     def.columns,
	}, nil
}

// ── SOC 2 checks ──────────────────────────────────────────────────────────────

var soc2Checks = []checkDef{
	{
		name:        "CC6.1 — Logical Access Controls",
		description: "Identifies flows from unauthorised source IPs hitting internal admin ports (22, 3389, 5432, 6379, 27017).",
		query: `SELECT src_ip, dst_ip, dst_port, count() AS attempts, max(ts) AS last_seen
			FROM flows
			WHERE ts >= ? AND dst_port IN (22, 3389, 5432, 6379, 27017)
			  AND src_ip NOT LIKE '10.%' AND src_ip NOT LIKE '172.16.%' AND src_ip NOT LIKE '192.168.%'
			GROUP BY src_ip, dst_ip, dst_port
			ORDER BY attempts DESC LIMIT 100`,
		columns:    []string{"src_ip", "dst_ip", "dst_port", "attempts", "last_seen"},
		failOnRows: true,
	},
	{
		name:        "CC6.7 — Transmission Integrity",
		description: "Counts unencrypted HTTP flows carrying data — potential cleartext PII transmission.",
		query: `SELECT dst_ip, dst_port, count() AS flow_count, sum(bytes_out) AS total_bytes
			FROM flows
			WHERE ts >= ? AND protocol = 'HTTP' AND dst_port NOT IN (443,8443)
			  AND bytes_out > 512
			GROUP BY dst_ip, dst_port ORDER BY total_bytes DESC LIMIT 50`,
		columns: []string{"dst_ip", "dst_port", "flow_count", "total_bytes"},
	},
	{
		name:        "CC7.2 — Anomalous Activity Monitoring",
		description: "Lists all Sigma rule matches fired in the period.",
		query: `SELECT r.title, m.src_ip, m.dst_ip, m.severity, m.fired_at
			FROM sigma_matches m
			JOIN sigma_rules r ON r.id = m.rule_id
			WHERE m.fired_at >= ?
			ORDER BY m.fired_at DESC LIMIT 200`,
		columns:  []string{"rule", "src_ip", "dst_ip", "severity", "fired_at"},
		infoOnly: true,
	},
	{
		name:        "CC8.1 — Change Management (Agent Registrations)",
		description: "Lists all agent registrations and heartbeat gaps (offline > 1h) as change-management evidence.",
		query: `SELECT agent_id, hostname, version, registered_at, last_seen,
			       if(last_seen < now() - INTERVAL 1 HOUR, 'OFFLINE', 'ONLINE') AS status
			FROM agents FINAL
			WHERE registered_at >= ? OR last_seen >= ?
			ORDER BY registered_at DESC`,
		columns:  []string{"agent_id", "hostname", "version", "registered_at", "last_seen", "status"},
		infoOnly: true,
	},
	{
		name:        "A1.1 — Availability Monitoring",
		description: "Counts active agents and total flows in the period as availability evidence.",
		query: `SELECT count(DISTINCT agent_id) AS active_agents,
			       count() AS total_flows,
			       min(ts) AS first_flow, max(ts) AS last_flow
			FROM flows WHERE ts >= ?`,
		columns:    []string{"active_agents", "total_flows", "first_flow", "last_flow"},
		passOnRows: true,
		infoOnly:   true,
	},
}

// ── PCI-DSS checks ────────────────────────────────────────────────────────────

var pciChecks = []checkDef{
	{
		name:        "Req 1.3 — Prohibited Inbound Traffic",
		description: "Detects inbound flows to cardholder-environment ports from external IPs.",
		query: `SELECT src_ip, dst_ip, dst_port, protocol, count() AS cnt
			FROM flows
			WHERE ts >= ? AND src_ip NOT LIKE '10.%' AND src_ip NOT LIKE '172.16.%' AND src_ip NOT LIKE '192.168.%'
			  AND dst_port IN (443, 8443, 3306, 5432, 1433, 6379, 27017)
			GROUP BY src_ip, dst_ip, dst_port, protocol
			ORDER BY cnt DESC LIMIT 100`,
		columns:    []string{"src_ip", "dst_ip", "dst_port", "protocol", "count"},
		failOnRows: true,
	},
	{
		name:        "Req 2.2 — Default / Known Credentials",
		description: "Flags flows using standard service ports that may carry default credentials over cleartext.",
		query: `SELECT src_ip, dst_ip, dst_port, protocol, count() AS cnt
			FROM flows
			WHERE ts >= ? AND protocol IN ('TELNET','FTP','HTTP') AND dst_port IN (21, 23, 80, 8080)
			  AND dst_ip NOT LIKE '192.168.%'
			GROUP BY src_ip, dst_ip, dst_port, protocol
			ORDER BY cnt DESC LIMIT 50`,
		columns: []string{"src_ip", "dst_ip", "dst_port", "protocol", "count"},
	},
	{
		name:        "Req 10.2 — Audit Trail Completeness",
		description: "Verifies audit events were captured in the period.",
		query: `SELECT action, count() AS event_count, min(occurred_at) AS first, max(occurred_at) AS last
			FROM audit_events WHERE occurred_at >= ?
			GROUP BY action ORDER BY event_count DESC`,
		columns:    []string{"action", "event_count", "first", "last"},
		passOnRows: true,
		infoOnly:   true,
	},
	{
		name:        "Req 11.4 — Intrusion Detection",
		description: "Lists all port-scan and lateral-movement Sigma matches as IDS evidence.",
		query: `SELECT r.title, m.src_ip, count() AS fires, max(m.fired_at) AS last_fired
			FROM sigma_matches m JOIN sigma_rules r ON r.id = m.rule_id
			WHERE m.fired_at >= ? AND (r.tags LIKE '%portscan%' OR r.tags LIKE '%lateral%')
			GROUP BY r.title, m.src_ip ORDER BY fires DESC LIMIT 100`,
		columns:  []string{"rule", "src_ip", "fires", "last_fired"},
		infoOnly: true,
	},
}

// ── HIPAA checks ──────────────────────────────────────────────────────────────

var hipaaChecks = []checkDef{
	{
		name:        "§164.312(e)(1) — Transmission Security",
		description: "Identifies unencrypted flows that may carry ePHI — HTTP on non-standard ports.",
		query: `SELECT src_ip, dst_ip, dst_port, count() AS cnt, sum(bytes_out) AS bytes
			FROM flows
			WHERE ts >= ? AND protocol = 'HTTP' AND dst_port NOT IN (443, 8443)
			GROUP BY src_ip, dst_ip, dst_port
			ORDER BY bytes DESC LIMIT 100`,
		columns: []string{"src_ip", "dst_ip", "dst_port", "count", "bytes"},
	},
	{
		name:        "§164.312(b) — Audit Controls",
		description: "Confirms user-access and authentication audit events were captured.",
		query: `SELECT action, actor_email, count() AS cnt, max(occurred_at) AS last
			FROM audit_events WHERE occurred_at >= ? AND action IN ('login','logout','invite_accepted','password_reset')
			GROUP BY action, actor_email ORDER BY cnt DESC LIMIT 200`,
		columns:    []string{"action", "actor_email", "count", "last"},
		passOnRows: true,
		infoOnly:   true,
	},
	{
		name:        "§164.308(a)(1) — Workforce Access Review",
		description: "Lists all active org members and their roles for workforce authorisation review.",
		query: `SELECT email, role, joined_at, last_seen
			FROM org_members FINAL
			WHERE org_id = 'default' AND status = 'active' AND joined_at >= ?
			ORDER BY joined_at DESC`,
		columns:  []string{"email", "role", "joined_at", "last_seen"},
		infoOnly: true,
	},
	{
		name:        "§164.312(a)(1) — Unique User ID / Access",
		description: "Flags accounts with no recent login (> 90 days) — potential orphan accounts.",
		query: `SELECT email, role, last_seen
			FROM org_members FINAL
			WHERE org_id = 'default' AND status = 'active'
			  AND last_seen < now() - INTERVAL 90 DAY
			ORDER BY last_seen ASC LIMIT 50`,
		columns: []string{"email", "role", "last_seen"},
	},
}
