package handlers

import (
	"context"
	"crypto/rand"
	"fmt"
	"log/slog"
	"time"

	"github.com/gofiber/fiber/v2"
	"golang.org/x/crypto/bcrypt"

	"github.com/netscope/hub-api/alerting"
	"github.com/netscope/hub-api/clickhouse"
	"github.com/netscope/hub-api/sessions"
)

// InviteHandler handles invite acceptance and password reset flows.
// These endpoints are public (no prior auth required) because the caller
// is identified by a short-lived single-use token.
type InviteHandler struct {
	CH           *clickhouse.Client
	Sessions     *sessions.Store
	SMTP         alerting.SMTPConfig
	FrontendURL  string
	SecureCookie bool
}

// ── Invite acceptance ─────────────────────────────────────────────────────────

type acceptInviteRequest struct {
	Token    string `json:"token"`
	Password string `json:"password"`
}

// AcceptInvite handles POST /api/v1/enterprise/auth/invite/accept
//
// Validates the invite token, sets the user's password, creates a session,
// and returns a Set-Cookie header so the browser is immediately logged in.
func (h *InviteHandler) AcceptInvite(c *fiber.Ctx) error {
	if h.CH == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "database unavailable"})
	}

	var req acceptInviteRequest
	if err := c.BodyParser(&req); err != nil || req.Token == "" || req.Password == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "token and password are required"})
	}
	if len(req.Password) < 12 {
		return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{
			"error": "password must be at least 12 characters",
		})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	// Look up and validate the invite token.
	var userID, email string
	// Avoid FINAL: ORDER BY version DESC returns the latest row for this token
	// without waiting for a background merge (ClickHouse 24.x race condition).
	rows, err := h.CH.Query(ctx,
		`SELECT user_id, email FROM invite_tokens
		 WHERE token = ? AND used = 0 AND expires_at > now64()
		 ORDER BY version DESC LIMIT 1`, req.Token)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "database error"})
	}
	if rows.Next() {
		_ = rows.Scan(&userID, &email)
	}
	rows.Close()

	if userID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "invite link is invalid or has expired",
		})
	}

	// Hash and store the password.
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "password hashing failed"})
	}
	now := time.Now()
	if err := h.CH.Exec(ctx,
		`INSERT INTO local_credentials (user_id, org_id, password_hash, updated_at, version)
		 VALUES (?, 'default', ?, ?, ?)`,
		userID, string(hash), now, now.UnixMilli(),
	); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "could not save password"})
	}

	// Mark the invite token as used and update sso_provider to 'local'.
	_ = h.CH.Exec(ctx,
		`INSERT INTO invite_tokens (token, user_id, email, expires_at, used, version)
		 VALUES (?, ?, ?, now64(), 1, ?)`,
		req.Token, userID, email, now.UnixMilli(),
	)
	_ = h.CH.Exec(ctx,
		`INSERT INTO org_members
		 (user_id, org_id, email, display_name, role, sso_provider, sso_subject, is_active, created_at, last_seen, version)
		 SELECT user_id, org_id, email, display_name, role, 'local', '', is_active, created_at, ?, ?
		 FROM org_members WHERE user_id = ? ORDER BY last_seen DESC LIMIT 1`,
		now, now.UnixMilli(), userID,
	)

	// Fetch display name + role for session.
	var displayName, role string
	rows2, err := h.CH.Query(ctx,
		`SELECT display_name, role FROM org_members
		 WHERE user_id = ? ORDER BY last_seen DESC LIMIT 1`, userID)
	if err == nil {
		if rows2.Next() {
			_ = rows2.Scan(&displayName, &role)
		}
		rows2.Close()
	}
	if displayName == "" {
		displayName = email
	}
	if role == "" {
		role = "viewer"
	}

	expiresAt := now.Add(sessions.DefaultTTL)
	sessionToken := h.Sessions.Create(sessions.Session{
		UserID:      userID,
		OrgID:       "default",
		Email:       email,
		DisplayName: displayName,
		Role:        role,
		SSOProvider: "local",
		CreatedAt:   now,
		ExpiresAt:   expiresAt,
	})

	c.Cookie(&fiber.Cookie{
		Name:     "ns_session",
		Value:    sessionToken,
		Path:     "/",
		HTTPOnly: true,
		SameSite: "Lax",
		Secure:   h.SecureCookie,
		Expires:  expiresAt,
	})

	slog.Info("invite accepted", "email", email, "user_id", userID)
	return c.JSON(fiber.Map{"ok": true, "email": email, "role": role})
}

// ── Password reset ────────────────────────────────────────────────────────────

type forgotPasswordRequest struct {
	Email string `json:"email"`
}

// ForgotPassword handles POST /api/v1/enterprise/auth/forgot-password
//
// Generates a time-limited reset token and emails a link to the user.
// Always returns 200 (doesn't reveal whether the email exists).
func (h *InviteHandler) ForgotPassword(c *fiber.Ctx) error {
	if h.CH == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "database unavailable"})
	}

	var req forgotPasswordRequest
	if err := c.BodyParser(&req); err != nil || req.Email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "email is required"})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Find user — but always return 200 to avoid email enumeration.
	var userID, displayName string
	rows, err := h.CH.Query(ctx,
		`SELECT user_id, display_name FROM org_members
		 WHERE org_id = 'default' AND email = ? AND is_active = 1
		 ORDER BY last_seen DESC LIMIT 1`, req.Email)
	if err == nil {
		if rows.Next() {
			_ = rows.Scan(&userID, &displayName)
		}
		rows.Close()
	}

	if userID != "" {
		token, genErr := generateSecureToken()
		if genErr == nil {
			expiresAt := time.Now().Add(time.Hour)
			_ = h.CH.Exec(ctx,
				`INSERT INTO password_reset_tokens (token, user_id, email, expires_at, used, version)
				 VALUES (?, ?, ?, ?, 0, ?)`,
				token, userID, req.Email, expiresAt, time.Now().UnixMilli(),
			)

			if h.SMTP.Host != "" {
				link := fmt.Sprintf("%s/reset-password?token=%s", h.FrontendURL, token)
				name := displayName
				if name == "" {
					name = req.Email
				}
				body := passwordResetEmailHTML(h.SMTP.OrgName, name, link)
				if err := alerting.SendTransactional(h.SMTP, req.Email,
					fmt.Sprintf("[%s] Reset your password", h.SMTP.OrgName), body); err != nil {
					slog.Warn("password reset email failed", "email", req.Email, "err", err)
				}
			} else {
				slog.Info("password reset token generated (SMTP not configured)",
					"email", req.Email, "token", token)
			}
		}
	}

	// Always 200 — don't reveal whether the email is registered.
	return c.JSON(fiber.Map{"ok": true, "message": "If that email is registered you will receive a reset link shortly"})
}

type resetPasswordRequest struct {
	Token    string `json:"token"`
	Password string `json:"password"`
}

// ResetPassword handles POST /api/v1/enterprise/auth/reset-password
//
// Validates the reset token and updates the user's password.
func (h *InviteHandler) ResetPassword(c *fiber.Ctx) error {
	if h.CH == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "database unavailable"})
	}

	var req resetPasswordRequest
	if err := c.BodyParser(&req); err != nil || req.Token == "" || req.Password == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "token and password are required"})
	}
	if len(req.Password) < 12 {
		return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{
			"error": "password must be at least 12 characters",
		})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	var userID, email string
	// Avoid FINAL: ORDER BY version DESC to get the latest row without background merge.
	rows, err := h.CH.Query(ctx,
		`SELECT user_id, email FROM password_reset_tokens
		 WHERE token = ? AND used = 0 AND expires_at > now64()
		 ORDER BY version DESC LIMIT 1`, req.Token)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "database error"})
	}
	if rows.Next() {
		_ = rows.Scan(&userID, &email)
	}
	rows.Close()

	if userID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "reset link is invalid or has expired",
		})
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "password hashing failed"})
	}

	now := time.Now()
	if err := h.CH.Exec(ctx,
		`INSERT INTO local_credentials (user_id, org_id, password_hash, updated_at, version)
		 VALUES (?, 'default', ?, ?, ?)`,
		userID, string(hash), now, now.UnixMilli(),
	); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "could not save password"})
	}

	// Mark token as used.
	_ = h.CH.Exec(ctx,
		`INSERT INTO password_reset_tokens (token, user_id, email, expires_at, used, version)
		 VALUES (?, ?, ?, now64(), 1, ?)`,
		req.Token, userID, email, now.UnixMilli(),
	)

	// Invalidate all existing sessions for this user (security: old sessions
	// remain valid only if the password change was intentional by the owner).
	if h.Sessions != nil {
		revoked := h.Sessions.DeleteByUserID(userID)
		if revoked > 0 {
			slog.Info("sessions revoked after password reset", "user_id", userID, "count", revoked)
		}
	}

	slog.Info("password reset completed", "email", email, "user_id", userID)
	return c.JSON(fiber.Map{"ok": true, "message": "Password updated — please sign in with your new password"})
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func generateSecureToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", b), nil
}

func passwordResetEmailHTML(orgName, name, link string) string {
	return fmt.Sprintf(`<!DOCTYPE html>
<html>
<body style="font-family:sans-serif;background:#0a0a14;color:#e2e8f0;padding:32px">
<div style="max-width:520px;margin:0 auto;background:#0d0d1a;border:1px solid rgba(255,255,255,0.08);
            border-radius:16px;padding:32px">
  <h2 style="color:#fff;margin:0 0 8px">Reset your %s password</h2>
  <p style="color:#94a3b8;margin:0 0 24px">Hi %s, click the button below to set a new password.
  This link expires in 1 hour.</p>
  <a href="%s" style="display:inline-block;background:#4f46e5;color:#fff;text-decoration:none;
                       padding:12px 24px;border-radius:8px;font-weight:600">Reset password</a>
  <p style="color:#475569;font-size:12px;margin:24px 0 0">
    If you didn&apos;t request this, you can safely ignore this email.
  </p>
</div>
</body>
</html>`, orgName, name, link)
}

// inviteEmailHTML is defined in orgs.go (same package).
