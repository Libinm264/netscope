package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/netscope/hub-api/clickhouse"
	"github.com/netscope/hub-api/sessions"
)

// AuthHandler provides the /me, /logout, /login, /password, /demo,
// /setup, and /google OAuth endpoints.
type AuthHandler struct {
	CH          *clickhouse.Client
	Sessions    *sessions.Store
	FrontendURL string // e.g. "http://localhost:3000"
	AppURL      string // e.g. "http://localhost:8080" — used for OAuth callback URIs
	DemoEnabled bool   // true when DEMO_ENABLED=true in env

	// Google OAuth2 (optional — leave empty to hide the Google button)
	GoogleClientID     string
	GoogleClientSecret string

	// in-memory CSRF state store for OAuth2 flows
	oauthStateMu sync.Mutex
	oauthStates  map[string]oauthStateEntry
}

// oauthStateEntry holds the redirect URI and TTL for a pending OAuth2 flow.
type oauthStateEntry struct {
	RedirectURI string
	ExpiresAt   time.Time
}

// storeOAuthState saves a state → redirectURI mapping valid for 10 minutes.
func (h *AuthHandler) storeOAuthState(state, redirectURI string) {
	h.oauthStateMu.Lock()
	defer h.oauthStateMu.Unlock()
	if h.oauthStates == nil {
		h.oauthStates = make(map[string]oauthStateEntry)
	}
	// Prune expired entries opportunistically (no background goroutine needed).
	now := time.Now()
	for k, v := range h.oauthStates {
		if now.After(v.ExpiresAt) {
			delete(h.oauthStates, k)
		}
	}
	h.oauthStates[state] = oauthStateEntry{RedirectURI: redirectURI, ExpiresAt: now.Add(10 * time.Minute)}
}

// popOAuthState validates and removes the state entry; returns ("", false) if
// absent or expired.
func (h *AuthHandler) popOAuthState(state string) (string, bool) {
	h.oauthStateMu.Lock()
	defer h.oauthStateMu.Unlock()
	if h.oauthStates == nil {
		return "", false
	}
	entry, ok := h.oauthStates[state]
	if !ok {
		return "", false
	}
	delete(h.oauthStates, state)
	if time.Now().After(entry.ExpiresAt) {
		return "", false
	}
	return entry.RedirectURI, true
}

const sessionCookieName = "ns_session"

// ── Auth event logging ────────────────────────────────────────────────────────

// writeAuthEvent records login/logout activity to audit_events.
// It is fire-and-forget: errors are logged but never returned to the caller.
func writeAuthEvent(ch *clickhouse.Client, userID, role, event, clientIP string) {
	if ch == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := ch.Exec(ctx,
			`INSERT INTO audit_events (id, token_id, role, method, path, status, client_ip, latency_ms, ts)
			 VALUES (?, ?, ?, 'AUTH', ?, 200, ?, 0, ?)`,
			uuid.NewString(), "sess:"+userID, role, event, clientIP, time.Now().UTC(),
		); err != nil {
			slog.Warn("auth audit write failed", "err", err)
		}
	}()
}

// ── Me ────────────────────────────────────────────────────────────────────────

// Me handles GET /api/v1/enterprise/auth/me
func (h *AuthHandler) Me(c *fiber.Ctx) error {
	token := c.Cookies(sessionCookieName)
	if token == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error":         "not authenticated",
			"authenticated": false,
		})
	}

	sess, ok := h.Sessions.Get(token)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error":         "session expired — please sign in again",
			"authenticated": false,
		})
	}

	return c.JSON(fiber.Map{
		"authenticated": true,
		"user_id":       sess.UserID,
		"email":         sess.Email,
		"display_name":  sess.DisplayName,
		"role":          sess.Role,
		"org_id":        sess.OrgID,
		"sso_provider":  sess.SSOProvider,
		"is_demo":       sess.IsDemo,
		"expires_at":    sess.ExpiresAt,
	})
}

// ── Logout ────────────────────────────────────────────────────────────────────

// Logout handles POST /api/v1/enterprise/auth/logout
func (h *AuthHandler) Logout(c *fiber.Ctx) error {
	token := c.Cookies(sessionCookieName)
	if token != "" {
		if sess, ok := h.Sessions.Get(token); ok {
			writeAuthEvent(h.CH, sess.UserID, sess.Role, "/auth/logout", c.IP())
		}
		h.Sessions.Delete(token)
		slog.Info("user logged out", "token_prefix", token[:8])
	}

	c.Cookie(&fiber.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Expires:  time.Unix(0, 0),
		HTTPOnly: true,
		SameSite: "Lax",
		Path:     "/",
	})

	return c.JSON(fiber.Map{"ok": true, "message": "signed out"})
}

// ── Local email/password login ────────────────────────────────────────────────

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// LocalLogin handles POST /api/v1/enterprise/auth/login
func (h *AuthHandler) LocalLogin(c *fiber.Ctx) error {
	if h.CH == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "database unavailable"})
	}

	var req loginRequest
	if err := c.BodyParser(&req); err != nil || req.Email == "" || req.Password == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "email and password are required"})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var userID, role, displayName string
	// NOTE: avoid FINAL — use ORDER BY last_seen DESC to get the latest row
	// without relying on ClickHouse background merges (see ClickHouse 24.x note).
	rows, err := h.CH.Query(ctx,
		`SELECT user_id, role, display_name
		 FROM org_members
		 WHERE org_id = 'default' AND email = ? AND is_active = 1
		 ORDER BY last_seen DESC
		 LIMIT 1`, req.Email)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "database error"})
	}
	if rows.Next() {
		_ = rows.Scan(&userID, &role, &displayName)
	}
	rows.Close()

	if userID == "" {
		_ = bcrypt.CompareHashAndPassword([]byte("$2a$12$placeholder"), []byte(req.Password))
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "invalid email or password"})
	}

	var hash string
	rows2, err := h.CH.Query(ctx,
		`SELECT password_hash FROM local_credentials
		 WHERE org_id = 'default' AND user_id = ?
		 ORDER BY updated_at DESC LIMIT 1`, userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "database error"})
	}
	if rows2.Next() {
		_ = rows2.Scan(&hash)
	}
	rows2.Close()

	if hash == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "no local password set — use SSO to sign in",
		})
	}

	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(req.Password)); err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "invalid email or password"})
	}

	if displayName == "" {
		displayName = req.Email
	}

	expiresAt := time.Now().Add(sessions.DefaultTTL)
	sessionToken := h.Sessions.Create(sessions.Session{
		UserID:      userID,
		OrgID:       "default",
		Email:       req.Email,
		DisplayName: displayName,
		Role:        role,
		SSOProvider: "local",
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

	writeAuthEvent(h.CH, userID, role, "/auth/login", c.IP())
	slog.Info("local login successful", "email", req.Email, "role", role)
	return c.JSON(fiber.Map{
		"ok":           true,
		"user_id":      userID,
		"email":        req.Email,
		"display_name": displayName,
		"role":         role,
	})
}

// ── Password management ───────────────────────────────────────────────────────

type setPasswordRequest struct {
	UserID      string `json:"user_id"`
	Password    string `json:"password"`
	OldPassword string `json:"old_password"`
}

// SetPassword handles PUT /api/v1/enterprise/auth/password
func (h *AuthHandler) SetPassword(c *fiber.Ctx) error {
	if h.CH == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "database unavailable"})
	}

	var req setPasswordRequest
	if err := c.BodyParser(&req); err != nil || req.Password == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "password is required"})
	}
	if len(req.Password) < 8 {
		return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{
			"error": "password must be at least 8 characters",
		})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	targetUserID := req.UserID

	if targetUserID == "" {
		token := c.Cookies(sessionCookieName)
		if token == "" {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "not authenticated"})
		}
		sess, ok := h.Sessions.Get(token)
		if !ok {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "session expired"})
		}
		targetUserID = sess.UserID

		if req.OldPassword != "" {
			var existingHash string
			rows, err := h.CH.Query(ctx,
				`SELECT password_hash FROM local_credentials FINAL
				 WHERE org_id = 'default' AND user_id = ?
				 ORDER BY updated_at DESC LIMIT 1`, targetUserID)
			if err == nil {
				if rows.Next() {
					_ = rows.Scan(&existingHash)
				}
				rows.Close()
			}
			if existingHash != "" {
				if err := bcrypt.CompareHashAndPassword([]byte(existingHash), []byte(req.OldPassword)); err != nil {
					return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "current password is incorrect"})
				}
			}
		}
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "password hashing failed"})
	}

	now := time.Now()
	if err := h.CH.Exec(ctx,
		`INSERT INTO local_credentials (user_id, org_id, password_hash, updated_at, version)
		 VALUES (?, 'default', ?, ?, ?)`,
		targetUserID, string(hash), now, now.UnixMilli(),
	); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "could not save password"})
	}

	slog.Info("password updated", "user_id", targetUserID)
	return c.JSON(fiber.Map{"ok": true})
}

// ── Demo session ──────────────────────────────────────────────────────────────

// DemoLogin handles POST /api/v1/auth/demo
//
// Creates a short-lived read-only session that requires no credentials.
// Gated by DemoEnabled — returns 404 when demo mode is off.
func (h *AuthHandler) DemoLogin(c *fiber.Ctx) error {
	if !h.DemoEnabled {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "demo mode is not enabled"})
	}

	demoTTL := 4 * time.Hour
	expiresAt := time.Now().Add(demoTTL)
	sessionToken := h.Sessions.Create(sessions.Session{
		UserID:      "demo-user",
		OrgID:       "default",
		Email:       "demo@netscope.local",
		DisplayName: "Demo User",
		Role:        "viewer",
		SSOProvider: "demo",
		IsDemo:      true,
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

	slog.Info("demo session created", "ip", c.IP())
	return c.JSON(fiber.Map{
		"ok":           true,
		"display_name": "Demo User",
		"role":         "viewer",
		"is_demo":      true,
		"expires_at":   expiresAt,
	})
}

// ── First-run setup ───────────────────────────────────────────────────────────

// SetupStatus handles GET /api/v1/auth/setup
//
// Returns {"needs_setup": bool, "demo_enabled": bool, "google_enabled": bool}
// so the frontend can show the correct CTA on the login page.
func (h *AuthHandler) SetupStatus(c *fiber.Ctx) error {
	googleEnabled := h.GoogleClientID != ""

	if h.CH == nil {
		return c.JSON(fiber.Map{
			"needs_setup":    false,
			"demo_enabled":   h.DemoEnabled,
			"google_enabled": googleEnabled,
		})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var count uint64
	rows, err := h.CH.Query(ctx,
		`SELECT count() FROM org_members FINAL WHERE org_id = 'default' AND is_active = 1`)
	if err != nil || !rows.Next() {
		if rows != nil {
			rows.Close()
		}
		return c.JSON(fiber.Map{
			"needs_setup":    true,
			"demo_enabled":   h.DemoEnabled,
			"google_enabled": googleEnabled,
		})
	}
	_ = rows.Scan(&count)
	rows.Close()

	return c.JSON(fiber.Map{
		"needs_setup":    count == 0,
		"demo_enabled":   h.DemoEnabled,
		"google_enabled": googleEnabled,
	})
}

// setupRequest is the body for POST /api/v1/auth/setup.
// The "name" json tag matches what the setup page sends.
type setupRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Name     string `json:"name"` // display name — maps to display_name column
}

// SetupAdmin handles POST /api/v1/auth/setup
//
// One-time endpoint: creates the first owner account when no users exist.
// Returns 409 if any active member already exists.
func (h *AuthHandler) SetupAdmin(c *fiber.Ctx) error {
	if h.CH == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "database unavailable"})
	}

	var req setupRequest
	if err := c.BodyParser(&req); err != nil || req.Email == "" || req.Password == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email and password are required",
		})
	}
	if len(req.Password) < 12 {
		return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{
			"error": "password must be at least 12 characters",
		})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Guard: refuse if any active member already exists.
	var count uint64
	rows, err := h.CH.Query(ctx,
		`SELECT count() FROM org_members FINAL WHERE org_id = 'default' AND is_active = 1`)
	if err == nil && rows.Next() {
		_ = rows.Scan(&count)
		rows.Close()
	}
	if count > 0 {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{
			"error": "setup already completed — sign in with your existing account",
		})
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "password hashing failed"})
	}

	displayName := req.Name
	if displayName == "" {
		displayName = req.Email
	}

	userID := uuid.NewString()
	now := time.Now().UTC()

	// Insert into org_members (column names match the CREATE TABLE schema).
	if err := h.CH.Exec(ctx,
		`INSERT INTO org_members
		 (user_id, org_id, email, display_name, role, sso_provider, sso_subject,
		  is_active, created_at, last_seen, version)
		 VALUES (?, 'default', ?, ?, 'owner', 'local', '', 1, ?, ?, ?)`,
		userID, req.Email, displayName, now, now, now.UnixMilli(),
	); err != nil {
		slog.Error("setup: org_members insert failed", "err", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "could not create user"})
	}

	// Insert local credentials.
	if err := h.CH.Exec(ctx,
		`INSERT INTO local_credentials (user_id, org_id, password_hash, updated_at, version)
		 VALUES (?, 'default', ?, ?, ?)`,
		userID, string(hash), now, now.UnixMilli(),
	); err != nil {
		slog.Error("setup: local_credentials insert failed", "err", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "could not save credentials"})
	}

	// Create a session so the user is logged in immediately after setup.
	expiresAt := now.Add(sessions.DefaultTTL)
	sessionToken := h.Sessions.Create(sessions.Session{
		UserID:      userID,
		OrgID:       "default",
		Email:       req.Email,
		DisplayName: displayName,
		Role:        "owner",
		SSOProvider: "local",
		CreatedAt:   now,
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

	writeAuthEvent(h.CH, userID, "owner", "/auth/setup", c.IP())
	slog.Info("first-run admin account created", "email", req.Email, "user_id", userID)
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"ok":           true,
		"user_id":      userID,
		"email":        req.Email,
		"display_name": displayName,
		"role":         "owner",
	})
}

// ── Google OAuth2 sign-in ─────────────────────────────────────────────────────

const (
	googleAuthURL  = "https://accounts.google.com/o/oauth2/v2/auth"
	googleTokenURL = "https://oauth2.googleapis.com/token"
	googleUserURL  = "https://www.googleapis.com/oauth2/v2/userinfo"
)

// GoogleInitiate handles GET /api/v1/auth/google/initiate
//
// Redirects the browser to Google's OAuth2 consent screen.
// Query param: redirect_uri — where to land after login (defaults to FrontendURL).
func (h *AuthHandler) GoogleInitiate(c *fiber.Ctx) error {
	if h.GoogleClientID == "" {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Google sign-in is not configured"})
	}

	redirectAfter := c.Query("redirect_uri", h.FrontendURL)
	state := uuid.NewString()
	h.storeOAuthState(state, redirectAfter)

	callbackURI := h.AppURL + "/api/v1/auth/google/callback"

	authURL := fmt.Sprintf(
		"%s?client_id=%s&redirect_uri=%s&response_type=code&scope=%s&state=%s&access_type=offline&prompt=select_account",
		googleAuthURL,
		url.QueryEscape(h.GoogleClientID),
		url.QueryEscape(callbackURI),
		url.QueryEscape("openid email profile"),
		url.QueryEscape(state),
	)

	return c.Redirect(authURL, fiber.StatusTemporaryRedirect)
}

// googleTokenResp is the JSON returned by the token exchange endpoint.
type googleTokenResp struct {
	AccessToken string `json:"access_token"`
	IDToken     string `json:"id_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
}

// googleUserInfo is the JSON returned by the userinfo endpoint.
type googleUserInfo struct {
	ID            string `json:"id"`
	Email         string `json:"email"`
	VerifiedEmail bool   `json:"verified_email"`
	Name          string `json:"name"`
	Picture       string `json:"picture"`
}

// GoogleCallback handles GET /api/v1/auth/google/callback
//
// Exchanges the authorisation code for a user identity, then either signs
// the user in (existing account) or creates a new account.
// New accounts get role "viewer" unless they are the first member of the org
// (in which case they get "owner").
func (h *AuthHandler) GoogleCallback(c *fiber.Ctx) error {
	if h.GoogleClientID == "" {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Google sign-in is not configured"})
	}

	// Validate CSRF state.
	state := c.Query("state")
	redirectAfter, ok := h.popOAuthState(state)
	if !ok {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid or expired OAuth state"})
	}

	code := c.Query("code")
	if code == "" {
		// User denied consent — redirect back to login with an error hint.
		return c.Redirect(h.FrontendURL+"/login?error=google_denied", fiber.StatusTemporaryRedirect)
	}

	// ── Exchange code for tokens ──────────────────────────────────────────────
	callbackURI := h.AppURL + "/api/v1/auth/google/callback"

	tokenBody := url.Values{
		"code":          {code},
		"client_id":     {h.GoogleClientID},
		"client_secret": {h.GoogleClientSecret},
		"redirect_uri":  {callbackURI},
		"grant_type":    {"authorization_code"},
	}

	tokenResp, err := http.PostForm(googleTokenURL, tokenBody)
	if err != nil || tokenResp.StatusCode != http.StatusOK {
		slog.Error("google token exchange failed", "err", err)
		return c.Redirect(h.FrontendURL+"/login?error=google_token_failed", fiber.StatusTemporaryRedirect)
	}
	defer tokenResp.Body.Close()

	var tok googleTokenResp
	if err := json.NewDecoder(tokenResp.Body).Decode(&tok); err != nil {
		return c.Redirect(h.FrontendURL+"/login?error=google_token_parse_failed", fiber.StatusTemporaryRedirect)
	}

	// ── Fetch user info ───────────────────────────────────────────────────────
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, googleUserURL, nil)
	req.Header.Set("Authorization", "Bearer "+tok.AccessToken)

	userResp, err := http.DefaultClient.Do(req)
	if err != nil || userResp.StatusCode != http.StatusOK {
		slog.Error("google userinfo fetch failed", "err", err)
		return c.Redirect(h.FrontendURL+"/login?error=google_userinfo_failed", fiber.StatusTemporaryRedirect)
	}
	defer userResp.Body.Close()

	body, _ := io.ReadAll(userResp.Body)
	var gUser googleUserInfo
	if err := json.Unmarshal(body, &gUser); err != nil || gUser.Email == "" {
		return c.Redirect(h.FrontendURL+"/login?error=google_userinfo_parse_failed", fiber.StatusTemporaryRedirect)
	}

	if !gUser.VerifiedEmail {
		return c.Redirect(h.FrontendURL+"/login?error=google_email_unverified", fiber.StatusTemporaryRedirect)
	}

	// ── Upsert user in ClickHouse ─────────────────────────────────────────────
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var userID, role, displayName string

	if h.CH != nil {
		rows, err := h.CH.Query(ctx,
			`SELECT user_id, role, display_name FROM org_members FINAL
			 WHERE org_id = 'default' AND email = ? AND is_active = 1 LIMIT 1`,
			gUser.Email)
		if err == nil {
			if rows.Next() {
				_ = rows.Scan(&userID, &role, &displayName)
			}
			rows.Close()
		}
	}

	now := time.Now().UTC()

	if userID == "" {
		// First-ever member → owner; otherwise viewer.
		userID = uuid.NewString()
		role = "viewer"
		displayName = gUser.Name
		if displayName == "" {
			displayName = gUser.Email
		}

		if h.CH != nil {
			// Determine role: owner if org has no members yet.
			var memberCount uint64
			cntRows, _ := h.CH.Query(ctx,
				`SELECT count() FROM org_members FINAL WHERE org_id = 'default' AND is_active = 1`)
			if cntRows != nil {
				if cntRows.Next() {
					_ = cntRows.Scan(&memberCount)
				}
				cntRows.Close()
			}
			if memberCount == 0 {
				role = "owner"
			}

			if err := h.CH.Exec(ctx,
				`INSERT INTO org_members
				 (user_id, org_id, email, display_name, role, sso_provider, sso_subject,
				  is_active, created_at, last_seen, version)
				 VALUES (?, 'default', ?, ?, ?, 'google', ?, 1, ?, ?, ?)`,
				userID, gUser.Email, displayName, role, gUser.ID, now, now, now.UnixMilli(),
			); err != nil {
				slog.Error("google: org_members insert failed", "err", err)
				return c.Redirect(h.FrontendURL+"/login?error=db_error", fiber.StatusTemporaryRedirect)
			}
		}
		slog.Info("google: new user created", "email", gUser.Email, "role", role)
	} else {
		// Update last_seen.
		if h.CH != nil {
			_ = h.CH.Exec(ctx,
				`INSERT INTO org_members
				 (user_id, org_id, email, display_name, role, sso_provider, sso_subject,
				  is_active, created_at, last_seen, version)
				 SELECT user_id, org_id, email, display_name, role, sso_provider, sso_subject,
				        is_active, created_at, ?, version + 1
				 FROM org_members FINAL WHERE org_id = 'default' AND user_id = ? LIMIT 1`,
				now, userID)
		}
		slog.Info("google: existing user signed in", "email", gUser.Email, "role", role)
	}

	// ── Create session ────────────────────────────────────────────────────────
	expiresAt := now.Add(sessions.DefaultTTL)
	sessionToken := h.Sessions.Create(sessions.Session{
		UserID:      userID,
		OrgID:       "default",
		Email:       gUser.Email,
		DisplayName: displayName,
		Role:        role,
		SSOProvider: "google",
		CreatedAt:   now,
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

	writeAuthEvent(h.CH, userID, role, "/auth/google/callback", c.IP())

	// Redirect to the originally requested page (or dashboard root).
	dest := redirectAfter
	if dest == "" || !strings.HasPrefix(dest, h.FrontendURL) {
		dest = h.FrontendURL + "/"
	}
	return c.Redirect(dest, fiber.StatusTemporaryRedirect)
}
