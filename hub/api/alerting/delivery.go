// Package alerting implements background alert-rule evaluation and webhook delivery.
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

// FireWebhook posts a WebhookPayload to the given URL.
// Returns true if the delivery succeeded (2xx response).
func FireWebhook(ctx context.Context, url string, payload models.WebhookPayload) bool {
	data, err := json.Marshal(payload)
	if err != nil {
		slog.Error("alert: marshal webhook payload", "err", err)
		return false
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		slog.Error("alert: build webhook request", "err", err)
		return false
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "NetScope-Hub/0.1")

	resp, err := httpClient.Do(req)
	if err != nil {
		slog.Warn("alert: webhook delivery failed", "url", url, "err", err)
		return false
	}
	defer resp.Body.Close()

	ok := resp.StatusCode >= 200 && resp.StatusCode < 300
	if !ok {
		slog.Warn("alert: webhook returned non-2xx", "url", url, "status", resp.StatusCode)
	}
	return ok
}

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
