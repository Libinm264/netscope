package models

// ── Service dependency graph ───────────────────────────────────────────────────

// ServiceNode represents a unique IP (host) observed in network flows.
type ServiceNode struct {
	ID        string `json:"id"`
	IP        string `json:"ip"`
	FlowCount int64  `json:"flow_count"`
	IsKnown   bool   `json:"is_known"`   // true if it matches a registered agent
	Hostname  string `json:"hostname"`
}

// ServiceEdge represents aggregated traffic between two IPs.
type ServiceEdge struct {
	Source       string  `json:"source"`
	Target       string  `json:"target"`
	Protocol     string  `json:"protocol"`
	Count        int64   `json:"count"`
	AvgLatencyMs float64 `json:"avg_latency_ms"`
	BytesTotal   int64   `json:"bytes_total"`
}

// ServiceGraph is the full topology returned by GET /api/v1/services/graph.
type ServiceGraph struct {
	Nodes  []ServiceNode `json:"nodes"`
	Edges  []ServiceEdge `json:"edges"`
	Window string        `json:"window"`
}

// ── HTTP endpoint analytics ────────────────────────────────────────────────────

// EndpointStat holds aggregated latency and error metrics for one HTTP endpoint.
type EndpointStat struct {
	Method       string  `json:"method"`
	Path         string  `json:"path"`
	Count        int64   `json:"count"`
	SuccessCount int64   `json:"success_count"`
	ErrorCount   int64   `json:"error_count"`
	ErrorRate    float64 `json:"error_rate"`     // percent (0-100)
	AvgLatencyMs float64 `json:"avg_latency_ms"`
	P50Ms        float64 `json:"p50_ms"`
	P95Ms        float64 `json:"p95_ms"`
	P99Ms        float64 `json:"p99_ms"`
}

// EndpointStatsResponse is returned by GET /api/v1/analytics/endpoints.
type EndpointStatsResponse struct {
	Endpoints []EndpointStat `json:"endpoints"`
	Window    string         `json:"window"`
	Total     int            `json:"total"`
}

// ── OpenTelemetry export ───────────────────────────────────────────────────────

// OtelAttribute is a key-value pair in an OTEL span.
type OtelAttribute struct {
	Key   string    `json:"key"`
	Value OtelValue `json:"value"`
}

// OtelValue holds a single typed value.
type OtelValue struct {
	StringValue string `json:"stringValue,omitempty"`
	IntValue    string `json:"intValue,omitempty"`
}

// OtelStatus mirrors the OTLP Status message.
type OtelStatus struct {
	Code    int    `json:"code"`
	Message string `json:"message,omitempty"`
}

// OtelSpan is a minimal OTLP-compatible span (JSON wire format).
type OtelSpan struct {
	TraceID           string          `json:"traceId"`
	SpanID            string          `json:"spanId"`
	Name              string          `json:"name"`
	Kind              int             `json:"kind"`
	StartTimeUnixNano string          `json:"startTimeUnixNano"`
	EndTimeUnixNano   string          `json:"endTimeUnixNano"`
	Attributes        []OtelAttribute `json:"attributes"`
	Status            OtelStatus      `json:"status"`
}
