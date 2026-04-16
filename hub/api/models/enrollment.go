package models

import "time"

// EnrollmentToken is a one-time-use token an admin generates to authorise
// a new agent to join the fleet without exposing the bootstrap API key.
type EnrollmentToken struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`       // human label, e.g. "prod-web-01"
	Token     string    `json:"token"`      // the secret UUID shown once
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
	UsedCount int       `json:"used_count"` // 0 = unused
	Revoked   bool      `json:"revoked"`
}

// CreateEnrollmentTokenRequest is the POST body for generating a new token.
type CreateEnrollmentTokenRequest struct {
	Name      string `json:"name"`
	ExpiresIn string `json:"expires_in"` // "24h" | "7d" | "30d" | "never"
}

// EnrollRequest is posted by an agent to /api/v1/agents/enroll.
// It must carry a valid, unexpired, unrevoked enrollment token.
type EnrollRequest struct {
	Token     string `json:"token"`
	Hostname  string `json:"hostname"`
	Version   string `json:"version"`
	Interface string `json:"interface"`
}

// EnrollResponse is returned to the agent on successful enrolment.
type EnrollResponse struct {
	AgentID string `json:"agent_id"`
	APIKey  string `json:"api_key"`
	HubURL  string `json:"hub_url"`
}
