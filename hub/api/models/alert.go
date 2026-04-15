package models

import "time"

// AlertRule defines a threshold-based monitoring rule.
// When the measured metric satisfies the condition against the threshold,
// the rule fires a webhook and records an AlertEvent.
type AlertRule struct {
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	// Metric is one of: flows_per_minute, http_error_rate, dns_nxdomain_rate
	Metric          string    `json:"metric"`
	// Condition is "gt" (greater-than) or "lt" (less-than)
	Condition       string    `json:"condition"`
	Threshold       float64   `json:"threshold"`
	// WindowMinutes is the look-back window used to compute the metric
	WindowMinutes   uint32    `json:"window_minutes"`
	WebhookURL      string    `json:"webhook_url"`
	Enabled         bool      `json:"enabled"`
	// CooldownMinutes prevents repeated firings within this period
	CooldownMinutes uint32    `json:"cooldown_minutes"`
	CreatedAt       time.Time `json:"created_at"`
}

// AlertEvent records a single firing of an AlertRule.
type AlertEvent struct {
	ID        string    `json:"id"`
	RuleID    string    `json:"rule_id"`
	RuleName  string    `json:"rule_name"`
	Metric    string    `json:"metric"`
	Value     float64   `json:"value"`
	Threshold float64   `json:"threshold"`
	FiredAt   time.Time `json:"fired_at"`
	Delivered bool      `json:"delivered"`
}

// CreateAlertRuleRequest is the payload for POST /api/v1/alerts.
type CreateAlertRuleRequest struct {
	Name            string  `json:"name"`
	Metric          string  `json:"metric"`
	Condition       string  `json:"condition"`
	Threshold       float64 `json:"threshold"`
	WindowMinutes   uint32  `json:"window_minutes"`
	WebhookURL      string  `json:"webhook_url"`
	CooldownMinutes uint32  `json:"cooldown_minutes"`
}

// WebhookPayload is posted to the rule's WebhookURL when it fires.
type WebhookPayload struct {
	AlertID   string    `json:"alert_id"`
	RuleName  string    `json:"rule_name"`
	Metric    string    `json:"metric"`
	Value     float64   `json:"value"`
	Threshold float64   `json:"threshold"`
	Condition string    `json:"condition"`
	FiredAt   time.Time `json:"fired_at"`
	Message   string    `json:"message"`
}
