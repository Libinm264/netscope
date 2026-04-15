package models

import "time"

// Agent represents a registered NetScope agent.
type Agent struct {
	AgentID      string    `json:"agent_id"`
	Hostname     string    `json:"hostname"`
	Version      string    `json:"version"`
	Interface    string    `json:"interface"`
	LastSeen     time.Time `json:"last_seen"`
	RegisteredAt time.Time `json:"registered_at"`
}

// RegisterRequest is the payload sent to POST /api/v1/agents/register.
type RegisterRequest struct {
	AgentID   string `json:"agent_id"`
	Hostname  string `json:"hostname"`
	Version   string `json:"version"`
	Interface string `json:"interface"`
}
