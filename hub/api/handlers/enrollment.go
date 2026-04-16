package handlers

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/netscope/hub-api/clickhouse"
	"github.com/netscope/hub-api/config"
	"github.com/netscope/hub-api/models"
)

// EnrollmentHandler manages enrollment tokens and the agent enrol flow.
type EnrollmentHandler struct {
	CH  *clickhouse.Client
	Cfg *config.Config
}

// ListTokens handles GET /api/v1/enrollment-tokens.
func (h *EnrollmentHandler) ListTokens(c *fiber.Ctx) error {
	if h.CH == nil {
		return c.Status(503).JSON(fiber.Map{"error": "ClickHouse unavailable"})
	}

	rows, err := h.CH.Query(c.Context(),
		`SELECT id, name, token, created_at, expires_at, used_count, revoked
		 FROM enrollment_tokens
		 ORDER BY created_at DESC`)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	defer rows.Close()

	tokens := make([]models.EnrollmentToken, 0)
	for rows.Next() {
		var t models.EnrollmentToken
		var revokedInt uint8
		if err := rows.Scan(
			&t.ID, &t.Name, &t.Token,
			&t.CreatedAt, &t.ExpiresAt, &t.UsedCount, &revokedInt,
		); err != nil {
			continue
		}
		t.Revoked = revokedInt == 1
		tokens = append(tokens, t)
	}
	return c.JSON(fiber.Map{"tokens": tokens})
}

// CreateToken handles POST /api/v1/enrollment-tokens.
func (h *EnrollmentHandler) CreateToken(c *fiber.Ctx) error {
	if h.CH == nil {
		return c.Status(503).JSON(fiber.Map{"error": "ClickHouse unavailable"})
	}

	var req models.CreateEnrollmentTokenRequest
	if err := c.BodyParser(&req); err != nil || req.Name == "" {
		return c.Status(400).JSON(fiber.Map{"error": "name is required"})
	}

	expiresIn := req.ExpiresIn
	if expiresIn == "" {
		expiresIn = "7d"
	}
	expiresAt := expiresInToTime(expiresIn)

	id := uuid.New().String()
	token := uuid.New().String()
	now := time.Now().UTC()

	if err := h.CH.Exec(c.Context(),
		`INSERT INTO enrollment_tokens (id, name, token, created_at, expires_at, used_count, revoked)
		 VALUES (?, ?, ?, ?, ?, 0, 0)`,
		id, req.Name, token, now, expiresAt,
	); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	slog.Info("enrollment token created", "name", req.Name, "id", id)
	return c.Status(201).JSON(models.EnrollmentToken{
		ID: id, Name: req.Name, Token: token,
		CreatedAt: now, ExpiresAt: expiresAt,
	})
}

// RevokeToken handles DELETE /api/v1/enrollment-tokens/:id.
func (h *EnrollmentHandler) RevokeToken(c *fiber.Ctx) error {
	if h.CH == nil {
		return c.Status(503).JSON(fiber.Map{"error": "ClickHouse unavailable"})
	}
	id := c.Params("id")
	// ClickHouse MergeTree: we INSERT a new row with revoked=1; ReplacingMergeTree picks latest
	if err := h.CH.Exec(c.Context(),
		`INSERT INTO enrollment_tokens (id, name, token, created_at, expires_at, used_count, revoked)
		 SELECT id, name, token, created_at, expires_at, used_count, 1
		 FROM enrollment_tokens WHERE id = ? LIMIT 1`, id,
	); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"revoked": true})
}

// Enroll handles POST /api/v1/agents/enroll — unauthenticated, validates enrollment token.
func (h *EnrollmentHandler) Enroll(c *fiber.Ctx) error {
	if h.CH == nil {
		return c.Status(503).JSON(fiber.Map{"error": "ClickHouse unavailable"})
	}

	var req models.EnrollRequest
	if err := c.BodyParser(&req); err != nil || req.Token == "" || req.Hostname == "" {
		return c.Status(400).JSON(fiber.Map{"error": "token and hostname are required"})
	}

	// Validate token: must exist, be unexpired and unrevoked
	rows, err := h.CH.Query(c.Context(),
		`SELECT id, expires_at, revoked FROM enrollment_tokens FINAL
		 WHERE token = ? LIMIT 1`, req.Token)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	defer rows.Close()

	if !rows.Next() {
		return c.Status(401).JSON(fiber.Map{"error": "invalid enrollment token"})
	}
	var tokenID string
	var expiresAt time.Time
	var revokedInt uint8
	rows.Scan(&tokenID, &expiresAt, &revokedInt)
	rows.Close()

	if revokedInt == 1 {
		return c.Status(401).JSON(fiber.Map{"error": "enrollment token has been revoked"})
	}
	if time.Now().After(expiresAt) {
		return c.Status(401).JSON(fiber.Map{"error": "enrollment token has expired"})
	}

	// Register the agent
	agentID := uuid.New().String()
	now := time.Now().UTC()
	if err := h.CH.Exec(c.Context(),
		`INSERT INTO agents (agent_id, hostname, version, interface, last_seen, registered_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		agentID, req.Hostname, req.Version, req.Interface, now, now,
	); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	// Increment used_count (insert updated row; ReplacingMergeTree keeps latest)
	_ = h.CH.Exec(c.Context(),
		`INSERT INTO enrollment_tokens (id, name, token, created_at, expires_at, used_count, revoked)
		 SELECT id, name, token, created_at, expires_at, used_count + 1, revoked
		 FROM enrollment_tokens FINAL WHERE id = ?`, tokenID)

	slog.Info("agent enrolled", "agent_id", agentID, "hostname", req.Hostname)
	return c.Status(201).JSON(models.EnrollResponse{
		AgentID: agentID,
		APIKey:  h.Cfg.APIKey,
		HubURL:  fmt.Sprintf("http://localhost:%s", h.Cfg.Port),
	})
}

// InstallScript handles GET /install — returns a shell install script parameterised
// with the enrollment token from the ?token= query param.
func (h *EnrollmentHandler) InstallScript(c *fiber.Ctx) error {
	token := c.Query("token", "YOUR_ENROLLMENT_TOKEN")
	hubURL := c.Query("hub", fmt.Sprintf("http://%s", c.Hostname()))

	script := fmt.Sprintf(`#!/bin/sh
# NetScope Agent — one-line install
# Usage: curl -sSL '%s/install?token=<enrollment_token>' | sh
set -e

ENROLLMENT_TOKEN="%s"
HUB_URL="%s"
INTERFACE="${INTERFACE:-en0}"

echo "==> NetScope Agent installer"
echo "    Hub: $HUB_URL"
echo "    Interface: $INTERFACE"

# Enrol with the Hub to get an API key
echo "==> Enrolling with hub..."
ENROLL_RESP=$(curl -sSf -X POST "$HUB_URL/api/v1/agents/enroll" \
  -H 'Content-Type: application/json' \
  -d "{\"token\":\"$ENROLLMENT_TOKEN\",\"hostname\":\"$(hostname)\",\"version\":\"0.1.0\",\"interface\":\"$INTERFACE\"}")

API_KEY=$(echo "$ENROLL_RESP" | grep -o '"api_key":"[^"]*"' | cut -d'"' -f4)
AGENT_ID=$(echo "$ENROLL_RESP" | grep -o '"agent_id":"[^"]*"' | cut -d'"' -f4)

if [ -z "$API_KEY" ]; then
  echo "ERROR: Enrolment failed. Check the token and hub URL."
  exit 1
fi

echo "==> Enrolled as agent: $AGENT_ID"

# Download the agent binary
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
case "$ARCH" in
  x86_64) ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
esac

echo "==> Downloading netscope-agent ($OS/$ARCH)..."
curl -sSfL "https://github.com/netscope/netscope/releases/latest/download/netscope-agent-$OS-$ARCH" \
  -o /usr/local/bin/netscope-agent
chmod +x /usr/local/bin/netscope-agent

echo "==> Starting capture on $INTERFACE..."
exec netscope-agent capture \
  --interface "$INTERFACE" \
  --output hub \
  --hub-url "$HUB_URL" \
  --api-key "$API_KEY"
`, c.BaseURL(), token, hubURL)

	c.Set("Content-Type", "text/plain; charset=utf-8")
	return c.SendString(script)
}

// ── helpers ───────────────────────────────────────────────────────────────────

func expiresInToTime(s string) time.Time {
	now := time.Now().UTC()
	switch s {
	case "24h":
		return now.Add(24 * time.Hour)
	case "30d":
		return now.Add(30 * 24 * time.Hour)
	case "never":
		return now.Add(100 * 365 * 24 * time.Hour)
	default: // "7d"
		return now.Add(7 * 24 * time.Hour)
	}
}
