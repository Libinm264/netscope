// Enterprise Edition — see hub/enterprise/LICENSE (BSL-1.1)
//
// Package scim implements the SCIM 2.0 Users endpoint used by identity
// providers (Okta, Azure AD, OneLogin) to automatically provision and
// deprovision members in the NetScope Hub org_members table.
//
// Supported operations:
//   GET    /scim/v2/Users          — list users (supports filter=userName eq "x")
//   POST   /scim/v2/Users          — provision a new user
//   GET    /scim/v2/Users/:id      — get a single user
//   PUT    /scim/v2/Users/:id      — replace user attributes
//   PATCH  /scim/v2/Users/:id      — update active/displayName
//   DELETE /scim/v2/Users/:id      — deprovision user (sets is_active=0)
//   GET    /scim/v2/ServiceProviderConfig — advertise capabilities
//
// Auth: Bearer token validated against the SCIM_BEARER_TOKEN env var.
// Configure this token in your IdP's SCIM provisioning settings.
package scim

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	chclient "github.com/netscope/hub-api/clickhouse"
	"github.com/netscope/hub-api/enterprise/license"
)

// Handler implements SCIM 2.0 for the NetScope Hub enterprise edition.
type Handler struct {
	CH          *chclient.Client
	License     *license.License
	BearerToken string // value of SCIM_BEARER_TOKEN env var
}

// ── SCIM data types ───────────────────────────────────────────────────────────

const (
	scimUserSchema = "urn:ietf:params:scim:schemas:core:2.0:User"
	scimListSchema = "urn:ietf:params:scim:api:messages:2.0:ListResponse"
	scimErrSchema  = "urn:ietf:params:scim:api:messages:2.0:Error"
)

type scimName struct {
	Formatted  string `json:"formatted,omitempty"`
	GivenName  string `json:"givenName,omitempty"`
	FamilyName string `json:"familyName,omitempty"`
}

type scimEmail struct {
	Value   string `json:"value"`
	Primary bool   `json:"primary"`
}

type scimMeta struct {
	ResourceType string `json:"resourceType"`
	Created      string `json:"created,omitempty"`
	LastModified string `json:"lastModified,omitempty"`
	Location     string `json:"location,omitempty"`
}

type scimUser struct {
	Schemas    []string   `json:"schemas"`
	ID         string     `json:"id,omitempty"`
	ExternalID string     `json:"externalId,omitempty"`
	UserName   string     `json:"userName"`
	Name       scimName   `json:"name,omitempty"`
	Emails     []scimEmail `json:"emails,omitempty"`
	Active     bool       `json:"active"`
	Meta       scimMeta   `json:"meta,omitempty"`
}

type scimPatchOp struct {
	Schemas    []string        `json:"schemas"`
	Operations []scimOperation `json:"Operations"`
}

type scimOperation struct {
	Op    string         `json:"op"`
	Path  string         `json:"path,omitempty"`
	Value map[string]any `json:"value,omitempty"`
}

type scimListResponse struct {
	Schemas      []string   `json:"schemas"`
	TotalResults int        `json:"totalResults"`
	StartIndex   int        `json:"startIndex"`
	ItemsPerPage int        `json:"itemsPerPage"`
	Resources    []scimUser `json:"Resources"`
}

// ── Middleware ────────────────────────────────────────────────────────────────

// BearerAuth validates the SCIM bearer token. Attach before all SCIM routes.
func (h *Handler) BearerAuth(c *fiber.Ctx) error {
	if !h.License.HasFeature(license.FeatureSCIM) {
		return scimError(c, fiber.StatusPaymentRequired,
			"scimError", "SCIM 2.0 provisioning requires Team or Enterprise plan")
	}

	auth := c.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		return scimError(c, fiber.StatusUnauthorized, "invalidCredentials",
			"missing Bearer token")
	}

	token := strings.TrimPrefix(auth, "Bearer ")
	if h.BearerToken == "" || token != h.BearerToken {
		return scimError(c, fiber.StatusUnauthorized, "invalidCredentials",
			"invalid SCIM bearer token")
	}

	return c.Next()
}

// ── Endpoint handlers ─────────────────────────────────────────────────────────

// ServiceProviderConfig handles GET /scim/v2/ServiceProviderConfig
func (h *Handler) ServiceProviderConfig(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"schemas": []string{"urn:ietf:params:scim:schemas:core:2.0:ServiceProviderConfig"},
		"patch":   fiber.Map{"supported": true},
		"bulk":    fiber.Map{"supported": false, "maxOperations": 0, "maxPayloadSize": 0},
		"filter":  fiber.Map{"supported": true, "maxResults": 200},
		"changePassword": fiber.Map{"supported": false},
		"sort":    fiber.Map{"supported": false},
		"etag":    fiber.Map{"supported": false},
		"authenticationSchemes": []fiber.Map{
			{"type": "oauthbearertoken", "name": "OAuth Bearer Token", "description": "Authentication using a Bearer token"},
		},
	})
}

// ListUsers handles GET /scim/v2/Users
func (h *Handler) ListUsers(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Optional SCIM filter: filter=userName eq "alice@example.com"
	filterRaw := c.Query("filter", "")
	emailFilter := ""
	if strings.Contains(filterRaw, "userName eq") {
		parts := strings.SplitN(filterRaw, "\"", 3)
		if len(parts) >= 2 {
			emailFilter = parts[1]
		}
	}

	query := `SELECT user_id, email, display_name, role, sso_subject, is_active, created_at, last_seen
	          FROM org_members FINAL WHERE org_id = 'default'`
	args := []interface{}{}
	if emailFilter != "" {
		query += " AND email = ?"
		args = append(args, emailFilter)
	}
	query += " ORDER BY created_at ASC LIMIT 200"

	rows, err := h.CH.Query(ctx, query, args...)
	if err != nil {
		return scimError(c, fiber.StatusInternalServerError, "serverError", err.Error())
	}
	defer rows.Close()

	users := make([]scimUser, 0)
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			slog.Warn("scim: scan user", "err", err)
			continue
		}
		users = append(users, u)
	}

	return c.JSON(scimListResponse{
		Schemas:      []string{scimListSchema},
		TotalResults: len(users),
		StartIndex:   1,
		ItemsPerPage: len(users),
		Resources:    users,
	})
}

// GetUser handles GET /scim/v2/Users/:id
func (h *Handler) GetUser(c *fiber.Ctx) error {
	id := c.Params("id")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := h.CH.Query(ctx,
		`SELECT user_id, email, display_name, role, sso_subject, is_active, created_at, last_seen
		 FROM org_members FINAL
		 WHERE org_id = 'default' AND user_id = ? LIMIT 1`, id)
	if err != nil {
		return scimError(c, fiber.StatusInternalServerError, "serverError", err.Error())
	}
	defer rows.Close()

	if !rows.Next() {
		return scimError(c, fiber.StatusNotFound, "notFound", "user not found")
	}
	u, err := scanUser(rows)
	if err != nil {
		return scimError(c, fiber.StatusInternalServerError, "serverError", err.Error())
	}
	return c.JSON(u)
}

// CreateUser handles POST /scim/v2/Users (IdP provisioning a new member)
func (h *Handler) CreateUser(c *fiber.Ctx) error {
	var req scimUser
	if err := c.BodyParser(&req); err != nil {
		return scimError(c, fiber.StatusBadRequest, "invalidValue", "invalid JSON body")
	}

	email := req.UserName
	if email == "" && len(req.Emails) > 0 {
		email = req.Emails[0].Value
	}
	if email == "" {
		return scimError(c, fiber.StatusBadRequest, "invalidValue", "userName / email is required")
	}

	displayName := req.Name.Formatted
	if displayName == "" {
		displayName = strings.TrimSpace(req.Name.GivenName + " " + req.Name.FamilyName)
	}

	isActive := uint8(0)
	if req.Active {
		isActive = 1
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	userID := uuid.NewString()
	if err := h.CH.Exec(ctx,
		`INSERT INTO org_members
		 (user_id, org_id, email, display_name, role, sso_provider, sso_subject, is_active, created_at, last_seen)
		 VALUES (?, 'default', ?, ?, 'viewer', 'scim', ?, ?, now64(), now64())`,
		userID, email, displayName, req.ExternalID, isActive,
	); err != nil {
		return scimError(c, fiber.StatusInternalServerError, "serverError", err.Error())
	}

	slog.Info("scim: user provisioned", "email", email, "user_id", userID)

	resp := buildScimUser(userID, email, displayName, "viewer", req.ExternalID, req.Active, time.Now())
	c.Set("Location", fmt.Sprintf("/scim/v2/Users/%s", userID))
	return c.Status(fiber.StatusCreated).JSON(resp)
}

// ReplaceUser handles PUT /scim/v2/Users/:id (full replace)
func (h *Handler) ReplaceUser(c *fiber.Ctx) error {
	id := c.Params("id")
	var req scimUser
	if err := c.BodyParser(&req); err != nil {
		return scimError(c, fiber.StatusBadRequest, "invalidValue", "invalid JSON body")
	}

	displayName := req.Name.Formatted
	if displayName == "" {
		displayName = strings.TrimSpace(req.Name.GivenName + " " + req.Name.FamilyName)
	}
	isActive := uint8(0)
	if req.Active {
		isActive = 1
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Read existing to keep role/sso_provider
	rows, err := h.CH.Query(ctx,
		`SELECT role, sso_provider FROM org_members FINAL
		 WHERE org_id='default' AND user_id=? LIMIT 1`, id)
	if err != nil {
		return scimError(c, fiber.StatusInternalServerError, "serverError", err.Error())
	}
	var role, ssoProvider string
	if rows.Next() {
		_ = rows.Scan(&role, &ssoProvider)
	}
	rows.Close()
	if role == "" {
		return scimError(c, fiber.StatusNotFound, "notFound", "user not found")
	}

	email := req.UserName
	if email == "" && len(req.Emails) > 0 {
		email = req.Emails[0].Value
	}

	if err := h.CH.Exec(ctx,
		`INSERT INTO org_members
		 (user_id, org_id, email, display_name, role, sso_provider, sso_subject, is_active, created_at, last_seen)
		 VALUES (?, 'default', ?, ?, ?, ?, ?, ?, now64(), now64())`,
		id, email, displayName, role, ssoProvider, req.ExternalID, isActive,
	); err != nil {
		return scimError(c, fiber.StatusInternalServerError, "serverError", err.Error())
	}

	slog.Info("scim: user replaced", "user_id", id, "active", req.Active)
	return c.JSON(buildScimUser(id, email, displayName, role, req.ExternalID, req.Active, time.Now()))
}

// PatchUser handles PATCH /scim/v2/Users/:id (partial update — typically active toggle)
func (h *Handler) PatchUser(c *fiber.Ctx) error {
	id := c.Params("id")
	var patch scimPatchOp
	if err := c.BodyParser(&patch); err != nil {
		return scimError(c, fiber.StatusBadRequest, "invalidValue", "invalid JSON body")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Read current values
	rows, err := h.CH.Query(ctx,
		`SELECT email, display_name, role, sso_provider, sso_subject, is_active, created_at
		 FROM org_members FINAL WHERE org_id='default' AND user_id=? LIMIT 1`, id)
	if err != nil {
		return scimError(c, fiber.StatusInternalServerError, "serverError", err.Error())
	}
	var email, displayName, role, ssoProvider, ssoSubject string
	var isActive uint8
	var createdAt time.Time
	if rows.Next() {
		_ = rows.Scan(&email, &displayName, &role, &ssoProvider, &ssoSubject, &isActive, &createdAt)
	}
	rows.Close()
	if email == "" {
		return scimError(c, fiber.StatusNotFound, "notFound", "user not found")
	}

	// Apply patch operations
	for _, op := range patch.Operations {
		switch strings.ToLower(op.Op) {
		case "replace":
			if v, ok := op.Value["active"]; ok {
				if active, ok := v.(bool); ok {
					if active {
						isActive = 1
					} else {
						isActive = 0
					}
				}
			}
			if v, ok := op.Value["displayName"]; ok {
				if s, ok := v.(string); ok {
					displayName = s
				}
			}
		}
	}

	if err := h.CH.Exec(ctx,
		`INSERT INTO org_members
		 (user_id, org_id, email, display_name, role, sso_provider, sso_subject, is_active, created_at, last_seen)
		 VALUES (?, 'default', ?, ?, ?, ?, ?, ?, ?, now64())`,
		id, email, displayName, role, ssoProvider, ssoSubject, isActive, createdAt,
	); err != nil {
		return scimError(c, fiber.StatusInternalServerError, "serverError", err.Error())
	}

	slog.Info("scim: user patched", "user_id", id, "active", isActive == 1)
	return c.JSON(buildScimUser(id, email, displayName, role, ssoSubject, isActive == 1, createdAt))
}

// DeleteUser handles DELETE /scim/v2/Users/:id (soft-delete: sets is_active=0)
func (h *Handler) DeleteUser(c *fiber.Ctx) error {
	id := c.Params("id")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := h.CH.Query(ctx,
		`SELECT email, display_name, role, sso_provider, sso_subject, created_at
		 FROM org_members FINAL WHERE org_id='default' AND user_id=? LIMIT 1`, id)
	if err != nil {
		return scimError(c, fiber.StatusInternalServerError, "serverError", err.Error())
	}
	var email, displayName, role, ssoProvider, ssoSubject string
	var createdAt time.Time
	if rows.Next() {
		_ = rows.Scan(&email, &displayName, &role, &ssoProvider, &ssoSubject, &createdAt)
	}
	rows.Close()
	if email == "" {
		return scimError(c, fiber.StatusNotFound, "notFound", "user not found")
	}

	if err := h.CH.Exec(ctx,
		`INSERT INTO org_members
		 (user_id, org_id, email, display_name, role, sso_provider, sso_subject, is_active, created_at, last_seen)
		 VALUES (?, 'default', ?, ?, ?, ?, ?, 0, ?, now64())`,
		id, email, displayName, role, ssoProvider, ssoSubject, createdAt,
	); err != nil {
		return scimError(c, fiber.StatusInternalServerError, "serverError", err.Error())
	}

	slog.Info("scim: user deprovisioned", "user_id", id, "email", email)
	return c.SendStatus(fiber.StatusNoContent)
}

// ── helpers ───────────────────────────────────────────────────────────────────

type scannable interface {
	Scan(dest ...interface{}) error
}

func scanUser(rows scannable) (scimUser, error) {
	var (
		userID, email, displayName, role, ssoSubject string
		isActive                                      uint8
		createdAt, lastSeen                           time.Time
	)
	if err := rows.Scan(
		&userID, &email, &displayName, &role,
		&ssoSubject, &isActive, &createdAt, &lastSeen,
	); err != nil {
		return scimUser{}, err
	}
	return buildScimUser(userID, email, displayName, role, ssoSubject, isActive == 1, createdAt), nil
}

func buildScimUser(id, email, displayName, role, externalID string, active bool, createdAt time.Time) scimUser {
	parts := strings.SplitN(displayName, " ", 2)
	given, family := displayName, ""
	if len(parts) == 2 {
		given, family = parts[0], parts[1]
	}

	return scimUser{
		Schemas:    []string{scimUserSchema},
		ID:         id,
		ExternalID: externalID,
		UserName:   email,
		Name: scimName{
			Formatted:  displayName,
			GivenName:  given,
			FamilyName: family,
		},
		Emails: []scimEmail{{Value: email, Primary: true}},
		Active: active,
		Meta: scimMeta{
			ResourceType: "User",
			Created:      createdAt.Format(time.RFC3339),
			Location:     fmt.Sprintf("/scim/v2/Users/%s", id),
		},
	}
}

func scimError(c *fiber.Ctx, status int, scimType, detail string) error {
	return c.Status(status).JSON(fiber.Map{
		"schemas":  []string{scimErrSchema},
		"scimType": scimType,
		"detail":   detail,
		"status":   fmt.Sprintf("%d", status),
	})
}
