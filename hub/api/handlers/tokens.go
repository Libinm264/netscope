package handlers

import (
	"log/slog"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/netscope/hub-api/clickhouse"
	"github.com/netscope/hub-api/models"
)

// TokenHandler manages API access tokens (RBAC).
type TokenHandler struct {
	CH *clickhouse.Client
}

// List handles GET /api/v1/tokens.
func (h *TokenHandler) List(c *fiber.Ctx) error {
	if h.CH == nil {
		return c.Status(503).JSON(fiber.Map{"error": "ClickHouse unavailable"})
	}

	rows, err := h.CH.Query(c.Context(),
		`SELECT id, name, role, token, created_at, last_used, revoked
		 FROM api_tokens FINAL
		 ORDER BY created_at DESC`)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	defer rows.Close()

	tokens := make([]models.APIToken, 0)
	for rows.Next() {
		var t models.APIToken
		var revokedInt uint8
		if err := rows.Scan(
			&t.ID, &t.Name, &t.Role, &t.Token,
			&t.CreatedAt, &t.LastUsed, &revokedInt,
		); err != nil {
			continue
		}
		t.Revoked = revokedInt == 1
		// Mask the token after creation — only show last 4 chars
		if len(t.Token) > 4 {
			t.Token = "••••••••-" + t.Token[len(t.Token)-4:]
		}
		tokens = append(tokens, t)
	}
	return c.JSON(fiber.Map{"tokens": tokens})
}

// Create handles POST /api/v1/tokens.
// Returns the full token exactly once — it cannot be retrieved again.
func (h *TokenHandler) Create(c *fiber.Ctx) error {
	if h.CH == nil {
		return c.Status(503).JSON(fiber.Map{"error": "ClickHouse unavailable"})
	}

	var req models.CreateTokenRequest
	if err := c.BodyParser(&req); err != nil || req.Name == "" {
		return c.Status(400).JSON(fiber.Map{"error": "name is required"})
	}
	if req.Role != "admin" && req.Role != "viewer" {
		req.Role = "viewer"
	}

	id := uuid.New().String()
	token := uuid.New().String()
	now := time.Now().UTC()

	if err := h.CH.Exec(c.Context(),
		`INSERT INTO api_tokens (id, name, role, token, created_at, last_used, revoked)
		 VALUES (?, ?, ?, ?, ?, ?, 0)`,
		id, req.Name, req.Role, token, now, now,
	); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	slog.Info("api token created", "name", req.Name, "role", req.Role)
	// Return the full token in the creation response only
	return c.Status(201).JSON(models.APIToken{
		ID: id, Name: req.Name, Role: req.Role, Token: token, CreatedAt: now,
	})
}

// Revoke handles DELETE /api/v1/tokens/:id.
func (h *TokenHandler) Revoke(c *fiber.Ctx) error {
	if h.CH == nil {
		return c.Status(503).JSON(fiber.Map{"error": "ClickHouse unavailable"})
	}
	id := c.Params("id")
	if err := h.CH.Exec(c.Context(),
		`INSERT INTO api_tokens (id, name, role, token, created_at, last_used, revoked)
		 SELECT id, name, role, token, created_at, last_used, 1
		 FROM api_tokens FINAL WHERE id = ? LIMIT 1`, id,
	); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"revoked": true})
}
