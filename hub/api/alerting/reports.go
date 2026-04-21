package alerting

import (
	"context"
	"fmt"
	"log/slog"
	"net/smtp"
	"strings"
	"time"

	chclient "github.com/netscope/hub-api/clickhouse"
)

// ReportConfig holds the schedule and delivery settings for periodic reports.
type ReportConfig struct {
	SMTP     SMTPConfig
	Email    string // recipient address; empty disables reports
	Schedule string // "daily" or "weekly"
}

// Reporter sends daily or weekly network summaries via SMTP.
type Reporter struct {
	ch     *chclient.Client
	cfg    ReportConfig
	stopCh chan struct{}
}

// NewReporter creates a Reporter. Call Start() to begin the schedule.
func NewReporter(ch *chclient.Client, cfg ReportConfig) *Reporter {
	return &Reporter{ch: ch, cfg: cfg, stopCh: make(chan struct{})}
}

func (r *Reporter) Start() { go r.run() }
func (r *Reporter) Stop()  { close(r.stopCh) }

func (r *Reporter) run() {
	period := 24 * time.Hour
	if r.cfg.Schedule == "weekly" {
		period = 7 * 24 * time.Hour
	}

	// Fire at 8:00 AM local time, then every period.
	now := time.Now()
	next := time.Date(now.Year(), now.Month(), now.Day(), 8, 0, 0, 0, now.Location())
	if !next.After(now) {
		next = next.Add(period)
	}
	timer := time.NewTimer(time.Until(next))
	defer timer.Stop()

	for {
		select {
		case <-timer.C:
			r.sendReport()
			timer.Reset(period)
		case <-r.stopCh:
			return
		}
	}
}

// ── Report data model ─────────────────────────────────────────────────────────

type reportData struct {
	TotalFlows   uint64
	ActiveAgents uint64
	AlertEvents  uint64
	TopTalkers   []talkerRow
	TopProtos    []protoRow
	Period       string
}

type talkerRow struct{ IP string; Flows, Bytes uint64 }
type protoRow struct{ Protocol string; Count uint64 }

// ── Data gathering ────────────────────────────────────────────────────────────

func (r *Reporter) gatherData(ctx context.Context) (reportData, error) {
	var d reportData
	period := "1 DAY"
	d.Period = "24 hours"
	if r.cfg.Schedule == "weekly" {
		period = "7 DAY"
		d.Period = "7 days"
	}

	scan1 := func(q string, dest ...any) {
		rows, err := r.ch.Query(ctx, q)
		if err != nil {
			return
		}
		defer rows.Close()
		if rows.Next() {
			_ = rows.Scan(dest...)
		}
	}

	scan1(fmt.Sprintf(`SELECT count() FROM flows WHERE ts > now() - INTERVAL %s`, period), &d.TotalFlows)
	scan1(`SELECT count(DISTINCT agent_id) FROM flows WHERE ts > now() - INTERVAL 5 MINUTE`, &d.ActiveAgents)
	scan1(fmt.Sprintf(`SELECT count() FROM alert_events WHERE fired_at > now() - INTERVAL %s`, period), &d.AlertEvents)

	if rows, err := r.ch.Query(ctx, fmt.Sprintf(
		`SELECT src_ip, count() AS flows, sum(bytes_in+bytes_out) AS bytes
		 FROM flows WHERE ts > now() - INTERVAL %s
		 GROUP BY src_ip ORDER BY bytes DESC LIMIT 5`, period,
	)); err == nil {
		for rows.Next() {
			var t talkerRow
			if rows.Scan(&t.IP, &t.Flows, &t.Bytes) == nil {
				d.TopTalkers = append(d.TopTalkers, t)
			}
		}
		rows.Close()
	}

	if rows, err := r.ch.Query(ctx, fmt.Sprintf(
		`SELECT protocol, count() FROM flows WHERE ts > now() - INTERVAL %s
		 GROUP BY protocol ORDER BY count() DESC LIMIT 6`, period,
	)); err == nil {
		for rows.Next() {
			var p protoRow
			if rows.Scan(&p.Protocol, &p.Count) == nil {
				d.TopProtos = append(d.TopProtos, p)
			}
		}
		rows.Close()
	}

	return d, nil
}

// ── Email delivery ────────────────────────────────────────────────────────────

func (r *Reporter) sendReport() {
	if r.cfg.Email == "" || r.cfg.SMTP.Host == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	data, err := r.gatherData(ctx)
	if err != nil {
		slog.Warn("reporter: gather data failed", "err", err)
		return
	}

	label := "Daily"
	if r.cfg.Schedule == "weekly" {
		label = "Weekly"
	}
	subject := fmt.Sprintf("[%s Alert] NetScope %s Report — %s",
		r.cfg.SMTP.OrgName, label, time.Now().Format("Jan 2, 2006"))

	body := r.buildHTML(data, label)
	msg := strings.Join([]string{
		"From: " + r.cfg.SMTP.From,
		"To: " + r.cfg.Email,
		"Subject: " + subject,
		"MIME-Version: 1.0",
		"Content-Type: text/html; charset=UTF-8",
		"",
		body,
	}, "\r\n")

	addr := fmt.Sprintf("%s:%d", r.cfg.SMTP.Host, r.cfg.SMTP.Port)
	var auth smtp.Auth
	if r.cfg.SMTP.User != "" {
		auth = smtp.PlainAuth("", r.cfg.SMTP.User, r.cfg.SMTP.Password, r.cfg.SMTP.Host)
	}
	if err := smtp.SendMail(addr, auth, r.cfg.SMTP.From, []string{r.cfg.Email}, []byte(msg)); err != nil {
		slog.Warn("reporter: send failed", "err", err)
		return
	}
	slog.Info("reporter: report sent", "to", r.cfg.Email, "schedule", r.cfg.Schedule)
}

// ── HTML template ─────────────────────────────────────────────────────────────

func (r *Reporter) buildHTML(d reportData, label string) string {
	talkersHTML := ""
	for i, t := range d.TopTalkers {
		bg := ""
		if i%2 == 0 {
			bg = "background:#0f172a;"
		}
		talkersHTML += fmt.Sprintf(
			`<tr style="%s"><td style="padding:8px 12px;font-family:monospace;font-size:12px;color:#94a3b8">%s</td>`+
				`<td style="padding:8px 12px;font-size:12px;color:#e2e8f0;text-align:right">%d</td>`+
				`<td style="padding:8px 12px;font-size:12px;color:#e2e8f0;text-align:right">%s</td></tr>`,
			bg, t.IP, t.Flows, fmtBytes(t.Bytes))
	}
	if talkersHTML == "" {
		talkersHTML = `<tr><td colspan="3" style="padding:12px;text-align:center;color:#64748b;font-size:12px">No data yet</td></tr>`
	}

	protosHTML := ""
	for _, p := range d.TopProtos {
		protosHTML += fmt.Sprintf(
			`<span style="display:inline-block;background:#1e293b;border:1px solid #334155;border-radius:4px;`+
				`padding:4px 10px;margin:3px;font-size:12px;color:#94a3b8">`+
				`<strong style="color:#e2e8f0">%s</strong>&nbsp;%d</span>`,
			p.Protocol, p.Count)
	}
	if protosHTML == "" {
		protosHTML = `<span style="font-size:12px;color:#64748b">No flows yet</span>`
	}

	alertColor := "#22c55e"
	alertBadge := `<span style="color:#22c55e">&#10003; No alerts fired</span>`
	if d.AlertEvents > 0 {
		alertColor = "#ef4444"
		alertBadge = fmt.Sprintf(`<span style="color:#ef4444">&#9888; %d alert(s) fired</span>`, d.AlertEvents)
	}

	return fmt.Sprintf(`<!DOCTYPE html>
<html><head><meta charset="UTF-8"></head>
<body style="font-family:sans-serif;background:#0d0d1a;color:#e2e8f0;margin:0;padding:24px">
<div style="max-width:600px;margin:0 auto">

  <div style="border-bottom:1px solid #1e293b;padding-bottom:16px;margin-bottom:24px;display:flex;align-items:center;gap:12px">
    <div style="background:#6366f1;width:38px;height:38px;border-radius:10px;display:flex;align-items:center;justify-content:center;font-size:18px">&#128225;</div>
    <div>
      <h1 style="margin:0;font-size:20px;color:#fff">NetScope %s Report</h1>
      <p style="margin:2px 0 0;font-size:13px;color:#64748b">%s &middot; %s</p>
    </div>
  </div>

  <div style="display:grid;grid-template-columns:1fr 1fr 1fr;gap:12px;margin-bottom:24px">
    <div style="background:#0f172a;border:1px solid #1e293b;border-radius:8px;padding:16px;text-align:center">
      <p style="margin:0;font-size:26px;font-weight:700;color:#6366f1">%d</p>
      <p style="margin:4px 0 0;font-size:10px;color:#64748b;text-transform:uppercase;letter-spacing:.05em">Total Flows</p>
    </div>
    <div style="background:#0f172a;border:1px solid #1e293b;border-radius:8px;padding:16px;text-align:center">
      <p style="margin:0;font-size:26px;font-weight:700;color:#22c55e">%d</p>
      <p style="margin:4px 0 0;font-size:10px;color:#64748b;text-transform:uppercase;letter-spacing:.05em">Active Agents</p>
    </div>
    <div style="background:#0f172a;border:1px solid #1e293b;border-radius:8px;padding:16px;text-align:center">
      <p style="margin:0;font-size:26px;font-weight:700;color:%s">%d</p>
      <p style="margin:4px 0 0;font-size:10px;color:#64748b;text-transform:uppercase;letter-spacing:.05em">Alerts Fired</p>
    </div>
  </div>

  <div style="background:#0f172a;border:1px solid #1e293b;border-radius:8px;margin-bottom:20px;overflow:hidden">
    <div style="padding:12px 16px;border-bottom:1px solid #1e293b">
      <p style="margin:0;font-size:13px;font-weight:600;color:#fff">Top Talkers</p>
      <p style="margin:2px 0 0;font-size:11px;color:#64748b">Highest-volume sources over %s</p>
    </div>
    <table style="width:100%%;border-collapse:collapse">
      <thead><tr>
        <th style="padding:8px 12px;text-align:left;font-size:10px;font-weight:600;text-transform:uppercase;color:#475569">Source IP</th>
        <th style="padding:8px 12px;text-align:right;font-size:10px;font-weight:600;text-transform:uppercase;color:#475569">Flows</th>
        <th style="padding:8px 12px;text-align:right;font-size:10px;font-weight:600;text-transform:uppercase;color:#475569">Bytes</th>
      </tr></thead>
      <tbody>%s</tbody>
    </table>
  </div>

  <div style="background:#0f172a;border:1px solid #1e293b;border-radius:8px;padding:16px;margin-bottom:20px">
    <p style="margin:0 0 10px;font-size:13px;font-weight:600;color:#fff">Protocol Mix</p>
    <div>%s</div>
  </div>

  <div style="background:#0f172a;border:1px solid #1e293b;border-radius:8px;padding:16px;margin-bottom:24px">
    <p style="margin:0 0 4px;font-size:13px;font-weight:600;color:#fff">Alert Summary</p>
    <p style="margin:0;font-size:14px">%s</p>
  </div>

  <div style="text-align:center;margin-bottom:24px">
    <a href="%s" style="display:inline-block;background:#6366f1;color:#fff;text-decoration:none;
       padding:12px 28px;border-radius:8px;font-size:14px;font-weight:600">Open Hub Dashboard &#8594;</a>
  </div>

  <p style="margin:0;font-size:11px;color:#475569;text-align:center">
    Sent by %s &middot; <a href="%s/settings" style="color:#6366f1">Manage reports</a>
  </p>
</div>
</body></html>`,
		label,
		r.cfg.SMTP.OrgName, time.Now().Format("Jan 2, 2006"),
		d.TotalFlows,
		d.ActiveAgents,
		alertColor, d.AlertEvents,
		d.Period,
		talkersHTML,
		protosHTML,
		alertBadge,
		r.cfg.SMTP.AppURL,
		r.cfg.SMTP.OrgName, r.cfg.SMTP.AppURL,
	)
}

func fmtBytes(b uint64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)
	switch {
	case b >= GB:
		return fmt.Sprintf("%.1f GB", float64(b)/GB)
	case b >= MB:
		return fmt.Sprintf("%.1f MB", float64(b)/MB)
	case b >= KB:
		return fmt.Sprintf("%.1f KB", float64(b)/KB)
	default:
		return fmt.Sprintf("%d B", b)
	}
}
