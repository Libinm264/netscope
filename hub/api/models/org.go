package models

import "time"

// Organisation is a top-level tenant. All resources (agents, flows, alerts)
// belong to exactly one org. The built-in "default" org always exists.
type Organisation struct {
	OrgID           string    `json:"org_id"`
	Name            string    `json:"name"`
	Slug            string    `json:"slug"`
	AgentQuota      int       `json:"agent_quota"`
	RetentionDays   int       `json:"retention_days"`
	Plan            string    `json:"plan"` // "community" | "team" | "enterprise"
	CreatedAt       time.Time `json:"created_at"`
	// OtelBackendURL is the base URL of the OTel trace backend (e.g. "http://jaeger:16686").
	// When set, trace_id values in flows become clickable links in the hub UI.
	OtelBackendURL  string    `json:"otel_backend_url,omitempty"`
}

// OrgMember is a user within an organisation.
// Identity/credential management is delegated to the SSO provider (Dex).
// This record holds the identity mapping + role assignment only.
type OrgMember struct {
	UserID      string    `json:"user_id"`
	OrgID       string    `json:"org_id"`
	Email       string    `json:"email"`
	DisplayName string    `json:"display_name"`
	Role        string    `json:"role"` // "owner" | "admin" | "analyst" | "viewer"
	SSOProvider string    `json:"sso_provider,omitempty"` // "saml" | "oidc" | "local"
	SSOSubject  string    `json:"sso_subject,omitempty"`  // IdP subject / nameID
	IsActive    bool      `json:"is_active"`
	CreatedAt   time.Time `json:"created_at"`
	LastSeen    time.Time `json:"last_seen,omitempty"`
}

// Team is a logical grouping of members used to scope agent access
// and alert delivery routing.
type Team struct {
	TeamID      string    `json:"team_id"`
	OrgID       string    `json:"org_id"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	MemberCount int       `json:"member_count,omitempty"` // computed at query time
	CreatedAt   time.Time `json:"created_at"`
}

// TeamMember records which users belong to which team.
type TeamMember struct {
	TeamID      string    `json:"team_id"`
	UserID      string    `json:"user_id"`
	OrgID       string    `json:"org_id"`
	Email       string    `json:"email,omitempty"`
	DisplayName string    `json:"display_name,omitempty"`
	Role        string    `json:"role,omitempty"`
	AddedAt     time.Time `json:"added_at"`
}

// SSOConfig holds IdP metadata for SAML or OIDC. Client secrets are
// never stored in ClickHouse — use the SSO_CLIENT_SECRET env var.
type SSOConfig struct {
	OrgID       string    `json:"org_id"`
	Provider    string    `json:"provider"` // "saml" | "oidc"
	Enabled     bool      `json:"enabled"`
	// SAML
	EntityID    string    `json:"entity_id,omitempty"`
	SSOURL      string    `json:"sso_url,omitempty"`
	Certificate string    `json:"certificate,omitempty"` // IdP signing cert (PEM)
	// OIDC / Dex
	IssuerURL   string    `json:"issuer_url,omitempty"`
	ClientID    string    `json:"client_id,omitempty"`
	// ClientSecret intentionally omitted from JSON (write-only)
	UpdatedAt   time.Time `json:"updated_at"`
}

// ── Request / Response bodies ─────────────────────────────────────────────────

type UpdateOrgRequest struct {
	Name           string `json:"name"`
	AgentQuota     int    `json:"agent_quota"`
	RetentionDays  int    `json:"retention_days"`
	OtelBackendURL string `json:"otel_backend_url,omitempty"`
}

type InviteMemberRequest struct {
	Email string `json:"email"`
	Name  string `json:"name"`
	Role  string `json:"role"` // "admin" | "analyst" | "viewer"
}

type UpdateRoleRequest struct {
	Role string `json:"role"`
}

type CreateTeamRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type AddTeamMemberRequest struct {
	UserID string `json:"user_id"`
}

type UpdateSSOConfigRequest struct {
	Provider    string `json:"provider"`
	Enabled     bool   `json:"enabled"`
	EntityID    string `json:"entity_id"`
	SSOURL      string `json:"sso_url"`
	Certificate string `json:"certificate"`
	IssuerURL   string `json:"issuer_url"`
	ClientID    string `json:"client_id"`
	ClientSecret string `json:"client_secret,omitempty"` // write-only, stored in env
}
