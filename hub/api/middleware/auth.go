package middleware

import (
	"context"
	"time"

	"github.com/gofiber/fiber/v2"

	chclient "github.com/netscope/hub-api/clickhouse"
	"github.com/netscope/hub-api/sessions"
)

// TokenAuth returns a Fiber middleware that validates API keys against:
//  1. The bootstrap admin key (from config/env) — always admin role
//  2. Rows in the api_tokens table — viewer or admin role
//
// The resolved role ("admin" | "viewer") is stored in c.Locals("role").
func TokenAuth(bootstrapKey string, ch *chclient.Client) fiber.Handler {
	return func(c *fiber.Ctx) error {
		key := c.Get("X-Api-Key")
		if key == "" {
			key = c.Query("api_key")
		}
		if key == "" {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "unauthorized: missing API key",
			})
		}

		// Bootstrap key is always admin
		if key == bootstrapKey {
			c.Locals("role", "admin")
			return c.Next()
		}

		// Look up in api_tokens table
		if ch != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()

			rows, err := ch.Query(ctx,
				`SELECT id, role FROM api_tokens FINAL
				 WHERE token = ? AND revoked = 0 LIMIT 1`, key)
			if err == nil {
				if rows.Next() {
					var tokenID, role string
					if scanErr := rows.Scan(&tokenID, &role); scanErr != nil {
						rows.Close()
						// Scan failure — treat as invalid token rather than granting access
					} else {
						rows.Close()

						// Update last_used asynchronously
						go func() {
							_ = ch.Exec(context.Background(),
								`INSERT INTO api_tokens (id, name, role, token, created_at, last_used, revoked)
								 SELECT id, name, role, token, created_at, ?, revoked
								 FROM api_tokens FINAL WHERE id = ? LIMIT 1`,
								time.Now().UTC(), tokenID)
						}()

						c.Locals("role", role)
						c.Locals("token_id", tokenID)
						return c.Next()
					}
				} else {
					rows.Close()
				}
			}
		}

		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "unauthorized: invalid API key",
		})
	}
}

// RequireAdmin rejects requests from viewer-role tokens with 403.
// Place after TokenAuth in the middleware chain.
func RequireAdmin() fiber.Handler {
	return func(c *fiber.Ctx) error {
		role, _ := c.Locals("role").(string)
		if role != "admin" {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"error": "forbidden: admin role required",
			})
		}
		return c.Next()
	}
}

// APIKeyAuth is kept for backwards-compatibility — wraps TokenAuth without CH.
func APIKeyAuth(apiKey string) fiber.Handler {
	return TokenAuth(apiKey, nil)
}

// ── Enterprise session-aware auth ─────────────────────────────────────────────

// EnterpriseAuth is the middleware for enterprise routes.
// Priority order:
//  1. ns_session cookie — session role ("owner"|"admin"|"analyst"|"viewer")
//  2. X-Api-Key / api_key — bootstrap key gets "owner", token table sets role
//
// Locals set: "role", "user_id" (session) or "token_id" (API key), "auth_method".
func EnterpriseAuth(bootstrapKey string, ch *chclient.Client, sess *sessions.Store) fiber.Handler {
	return func(c *fiber.Ctx) error {
		// 1. Session cookie takes precedence — carries the actual user's role.
		if sess != nil {
			if token := c.Cookies("ns_session"); token != "" {
				if s, ok := sess.Get(token); ok {
					c.Locals("role", s.Role)
					c.Locals("user_id", s.UserID)
					c.Locals("email", s.Email)
					c.Locals("org_id", s.OrgID)
					c.Locals("auth_method", "session")
					return c.Next()
				}
			}
		}

		// 2. API key fallback (agents, CLI, server-to-server).
		key := c.Get("X-Api-Key")
		if key == "" {
			key = c.Query("api_key")
		}
		if key == "" {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "not authenticated — provide X-Api-Key or a valid session cookie",
			})
		}

		if bootstrapKey != "" && key == bootstrapKey {
			c.Locals("role", "owner")
			c.Locals("auth_method", "api_key")
			return c.Next()
		}

		if ch != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			rows, err := ch.Query(ctx,
				`SELECT id, role FROM api_tokens FINAL
				 WHERE token = ? AND revoked = 0 LIMIT 1`, key)
			if err == nil {
				if rows.Next() {
					var tokenID, role string
					if scanErr := rows.Scan(&tokenID, &role); scanErr == nil {
						rows.Close()
						go func() {
							_ = ch.Exec(context.Background(),
								`INSERT INTO api_tokens (id, name, role, token, created_at, last_used, revoked)
								 SELECT id, name, role, token, created_at, ?, revoked
								 FROM api_tokens FINAL WHERE id = ? LIMIT 1`,
								time.Now().UTC(), tokenID)
						}()
						c.Locals("role", role)
						c.Locals("token_id", tokenID)
						c.Locals("auth_method", "api_key")
						return c.Next()
					}
				}
				rows.Close()
			}
		}

		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "unauthorized: invalid API key",
		})
	}
}

// RequireAdminOrAbove allows "owner" and "admin" roles through.
// Use after EnterpriseAuth for write operations on enterprise resources.
func RequireAdminOrAbove() fiber.Handler {
	return func(c *fiber.Ctx) error {
		role, _ := c.Locals("role").(string)
		switch role {
		case "owner", "admin":
			return c.Next()
		default:
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"error": "forbidden: admin role or above required",
			})
		}
	}
}

// RequireOwner allows only the "owner" role (e.g. license management, org deletion).
func RequireOwner() fiber.Handler {
	return func(c *fiber.Ctx) error {
		if role, _ := c.Locals("role").(string); role != "owner" {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"error": "forbidden: owner role required",
			})
		}
		return c.Next()
	}
}
