package handlers

import (
	"fmt"
	"log/slog"
	"net/url"
	"regexp"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/netscope/hub-api/clickhouse"
	"github.com/netscope/hub-api/config"
	"github.com/netscope/hub-api/models"
	"github.com/netscope/hub-api/util"
)

// tokenRE accepts only the characters that appear in UUIDs and hex tokens.
var tokenRE = regexp.MustCompile(`^[0-9a-fA-F\-]+$`)

// sanitizeShellURL parses raw as a URL and returns only the scheme+host
// portion, preventing shell injection via crafted query parameters.
// Returns an empty string if the URL is malformed or uses a non-HTTP scheme.
func sanitizeShellURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return ""
	}
	return u.Scheme + "://" + u.Host
}

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
		`SELECT id, name, token, created_at, expires_at, used_count, max_uses, revoked
		 FROM enrollment_tokens
		 ORDER BY created_at DESC`)
	if err != nil {
		return util.InternalError(c, err)
	}
	defer rows.Close()

	tokens := make([]models.EnrollmentToken, 0)
	for rows.Next() {
		var t models.EnrollmentToken
		var revokedInt uint8
		if err := rows.Scan(
			&t.ID, &t.Name, &t.Token,
			&t.CreatedAt, &t.ExpiresAt, &t.UsedCount, &t.MaxUses, &revokedInt,
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

	maxUses := req.MaxUses
	if maxUses < 0 {
		maxUses = 0
	}

	if err := h.CH.Exec(c.Context(),
		`INSERT INTO enrollment_tokens (id, name, token, created_at, expires_at, used_count, max_uses, revoked)
		 VALUES (?, ?, ?, ?, ?, 0, ?, 0)`,
		id, req.Name, token, now, expiresAt, maxUses,
	); err != nil {
		return util.InternalError(c, err)
	}

	slog.Info("enrollment token created", "name", req.Name, "id", id, "max_uses", maxUses)
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
		`INSERT INTO enrollment_tokens (id, name, token, created_at, expires_at, used_count, max_uses, revoked)
		 SELECT id, name, token, created_at, expires_at, used_count, max_uses, 1
		 FROM enrollment_tokens WHERE id = ?
		 ORDER BY created_at DESC LIMIT 1`, id,
	); err != nil {
		return util.InternalError(c, err)
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

	// Validate token: must exist, be unexpired and unrevoked.
	//
	// NOTE: we deliberately do NOT use FINAL here. With ReplacingMergeTree,
	// FINAL triggers an in-query merge that can miss rows inserted in the same
	// second on ClickHouse 24.x.  Instead we ORDER BY created_at DESC LIMIT 1
	// which naturally returns the most-recent version of the row (handling the
	// revocation case where a new row with revoked=1 is inserted).
	rows, err := h.CH.Query(c.Context(),
		`SELECT id, expires_at, used_count, max_uses, revoked FROM enrollment_tokens
		 WHERE token = ? ORDER BY created_at DESC LIMIT 1`, req.Token)
	if err != nil {
		return util.InternalError(c, err)
	}
	defer rows.Close()

	if !rows.Next() {
		slog.Warn("enrollment: token not found in DB", "token_prefix", req.Token[:8])
		return c.Status(401).JSON(fiber.Map{"error": "invalid enrollment token"})
	}
	var tokenID string
	var expiresAt time.Time
	var usedCount, maxUses uint32
	var revokedInt uint8
	if err := rows.Scan(&tokenID, &expiresAt, &usedCount, &maxUses, &revokedInt); err != nil {
		return util.InternalError(c, err)
	}
	rows.Close()

	if revokedInt == 1 {
		return c.Status(401).JSON(fiber.Map{"error": "enrollment token has been revoked"})
	}
	if time.Now().After(expiresAt) {
		return c.Status(401).JSON(fiber.Map{"error": "enrollment token has expired"})
	}
	// Enforce max_uses cap (0 = unlimited).
	if maxUses > 0 && usedCount >= maxUses {
		return c.Status(401).JSON(fiber.Map{"error": "enrollment token usage limit reached"})
	}

	// Register the agent
	agentID := uuid.New().String()
	now := time.Now().UTC()
	if err := h.CH.Exec(c.Context(),
		`INSERT INTO agents (agent_id, hostname, version, interface, last_seen, registered_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		agentID, req.Hostname, req.Version, req.Interface, now, now,
	); err != nil {
		return util.InternalError(c, err)
	}

	// Increment used_count (insert updated row; ReplacingMergeTree keeps latest)
	_ = h.CH.Exec(c.Context(),
		`INSERT INTO enrollment_tokens (id, name, token, created_at, expires_at, used_count, max_uses, revoked)
		 SELECT id, name, token, created_at, expires_at, used_count + 1, max_uses, revoked
		 FROM enrollment_tokens WHERE id = ?
		 ORDER BY created_at DESC LIMIT 1`, tokenID)

	// Issue a scoped viewer token for this agent — never hand out the global admin key.
	agentTokenID := uuid.New().String()
	agentToken := uuid.New().String()
	tokenNow := time.Now().UTC()
	if err := h.CH.Exec(c.Context(),
		`INSERT INTO api_tokens (id, name, role, token, created_at, last_used, revoked)
		 VALUES (?, ?, 'viewer', ?, ?, ?, 0)`,
		agentTokenID,
		fmt.Sprintf("agent:%s", req.Hostname),
		agentToken,
		tokenNow,
		tokenNow,
	); err != nil {
		return util.InternalError(c, err)
	}

	// Build hub URL from the incoming request (works behind a proxy / k8s service).
	hubURL := fmt.Sprintf("%s://%s", c.Protocol(), c.Hostname())

	slog.Info("agent enrolled", "agent_id", agentID, "hostname", req.Hostname, "token_id", agentTokenID)
	return c.Status(201).JSON(models.EnrollResponse{
		AgentID: agentID,
		APIKey:  agentToken,
		HubURL:  hubURL,
	})
}

// InstallScript handles GET /install — returns a shell install script parameterised
// with the enrollment token from the ?token= query param.
func (h *EnrollmentHandler) InstallScript(c *fiber.Ctx) error {
	rawToken := c.Query("token", "YOUR_ENROLLMENT_TOKEN")
	// Only allow characters that appear in UUIDs and hex tokens to prevent
	// shell injection; fall back to a safe placeholder for display purposes.
	token := rawToken
	if rawToken != "YOUR_ENROLLMENT_TOKEN" && !tokenRE.MatchString(rawToken) {
		token = "INVALID_TOKEN"
	}

	// Sanitize the hub URL: parse and reconstruct from components so that an
	// attacker cannot inject shell metacharacters via the ?hub= parameter.
	rawHub := c.Query("hub", fmt.Sprintf("http://%s", c.Hostname()))
	hubURL := sanitizeShellURL(rawHub)
	if hubURL == "" {
		hubURL = fmt.Sprintf("http://%s", c.Hostname())
	}

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
DOWNLOAD_URL="https://github.com/Libinm264/netscope/releases/latest/download/netscope-agent-$OS-$ARCH"
if ! curl -sSfL "$DOWNLOAD_URL" -o /usr/local/bin/netscope-agent 2>/dev/null; then
  echo "ERROR: Could not download agent binary from:"
  echo "       $DOWNLOAD_URL"
  echo "       Please download manually from https://github.com/Libinm264/netscope/releases"
  echo "       and place it at /usr/local/bin/netscope-agent"
  exit 1
fi
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
