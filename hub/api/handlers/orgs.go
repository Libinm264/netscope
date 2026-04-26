package handlers

import (
	"context"
	"crypto/rand"
	"fmt"
	"log/slog"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/netscope/hub-api/alerting"
	chclient "github.com/netscope/hub-api/clickhouse"
	"github.com/netscope/hub-api/enterprise/license"
	"github.com/netscope/hub-api/models"
	"github.com/netscope/hub-api/sessions"
	"github.com/netscope/hub-api/util"
)

// EnterpriseHandler handles all org / member / team / SSO / license endpoints.
// All mutating operations require admin role; reads require any authenticated role.
type EnterpriseHandler struct {
	CH       *chclient.Client
	License  *license.License
	Sessions *sessions.Store    // for session invalidation on role change
	SMTP     alerting.SMTPConfig
	FrontendURL string
}

// ── Org ───────────────────────────────────────────────────────────────────────

// GetOrg handles GET /api/v1/enterprise/org
func (h *EnterpriseHandler) GetOrg(c *fiber.Ctx) error {
	if h.CH == nil {
		return serviceUnavailable(c)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := h.CH.Query(ctx,
		`SELECT org_id, name, slug, agent_quota, retention_days, plan, created_at
		 FROM organisations FINAL
		 WHERE org_id = 'default'
		 LIMIT 1`)
	if err != nil {
		return util.InternalError(c, err)
	}
	defer rows.Close()

	if !rows.Next() {
		// Return a sensible default if the org row hasn't been created yet
		return c.JSON(models.Organisation{
			OrgID:         "default",
			Name:          "Default Organisation",
			Slug:          "default",
			AgentQuota:    h.License.AgentQuota,
			RetentionDays: 90,
			Plan:          h.License.Plan,
		})
	}

	var org models.Organisation
	if err := rows.Scan(
		&org.OrgID, &org.Name, &org.Slug,
		&org.AgentQuota, &org.RetentionDays, &org.Plan,
		&org.CreatedAt,
	); err != nil {
		return util.InternalError(c, err)
	}
	// Surface live plan from license key (overrides stored value)
	org.Plan = h.License.Plan
	org.AgentQuota = h.License.AgentQuota
	return c.JSON(org)
}

// UpdateOrg handles PUT /api/v1/enterprise/org
func (h *EnterpriseHandler) UpdateOrg(c *fiber.Ctx) error {
	var req models.UpdateOrgRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid body"})
	}
	if req.Name == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "name is required"})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := h.CH.Exec(ctx,
		`INSERT INTO organisations (org_id, name, slug, agent_quota, retention_days, plan, created_at)
		 VALUES ('default', ?, 'default', ?, ?, 'community', now64())`,
		req.Name, req.AgentQuota, req.RetentionDays,
	); err != nil {
		return util.InternalError(c, err)
	}
	return c.JSON(fiber.Map{"ok": true})
}

// ── Members ──────────────────────────────────────────────────────────────────

// ListMembers handles GET /api/v1/enterprise/members
func (h *EnterpriseHandler) ListMembers(c *fiber.Ctx) error {
	if h.CH == nil {
		return serviceUnavailable(c)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := h.CH.Query(ctx,
		`SELECT user_id, org_id, email, display_name, role,
		        sso_provider, is_active, created_at, last_seen
		 FROM org_members FINAL
		 WHERE org_id = 'default' AND is_active = 1
		 ORDER BY created_at ASC`)
	if err != nil {
		return util.InternalError(c, err)
	}
	defer rows.Close()

	members := make([]models.OrgMember, 0)
	for rows.Next() {
		var m models.OrgMember
		var isActive uint8
		if err := rows.Scan(
			&m.UserID, &m.OrgID, &m.Email, &m.DisplayName, &m.Role,
			&m.SSOProvider, &isActive, &m.CreatedAt, &m.LastSeen,
		); err != nil {
			slog.Warn("scan member", "err", err)
			continue
		}
		m.IsActive = isActive == 1
		members = append(members, m)
	}
	return c.JSON(fiber.Map{"members": members})
}

// InviteMember handles POST /api/v1/enterprise/members
func (h *EnterpriseHandler) InviteMember(c *fiber.Ctx) error {
	var req models.InviteMemberRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid body"})
	}
	if req.Email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "email is required"})
	}
	if req.Role == "" {
		req.Role = "viewer"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	userID := uuid.NewString()
	now := time.Now()
	if err := h.CH.Exec(ctx,
		`INSERT INTO org_members
		 (user_id, org_id, email, display_name, role, sso_provider, sso_subject,
		  is_active, created_at, last_seen, version)
		 VALUES (?, 'default', ?, ?, ?, 'pending', '', 1, ?, ?, ?)`,
		userID, req.Email, req.Name, req.Role, now, now, now.UnixMilli(),
	); err != nil {
		return util.InternalError(c, err)
	}

	// Generate invite token (7-day TTL).
	inviteToken, tokenErr := genRandomHex(32)
	inviteURL := ""
	if tokenErr == nil {
		expiresAt := now.Add(7 * 24 * time.Hour)
		_ = h.CH.Exec(ctx,
			`INSERT INTO invite_tokens (token, user_id, email, expires_at, used, version)
			 VALUES (?, ?, ?, ?, 0, ?)`,
			inviteToken, userID, req.Email, expiresAt, now.UnixMilli(),
		)
		if h.FrontendURL != "" {
			inviteURL = fmt.Sprintf("%s/accept-invite?token=%s", h.FrontendURL, inviteToken)
		}
	}

	// Send invite email if SMTP is configured.
	inviterName := h.SMTP.OrgName
	if callerEmail, ok := c.Locals("email").(string); ok && callerEmail != "" {
		inviterName = callerEmail
	}
	if h.SMTP.Host != "" && inviteURL != "" {
		body := inviteEmailHTML(h.SMTP.OrgName, inviterName, req.Role, inviteURL)
		if err := alerting.SendTransactional(h.SMTP, req.Email,
			fmt.Sprintf("You've been invited to %s", h.SMTP.OrgName), body); err != nil {
			slog.Warn("invite email failed", "email", req.Email, "err", err)
		}
	}

	slog.Info("member invited", "email", req.Email, "role", req.Role)
	resp := fiber.Map{
		"user_id":    userID,
		"email":      req.Email,
		"role":       req.Role,
		"invite_url": inviteURL, // returned for CLI/admin use when SMTP is absent
	}
	return c.Status(fiber.StatusCreated).JSON(resp)
}

// inviteEmailHTML builds the invite email body (defined in invite.go).
// Declared here to avoid circular imports — both files are in the same package.
func inviteEmailHTML(orgName, inviterName, role, link string) string {
	return fmt.Sprintf(`<!DOCTYPE html>
<html><body style="font-family:sans-serif;background:#0a0a14;color:#e2e8f0;padding:32px">
<div style="max-width:520px;margin:0 auto;background:#0d0d1a;border:1px solid rgba(255,255,255,0.08);
            border-radius:16px;padding:32px">
  <h2 style="color:#fff;margin:0 0 8px">You&apos;ve been invited to %s</h2>
  <p style="color:#94a3b8;margin:0 0 4px">%s has added you as a <strong style="color:#e2e8f0">%s</strong>.</p>
  <p style="color:#94a3b8;margin:0 0 24px">Click below to set your password and get started.</p>
  <a href="%s" style="display:inline-block;background:#4f46e5;color:#fff;text-decoration:none;
                       padding:12px 24px;border-radius:8px;font-weight:600">Accept invitation</a>
  <p style="color:#475569;font-size:12px;margin:24px 0 0">This invite link expires in 7 days.</p>
</div></body></html>`, orgName, inviterName, role, link)
}

func genRandomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", b), nil
}

// UpdateMemberRole handles PATCH /api/v1/enterprise/members/:id/role
func (h *EnterpriseHandler) UpdateMemberRole(c *fiber.Ctx) error {
	userID := c.Params("id")
	var req models.UpdateRoleRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid body"})
	}

	validRoles := map[string]bool{"owner": true, "admin": true, "analyst": true, "viewer": true}
	if !validRoles[req.Role] {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "role must be owner, admin, analyst, or viewer",
		})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Re-insert the row with updated role — ReplacingMergeTree will deduplicate.
	rows, err := h.CH.Query(ctx,
		`SELECT email, display_name, sso_provider, sso_subject, created_at
		 FROM org_members FINAL WHERE user_id = ? AND org_id = 'default' LIMIT 1`, userID)
	if err != nil {
		return util.InternalError(c, err)
	}
	defer rows.Close()

	if !rows.Next() {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "member not found"})
	}
	var email, displayName, ssoProvider, ssoSubject string
	var createdAt time.Time
	_ = rows.Scan(&email, &displayName, &ssoProvider, &ssoSubject, &createdAt)
	rows.Close()

	now := time.Now()
	if err := h.CH.Exec(ctx,
		`INSERT INTO org_members
		 (user_id, org_id, email, display_name, role, sso_provider, sso_subject,
		  is_active, created_at, last_seen, version)
		 VALUES (?, 'default', ?, ?, ?, ?, ?, 1, ?, ?, ?)`,
		userID, email, displayName, req.Role, ssoProvider, ssoSubject,
		createdAt, now, now.UnixMilli(),
	); err != nil {
		return util.InternalError(c, err)
	}

	// Invalidate all active sessions for this user — new role takes effect
	// on next login rather than silently continuing with a stale role.
	if h.Sessions != nil {
		if n := h.Sessions.DeleteByUserID(userID); n > 0 {
			slog.Info("sessions invalidated after role change",
				"user_id", userID, "new_role", req.Role, "sessions_revoked", n)
		}
	}

	return c.JSON(fiber.Map{"ok": true})
}

// RemoveMember handles DELETE /api/v1/enterprise/members/:id
func (h *EnterpriseHandler) RemoveMember(c *fiber.Ctx) error {
	userID := c.Params("id")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := h.CH.Query(ctx,
		`SELECT email, display_name, role, sso_provider, sso_subject, created_at
		 FROM org_members FINAL WHERE user_id = ? AND org_id = 'default' LIMIT 1`, userID)
	if err != nil {
		return util.InternalError(c, err)
	}
	defer rows.Close()

	if !rows.Next() {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "member not found"})
	}
	var email, displayName, role, ssoProvider, ssoSubject string
	var createdAt time.Time
	_ = rows.Scan(&email, &displayName, &role, &ssoProvider, &ssoSubject, &createdAt)
	rows.Close()

	if err := h.CH.Exec(ctx,
		`INSERT INTO org_members
		 (user_id, org_id, email, display_name, role, sso_provider, sso_subject, is_active, created_at, last_seen)
		 VALUES (?, 'default', ?, ?, ?, ?, ?, 0, ?, now64())`,
		userID, email, displayName, role, ssoProvider, ssoSubject, createdAt,
	); err != nil {
		return util.InternalError(c, err)
	}
	return c.JSON(fiber.Map{"ok": true})
}

// ── Teams ─────────────────────────────────────────────────────────────────────

// ListTeams handles GET /api/v1/enterprise/teams
func (h *EnterpriseHandler) ListTeams(c *fiber.Ctx) error {
	if h.CH == nil {
		return serviceUnavailable(c)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := h.CH.Query(ctx,
		`SELECT t.team_id, t.org_id, t.name, t.description, t.created_at,
		        count(tm.user_id) AS member_count
		 FROM teams t FINAL
		 LEFT JOIN team_members tm FINAL ON t.team_id = tm.team_id
		 WHERE t.org_id = 'default'
		 GROUP BY t.team_id, t.org_id, t.name, t.description, t.created_at
		 ORDER BY t.created_at ASC`)
	if err != nil {
		return util.InternalError(c, err)
	}
	defer rows.Close()

	teams := make([]models.Team, 0)
	for rows.Next() {
		var t models.Team
		if err := rows.Scan(
			&t.TeamID, &t.OrgID, &t.Name, &t.Description, &t.CreatedAt,
			&t.MemberCount,
		); err != nil {
			slog.Warn("scan team", "err", err)
			continue
		}
		teams = append(teams, t)
	}
	return c.JSON(fiber.Map{"teams": teams})
}

// CreateTeam handles POST /api/v1/enterprise/teams
func (h *EnterpriseHandler) CreateTeam(c *fiber.Ctx) error {
	var req models.CreateTeamRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid body"})
	}
	if req.Name == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "name is required"})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	teamID := uuid.NewString()
	if err := h.CH.Exec(ctx,
		`INSERT INTO teams (team_id, org_id, name, description, created_at)
		 VALUES (?, 'default', ?, ?, now64())`,
		teamID, req.Name, req.Description,
	); err != nil {
		return util.InternalError(c, err)
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"team_id": teamID})
}

// DeleteTeam handles DELETE /api/v1/enterprise/teams/:id
func (h *EnterpriseHandler) DeleteTeam(c *fiber.Ctx) error {
	teamID := c.Params("id")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// ClickHouse doesn't support DELETE; mark by re-inserting with empty name
	// and handle "name=''" as deleted in queries. Alternatively, use ALTER TABLE DELETE.
	if err := h.CH.Exec(ctx,
		`ALTER TABLE teams DELETE WHERE team_id = ? AND org_id = 'default'`, teamID,
	); err != nil {
		return util.InternalError(c, err)
	}
	if err := h.CH.Exec(ctx,
		`ALTER TABLE team_members DELETE WHERE team_id = ?`, teamID,
	); err != nil {
		slog.Warn("team member cleanup", "err", err)
	}
	return c.JSON(fiber.Map{"ok": true})
}

// ListTeamMembers handles GET /api/v1/enterprise/teams/:id/members
func (h *EnterpriseHandler) ListTeamMembers(c *fiber.Ctx) error {
	teamID := c.Params("id")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := h.CH.Query(ctx,
		`SELECT tm.user_id, tm.added_at, m.email, m.display_name, m.role
		 FROM team_members tm FINAL
		 JOIN org_members m FINAL ON tm.user_id = m.user_id
		 WHERE tm.team_id = ? AND m.is_active = 1`, teamID)
	if err != nil {
		return util.InternalError(c, err)
	}
	defer rows.Close()

	members := make([]models.TeamMember, 0)
	for rows.Next() {
		var m models.TeamMember
		m.TeamID = teamID
		if err := rows.Scan(&m.UserID, &m.AddedAt, &m.Email, &m.DisplayName, &m.Role); err != nil {
			continue
		}
		members = append(members, m)
	}
	return c.JSON(fiber.Map{"members": members})
}

// AddTeamMember handles POST /api/v1/enterprise/teams/:id/members
func (h *EnterpriseHandler) AddTeamMember(c *fiber.Ctx) error {
	teamID := c.Params("id")
	var req models.AddTeamMemberRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid body"})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := h.CH.Exec(ctx,
		`INSERT INTO team_members (team_id, user_id, org_id, added_at)
		 VALUES (?, ?, 'default', now64())`,
		teamID, req.UserID,
	); err != nil {
		return util.InternalError(c, err)
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"ok": true})
}

// RemoveTeamMember handles DELETE /api/v1/enterprise/teams/:id/members/:uid
func (h *EnterpriseHandler) RemoveTeamMember(c *fiber.Ctx) error {
	teamID := c.Params("id")
	userID := c.Params("uid")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := h.CH.Exec(ctx,
		`ALTER TABLE team_members DELETE WHERE team_id = ? AND user_id = ?`,
		teamID, userID,
	); err != nil {
		return util.InternalError(c, err)
	}
	return c.JSON(fiber.Map{"ok": true})
}

// ── SSO Config ────────────────────────────────────────────────────────────────

// GetSSOConfig handles GET /api/v1/enterprise/sso/config
func (h *EnterpriseHandler) GetSSOConfig(c *fiber.Ctx) error {
	if h.CH == nil {
		return serviceUnavailable(c)
	}
	if !h.License.HasFeature(license.FeatureSSO) {
		return c.Status(fiber.StatusPaymentRequired).JSON(fiber.Map{
			"error":    "SSO requires Team or Enterprise plan",
			"upgrade":  true,
			"feature":  license.FeatureSSO,
		})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := h.CH.Query(ctx,
		`SELECT provider, enabled, entity_id, sso_url, certificate,
		        issuer_url, client_id, updated_at
		 FROM sso_configs FINAL
		 WHERE org_id = 'default'
		 ORDER BY updated_at DESC
		 LIMIT 1`)
	if err != nil {
		return util.InternalError(c, err)
	}
	defer rows.Close()

	if !rows.Next() {
		return c.JSON(models.SSOConfig{OrgID: "default"})
	}

	var cfg models.SSOConfig
	var enabled uint8
	cfg.OrgID = "default"
	_ = rows.Scan(
		&cfg.Provider, &enabled, &cfg.EntityID, &cfg.SSOURL,
		&cfg.Certificate, &cfg.IssuerURL, &cfg.ClientID, &cfg.UpdatedAt,
	)
	cfg.Enabled = enabled == 1
	return c.JSON(cfg)
}

// UpdateSSOConfig handles PUT /api/v1/enterprise/sso/config
func (h *EnterpriseHandler) UpdateSSOConfig(c *fiber.Ctx) error {
	if !h.License.HasFeature(license.FeatureSSO) {
		return c.Status(fiber.StatusPaymentRequired).JSON(fiber.Map{
			"error": "SSO requires Team or Enterprise plan", "upgrade": true,
		})
	}

	var req models.UpdateSSOConfigRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid body"})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var enabled uint8
	if req.Enabled {
		enabled = 1
	}
	if err := h.CH.Exec(ctx,
		`INSERT INTO sso_configs
		 (org_id, provider, enabled, entity_id, sso_url, certificate,
		  issuer_url, client_id, updated_at)
		 VALUES ('default', ?, ?, ?, ?, ?, ?, ?, now64())`,
		req.Provider, enabled, req.EntityID, req.SSOURL, req.Certificate,
		req.IssuerURL, req.ClientID,
	); err != nil {
		return util.InternalError(c, err)
	}

	// If client_secret was provided, log that it should be set as SSO_CLIENT_SECRET env var.
	// We deliberately do not persist secrets in ClickHouse.
	if req.ClientSecret != "" {
		slog.Info("SSO client_secret provided — store as SSO_CLIENT_SECRET env var; not persisted in DB")
	}

	return c.JSON(fiber.Map{"ok": true})
}

// ── License ───────────────────────────────────────────────────────────────────

// GetLicense handles GET /api/v1/enterprise/license
func (h *EnterpriseHandler) GetLicense(c *fiber.Ctx) error {
	l := h.License
	featureList := make([]string, 0, len(l.Features))
	for f := range l.Features {
		featureList = append(featureList, f)
	}

	resp := fiber.Map{
		"valid":       l.Valid,
		"expired":     l.Expired,
		"plan":        l.Plan,
		"plan_badge":  l.PlanBadge(),
		"org_id":      l.OrgID,
		"org_name":    l.OrgName,
		"agent_quota": l.AgentQuota,
		"features":    featureList,
	}
	if !l.ExpiresAt.IsZero() {
		resp["expires_at"] = l.ExpiresAt
	}
	return c.JSON(resp)
}

// ── helpers ───────────────────────────────────────────────────────────────────

func serviceUnavailable(c *fiber.Ctx) error {
	return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
		"error": "ClickHouse is not available",
	})
}
