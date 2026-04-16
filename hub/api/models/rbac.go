package models

import "time"

// APIToken is a named, scoped access token for the Hub API.
// The bootstrap key (from env) always has admin role and is not stored here.
type APIToken struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Role      string    `json:"role"`       // "admin" | "viewer"
	Token     string    `json:"token"`      // shown once on creation
	CreatedAt time.Time `json:"created_at"`
	LastUsed  time.Time `json:"last_used"`
	Revoked   bool      `json:"revoked"`
}

// CreateTokenRequest is the POST body for generating a new API token.
type CreateTokenRequest struct {
	Name string `json:"name"`
	Role string `json:"role"` // "admin" | "viewer"
}
