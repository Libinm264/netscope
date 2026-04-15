package middleware

import "github.com/gofiber/fiber/v2"

// APIKeyAuth returns a Fiber middleware that checks the X-Api-Key header (or
// ?api_key query param for SSE clients that can't set custom headers).
func APIKeyAuth(apiKey string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		key := c.Get("X-Api-Key")
		if key == "" {
			key = c.Query("api_key")
		}
		if key == "" || key != apiKey {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "unauthorized: invalid or missing API key",
			})
		}
		return c.Next()
	}
}
