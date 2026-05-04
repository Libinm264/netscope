package util

import (
	"encoding/json"
	"errors"
	"io"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
)

// newTestApp returns a minimal Fiber app with the provided handler registered
// at GET /test.  The handler receives a real *fiber.Ctx so InternalError can
// introspect Method() and Path().
func newTestApp(handler fiber.Handler) *fiber.App {
	app := fiber.New(fiber.Config{
		// Suppress the default error handler's output during tests.
		ErrorHandler: func(c *fiber.Ctx, _ error) error {
			return c.Status(500).SendString("test error handler")
		},
	})
	app.Get("/test", handler)
	return app
}

// TestInternalError_StatusCode verifies that InternalError always returns 500.
func TestInternalError_StatusCode(t *testing.T) {
	app := newTestApp(func(c *fiber.Ctx) error {
		return InternalError(c, errors.New("db connection failed"))
	})

	req := httptest.NewRequest("GET", "/test", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 500 {
		t.Errorf("want status 500, got %d", resp.StatusCode)
	}
}

// TestInternalError_JSONBody verifies the response body is JSON with "error" key
// and that the internal error message is NOT leaked to the client.
func TestInternalError_JSONBody(t *testing.T) {
	sensitiveMsg := "SELECT * FROM passwords WHERE user='admin'"
	app := newTestApp(func(c *fiber.Ctx) error {
		return InternalError(c, errors.New(sensitiveMsg))
	})

	req := httptest.NewRequest("GET", "/test", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	// Must be valid JSON.
	var payload map[string]interface{}
	if jsonErr := json.Unmarshal(body, &payload); jsonErr != nil {
		t.Fatalf("response is not valid JSON: %v — body: %s", jsonErr, body)
	}

	// Must have an "error" key.
	errVal, ok := payload["error"]
	if !ok {
		t.Fatalf("response JSON has no 'error' key; body: %s", body)
	}

	// The value must be a non-empty string.
	errStr, ok := errVal.(string)
	if !ok || errStr == "" {
		t.Errorf("'error' value should be a non-empty string, got %T(%v)", errVal, errVal)
	}

	// Must NOT leak the sensitive error detail.
	if errStr == sensitiveMsg {
		t.Errorf("InternalError leaked internal error message to client: %q", errStr)
	}
}

// TestInternalError_ContentType verifies the response is application/json.
func TestInternalError_ContentType(t *testing.T) {
	app := newTestApp(func(c *fiber.Ctx) error {
		return InternalError(c, errors.New("boom"))
	})

	req := httptest.NewRequest("GET", "/test", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer resp.Body.Close()

	ct := resp.Header.Get("Content-Type")
	// Fiber sets application/json; check the prefix.
	if len(ct) < 16 || ct[:16] != "application/json" {
		t.Errorf("want Content-Type application/json, got %q", ct)
	}
}

// TestInternalError_MultipleErrors verifies that repeated calls each return 500
// (regression check that the helper is stateless).
func TestInternalError_MultipleErrors(t *testing.T) {
	messages := []string{"err one", "err two", "err three"}
	for _, msg := range messages {
		capturedMsg := msg
		app := newTestApp(func(c *fiber.Ctx) error {
			return InternalError(c, errors.New(capturedMsg))
		})
		req := httptest.NewRequest("GET", "/test", nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("app.Test (%q): %v", capturedMsg, err)
		}
		resp.Body.Close()
		if resp.StatusCode != 500 {
			t.Errorf("msg %q: want 500, got %d", capturedMsg, resp.StatusCode)
		}
	}
}
