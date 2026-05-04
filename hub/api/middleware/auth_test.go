package middleware

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
)

// successHandler is a trivial handler that returns 200 with the caller's role.
var successHandler = func(c *fiber.Ctx) error {
	role, _ := c.Locals("role").(string)
	return c.JSON(fiber.Map{"role": role})
}

// buildApp registers the given middleware chain and successHandler at GET /protected.
func buildApp(middlewares ...fiber.Handler) *fiber.App {
	app := fiber.New(fiber.Config{
		// Surface panics as 500 instead of crashing the test.
		ErrorHandler: fiber.DefaultErrorHandler,
	})
	app.Get("/protected", append(middlewares, successHandler)...)
	return app
}

// parseBody reads resp.Body and decodes it into a map.
func parseBody(t *testing.T, r io.Reader) map[string]interface{} {
	t.Helper()
	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("parse JSON (%s): %v", data, err)
	}
	return m
}

// ── TokenAuth ─────────────────────────────────────────────────────────────────

func TestTokenAuth_MissingKey_Returns401(t *testing.T) {
	app := buildApp(TokenAuth("bootstrap-secret", nil))
	req := httptest.NewRequest("GET", "/protected", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Errorf("want 401, got %d", resp.StatusCode)
	}
}

func TestTokenAuth_WrongKey_Returns401(t *testing.T) {
	app := buildApp(TokenAuth("bootstrap-secret", nil))
	req := httptest.NewRequest("GET", "/protected", nil)
	req.Header.Set("X-Api-Key", "wrong-key")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Errorf("want 401, got %d", resp.StatusCode)
	}
}

func TestTokenAuth_BootstrapKey_SetsAdminRole(t *testing.T) {
	app := buildApp(TokenAuth("bootstrap-secret", nil))
	req := httptest.NewRequest("GET", "/protected", nil)
	req.Header.Set("X-Api-Key", "bootstrap-secret")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("want 200, got %d", resp.StatusCode)
	}
	body := parseBody(t, resp.Body)
	if body["role"] != "admin" {
		t.Errorf("want role=admin, got %v", body["role"])
	}
}

func TestTokenAuth_KeyViaQueryParam_Accepted(t *testing.T) {
	app := buildApp(TokenAuth("qs-key", nil))
	req := httptest.NewRequest("GET", "/protected?api_key=qs-key", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("want 200, got %d", resp.StatusCode)
	}
}

func TestTokenAuth_NilCH_WrongKey_Returns401(t *testing.T) {
	// When CH is nil, any non-bootstrap key must be rejected.
	app := buildApp(TokenAuth("bootstrap", nil))
	req := httptest.NewRequest("GET", "/protected", nil)
	req.Header.Set("X-Api-Key", "some-db-token")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Errorf("want 401, got %d", resp.StatusCode)
	}
}

// ── RequireAdmin ──────────────────────────────────────────────────────────────

func TestRequireAdmin_AdminRole_Passes(t *testing.T) {
	// Inject role via a stub middleware that sets Locals directly.
	setAdmin := func(c *fiber.Ctx) error {
		c.Locals("role", "admin")
		return c.Next()
	}
	app := buildApp(setAdmin, RequireAdmin())
	req := httptest.NewRequest("GET", "/protected", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("want 200, got %d", resp.StatusCode)
	}
}

func TestRequireAdmin_ViewerRole_Returns403(t *testing.T) {
	setViewer := func(c *fiber.Ctx) error {
		c.Locals("role", "viewer")
		return c.Next()
	}
	app := buildApp(setViewer, RequireAdmin())
	req := httptest.NewRequest("GET", "/protected", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 403 {
		t.Errorf("want 403, got %d", resp.StatusCode)
	}
}

func TestRequireAdmin_NoRole_Returns403(t *testing.T) {
	// No Locals set at all — should also be forbidden.
	app := buildApp(RequireAdmin())
	req := httptest.NewRequest("GET", "/protected", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 403 {
		t.Errorf("want 403, got %d", resp.StatusCode)
	}
}

// ── APIKeyAuth backwards-compat wrapper ───────────────────────────────────────

func TestAPIKeyAuth_BootstrapKey_Returns200(t *testing.T) {
	app := buildApp(APIKeyAuth("compat-key"))
	req := httptest.NewRequest("GET", "/protected", nil)
	req.Header.Set("X-Api-Key", "compat-key")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("want 200, got %d", resp.StatusCode)
	}
}

// ── RequireAdminOrAbove ────────────────────────────────────────────────────────

func TestRequireAdminOrAbove_Owner_Passes(t *testing.T) {
	setOwner := func(c *fiber.Ctx) error {
		c.Locals("role", "owner")
		return c.Next()
	}
	app := buildApp(setOwner, RequireAdminOrAbove())
	req := httptest.NewRequest("GET", "/protected", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("want 200, got %d", resp.StatusCode)
	}
}

func TestRequireAdminOrAbove_Admin_Passes(t *testing.T) {
	setAdmin := func(c *fiber.Ctx) error {
		c.Locals("role", "admin")
		return c.Next()
	}
	app := buildApp(setAdmin, RequireAdminOrAbove())
	req := httptest.NewRequest("GET", "/protected", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("want 200, got %d", resp.StatusCode)
	}
}

func TestRequireAdminOrAbove_Viewer_Returns403(t *testing.T) {
	setViewer := func(c *fiber.Ctx) error {
		c.Locals("role", "viewer")
		return c.Next()
	}
	app := buildApp(setViewer, RequireAdminOrAbove())
	req := httptest.NewRequest("GET", "/protected", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 403 {
		t.Errorf("want 403, got %d", resp.StatusCode)
	}
}

func TestRequireAdminOrAbove_Analyst_Returns403(t *testing.T) {
	setAnalyst := func(c *fiber.Ctx) error {
		c.Locals("role", "analyst")
		return c.Next()
	}
	app := buildApp(setAnalyst, RequireAdminOrAbove())
	req := httptest.NewRequest("GET", "/protected", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 403 {
		t.Errorf("want 403, got %d", resp.StatusCode)
	}
}

// ── RequireOwner ──────────────────────────────────────────────────────────────

func TestRequireOwner_Owner_Passes(t *testing.T) {
	setOwner := func(c *fiber.Ctx) error {
		c.Locals("role", "owner")
		return c.Next()
	}
	app := buildApp(setOwner, RequireOwner())
	req := httptest.NewRequest("GET", "/protected", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("want 200, got %d", resp.StatusCode)
	}
}

func TestRequireOwner_Admin_Returns403(t *testing.T) {
	setAdmin := func(c *fiber.Ctx) error {
		c.Locals("role", "admin")
		return c.Next()
	}
	app := buildApp(setAdmin, RequireOwner())
	req := httptest.NewRequest("GET", "/protected", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 403 {
		t.Errorf("want 403, got %d", resp.StatusCode)
	}
}

// ── DemoGuard ─────────────────────────────────────────────────────────────────

func TestDemoGuard_GetAllowed_ForDemoSession(t *testing.T) {
	setDemo := func(c *fiber.Ctx) error {
		c.Locals("is_demo", true)
		return c.Next()
	}
	app := buildApp(setDemo, DemoGuard())
	req := httptest.NewRequest("GET", "/protected", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("want 200 for GET in demo mode, got %d", resp.StatusCode)
	}
}

func TestDemoGuard_PostBlocked_ForDemoSession(t *testing.T) {
	setDemo := func(c *fiber.Ctx) error {
		c.Locals("is_demo", true)
		return c.Next()
	}
	app := fiber.New()
	app.Post("/protected", setDemo, DemoGuard(), successHandler)
	req := httptest.NewRequest("POST", "/protected", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 403 {
		t.Errorf("want 403 for POST in demo mode, got %d", resp.StatusCode)
	}
}

func TestDemoGuard_PostAllowed_WhenNotDemo(t *testing.T) {
	setNotDemo := func(c *fiber.Ctx) error {
		c.Locals("is_demo", false)
		return c.Next()
	}
	app := fiber.New()
	app.Post("/protected", setNotDemo, DemoGuard(), successHandler)
	req := httptest.NewRequest("POST", "/protected", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("want 200 for POST when not demo, got %d", resp.StatusCode)
	}
}
