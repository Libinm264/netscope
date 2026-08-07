package compliance

import (
	"context"
	"testing"
	"time"

	"github.com/klyzar/hub-api/enterprise/license"
	"github.com/klyzar/hub-api/testutil"
)

// TestScheduler_FiresDueSchedule_RecordsRun is REMAINING_WORK.md's V3:
// "Compliance report scheduler fires a PDF at the configured cron and stores
// a compliance_report_runs row".
//
// This is an internal (package compliance) test so it can call the
// unexported tick() synchronously instead of racing the Start() goroutine.
//
// NOTE: requires a real Docker daemon capable of pulling
// clickhouse/clickhouse-server:24.3-alpine. On JNJ/Zscaler-restricted
// machines this container pull is blocked — run this on an unrestricted
// machine (e.g. a MacBook) with:
//
//	go test ./enterprise/compliance/... -run TestScheduler_FiresDueSchedule -v
func TestScheduler_FiresDueSchedule_RecordsRun(t *testing.T) {
	ch := testutil.StartClickHouse(t)
	ctx := context.Background()

	scheduleID := "sched-soc2-daily-1"
	epoch := time.Unix(0, 0).UTC() // last_sent = epoch → "never run" → always due

	if err := ch.Exec(ctx,
		`INSERT INTO compliance_report_schedules
		 (id, name, framework, format, schedule, recipients, enabled, last_sent, created_at, version)
		 VALUES (?, 'Daily SOC2', 'soc2', 'pdf', 'daily', '[]', 1, ?, now64(), 1)`,
		scheduleID, epoch,
	); err != nil {
		t.Fatalf("insert schedule: %v", err)
	}

	// A little flow data so the report has something to summarise (not
	// strictly required for the run to succeed, but keeps this realistic).
	if err := ch.Exec(ctx,
		`INSERT INTO flows (agent_id, hostname, ts, protocol, src_ip, src_port, dst_ip, dst_port)
		 VALUES ('agent-1', 'web01', now64(), 'TCP', '10.0.0.5', 443, '203.0.113.9', 443)`,
	); err != nil {
		t.Fatalf("insert fixture flow: %v", err)
	}

	lic := &license.License{
		Valid: true,
		Plan:  license.PlanEnterprise, // Enterprise implicitly has all features
	}
	scheduler := New(ch, lic, nil) // nil SMTP — report generated, not emailed

	// Call tick() directly (synchronous, no goroutine race) — this loads
	// enabled schedules, finds ours "due" (never sent), and runs it.
	scheduler.tick()

	// Verify a compliance_report_runs row was recorded for our schedule.
	rows, err := ch.Query(ctx,
		`SELECT rows, error FROM compliance_report_runs
		 WHERE schedule_id = ? ORDER BY sent_at DESC LIMIT 1`,
		scheduleID,
	)
	if err != nil {
		t.Fatalf("query compliance_report_runs: %v", err)
	}
	defer rows.Close()

	if !rows.Next() {
		t.Fatal("expected a compliance_report_runs row for the fired schedule, found none")
	}
	var rowCount uint64
	var runErr string
	if err := rows.Scan(&rowCount, &runErr); err != nil {
		t.Fatalf("scan run row: %v", err)
	}
	if runErr != "" {
		t.Errorf("expected empty error on the run, got: %q", runErr)
	}

	// Verify last_sent was advanced on the schedule (so it won't re-fire
	// immediately on the next tick).
	schedRows, err := ch.Query(ctx,
		`SELECT last_sent FROM compliance_report_schedules
		 WHERE id = ? ORDER BY version DESC LIMIT 1`,
		scheduleID,
	)
	if err != nil {
		t.Fatalf("query schedule: %v", err)
	}
	defer schedRows.Close()
	if !schedRows.Next() {
		t.Fatal("expected schedule row to still exist")
	}
	var lastSent time.Time
	if err := schedRows.Scan(&lastSent); err != nil {
		t.Fatalf("scan last_sent: %v", err)
	}
	if !lastSent.After(epoch) {
		t.Errorf("expected last_sent to advance past epoch, got %v", lastSent)
	}
}
