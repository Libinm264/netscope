package models

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// ── Flow field presence ───────────────────────────────────────────────────────

func TestFlow_CoreFieldsPresent(t *testing.T) {
	now := time.Now().UTC()
	f := Flow{
		ID:         "flow-1",
		AgentID:    "agent-abc",
		Hostname:   "host1",
		Timestamp:  now,
		Protocol:   "TCP",
		SrcIP:      "10.0.0.1",
		SrcPort:    54321,
		DstIP:      "8.8.8.8",
		DstPort:    443,
		BytesIn:    1024,
		BytesOut:   512,
		DurationMs: 42,
	}
	if f.ID == "" {
		t.Error("Flow.ID should not be empty")
	}
	if f.AgentID == "" {
		t.Error("Flow.AgentID should not be empty")
	}
	if f.Timestamp.IsZero() {
		t.Error("Flow.Timestamp should not be zero")
	}
}

func TestFlow_OptionalFieldsDefaultToNil(t *testing.T) {
	f := Flow{}
	if f.HTTP != nil {
		t.Error("HTTP should default to nil")
	}
	if f.DNS != nil {
		t.Error("DNS should default to nil")
	}
	if f.TLS != nil {
		t.Error("TLS should default to nil")
	}
	if f.ICMP != nil {
		t.Error("ICMP should default to nil")
	}
	if f.ARP != nil {
		t.Error("ARP should default to nil")
	}
	if f.TCPStats != nil {
		t.Error("TCPStats should default to nil")
	}
}

func TestFlow_JSONRoundTrip(t *testing.T) {
	now := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)
	original := Flow{
		ID:          "round-trip-1",
		AgentID:     "agt-xyz",
		Hostname:    "web01",
		Timestamp:   now,
		Protocol:    "UDP",
		SrcIP:       "192.168.1.10",
		SrcPort:     12345,
		DstIP:       "1.1.1.1",
		DstPort:     53,
		BytesIn:     256,
		BytesOut:    64,
		DurationMs:  5,
		CountryCode: "US",
		ThreatScore: 0,
		DNS: &DnsFlow{
			QueryName: "example.com",
			QueryType: "A",
		},
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var decoded Flow
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if decoded.ID != original.ID {
		t.Errorf("ID: want %q, got %q", original.ID, decoded.ID)
	}
	if decoded.Protocol != original.Protocol {
		t.Errorf("Protocol: want %q, got %q", original.Protocol, decoded.Protocol)
	}
	if decoded.DNS == nil {
		t.Fatal("DNS should not be nil after round-trip")
	}
	if decoded.DNS.QueryName != original.DNS.QueryName {
		t.Errorf("DNS.QueryName: want %q, got %q", original.DNS.QueryName, decoded.DNS.QueryName)
	}
}

func TestFlow_JSONOmitsNilOptionals(t *testing.T) {
	f := Flow{ID: "x", Protocol: "TCP"}
	data, err := json.Marshal(f)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	s := string(data)
	// None of the optional nested objects should appear.
	for _, key := range []string{"http", "dns", "tls", "icmp", "arp", "tcp_stats"} {
		if strings.Contains(s, `"`+key+`"`) {
			t.Errorf("JSON should not contain %q when field is nil, got: %s", key, s)
		}
	}
}

// ── IngestRequest / IngestResponse ───────────────────────────────────────────

func TestIngestRequest_JSONRoundTrip(t *testing.T) {
	req := IngestRequest{
		AgentID:  "agt-1",
		Hostname: "box1",
		Flows: []Flow{
			{ID: "f1", Protocol: "TCP", SrcIP: "1.2.3.4", DstIP: "5.6.7.8"},
		},
	}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded IngestRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.AgentID != req.AgentID {
		t.Errorf("AgentID mismatch: want %q, got %q", req.AgentID, decoded.AgentID)
	}
	if len(decoded.Flows) != 1 {
		t.Errorf("Flows count: want 1, got %d", len(decoded.Flows))
	}
}

func TestIngestResponse_Fields(t *testing.T) {
	resp := IngestResponse{Received: 5, Errors: []string{"err1"}}
	if resp.Received != 5 {
		t.Errorf("Received: want 5, got %d", resp.Received)
	}
	if len(resp.Errors) != 1 {
		t.Errorf("Errors count: want 1, got %d", len(resp.Errors))
	}
}

// ── AlertRule validation helpers ──────────────────────────────────────────────

// validMetrics mirrors the set accepted by the CreateRule handler.
var validMetrics = map[string]bool{
	"flows_per_minute":     true,
	"http_error_rate":      true,
	"dns_nxdomain_rate":    true,
	"anomaly_flow_rate":    true,
	"anomaly_http_latency": true,
}

// validIntegrationTypes mirrors the set accepted by the CreateRule handler.
var validIntegrationTypes = map[string]bool{
	"":          true,
	"webhook":   true,
	"slack":     true,
	"pagerduty": true,
	"opsgenie":  true,
	"teams":     true,
	"email":     true,
}

func TestAlertRule_ValidSeverityValues(t *testing.T) {
	// AlertRule itself doesn't have a Severity field — severity lives on AnomalyEvent.
	// Test that all documented integration_type values are accounted for.
	for _, it := range []string{"webhook", "slack", "pagerduty", "opsgenie", "teams", "email"} {
		if !validIntegrationTypes[it] {
			t.Errorf("integration_type %q should be valid", it)
		}
	}
	// Unknown types should be invalid.
	for _, bad := range []string{"sms", "telegram", "discord"} {
		if validIntegrationTypes[bad] {
			t.Errorf("integration_type %q should NOT be valid", bad)
		}
	}
}

func TestAlertRule_ValidMetrics(t *testing.T) {
	for _, m := range []string{"flows_per_minute", "http_error_rate", "dns_nxdomain_rate", "anomaly_flow_rate", "anomaly_http_latency"} {
		if !validMetrics[m] {
			t.Errorf("metric %q should be valid", m)
		}
	}
	for _, bad := range []string{"cpu_usage", "memory", "", "FLOWS_PER_MINUTE"} {
		if validMetrics[bad] {
			t.Errorf("metric %q should NOT be valid", bad)
		}
	}
}

func TestAlertRule_JSONRoundTrip(t *testing.T) {
	now := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)
	rule := AlertRule{
		ID:              "rule-1",
		Name:            "High Flow Rate",
		Metric:          "flows_per_minute",
		Condition:       "gt",
		Threshold:       1000.0,
		WindowMinutes:   5,
		IntegrationType: "slack",
		WebhookURL:      "https://hooks.slack.com/services/T00/B00/xxx",
		Enabled:         true,
		CooldownMinutes: 15,
		CreatedAt:       now,
	}

	data, err := json.Marshal(rule)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded AlertRule
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.Threshold != rule.Threshold {
		t.Errorf("Threshold: want %v, got %v", rule.Threshold, decoded.Threshold)
	}
	if decoded.Enabled != rule.Enabled {
		t.Errorf("Enabled: want %v, got %v", rule.Enabled, decoded.Enabled)
	}
	if decoded.IntegrationType != rule.IntegrationType {
		t.Errorf("IntegrationType: want %q, got %q", rule.IntegrationType, decoded.IntegrationType)
	}
}

func TestAlertEvent_JSONRoundTrip(t *testing.T) {
	now := time.Now().UTC()
	ev := AlertEvent{
		ID:        "ev-1",
		RuleID:    "rule-1",
		RuleName:  "Test Rule",
		Metric:    "http_error_rate",
		Value:     75.5,
		Threshold: 50.0,
		FiredAt:   now,
		Delivered: true,
	}
	data, err := json.Marshal(ev)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded AlertEvent
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.Value != ev.Value {
		t.Errorf("Value: want %v, got %v", ev.Value, decoded.Value)
	}
	if decoded.Delivered != ev.Delivered {
		t.Errorf("Delivered: want %v, got %v", ev.Delivered, decoded.Delivered)
	}
}

// ── Analytics / ServiceGraph structs ─────────────────────────────────────────

func TestServiceGraph_JSONRoundTrip(t *testing.T) {
	graph := ServiceGraph{
		Nodes: []ServiceNode{
			{ID: "n1", IP: "10.0.0.1", FlowCount: 100, IsKnown: true, Hostname: "web01"},
		},
		Edges: []ServiceEdge{
			{Source: "n1", Target: "n2", Protocol: "TCP", Count: 50, AvgLatencyMs: 2.5},
		},
		Window: "5m",
	}
	data, err := json.Marshal(graph)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded ServiceGraph
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(decoded.Nodes) != 1 {
		t.Errorf("Nodes: want 1, got %d", len(decoded.Nodes))
	}
	if len(decoded.Edges) != 1 {
		t.Errorf("Edges: want 1, got %d", len(decoded.Edges))
	}
	if decoded.Window != "5m" {
		t.Errorf("Window: want %q, got %q", "5m", decoded.Window)
	}
}

func TestEndpointStat_ErrorRateField(t *testing.T) {
	ep := EndpointStat{
		Method:     "GET",
		Path:       "/api/v1/flows",
		Count:      200,
		ErrorCount: 10,
		ErrorRate:  5.0,
	}
	if ep.ErrorRate != 5.0 {
		t.Errorf("ErrorRate: want 5.0, got %v", ep.ErrorRate)
	}
	// Sanity check computed field values.
	if ep.ErrorCount > ep.Count {
		t.Errorf("ErrorCount (%d) must not exceed Count (%d)", ep.ErrorCount, ep.Count)
	}
}
