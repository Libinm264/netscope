package middleware

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/limiter"
)

// RateLimit returns a Fiber middleware that caps requests per API key.
//
//   - max       maximum requests allowed in the window
//   - window    length of the sliding window
func RateLimit(max int, window time.Duration) fiber.Handler {
	return limiter.New(limiter.Config{
		Max:        max,
		Expiration: window,
		// Key per API key so different agents share their own bucket.
		KeyGenerator: func(c *fiber.Ctx) string {
			if key := c.Get("X-Api-Key"); key != "" {
				return "key:" + key
			}
			return "ip:" + c.IP()
		},
		LimitReached: func(c *fiber.Ctx) error {
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
				"error": "rate limit exceeded — back off and retry",
			})
		},
		// Store per-instance (in-memory); swap for Redis in multi-replica setups.
		Storage: nil,
	})
}
