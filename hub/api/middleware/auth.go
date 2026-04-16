package middleware

import (
	"context"
	"time"

	"github.com/gofiber/fiber/v2"

	chclient "github.com/netscope/hub-api/clickhouse"
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
					rows.Scan(&tokenID, &role)
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
					return c.Next()
				}
				rows.Close()
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
