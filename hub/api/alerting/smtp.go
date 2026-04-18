package alerting

import (
	"fmt"
	"net/smtp"
	"strings"
	"time"

	"github.com/netscope/hub-api/models"
)

// SMTPConfig holds the SMTP connection details.
type SMTPConfig struct {
	Host     string
	Port     int
	User     string
	Password string
	From     string
	OrgName  string
	AppURL   string
}

// FireEmail sends an alert notification via SMTP using stdlib net/smtp.
// No external dependencies — uses PLAIN auth and STARTTLS-compatible addressing.
func FireEmail(cfg SMTPConfig, to string, payload models.WebhookPayload) bool {
	if cfg.Host == "" || to == "" {
		return false
	}

	subject := fmt.Sprintf("[%s Alert] %s", cfg.OrgName, payload.RuleName)
	body := buildEmailBody(cfg, payload)

	msg := strings.Join([]string{
		"From: " + cfg.From,
		"To: " + to,
		"Subject: " + subject,
		"MIME-Version: 1.0",
		"Content-Type: text/html; charset=UTF-8",
		"",
		body,
	}, "\r\n")

	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	var auth smtp.Auth
	if cfg.User != "" {
		auth = smtp.PlainAuth("", cfg.User, cfg.Password, cfg.Host)
	}

	if err := smtp.SendMail(addr, auth, cfg.From, []string{to}, []byte(msg)); err != nil {
		return false
	}
	return true
}

func buildEmailBody(cfg SMTPConfig, p models.WebhookPayload) string {
	condStr := "exceeded"
	if p.Condition == "lt" {
		condStr = "fell below"
	}
	color := "#ef4444"
	if p.Condition == "lt" {
		color = "#f59e0b"
	}

	return fmt.Sprintf(`<!DOCTYPE html>
<html><head><meta charset="UTF-8"></head>
<body style="font-family:sans-serif;background:#0d0d1a;color:#e2e8f0;margin:0;padding:24px">
  <div style="max-width:520px;margin:0 auto">
    <div style="background:%s;border-radius:6px;padding:3px 12px;display:inline-block;margin-bottom:16px">
      <span style="color:#fff;font-size:13px;font-weight:600">🚨 %s Alert</span>
    </div>
    <h2 style="margin:0 0 8px;font-size:20px;color:#fff">%s</h2>
    <p style="color:#94a3b8;margin:0 0 20px;font-size:14px">%s</p>
    <table style="width:100%%;border-collapse:collapse;font-size:13px">
      <tr style="border-bottom:1px solid #1e293b">
        <td style="padding:10px 0;color:#64748b;width:140px">Metric</td>
        <td style="padding:10px 0;color:#e2e8f0">%s</td>
      </tr>
      <tr style="border-bottom:1px solid #1e293b">
        <td style="padding:10px 0;color:#64748b">Current value</td>
        <td style="padding:10px 0;color:#f87171;font-weight:600">%.2f</td>
      </tr>
      <tr style="border-bottom:1px solid #1e293b">
        <td style="padding:10px 0;color:#64748b">Threshold</td>
        <td style="padding:10px 0;color:#e2e8f0">%s %.2f</td>
      </tr>
      <tr>
        <td style="padding:10px 0;color:#64748b">Fired at</td>
        <td style="padding:10px 0;color:#e2e8f0">%s</td>
      </tr>
    </table>
    <div style="margin-top:24px">
      <a href="%s/alerts" style="background:#6366f1;color:#fff;text-decoration:none;
         padding:10px 20px;border-radius:6px;font-size:13px;font-weight:600">
        View in NetScope Hub →
      </a>
    </div>
    <p style="margin-top:24px;font-size:11px;color:#475569">
      Sent by %s · <a href="%s/settings" style="color:#6366f1">Manage alerts</a>
    </p>
  </div>
</body></html>`,
		color, cfg.OrgName,
		p.RuleName, p.Message,
		p.Metric,
		p.Value,
		condStr, p.Threshold,
		p.FiredAt.UTC().Format(time.RFC1123),
		cfg.AppURL,
		cfg.OrgName, cfg.AppURL,
	)
}
