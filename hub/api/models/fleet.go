package models

import "time"

// ClusterSummary aggregates health metrics for one logical cluster.
type ClusterSummary struct {
	Cluster     string   `json:"cluster"`
	AgentCount  uint64   `json:"agent_count"`
	OnlineCount uint64   `json:"online_count"`  // seen in last 5 min
	Versions    []string `json:"versions"`       // distinct agent versions
	Flows1h     uint64   `json:"flows_1h"`
}

// AgentConfigPush is the payload an admin sends to configure a remote agent.
type AgentConfigPush struct {
	// Config is a free-form JSON object — the agent merges it into its running config.
	// Recognised keys: capture_mode, bpf_filter, hub_url, batch_size, interval_ms.
	Config map[string]any `json:"config"`
}

// AgentConfigRecord is stored in the agent_configs table.
type AgentConfigRecord struct {
	AgentID  string    `json:"agent_id"`
	Config   string    `json:"config"` // raw JSON
	PushedAt time.Time `json:"pushed_at"`
	AckAt    time.Time `json:"ack_at,omitempty"`
	Version  uint64    `json:"version"`
}

// ConfigAck is sent by the agent after applying a pushed config.
type ConfigAck struct {
	AgentID       string `json:"agent_id"`
	ConfigVersion string `json:"config_version"`
}

// CrossClusterSearchRequest wraps flow filter params for a cross-cluster query.
type CrossClusterSearchRequest struct {
	Cluster   string `query:"cluster"`
	SrcIP     string `query:"src_ip"`
	DstIP     string `query:"dst_ip"`
	Protocol  string `query:"protocol"`
	StartTime string `query:"start"`
	EndTime   string `query:"end"`
	Limit     int    `query:"limit"`
}
