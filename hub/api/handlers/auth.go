package handlers

import (
	"log/slog"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/netscope/hub-api/sessions"
)

// AuthHandler provides the /me and /logout enterprise auth endpoints.
// The login and SSO callback endpoints are added in Pass B (OIDC/SAML handlers).
type AuthHandler struct {
	Sessions    *sessions.Store
	FrontendURL string // e.g. "http://localhost:3000" — used for post-SSO redirects
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
