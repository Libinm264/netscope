// Package alerting implements background alert-rule evaluation and multi-channel delivery.
package alerting

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/netscope/hub-api/models"
)

var httpClient = &http.Client{Timeout: 10 * time.Second}

// FireAlert dispatches an alert to the correct channel based on rule.IntegrationType.
// Falls back to generic webhook when the type is empty or unrecognised.
func FireAlert(ctx context.Context, rule models.AlertRule, payload models.WebhookPayload) bool {
	if rule.WebhookURL == "" {
		return false
	}
	switch rule.IntegrationType {
	case "slack":
		return FireSlack(ctx, rule.WebhookURL, payload)
	case "pagerduty":
		return FirePagerDuty(ctx, rule.WebhookURL, payload)
	case "opsgenie":
		return FireOpsGenie(ctx, rule.WebhookURL, payload)
	case "teams":
		return FireTeams(ctx, rule.WebhookURL, payload)
	default:
		return FireWebhook(ctx, rule.WebhookURL, payload)
	}
}

// ── Generic webhook ───────────────────────────────────────────────────────────

// FireWebhook posts the raw WebhookPayload JSON to any HTTP endpoint.
func FireWebhook(ctx context.Context, url string, payload models.WebhookPayload) bool {
	return postJSON(ctx, url, payload, nil)
}

// ── Slack ─────────────────────────────────────────────────────────────────────

type slackPayload struct {
	Text        string            `json:"text"`
	Attachments []slackAttachment `json:"attachments"`
}

type slackAttachment struct {
	Color  string       `json:"color"`
	Fields []slackField `json:"fields"`
	Footer string       `json:"footer"`
	Ts     int64        `json:"ts"`
}

type slackField struct {
	Title string `json:"title"`
	Value string `json:"value"`
	Short bool   `json:"short"`
}

// FireSlack posts a formatted message to a Slack Incoming Webhook URL.
func FireSlack(ctx context.Context, webhookURL string, payload models.WebhookPayload) bool {
	color := "danger"
	if payload.Condition == "lt" {
		color = "warning"
	}

	body := slackPayload{
		Text: fmt.Sprintf("🚨 *NetScope Alert*: %s", payload.RuleName),
		Attachments: []slackAttachment{
			{
				Color: color,
				Fields: []slackField{
					{Title: "Metric", Value: payload.Metric, Short: true},
					{Title: "Value", Value: fmt.Sprintf("%.2f", payload.Value), Short: true},
					{Title: "Threshold", Value: fmt.Sprintf("%s %.2f", payload.Condition, payload.Threshold), Short: true},
					{Title: "Message", Value: payload.Message, Short: false},
				},
				Footer: "NetScope Hub",
				Ts:     payload.FiredAt.Unix(),
			},
		},
	}
	return postJSON(ctx, webhookURL, body, nil)
}

// ── PagerDuty ─────────────────────────────────────────────────────────────────

type pdPayload struct {
	RoutingKey  string    `json:"routing_key"`
	EventAction string    `json:"event_action"`
	DedupKey    string    `json:"dedup_key"`
	Payload     pdDetails `json:"payload"`
}

type pdDetails struct {
	Summary       string            `json:"summary"`
	Severity      string            `json:"severity"`
	Source        string            `json:"source"`
	Timestamp     string            `json:"timestamp"`
	CustomDetails map[string]string `json:"custom_details"`
}

// FirePagerDuty sends an event to the PagerDuty Events API v2.
// webhookURL should be the integration routing key (not a URL).
func FirePagerDuty(ctx context.Context, routingKey string, payload models.WebhookPayload) bool {
	body := pdPayload{
		RoutingKey:  routingKey,
		EventAction: "trigger",
		DedupKey:    fmt.Sprintf("netscope-%s", payload.AlertID),
		Payload: pdDetails{
			Summary:   payload.Message,
			Severity:  "critical",
			Source:    "netscope-hub",
			Timestamp: payload.FiredAt.UTC().Format(time.RFC3339),
			CustomDetails: map[string]string{
				"metric":    payload.Metric,
				"value":     fmt.Sprintf("%.2f", payload.Value),
				"threshold": fmt.Sprintf("%.2f", payload.Threshold),
				"rule":      payload.RuleName,
			},
		},
	}
	return postJSON(ctx, "https://events.pagerduty.com/v2/enqueue", body, nil)
}

// ── OpsGenie ──────────────────────────────────────────────────────────────────

type opsgeniePayload struct {
	Message     string            `json:"message"`
	Alias       string            `json:"alias"`
	Description string            `json:"description"`
	Priority    string            `json:"priority"`
	Tags        []string          `json:"tags"`
	Details     map[string]string `json:"details"`
}

// FireOpsGenie sends an alert to the OpsGenie Alerts API.
// webhookURL should be the OpsGenie API key.
func FireOpsGenie(ctx context.Context, apiKey string, payload models.WebhookPayload) bool {
	body := opsgeniePayload{
		Message:     fmt.Sprintf("[NetScope] %s", payload.RuleName),
		Alias:       fmt.Sprintf("netscope-%s", payload.AlertID),
		Description: payload.Message,
		Priority:    "P2",
		Tags:        []string{"netscope", payload.Metric},
		Details: map[string]string{
			"metric":    payload.Metric,
			"value":     fmt.Sprintf("%.2f", payload.Value),
			"threshold": fmt.Sprintf("%.2f", payload.Threshold),
		},
	}
	headers := map[string]string{
		"Authorization": "GenieKey " + apiKey,
	}
	return postJSON(ctx, "https://api.opsgenie.com/v2/alerts", body, headers)
}

// ── Microsoft Teams ───────────────────────────────────────────────────────────

type teamsPayload struct {
	Type       string       `json:"@type"`
	Context    string       `json:"@context"`
	ThemeColor string       `json:"themeColor"`
	Summary    string       `json:"summary"`
	Sections   []teamsSection `json:"sections"`
}

type teamsSection struct {
	ActivityTitle string      `json:"activityTitle"`
	Facts         []teamsFact `json:"facts"`
}

type teamsFact struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// FireTeams posts a card to a Microsoft Teams Incoming Webhook.
func FireTeams(ctx context.Context, webhookURL string, payload models.WebhookPayload) bool {
	body := teamsPayload{
		Type:       "MessageCard",
		Context:    "http://schema.org/extensions",
		ThemeColor: "FF0000",
		Summary:    fmt.Sprintf("NetScope Alert: %s", payload.RuleName),
		Sections: []teamsSection{{
			ActivityTitle: fmt.Sprintf("🚨 %s", payload.RuleName),
			Facts: []teamsFact{
				{Name: "Metric", Value: payload.Metric},
				{Name: "Value", Value: fmt.Sprintf("%.2f", payload.Value)},
				{Name: "Threshold", Value: fmt.Sprintf("%s %.2f", payload.Condition, payload.Threshold)},
				{Name: "Message", Value: payload.Message},
				{Name: "Time", Value: payload.FiredAt.UTC().Format(time.RFC3339)},
			},
		}},
	}
	return postJSON(ctx, webhookURL, body, nil)
}

// ── BuildMessage ──────────────────────────────────────────────────────────────

// BuildMessage returns a human-readable alert description.
func BuildMessage(rule models.AlertRule, value float64) string {
	condStr := map[string]string{"gt": "exceeded", "lt": "fell below"}[rule.Condition]
	if condStr == "" {
		condStr = rule.Condition
	}
	return fmt.Sprintf(
		"[NetScope] %s: %s is %.2f (%s threshold %.2f over last %d min)",
		rule.Name, rule.Metric, value, condStr, rule.Threshold, rule.WindowMinutes,
	)
}

// ── internal helpers ──────────────────────────────────────────────────────────

func postJSON(ctx context.Context, url string, body any, extraHeaders map[string]string) bool {
	data, err := json.Marshal(body)
	if err != nil {
		slog.Error("alert delivery: marshal", "err", err)
		return false
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		slog.Error("alert delivery: build request", "url", url, "err", err)
		return false
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "NetScope-Hub/0.1")
	for k, v := range extraHeaders {
		req.Header.Set(k, v)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		slog.Warn("alert delivery: request failed", "url", url, "err", err)
		return false
	}
	defer resp.Body.Close()

	ok := resp.StatusCode >= 200 && resp.StatusCode < 300
	if !ok {
		slog.Warn("alert delivery: non-2xx", "url", url, "status", resp.StatusCode)
	}
	return ok
}
