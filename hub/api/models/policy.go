package models

import "time"

type ProcessPolicy struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	ProcessName string    `json:"process_name"`
	Action      string    `json:"action"`      // "alert" | "deny"
	DstIPCIDR   string    `json:"dst_ip_cidr"` // "" = any
	DstPort     uint16    `json:"dst_port"`    // 0 = any
	Description string    `json:"description"`
	Enabled     bool      `json:"enabled"`
	CreatedAt   time.Time `json:"created_at"`
}

type PolicyViolation struct {
	ID          string    `json:"id"`
	PolicyID    string    `json:"policy_id"`
	PolicyName  string    `json:"policy_name"`
	ProcessName string    `json:"process_name"`
	PID         uint32    `json:"pid"`
	SrcIP       string    `json:"src_ip"`
	DstIP       string    `json:"dst_ip"`
	DstPort     uint16    `json:"dst_port"`
	Protocol    string    `json:"protocol"`
	AgentID     string    `json:"agent_id"`
	Hostname    string    `json:"hostname"`
	ViolatedAt  time.Time `json:"violated_at"`
}

type CreatePolicyRequest struct {
	Name        string `json:"name"`
	ProcessName string `json:"process_name"`
	Action      string `json:"action"`
	DstIPCIDR   string `json:"dst_ip_cidr"`
	DstPort     uint16 `json:"dst_port"`
	Description string `json:"description"`
}
