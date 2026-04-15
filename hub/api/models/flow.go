package models

import "time"

// HttpFlow contains HTTP-specific metadata for a flow.
type HttpFlow struct {
	Method    string `json:"method"`
	Path      string `json:"path"`
	Status    int    `json:"status"`
	LatencyMs int64  `json:"latency_ms"`
}

// DnsFlow contains DNS-specific metadata for a flow.
type DnsFlow struct {
	QueryName  string   `json:"query_name"`
	QueryType  string   `json:"query_type"`
	IsResponse bool     `json:"is_response"`
	Answers    []string `json:"answers"`
	Rcode      int      `json:"rcode"`
}

// Flow represents a single network flow record as emitted by an agent.
type Flow struct {
	ID         string    `json:"id"`
	AgentID    string    `json:"agent_id"`
	Hostname   string    `json:"hostname"`
	Timestamp  time.Time `json:"timestamp"`
	Protocol   string    `json:"protocol"`
	SrcIP      string    `json:"src_ip"`
	SrcPort    uint16    `json:"src_port"`
	DstIP      string    `json:"dst_ip"`
	DstPort    uint16    `json:"dst_port"`
	BytesIn    uint64    `json:"bytes_in"`
	BytesOut   uint64    `json:"bytes_out"`
	DurationMs uint32    `json:"duration_ms"`
	Info       string    `json:"info"`
	HTTP       *HttpFlow `json:"http,omitempty"`
	DNS        *DnsFlow  `json:"dns,omitempty"`
}

// IngestRequest is the payload sent by an agent to POST /api/v1/ingest.
type IngestRequest struct {
	AgentID  string `json:"agent_id"`
	Hostname string `json:"hostname"`
	Flows    []Flow `json:"flows"`
}

// IngestResponse is returned after a successful ingestion.
type IngestResponse struct {
	Received int      `json:"received"`
	Errors   []string `json:"errors"`
}
