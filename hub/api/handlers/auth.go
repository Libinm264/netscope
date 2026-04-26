package handlers

import (
	"context"
	"log/slog"
	"time"

	"github.com/gofiber/fiber/v2"
	"golang.org/x/crypto/bcrypt"

	"github.com/netscope/hub-api/clickhouse"
	"github.com/netscope/hub-api/sessions"
)

// AuthHandler provides the /me, /logout, /login, and /password endpoints.
type AuthHandler struct {
	CH          *clickhouse.Client
	Sessions    *sessions.Store
	FrontendURL string // e.g. "http://localhost:3000"
}

const sessionCookieName = "ns_session"

// Me handles GET /api/v1/enterprise/auth/me
//
// Returns the identity of the currently authenticated user by reading the
// ns_session cookie and looking it up in the session store.
// Returns 401 when no session is found (unauthenticated or expired).
func (h *AuthHandler) Me(c *fiber.Ctx) error {
	token := c.Cookies(sessionCookieName)
	if token == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error":         "not authenticated",
			"authenticated": false,
		})
	}

	sess, ok := h.Sessions.Get(token)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error":         "session expired — please sign in again",
			"authenticated": false,
		})
	}

	return c.JSON(fiber.Map{
		"authenticated": true,
		"user_id":       sess.UserID,
		"email":         sess.Email,
		"display_name":  sess.DisplayName,
		"role":          sess.Role,
		"org_id":        sess.OrgID,
		"sso_provider":  sess.SSOProvider,
		"expires_at":    sess.ExpiresAt,
	})
}

// Logout handles POST /api/v1/enterprise/auth/logout
//
// Removes the session from the store and clears the ns_session cookie.
// Always returns 200 — idempotent (logging out twice is not an error).
func (h *AuthHandler) Logout(c *fiber.Ctx) error {
	token := c.Cookies(sessionCookieName)
	if token != "" {
		h.Sessions.Delete(token)
		slog.Info("user logged out", "token_prefix", token[:8])
	}

	// Clear the cookie by setting an expired value
	c.Cookie(&fiber.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Expires:  time.Unix(0, 0),
		HTTPOnly: true,
		SameSite: "Lax",
		Path:     "/",
	})

	return c.JSON(fiber.Map{"ok": true, "message": "signed out"})
}

// ── Local email/password login ────────────────────────────────────────────────

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// LocalLogin handles POST /api/v1/enterprise/auth/login
//
// Validates email + password against the local_credentials table, creates a
// session, sets the ns_session cookie, and returns user info.
func (h *AuthHandler) LocalLogin(c *fiber.Ctx) error {
	if h.CH == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "database unavailable"})
	}

	var req loginRequest
	if err := c.BodyParser(&req); err != nil || req.Email == "" || req.Password == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "email and password are required"})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Fetch user identity from org_members.
	var userID, role, displayName string
	rows, err := h.CH.Query(ctx,
		`SELECT user_id, role, display_name
		 FROM org_members FINAL
		 WHERE org_id = 'default' AND email = ? AND is_active = 1
		 LIMIT 1`, req.Email)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "database error"})
	}
	if rows.Next() {
		_ = rows.Scan(&userID, &role, &displayName)
	}
	rows.Close()

	if userID == "" {
		// Constant-time response — don't reveal whether email exists.
		_ = bcrypt.CompareHashAndPassword([]byte("$2a$12$placeholder"), []byte(req.Password))
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "invalid email or password"})
	}

	// Fetch stored password hash.
	var hash string
	rows2, err := h.CH.Query(ctx,
		`SELECT password_hash FROM local_credentials FINAL
		 WHERE org_id = 'default' AND user_id = ?
		 ORDER BY updated_at DESC LIMIT 1`, userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "database error"})
	}
	if rows2.Next() {
		_ = rows2.Scan(&hash)
	}
	rows2.Close()

	if hash == "" {
		// No local credentials — user must log in via SSO.
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "no local password set — use SSO to sign in",
		})
	}

	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(req.Password)); err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "invalid email or password"})
	}

	if displayName == "" {
		displayName = req.Email
	}

	expiresAt := time.Now().Add(sessions.DefaultTTL)
	sessionToken := h.Sessions.Create(sessions.Session{
		UserID:      userID,
		OrgID:       "default",
		Email:       req.Email,
		DisplayName: displayName,
		Role:        role,
		SSOProvider: "local",
		CreatedAt:   time.Now(),
		ExpiresAt:   expiresAt,
	})

	c.Cookie(&fiber.Cookie{
		Name:     sessionCookieName,
		Value:    sessionToken,
		Path:     "/",
		HTTPOnly: true,
		SameSite: "Lax",
		Expires:  expiresAt,
	})

	slog.Info("local login successful", "email", req.Email, "role", role)
	return c.JSON(fiber.Map{
		"ok":           true,
		"user_id":      userID,
		"email":        req.Email,
		"display_name": displayName,
		"role":         role,
	})
}

// ── Password management ───────────────────────────────────────────────────────

type setPasswordRequest struct {
	UserID      string `json:"user_id"`   // admin: set someone else's password
	Password    string `json:"password"`   // new password (plain — hashed server-side)
	OldPassword string `json:"old_password"` // required when changing own password
}

// SetPassword handles PUT /api/v1/enterprise/auth/password
//
// Two modes:
//   - Authenticated admin (X-Api-Key): can set any user's password via user_id.
//   - Session user: can change their own password by providing old_password.
func (h *AuthHandler) SetPassword(c *fiber.Ctx) error {
	if h.CH == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "database unavailable"})
	}

	var req setPasswordRequest
	if err := c.BodyParser(&req); err != nil || req.Password == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "password is required"})
	}
	if len(req.Password) < 12 {
		return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{
			"error": "password must be at least 12 characters",
		})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	targetUserID := req.UserID

	// If no user_id specified, apply to the currently-authenticated session user.
	if targetUserID == "" {
		token := c.Cookies(sessionCookieName)
		if token == "" {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "not authenticated"})
		}
		sess, ok := h.Sessions.Get(token)
		if !ok {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "session expired"})
		}
		targetUserID = sess.UserID

		// Changing own password requires verifying the current one.
		if req.OldPassword != "" {
			var existingHash string
			rows, err := h.CH.Query(ctx,
				`SELECT password_hash FROM local_credentials FINAL
				 WHERE org_id = 'default' AND user_id = ?
				 ORDER BY updated_at DESC LIMIT 1`, targetUserID)
			if err == nil {
				if rows.Next() {
					_ = rows.Scan(&existingHash)
				}
				rows.Close()
			}
			if existingHash != "" {
				if err := bcrypt.CompareHashAndPassword([]byte(existingHash), []byte(req.OldPassword)); err != nil {
					return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "current password is incorrect"})
				}
			}
		}
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "password hashing failed"})
	}

	now := time.Now()
	if err := h.CH.Exec(ctx,
		`INSERT INTO local_credentials (user_id, org_id, password_hash, updated_at, version)
		 VALUES (?, 'default', ?, ?, ?)`,
		targetUserID, string(hash), now, now.UnixMilli(),
	); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "could not save password"})
	}

	slog.Info("password updated", "user_id", targetUserID)
	return c.JSON(fiber.Map{"ok": true})
}
