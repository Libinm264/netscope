package util

import (
	"log/slog"

	"github.com/gofiber/fiber/v2"
)

// InternalError logs the full error server-side (with path context) and returns
// a generic "internal server error" message to the HTTP client.
//
// This prevents leaking database schema names, query text, or internal stack
// details to potential attackers while still preserving full diagnostics in
// structured server logs.
func InternalError(c *fiber.Ctx, err error) error {
	slog.Error("handler error",
		"method", c.Method(),
		"path",   c.Path(),
		"err",    err,
	)
	return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
		"error": "internal server error",
	})
}
