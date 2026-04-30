package models

import "time"

// ConnectionRecord is one row in the full connection audit log.
type ConnectionRecord struct {
	ID         string    `json:"id"`
	AgentID    string    `json:"agent_id"`
	Hostname   string    `json:"hostname"`
	Timestamp  time.Time `json:"timestamp"`
	Protocol   string    `json:"protocol"`
	SrcIP      string    `json:"src_ip"`
	SrcPort    uint16    `json:"src_port"`
	DstIP      string    `json:"dst_ip"`
	DstPort    uint16    `json:"dst_port"`
	BytesIn    uint64    `json:"bytes_in"`
	BytesOut   uint64    `json:"bytes_out"`
	DurationMs uint32    `json:"duration_ms"`
	Info        string    `json:"info"`
	IsExternal  bool      `json:"is_external"`
	// eBPF process attribution (empty/zero for pcap-mode flows)
	ProcessName string    `json:"process_name,omitempty"`
	PID         uint32    `json:"pid,omitempty"`
}

// TLSAuditRecord represents a connection with a TLS certificate problem.
type TLSAuditRecord struct {
	Fingerprint string    `json:"fingerprint"`
	CN          string    `json:"cn"`
	Issuer      string    `json:"issuer"`
	Expiry      string    `json:"expiry"`
	Expired     bool      `json:"expired"`
	DaysLeft    int       `json:"days_left"`
	Hostname    string    `json:"hostname"`
	DstIP       string    `json:"dst_ip"`
	LastSeen    time.Time `json:"last_seen"`
	Issue       string    `json:"issue"` // "expired" | "expiring_soon" | "weak_cipher" | "self_signed"
}

// TopTalker represents a host ordered by outbound bytes.
type TopTalker struct {
	IP          string  `json:"ip"`
	Hostname    string  `json:"hostname"`
	BytesOut    uint64  `json:"bytes_out"`
	BytesIn     uint64  `json:"bytes_in"`
	FlowCount   uint64  `json:"flow_count"`
	UniqueDestinations uint64 `json:"unique_destinations"`
}

// ComplianceSummary is the overview returned with each compliance report.
type ComplianceSummary struct {
	TotalConnections    uint64 `json:"total_connections"`
	ExternalConnections uint64 `json:"external_connections"`
	TLSIssues           uint64 `json:"tls_issues"`
	TopTalkerCount      int    `json:"top_talker_count"`
	Window              string `json:"window"`
}
