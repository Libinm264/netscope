package middleware

import (
	"context"
	"log/slog"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	chclient "github.com/netscope/hub-api/clickhouse"
)

// AuditLog returns a Fiber middleware that records every authenticated API
// request to the audit_events table.  It runs after TokenAuth so that
// c.Locals("role") and c.Locals("token_id") are already set.
//
// The write is fire-and-forget (goroutine) to avoid adding latency to the
// hot path.  If ClickHouse is unavailable the event is logged to stderr only.
func AuditLog(ch *chclient.Client) fiber.Handler {
	return func(c *fiber.Ctx) error {
		start := time.Now()
		err := c.Next() // run the actual handler first

		role, _    := c.Locals("role").(string)
		tokenID, _ := c.Locals("token_id").(string)
		userID, _  := c.Locals("user_id").(string)
		if role == "" {
			// Unauthenticated or public route — skip auditing.
			return err
		}
		// For session-based callers the token_id is the user_id prefixed with "sess:".
		if tokenID == "" && userID != "" {
			tokenID = "sess:" + userID
		}

		status   := c.Response().StatusCode()
		method   := c.Method()
		path     := c.Path()
		clientIP := c.IP()
		latencyMs := time.Since(start).Milliseconds()
		eventID  := uuid.New().String()
		now      := time.Now().UTC()

		slog.Debug("audit", "token_id", tokenID, "role", role,
			"method", method, "path", path, "status", status,
			"ip", clientIP, "latency_ms", latencyMs)

		if ch != nil {
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				if writeErr := ch.Exec(ctx,
					`INSERT INTO audit_events
					 (id, token_id, role, method, path, status, client_ip, latency_ms, ts)
					 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
					eventID, tokenID, role, method, path,
					uint16(status), clientIP, uint32(latencyMs), now,
				); writeErr != nil {
					slog.Warn("audit: write failed", "err", writeErr)
				}
			}()
		}

		return err
	}
}
