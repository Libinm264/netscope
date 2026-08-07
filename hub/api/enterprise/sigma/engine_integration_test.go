package sigma

import (
	"context"
	"testing"

	"github.com/klyzar/hub-api/enterprise/incidents"
	"github.com/klyzar/hub-api/enterprise/license"
	"github.com/klyzar/hub-api/testutil"
)

// dispatcherAdapter mirrors main.go's sigmaDispatcherAdapter — adapts
// incidents.Dispatcher to the sigma.Dispatcher interface so the test wires
// the two packages together exactly the way production does, without
// creating an import cycle (sigma → incidents is fine; incidents → sigma
// would not be).
type dispatcherAdapter struct {
	d *incidents.Dispatcher
}

func (a dispatcherAdapter) Dispatch(ctx context.Context, ev DispatchEvent) {
	a.d.Dispatch(ctx, incidents.SigmaMatchEvent{
		RuleID:    ev.RuleID,
		RuleTitle: ev.RuleTitle,
		Severity:  ev.Severity,
		SrcIP:     ev.SrcIP,
		DstIP:     ev.DstIP,
		FiredAt:   ev.FiredAt,
	})
}

// TestEngine_RuleMatch_DispatchesIncident is REMAINING_WORK.md's V4:
// "Sigma match triggers incidents.Dispatcher.Dispatch, creates an incident,
// and lands on the /incidents timeline".
//
// This is an internal (package sigma) test so it can call the unexported
// evaluate() synchronously instead of racing the Start() goroutine's
// 5-minute ticker.
//
// NOTE: requires a real Docker daemon capable of pulling
// clickhouse/clickhouse-server:24.3-alpine. On JNJ/Zscaler-restricted
// machines this container pull is blocked — run this on an unrestricted
// machine (e.g. a MacBook) with:
//
//	go test ./enterprise/sigma/... -run TestEngine_RuleMatch_DispatchesIncident -v
func TestEngine_RuleMatch_DispatchesIncident(t *testing.T) {
	ch := testutil.StartClickHouse(t)
	ctx := context.Background()

	const (
		ruleID  = "test-rule-fixture-1"
		srcIP   = "10.0.0.77"
		dstIP   = "203.0.113.50"
		dstPort = 6666
	)

	// A deterministic custom rule independent of the 5 seeded builtins —
	// fires whenever a fixture flow hits our fixture high port.
	if err := ch.Exec(ctx,
		`INSERT INTO sigma_rules
		 (id, title, description, severity, tags, query, enabled, builtin, created_at, updated_at, version)
		 VALUES (?, 'Fixture High Port Test', 'test fixture rule', 'high', '["test"]',
		         'SELECT src_ip, dst_ip FROM flows WHERE dst_port = 6666 AND protocol = ''TCP''',
		         1, 0, now64(), now64(), 1)`,
		ruleID,
	); err != nil {
		t.Fatalf("insert sigma rule: %v", err)
	}

	if err := ch.Exec(ctx,
		`INSERT INTO flows (agent_id, hostname, ts, protocol, src_ip, src_port, dst_ip, dst_port)
		 VALUES ('agent-1', 'web01', now64(), 'TCP', ?, 55555, ?, ?)`,
		srcIP, dstIP, uint16(dstPort),
	); err != nil {
		t.Fatalf("insert fixture flow: %v", err)
	}

	lic := &license.License{
		Valid: true,
		Plan:  license.PlanEnterprise, // Enterprise implicitly has FeatureIncidentWorkflow
	}
	dispatcher := incidents.New(ch, lic)

	engine := New(ch)
	engine.SetDispatcher(dispatcherAdapter{d: dispatcher})

	// Call evaluate() directly (synchronous) instead of Start()'s 5-minute
	// ticker goroutine.
	engine.evaluate()

	// 1. Verify sigma_matches recorded the fire.
	matchRows, err := ch.Query(ctx,
		`SELECT rule_title FROM sigma_matches WHERE rule_id = ? ORDER BY fired_at DESC LIMIT 1`,
		ruleID,
	)
	if err != nil {
		t.Fatalf("query sigma_matches: %v", err)
	}
	defer matchRows.Close()
	if !matchRows.Next() {
		t.Fatal("expected a sigma_matches row for the fired rule, found none")
	}

	// 2. Verify the incident dispatcher created an incident row.
	incRows, err := ch.Query(ctx,
		`SELECT title, severity, status, source FROM incidents
		 WHERE source = 'sigma' AND source_id = ? ORDER BY created_at DESC LIMIT 1`,
		ruleID,
	)
	if err != nil {
		t.Fatalf("query incidents: %v", err)
	}
	defer incRows.Close()
	if !incRows.Next() {
		t.Fatal("expected an incidents row created by the Sigma→incident dispatch, found none")
	}
	var title, severity, status, source string
	if err := incRows.Scan(&title, &severity, &status, &source); err != nil {
		t.Fatalf("scan incident row: %v", err)
	}
	if severity != "high" {
		t.Errorf("incident severity: want high, got %s", severity)
	}
	if status != "open" {
		t.Errorf("incident status: want open, got %s", status)
	}
	if source != "sigma" {
		t.Errorf("incident source: want sigma, got %s", source)
	}
}
