// Package sso implements OIDC and SAML authentication flows for the enterprise tier.
// Client secrets are never stored in ClickHouse; set SSO_CLIENT_SECRET in the environment.
package sso

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"golang.org/x/oauth2"

	"github.com/netscope/hub-api/clickhouse"
	"github.com/netscope/hub-api/enterprise/license"
	"github.com/netscope/hub-api/sessions"
)

const (
	stateExpiry       = 10 * time.Minute
	sessionCookieName = "ns_session"
)

// pendingState holds the data attached to an anti-CSRF state nonce.
type pendingState struct {
	redirectURI string    // where to send the browser after successful login
	expiry      time.Time
}

// OIDCHandler manages the OIDC authorisation-code flow.
// One instance is shared across requests; it is safe for concurrent use.
type OIDCHandler struct {
	CH           *clickhouse.Client
	Sessions     *sessions.Store
	License      *license.License
	AppURL       string // Go hub's public URL, used to construct the callback URI
	FrontendURL  string // Next.js origin, used as default post-login redirect
	ClientSecret string // value of SSO_CLIENT_SECRET env var

	mu     sync.Mutex
	states map[string]pendingState
}

// NewOIDCHandler creates an OIDCHandler and starts the background state-cleanup ticker.
func NewOIDCHandler(ch *clickhouse.Client, sess *sessions.Store, lic *license.License,
	appURL, frontendURL, clientSecret string) *OIDCHandler {

	h := &OIDCHandler{
		CH:           ch,
		Sessions:     sess,
		License:      lic,
		AppURL:       appURL,
		FrontendURL:  frontendURL,
		ClientSecret: clientSecret,
		states:       make(map[string]pendingState),
	}
	go h.cleanupStates()
	return h
}

// cleanupStates evicts expired nonces every 5 minutes.
func (h *OIDCHandler) cleanupStates() {
	t := time.NewTicker(5 * time.Minute)
	defer t.Stop()
	for range t.C {
		h.mu.Lock()
		for k, v := range h.states {
			if time.Now().After(v.expiry) {
				delete(h.states, k)
			}
		}
		h.mu.Unlock()
	}
}

func (h *OIDCHandler) storeState(redirectURI string) (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	nonce := hex.EncodeToString(b)
	h.mu.Lock()
	h.states[nonce] = pendingState{redirectURI: redirectURI, expiry: time.Now().Add(stateExpiry)}
	h.mu.Unlock()
	return nonce, nil
}

func (h *OIDCHandler) consumeState(state string) (pendingState, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	ps, ok := h.states[state]
	if !ok || time.Now().After(ps.expiry) {
		delete(h.states, state)
		return pendingState{}, false
	}
	delete(h.states, state)
	return ps, true
}

// loadOIDCConfig fetches issuer_url and client_id from the sso_configs table.
func (h *OIDCHandler) loadOIDCConfig(ctx context.Context) (issuerURL, clientID string, enabled bool, err error) {
	rows, err := h.CH.Query(ctx,
		`SELECT issuer_url, client_id, enabled
		 FROM sso_configs
		 WHERE org_id = 'default' AND provider = 'oidc'
		 ORDER BY updated_at DESC
		 LIMIT 1`)
	if err != nil {
		return "", "", false, err
	}
	defer rows.Close()
	if !rows.Next() {
		return "", "", false, nil
	}
	var e uint8
	_ = rows.Scan(&issuerURL, &clientID, &e)
	return issuerURL, clientID, e == 1, nil
}

// callbackURL returns the Go backend's OIDC callback endpoint.
// This URL must match what is registered with the IdP.
func (h *OIDCHandler) callbackURL() string {
	return h.AppURL + "/api/v1/enterprise/auth/oidc/callback"
}

// buildOAuthConfig constructs an oauth2.Config from the stored SSO settings.
func (h *OIDCHandler) buildOAuthConfig(provider *oidc.Provider, clientID string) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: h.ClientSecret,
		RedirectURL:  h.callbackURL(),
		Endpoint:     provider.Endpoint(),
		Scopes:       []string{oidc.ScopeOpenID, "profile", "email"},
	}
}

// ── Initiate ──────────────────────────────────────────────────────────────────

// Initiate handles GET /api/v1/enterprise/auth/oidc/initiate?redirect_uri=<url>
// It generates a state nonce and redirects the browser to the IdP.
func (h *OIDCHandler) Initiate(c *fiber.Ctx) error {
	if !h.License.HasFeature(license.FeatureSSO) {
		return c.Status(fiber.StatusPaymentRequired).JSON(fiber.Map{
			"error": "SSO requires Enterprise plan", "upgrade": true,
		})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	issuerURL, clientID, enabled, err := h.loadOIDCConfig(ctx)
	if err != nil {
		slog.Error("oidc: loadOIDCConfig failed", "err", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "SSO config unavailable"})
	}
	if !enabled || issuerURL == "" || clientID == "" {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "OIDC SSO is not configured or disabled"})
	}

	provider, err := oidc.NewProvider(ctx, issuerURL)
	if err != nil {
		slog.Error("oidc: provider discovery failed", "issuer", issuerURL, "err", err)
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{"error": "could not reach OIDC provider"})
	}

	frontendDest := c.Query("redirect_uri", h.FrontendURL+"/")
	state, err := h.storeState(frontendDest)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "state generation failed"})
	}

	authURL := h.buildOAuthConfig(provider, clientID).AuthCodeURL(state)
	return c.Redirect(authURL, fiber.StatusFound)
}

// ── Callback ──────────────────────────────────────────────────────────────────

// Callback handles GET /api/v1/enterprise/auth/oidc/callback?code=...&state=...
// It exchanges the code, verifies the ID token, upserts the user, creates a
// session, and redirects the browser back to the frontend.
func (h *OIDCHandler) Callback(c *fiber.Ctx) error {
	state := c.Query("state")
	code := c.Query("code")
	if state == "" || code == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "missing state or code"})
	}

	ps, ok := h.consumeState(state)
	if !ok {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid or expired state — please try signing in again"})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	issuerURL, clientID, enabled, err := h.loadOIDCConfig(ctx)
	if err != nil || !enabled || issuerURL == "" {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "SSO not available"})
	}

	provider, err := oidc.NewProvider(ctx, issuerURL)
	if err != nil {
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{"error": "OIDC provider unreachable"})
	}

	oauthCfg := h.buildOAuthConfig(provider, clientID)
	token, err := oauthCfg.Exchange(ctx, code)
	if err != nil {
		slog.Warn("oidc: token exchange failed", "err", err)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "token exchange failed"})
	}

	rawIDToken, ok2 := token.Extra("id_token").(string)
	if !ok2 || rawIDToken == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "id_token missing from provider response"})
	}

	verifier := provider.Verifier(&oidc.Config{ClientID: clientID})
	idToken, err := verifier.Verify(ctx, rawIDToken)
	if err != nil {
		slog.Warn("oidc: id_token verification failed", "err", err)
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "id_token verification failed"})
	}

	var claims struct {
		Email string `json:"email"`
		Name  string `json:"name"`
	}
	_ = idToken.Claims(&claims)
	if claims.Email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "email claim missing from ID token"})
	}

	userID, role, err := h.upsertUser(ctx, idToken.Subject, claims.Email, claims.Name)
	if err != nil {
		slog.Error("oidc: upsertUser failed", "err", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "user record could not be created"})
	}

	expiresAt := time.Now().Add(sessions.DefaultTTL)
	sessionToken := h.Sessions.Create(sessions.Session{
		UserID:      userID,
		OrgID:       "default",
		Email:       claims.Email,
		DisplayName: claims.Name,
		Role:        role,
		SSOProvider: "oidc",
		CreatedAt:   time.Now(),
		ExpiresAt:   expiresAt,
	})

	c.Cookie(&fiber.Cookie{
		Name:     sessionCookieName,
		Value:    sessionToken,
		Path:     "/",
		HTTPOnly: true,
		SameSite: "Lax",
		Expires:  expiresAt,
	})

	slog.Info("OIDC login successful", "email", claims.Email, "role", role)

	dest := ps.redirectURI
	if dest == "" {
		dest = h.FrontendURL + "/"
	}
	return c.Redirect(dest, fiber.StatusFound)
}

// ── User upsert ───────────────────────────────────────────────────────────────

// upsertUser finds an existing org_members record by OIDC subject (or email
// as fallback), then writes an updated row to keep last_seen current.
// Returns the user's ID and role.
func (h *OIDCHandler) upsertUser(ctx context.Context, subject, email, displayName string) (userID, role string, err error) {
	// Prefer lookup by stable SSO subject.
	rows, qErr := h.CH.Query(ctx,
		`SELECT user_id, role FROM org_members
		 WHERE org_id = 'default' AND sso_provider = 'oidc' AND sso_subject = ?
		   AND is_active = 1
		 ORDER BY last_seen DESC LIMIT 1`, subject)
	if qErr == nil {
		if rows.Next() {
			_ = rows.Scan(&userID, &role)
		}
		rows.Close()
	}

	// Fall back to email match (covers users pre-created via invite).
	if userID == "" {
		rows2, qErr2 := h.CH.Query(ctx,
			`SELECT user_id, role FROM org_members
			 WHERE org_id = 'default' AND email = ? AND is_active = 1
			 ORDER BY last_seen DESC LIMIT 1`, email)
		if qErr2 == nil {
			if rows2.Next() {
				_ = rows2.Scan(&userID, &role)
			}
			rows2.Close()
		}
	}

	if userID == "" {
		userID = uuid.NewString()
		role = "viewer" // default role for new SSO users
	}
	if displayName == "" {
		displayName = email
	}

	now := time.Now()
	err = h.CH.Exec(ctx,
		`INSERT INTO org_members
		 (user_id, org_id, email, display_name, role,
		  sso_provider, sso_subject, is_active, created_at, last_seen, version)
		 VALUES (?, 'default', ?, ?, ?, 'oidc', ?, 1, ?, ?, ?)`,
		userID, email, displayName, role, subject,
		now, now, now.UnixMilli(),
	)
	return userID, role, err
}
