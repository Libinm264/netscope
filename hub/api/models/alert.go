package models

import "time"

// AlertRule defines a threshold-based monitoring rule.
type AlertRule struct {
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	// Metric: flows_per_minute | http_error_rate | dns_nxdomain_rate |
	//         anomaly_flow_rate | anomaly_http_latency
	Metric          string    `json:"metric"`
	// Condition: "gt" or "lt"
	Condition       string    `json:"condition"`
	Threshold       float64   `json:"threshold"`
	WindowMinutes   uint32    `json:"window_minutes"`
	// IntegrationType: "webhook" | "slack" | "pagerduty" | "opsgenie" | "teams" | "email"
	IntegrationType string    `json:"integration_type"`
	// WebhookURL holds the delivery target — a URL for webhook/slack/teams,
	// or an API/routing key for pagerduty/opsgenie.
	WebhookURL      string    `json:"webhook_url"`
	// WebhookSecret is used to sign webhook deliveries with HMAC-SHA256.
	// Returned only at creation time; stored hashed.
	WebhookSecret   string    `json:"webhook_secret,omitempty"`
	// EmailTo is the recipient for "email" integration type.
	EmailTo         string    `json:"email_to,omitempty"`
	Enabled         bool      `json:"enabled"`
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
	IntegrationType string  `json:"integration_type"`
	WebhookURL      string  `json:"webhook_url"`
	WebhookSecret   string  `json:"webhook_secret,omitempty"`
	EmailTo         string  `json:"email_to,omitempty"`
	CooldownMinutes uint32  `json:"cooldown_minutes"`
}

// FlowSummary is a compact flow record included in alert payloads for context.
type FlowSummary struct {
	Protocol string `json:"protocol"`
	SrcIP    string `json:"src_ip"`
	DstIP    string `json:"dst_ip"`
	DstPort  uint16 `json:"dst_port"`
	Info     string `json:"info"`
}

// WebhookPayload is posted to the rule's WebhookURL when it fires.
type WebhookPayload struct {
	AlertID   string        `json:"alert_id"`
	RuleName  string        `json:"rule_name"`
	Metric    string        `json:"metric"`
	Value     float64       `json:"value"`
	Threshold float64       `json:"threshold"`
	Condition string        `json:"condition"`
	FiredAt   time.Time     `json:"fired_at"`
	Message   string        `json:"message"`
	// V4 enrichment: top 5 recent flows for inline context in Slack/Teams threads.
	TopFlows  []FlowSummary `json:"top_flows,omitempty"`
	// HubURL is the base URL of the Hub dashboard (e.g. "https://hub.example.com").
	HubURL    string        `json:"hub_url,omitempty"`
	// ReplayURL is a deep link to the incident replay timeline.
	ReplayURL string        `json:"replay_url,omitempty"`
}
