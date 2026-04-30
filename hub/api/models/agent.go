package models

import "time"

type Agent struct {
	AgentID      string    `json:"agent_id"`
	Hostname     string    `json:"hostname"`
	Version      string    `json:"version"`
	Interface    string    `json:"interface"`
	LastSeen     time.Time `json:"last_seen"`
	RegisteredAt time.Time `json:"registered_at"`
	OS           string    `json:"os"`
	CaptureMode  string    `json:"capture_mode"`
	EbpfEnabled  bool      `json:"ebpf_enabled"`
	FlowCount1h  uint64    `json:"flow_count_1h"`
	// Cluster is an optional logical grouping label (e.g. "prod-eu", "staging-us").
	// Set via AGENT_CLUSTER env var on the agent or the enrollment token label prefix.
	Cluster      string    `json:"cluster,omitempty"`
}

type RegisterRequest struct {
	AgentID     string `json:"agent_id"`
	Hostname    string `json:"hostname"`
	Version     string `json:"version"`
	Interface   string `json:"interface"`
	OS          string `json:"os"`
	CaptureMode string `json:"capture_mode"`
	EbpfEnabled bool   `json:"ebpf_enabled"`
	Cluster     string `json:"cluster,omitempty"`
}
