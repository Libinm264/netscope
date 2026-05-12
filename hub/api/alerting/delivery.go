// Package alerting implements background alert-rule evaluation and multi-channel delivery.
package alerting

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/klyzar/hub-api/models"
	"github.com/klyzar/hub-api/util"
)

var httpClient = &http.Client{Timeout: 10 * time.Second}

// FireAlert dispatches an alert to the correct channel based on rule.IntegrationType.
// Falls back to generic webhook when the type is empty or unrecognised.
func FireAlert(ctx context.Context, rule models.AlertRule, payload models.WebhookPayload, smtpCfg SMTPConfig) bool {
	if rule.IntegrationType == "email" {
		return FireEmail(smtpCfg, rule.EmailTo, payload)
	}
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
	default: // "webhook" or empty
		return FireWebhook(ctx, rule.WebhookURL, payload, rule.WebhookSecret)
	}
}

// ── Generic webhook ───────────────────────────────────────────────────────────

// FireWebhook posts the raw WebhookPayload JSON to any HTTP endpoint.
// If webhookSecret is non-empty, the request is signed with HMAC-SHA256 and the
// signature sent in the X-Nexor-Signature header.
func FireWebhook(ctx context.Context, url string, payload models.WebhookPayload, webhookSecret string) bool {
	headers := map[string]string{}
	if webhookSecret != "" {
		data, _ := json.Marshal(payload)
		mac := hmac.New(sha256.New, []byte(webhookSecret))
		mac.Write(data)
		sig := hex.EncodeToString(mac.Sum(nil))
		headers["X-Nexor-Signature"] = "sha256=" + sig
	}
	return postJSON(ctx, url, payload, headers)
}

// ── Slack ─────────────────────────────────────────────────────────────────────

// ── Slack Block Kit ───────────────────────────────────────────────────────────
//
// Block Kit produces rich, interactive-looking alert threads in Slack with
// inline context rows for the top flows that triggered the alert.

type slackBlock struct {
	Type     string      `json:"type"`
	Text     *slackText  `json:"text,omitempty"`
	Fields   []slackText `json:"fields,omitempty"`
	Elements []slackElem `json:"elements,omitempty"`
}

type slackText struct {
	Type  string `json:"type"`
	Text  string `json:"text"`
	Emoji bool   `json:"emoji,omitempty"`
}

type slackElem struct {
	Type     string     `json:"type"`
	Text     *slackText `json:"text,omitempty"`
	URL      string     `json:"url,omitempty"`
	Style    string     `json:"style,omitempty"`
	ActionID string     `json:"action_id,omitempty"`
}

type slackBlockPayload struct {
	Text   string       `json:"text"` // fallback for notifications
	Blocks []slackBlock `json:"blocks"`
}

// FireSlack posts a Block Kit message to a Slack Incoming Webhook URL.
func FireSlack(ctx context.Context, webhookURL string, payload models.WebhookPayload) bool {
	emoji := "🚨"
	if payload.Condition == "lt" {
		emoji = "⚠️"
	}

	headerText := fmt.Sprintf("%s *Nexor Alert: %s*", emoji, payload.RuleName)

	fields := []slackText{
		{Type: "mrkdwn", Text: fmt.Sprintf("*Metric*\n%s", payload.Metric)},
		{Type: "mrkdwn", Text: fmt.Sprintf("*Value*\n`%.2f`", payload.Value)},
		{Type: "mrkdwn", Text: fmt.Sprintf("*Threshold*\n%s `%.2f`", payload.Condition, payload.Threshold)},
		{Type: "mrkdwn", Text: fmt.Sprintf("*Fired at*\n%s", payload.FiredAt.UTC().Format("2006-01-02 15:04:05 UTC"))},
	}

	blocks := []slackBlock{
		{
			Type: "header",
			Text: &slackText{Type: "plain_text", Text: fmt.Sprintf("Nexor Alert: %s", payload.RuleName), Emoji: true},
		},
		{Type: "section", Fields: fields},
		{Type: "divider"},
		{
			Type: "section",
			Text: &slackText{Type: "mrkdwn", Text: fmt.Sprintf("*Message*\n%s", payload.Message)},
		},
	}

	// Top flows context block.
	if len(payload.TopFlows) > 0 {
		flowLines := "*Recent flows:*\n"
		for i, f := range payload.TopFlows {
			if i >= 5 {
				break
			}
			flowLines += fmt.Sprintf("• `%s` %s → %s  _%s_\n", f.Protocol, f.SrcIP, f.DstIP, f.Info)
		}
		blocks = append(blocks, slackBlock{
			Type: "section",
			Text: &slackText{Type: "mrkdwn", Text: flowLines},
		})
	}

	// Action buttons.
	elems := []slackElem{
		{
			Type:     "button",
			Text:     &slackText{Type: "plain_text", Text: "View in Nexor", Emoji: true},
			URL:      payload.HubURL + "/anomalies",
			ActionID: "open_hub",
		},
	}
	if payload.ReplayURL != "" {
		elems = append(elems, slackElem{
			Type:     "button",
			Text:     &slackText{Type: "plain_text", Text: "▶ Replay Incident", Emoji: true},
			URL:      payload.ReplayURL,
			Style:    "primary",
			ActionID: "open_replay",
		})
	}
	blocks = append(blocks, slackBlock{Type: "actions", Elements: elems})

	body := slackBlockPayload{
		Text:   headerText,
		Blocks: blocks,
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
		DedupKey:    fmt.Sprintf("nexor-%s", payload.AlertID),
		Payload: pdDetails{
			Summary:   payload.Message,
			Severity:  "critical",
			Source:    "nexor-hub",
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
		Message:     fmt.Sprintf("[Nexor] %s", payload.RuleName),
		Alias:       fmt.Sprintf("nexor-%s", payload.AlertID),
		Description: payload.Message,
		Priority:    "P2",
		Tags:        []string{"nexor", payload.Metric},
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

// ── Microsoft Teams Adaptive Card ─────────────────────────────────────────────
//
// Uses the Adaptive Card schema (application/vnd.microsoft.card.adaptive) for
// a structured, scannable alert thread with inline flow context.

type teamsAdaptiveWrapper struct {
	Type        string           `json:"type"`
	Attachments []teamsAdaptive  `json:"attachments"`
}

type teamsAdaptive struct {
	ContentType string          `json:"contentType"`
	Content     teamsCardBody   `json:"content"`
}

type teamsCardBody struct {
	Schema  string        `json:"$schema"`
	Type    string        `json:"type"`
	Version string        `json:"version"`
	Body    []any         `json:"body"`
	Actions []teamsAction `json:"actions,omitempty"`
}

type teamsAction struct {
	Type  string `json:"type"`
	Title string `json:"title"`
	URL   string `json:"url"`
}

// FireTeams posts an Adaptive Card to a Microsoft Teams Incoming Webhook.
func FireTeams(ctx context.Context, webhookURL string, payload models.WebhookPayload) bool {
	color := "attention" // red in Adaptive Cards
	if payload.Condition == "lt" {
		color = "warning"
	}

	facts := []map[string]string{
		{"title": "Metric", "value": payload.Metric},
		{"title": "Value", "value": fmt.Sprintf("%.2f", payload.Value)},
		{"title": "Threshold", "value": fmt.Sprintf("%s %.2f", payload.Condition, payload.Threshold)},
		{"title": "Fired at", "value": payload.FiredAt.UTC().Format("2006-01-02 15:04:05 UTC")},
	}

	body := []any{
		map[string]any{
			"type": "TextBlock",
			"text": fmt.Sprintf("🚨 Nexor Alert: %s", payload.RuleName),
			"size": "Large", "weight": "Bolder", "color": color,
		},
		map[string]any{
			"type":  "FactSet",
			"facts": facts,
		},
		map[string]any{
			"type": "TextBlock",
			"text": payload.Message,
			"wrap": true,
		},
	}

	// Top flows context.
	if len(payload.TopFlows) > 0 {
		flowText := "**Recent flows:**\n\n"
		for i, f := range payload.TopFlows {
			if i >= 5 {
				break
			}
			flowText += fmt.Sprintf("- `%s` %s → %s — %s\n", f.Protocol, f.SrcIP, f.DstIP, f.Info)
		}
		body = append(body, map[string]any{
			"type": "TextBlock", "text": flowText, "wrap": true,
		})
	}

	actions := []teamsAction{{Type: "Action.OpenUrl", Title: "View in Nexor", URL: payload.HubURL + "/anomalies"}}
	if payload.ReplayURL != "" {
		actions = append(actions, teamsAction{Type: "Action.OpenUrl", Title: "▶ Replay Incident", URL: payload.ReplayURL})
	}

	card := teamsAdaptiveWrapper{
		Type: "message",
		Attachments: []teamsAdaptive{{
			ContentType: "application/vnd.microsoft.card.adaptive",
			Content: teamsCardBody{
				Schema:  "http://adaptivecards.io/schemas/adaptive-card.json",
				Type:    "AdaptiveCard",
				Version: "1.4",
				Body:    body,
				Actions: actions,
			},
		}},
	}
	return postJSON(ctx, webhookURL, card, nil)
}

// ── BuildMessage ──────────────────────────────────────────────────────────────

// BuildMessage returns a human-readable alert description.
func BuildMessage(rule models.AlertRule, value float64) string {
	condStr := map[string]string{"gt": "exceeded", "lt": "fell below"}[rule.Condition]
	if condStr == "" {
		condStr = rule.Condition
	}
	return fmt.Sprintf(
		"[Nexor] %s: %s is %.2f (%s threshold %.2f over last %d min)",
		rule.Name, rule.Metric, value, condStr, rule.Threshold, rule.WindowMinutes,
	)
}

// ── internal helpers ──────────────────────────────────────────────────────────

// retryDelays defines the wait intervals between delivery attempts (1 s, 5 s, 30 s).
var retryDelays = []time.Duration{1 * time.Second, 5 * time.Second, 30 * time.Second}

func postJSON(ctx context.Context, url string, body any, extraHeaders map[string]string) bool {
	// Defence-in-depth: re-validate the URL at delivery time to catch DNS-rebinding
	// attacks (the URL may have been valid when stored but re-resolve to an
	// internal address by the time the alert fires).
	if err := util.ValidateWebhookURL(url); err != nil {
		slog.Error("alert delivery: blocked SSRF attempt", "url", url, "err", err)
		return false
	}

	data, err := json.Marshal(body)
	if err != nil {
		slog.Error("alert delivery: marshal", "err", err)
		return false
	}

	// Attempt delivery up to 3 times with exponential back-off (1 s → 5 s → 30 s).
	maxAttempts := len(retryDelays) + 1
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		ok, fatal := tryPost(ctx, url, data, extraHeaders)
		if ok {
			if attempt > 1 {
				slog.Info("alert delivery: succeeded after retry", "url", url, "attempt", attempt)
			}
			return true
		}
		if fatal {
			// Non-retryable error (e.g. request build failure).
			return false
		}
		if attempt < maxAttempts {
			delay := retryDelays[attempt-1]
			slog.Warn("alert delivery: retrying", "url", url, "attempt", attempt, "next_in", delay)
			select {
			case <-ctx.Done():
				slog.Warn("alert delivery: context cancelled during retry", "url", url)
				return false
			case <-time.After(delay):
			}
		}
	}
	slog.Error("alert delivery: all attempts failed", "url", url, "attempts", maxAttempts)
	return false
}

// tryPost makes a single HTTP POST attempt.
// Returns (success bool, fatal bool) — fatal=true means do not retry.
func tryPost(ctx context.Context, url string, data []byte, extraHeaders map[string]string) (bool, bool) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		slog.Error("alert delivery: build request", "url", url, "err", err)
		return false, true // can't build request — fatal
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Nexor-Hub/0.1")
	for k, v := range extraHeaders {
		req.Header.Set(k, v)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		slog.Warn("alert delivery: request failed", "url", url, "err", err)
		return false, false
	}
	defer resp.Body.Close()

	// 429 or 5xx → retry; 4xx (except 429) → permanent failure
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return true, false
	}
	if resp.StatusCode != 429 && resp.StatusCode >= 400 && resp.StatusCode < 500 {
		slog.Warn("alert delivery: non-retryable error", "url", url, "status", resp.StatusCode)
		return false, true
	}
	slog.Warn("alert delivery: non-2xx (will retry)", "url", url, "status", resp.StatusCode)
	return false, false
}
