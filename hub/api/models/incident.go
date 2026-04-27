package models

import "time"

// Incident represents a security incident created from a Sigma match, alert, or manually.
type Incident struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Severity    string    `json:"severity"`     // "low" | "medium" | "high" | "critical"
	Status      string    `json:"status"`       // "open" | "ack" | "resolved"
	Source      string    `json:"source"`       // "sigma" | "alert" | "manual"
	SourceID    string    `json:"source_id"`    // sigma_match id or alert_event id
	Notes       string    `json:"notes,omitempty"`
	ExternalRef string    `json:"external_ref,omitempty"` // PD ID / OG alias / Jira key / Linear ID
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// IncidentWorkflowConfig is a persisted integration config for one platform.
type IncidentWorkflowConfig struct {
	Integration string    `json:"integration"` // "pagerduty" | "opsgenie" | "jira" | "linear"
	Enabled     bool      `json:"enabled"`
	Config      string    `json:"config"` // JSON — credentials redacted on read
	UpdatedAt   time.Time `json:"updated_at"`
}

// AddNoteRequest is the body for adding a note to an incident.
type AddNoteRequest struct {
	Note string `json:"note"`
}

// ResolveRequest is the body for resolving an incident.
type ResolveRequest struct {
	Note string `json:"note,omitempty"`
}
