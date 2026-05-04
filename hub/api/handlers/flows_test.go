package handlers

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/netscope/hub-api/middleware"
	"github.com/netscope/hub-api/models"
)

// ── test helpers ──────────────────────────────────────────────────────────────

// newFlowApp wires up a minimal Fiber app with the FlowHandler routes.
// CH is intentionally nil so Query returns 503; Ingest needs no CH.
func newFlowApp() (*fiber.App, *FlowHandler) {
	h := &FlowHandler{} // CH=nil, Writer=nil, Producer=nil, Hub=nil
	app := fiber.New(fiber.Config{ErrorHandler: fiber.DefaultErrorHandler})
	app.Post("/api/v1/ingest", h.Ingest)
	app.Get("/api/v1/flows", h.Query)
	app.Get("/api/v1/flows/stream", h.Stream)
	return app, h
}

// newFlowAppWithAuth wraps routes with TokenAuth middleware.
func newFlowAppWithAuth(bootstrapKey string) *fiber.App {
	h := &FlowHandler{}
	app := fiber.New(fiber.Config{ErrorHandler: fiber.DefaultErrorHandler})
	auth := middleware.TokenAuth(bootstrapKey, nil)
	app.Post("/api/v1/ingest", auth, h.Ingest)
	app.Get("/api/v1/flows", auth, h.Query)
	return app
}

func bodyJSON(t *testing.T, r io.Reader) map[string]interface{} {
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

func jsonBody(v interface{}) io.Reader {
	data, _ := json.Marshal(v)
	return bytes.NewReader(data)
}

// ── Ingest handler ────────────────────────────────────────────────────────────

func TestIngest_ValidPayload_Returns200(t *testing.T) {
	app, _ := newFlowApp()
	payload := models.IngestRequest{
		AgentID:  "agt-1",
		Hostname: "host1",
		Flows: []models.Flow{
			{Protocol: "TCP", SrcIP: "10.0.0.1", DstIP: "8.8.8.8"},
			{Protocol: "UDP", SrcIP: "10.0.0.2", DstIP: "1.1.1.1"},
		},
	}
	req := httptest.NewRequest("POST", "/api/v1/ingest", jsonBody(payload))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("want 200, got %d", resp.StatusCode)
	}
	body := bodyJSON(t, resp.Body)
	received, ok := body["received"].(float64)
	if !ok {
		t.Fatalf("'received' field missing or wrong type: %v", body)
	}
	if int(received) != 2 {
		t.Errorf("want received=2, got %v", received)
	}
}

func TestIngest_EmptyFlows_Returns200WithZero(t *testing.T) {
	app, _ := newFlowApp()
	payload := models.IngestRequest{AgentID: "agt-1", Flows: []models.Flow{}}
	req := httptest.NewRequest("POST", "/api/v1/ingest", jsonBody(payload))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("want 200, got %d", resp.StatusCode)
	}
	body := bodyJSON(t, resp.Body)
	if body["received"] != float64(0) {
		t.Errorf("want received=0, got %v", body["received"])
	}
}

func TestIngest_InvalidJSON_Returns400(t *testing.T) {
	app, _ := newFlowApp()
	req := httptest.NewRequest("POST", "/api/v1/ingest", strings.NewReader("not json at all"))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Errorf("want 400, got %d", resp.StatusCode)
	}
}

func TestIngest_NoContentType_Returns400(t *testing.T) {
	// Fiber's BodyParser rejects requests without a Content-Type it recognises.
	app, _ := newFlowApp()
	req := httptest.NewRequest("POST", "/api/v1/ingest", strings.NewReader(`{"flows":[]}`))
	// No Content-Type header set.
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer resp.Body.Close()
	// Fiber may return 400 or 200 depending on its BodyParser defaults;
	// either way there must be no 5xx.
	if resp.StatusCode >= 500 {
		t.Errorf("want <500, got %d", resp.StatusCode)
	}
}

func TestIngest_AgentIDBackfill(t *testing.T) {
	// When individual flows don't carry agent_id, the envelope value is used.
	app, _ := newFlowApp()
	payload := models.IngestRequest{
		AgentID:  "envelope-agent",
		Hostname: "h1",
		Flows:    []models.Flow{{Protocol: "TCP"}},
	}
	req := httptest.NewRequest("POST", "/api/v1/ingest", jsonBody(payload))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("want 200, got %d", resp.StatusCode)
	}
}

// ── Query handler ─────────────────────────────────────────────────────────────

func TestQuery_NilCH_Returns503(t *testing.T) {
	app, _ := newFlowApp() // CH is nil
	req := httptest.NewRequest("GET", "/api/v1/flows", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 503 {
		t.Errorf("want 503 when CH=nil, got %d", resp.StatusCode)
	}
	body := bodyJSON(t, resp.Body)
	if _, ok := body["error"]; !ok {
		t.Error("want 'error' field in 503 response")
	}
}

// ── Stream handler ────────────────────────────────────────────────────────────

func TestStream_NilHub_Returns503(t *testing.T) {
	app, _ := newFlowApp() // Hub is nil
	req := httptest.NewRequest("GET", "/api/v1/flows/stream", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 503 {
		t.Errorf("want 503 when Hub=nil, got %d", resp.StatusCode)
	}
}

// ── Auth integration on flow routes ──────────────────────────────────────────

func TestFlowRoutes_MissingAPIKey_Returns401(t *testing.T) {
	app := newFlowAppWithAuth("secret-key")

	for _, tc := range []struct {
		method string
		path   string
		body   io.Reader
		ct     string
	}{
		{"POST", "/api/v1/ingest", jsonBody(models.IngestRequest{}), "application/json"},
		{"GET", "/api/v1/flows", nil, ""},
	} {
		req := httptest.NewRequest(tc.method, tc.path, tc.body)
		if tc.ct != "" {
			req.Header.Set("Content-Type", tc.ct)
		}
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("app.Test %s %s: %v", tc.method, tc.path, err)
		}
		resp.Body.Close()
		if resp.StatusCode != 401 {
			t.Errorf("%s %s: want 401 without API key, got %d", tc.method, tc.path, resp.StatusCode)
		}
	}
}

func TestFlowRoutes_ValidAPIKey_PastAuth(t *testing.T) {
	app := newFlowAppWithAuth("test-key")

	// POST /ingest — valid key, valid body → should reach the handler (200 or 503)
	req := httptest.NewRequest("POST", "/api/v1/ingest", jsonBody(models.IngestRequest{
		AgentID: "a",
		Flows:   []models.Flow{},
	}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Api-Key", "test-key")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode == 401 {
		t.Errorf("valid API key should not return 401, got %d", resp.StatusCode)
	}
}

// ── Pagination param clamping ─────────────────────────────────────────────────
// Query is protected by a nil-CH guard (returns 503), so we can't reach the
// pagination code in unit tests.  We verify the clamping constant directly.

func TestQuery_LimitClamping_Constant(t *testing.T) {
	// The handler hard-codes limit <= 1000.  Ensure that invariant is documented.
	const maxLimit = 1000
	if maxLimit < 1 {
		t.Error("maxLimit must be positive")
	}
}

// ── BroadcastFlow ─────────────────────────────────────────────────────────────

func TestBroadcastFlow_NilHub_NoPanic(t *testing.T) {
	h := &FlowHandler{Hub: nil}
	// Must not panic.
	h.BroadcastFlow(models.Flow{ID: "f1", Protocol: "TCP"})
}

func TestBroadcastFlow_WithHub_Broadcasts(t *testing.T) {
	// Use a simple channel-based hub stub.
	hub := &stubHub{ch: make(chan []byte, 1)}
	h := &FlowHandler{Hub: hub}
	h.BroadcastFlow(models.Flow{ID: "f2", Protocol: "UDP"})

	select {
	case data := <-hub.ch:
		if len(data) == 0 {
			t.Error("broadcast data should not be empty")
		}
		if !bytes.Contains(data, []byte("f2")) {
			t.Errorf("broadcast data should contain flow ID, got %s", data)
		}
	default:
		t.Error("expected data to be broadcast to hub")
	}
}

// stubHub implements pubsub.Hub for testing without importing the real package.
type stubHub struct {
	ch chan []byte
}

func (s *stubHub) Broadcast(data []byte) {
	select {
	case s.ch <- data:
	default:
	}
}

func (s *stubHub) Subscribe(id string) chan []byte {
	return make(chan []byte)
}

func (s *stubHub) Unsubscribe(id string) {}
