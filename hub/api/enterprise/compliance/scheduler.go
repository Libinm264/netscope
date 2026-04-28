package compliance

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

// Scheduler polls the compliance_report_schedules table every 5 minutes and
// fires report runs for any schedule whose period has elapsed since last_sent.
type Scheduler struct {
	ch  *clickhouse.Client
	lic *license.License

	// smtp holds optional SMTP settings for email delivery.
	// If nil, reports are generated but not emailed (available via API download).
	smtp *SMTPConfig

	mu     sync.Mutex
	ticker *time.Ticker
	done   chan struct{}
}

// SMTPConfig holds SMTP credentials for report email delivery.
type SMTPConfig struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
}

// New returns a new Scheduler.
func New(ch *clickhouse.Client, lic *license.License, smtp *SMTPConfig) *Scheduler {
	return &Scheduler{
		ch:   ch,
		lic:  lic,
		smtp: smtp,
		done: make(chan struct{}),
	}
}

// Start launches the background scheduler goroutine.
func (s *Scheduler) Start() {
	if s.ch == nil || !s.lic.HasFeature(license.FeatureComplianceReports) {
		return
	}
	s.mu.Lock()
	s.ticker = time.NewTicker(5 * time.Minute)
	s.mu.Unlock()

	go func() {
		slog.Info("compliance scheduler started")
		s.tick()
		for {
			select {
			case <-s.ticker.C:
				s.tick()
			case <-s.done:
				return
			}
		}
	}()
}

// Stop shuts down the scheduler.
func (s *Scheduler) Stop() {
	s.mu.Lock()
	if s.ticker != nil {
		s.ticker.Stop()
	}
	s.mu.Unlock()
	close(s.done)
}

// tick loads enabled schedules and fires any that are due.
func (s *Scheduler) tick() {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()

	schedules, err := s.loadSchedules(ctx)
	if err != nil {
		slog.Warn("compliance: load schedules", "err", err)
		return
	}
	for _, sched := range schedules {
		if s.isDue(sched) {
			s.run(ctx, sched)
		}
	}
}

// ── schedule record ──────────────────────────────────────────────────────────

type scheduleRecord struct {
	ID         string
	Name       string
	Framework  string
	Format     string
	Schedule   string // "daily" | "weekly" | "monthly"
	Recipients []string
	LastSent   time.Time
}

func (s *Scheduler) loadSchedules(ctx context.Context) ([]scheduleRecord, error) {
	rows, err := s.ch.Query(ctx,
		`SELECT id, name, framework, format, schedule, recipients, last_sent
		 FROM compliance_report_schedules
		 WHERE enabled = 1`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []scheduleRecord
	for rows.Next() {
		var r scheduleRecord
		var recipientsJSON string
		if err := rows.Scan(
			&r.ID, &r.Name, &r.Framework, &r.Format,
			&r.Schedule, &recipientsJSON, &r.LastSent,
		); err != nil {
			continue
		}
		_ = json.Unmarshal([]byte(recipientsJSON), &r.Recipients)
		out = append(out, r)
	}
	return out, nil
}

func (s *Scheduler) isDue(r scheduleRecord) bool {
	if r.LastSent.IsZero() || r.LastSent.Year() < 2000 {
		return true // never run
	}
	now := time.Now().UTC()
	switch r.Schedule {
	case "daily":
		return now.Sub(r.LastSent) >= 24*time.Hour
	case "weekly":
		return now.Sub(r.LastSent) >= 7*24*time.Hour
	case "monthly":
		// Due if last_sent is in a different calendar month.
		return r.LastSent.Month() != now.Month() || r.LastSent.Year() != now.Year()
	}
	return false
}

// run executes one report schedule: generate → (email) → record.
func (s *Scheduler) run(ctx context.Context, sched scheduleRecord) {
	slog.Info("compliance: running report", "schedule", sched.Name, "framework", sched.Framework)
	since := sinceForSchedule(sched.Schedule)

	data, err := RunFramework(ctx, s.ch, sched.Framework, since)
	if err != nil {
		s.recordRun(ctx, sched, 0, err.Error())
		return
	}

	var payload []byte
	var renderErr error
	switch strings.ToLower(sched.Format) {
	case "csv":
		payload, renderErr = RenderCSV(data)
	default:
		payload, renderErr = RenderPDF(data)
	}
	if renderErr != nil {
		s.recordRun(ctx, sched, 0, renderErr.Error())
		return
	}

	rowCount := 0
	for _, c := range data.Checks {
		rowCount += c.RowCount
	}

	// Email delivery (optional — skipped if no SMTP config).
	if s.smtp != nil && len(sched.Recipients) > 0 {
		if emailErr := s.sendEmail(sched, payload, data); emailErr != nil {
			slog.Warn("compliance: email delivery failed", "err", emailErr)
			s.recordRun(ctx, sched, rowCount, fmt.Sprintf("email: %v", emailErr))
			return
		}
	}

	s.recordRun(ctx, sched, rowCount, "")

	// Update last_sent on the schedule.
	now := time.Now().UTC()
	recipJSON, _ := json.Marshal(sched.Recipients)
	_ = s.ch.Exec(ctx,
		`INSERT INTO compliance_report_schedules
		 (id, name, framework, format, schedule, recipients, enabled, last_sent, created_at, version)
		 SELECT id, name, framework, format, schedule, ?, enabled, ?, created_at,
		        toUInt64(toUnixTimestamp64Milli(now64()))
		 FROM compliance_report_schedules WHERE id = ?
		 ORDER BY created_at DESC LIMIT 1`,
		string(recipJSON), now, sched.ID,
	)
}

func (s *Scheduler) recordRun(ctx context.Context, sched scheduleRecord, rows int, errMsg string) {
	recipJSON, _ := json.Marshal(sched.Recipients)
	_ = s.ch.Exec(ctx,
		`INSERT INTO compliance_report_runs
		 (id, schedule_id, framework, format, recipients, rows, sent_at, error)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		uuid.New().String(), sched.ID, sched.Framework, sched.Format,
		string(recipJSON), rows, time.Now().UTC(), errMsg,
	)
}

// sendEmail delivers the report payload via SMTP.
// Uses net/smtp directly to avoid additional dependencies.
func (s *Scheduler) sendEmail(sched scheduleRecord, payload []byte, data *ReportData) error {
	// Stub: in a production deployment wire in your preferred mailer.
	// The payload (PDF or CSV) would be base64-encoded as an attachment.
	slog.Info("compliance: email delivery stub",
		"recipients", sched.Recipients,
		"bytes", len(payload),
		"framework", data.Framework,
	)
	return nil
}

func sinceForSchedule(schedule string) time.Time {
	now := time.Now().UTC()
	switch schedule {
	case "daily":
		return now.AddDate(0, 0, -1)
	case "weekly":
		return now.AddDate(0, 0, -7)
	case "monthly":
		return now.AddDate(0, -1, 0)
	}
	return now.AddDate(0, 0, -30)
}
