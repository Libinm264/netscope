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

// TlsFlow contains TLS handshake / alert metadata for a flow.
type TlsFlow struct {
	RecordType        string   `json:"record_type"`
	Version           string   `json:"version"`
	SNI               string   `json:"sni,omitempty"`
	CipherSuites      []string `json:"cipher_suites,omitempty"`
	HasWeakCipher     bool     `json:"has_weak_cipher"`
	ChosenCipher      string   `json:"chosen_cipher,omitempty"`
	NegotiatedVersion string   `json:"negotiated_version,omitempty"`
	CertCN            string   `json:"cert_cn,omitempty"`
	CertSANs          []string `json:"cert_sans,omitempty"`
	CertExpiry        string   `json:"cert_expiry,omitempty"`
	CertExpired       bool     `json:"cert_expired"`
	CertIssuer        string   `json:"cert_issuer,omitempty"`
	AlertLevel        string   `json:"alert_level,omitempty"`
	AlertDescription  string   `json:"alert_description,omitempty"`
}

// IcmpFlow contains ICMP-specific metadata for a flow.
type IcmpFlow struct {
	IcmpType uint8    `json:"icmp_type"`
	IcmpCode uint8    `json:"icmp_code"`
	TypeStr  string   `json:"type_str"`
	EchoID   *uint16  `json:"echo_id,omitempty"`
	EchoSeq  *uint16  `json:"echo_seq,omitempty"`
	RttMs    *float64 `json:"rtt_ms,omitempty"`
}

// ArpFlow contains ARP-specific metadata for a flow.
type ArpFlow struct {
	Operation  string `json:"operation"`   // "who-has" | "is-at"
	SenderIP   string `json:"sender_ip"`
	SenderMAC  string `json:"sender_mac"`
	TargetIP   string `json:"target_ip"`
	TargetMAC  string `json:"target_mac"`
}

// TcpStats contains TCP health counters for a flow.
type TcpStats struct {
	Retransmissions uint32 `json:"retransmissions"`
	OutOfOrder      uint32 `json:"out_of_order"`
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
	TLS        *TlsFlow  `json:"tls,omitempty"`
	ICMP       *IcmpFlow `json:"icmp,omitempty"`
	ARP        *ArpFlow  `json:"arp,omitempty"`
	TCPStats   *TcpStats `json:"tcp_stats,omitempty"`

	// Process attribution — populated by the agent in eBPF mode only.
	// ProcessName is the OS process name (up to 15 chars on Linux).
	// PID is 0 for pcap-mode flows where attribution is unavailable.
	ProcessName string `json:"process_name,omitempty"`
	PID         uint32 `json:"pid,omitempty"`

	// Geo enrichment — populated by the hub at ingest time, not sent by agents.
	CountryCode string `json:"country_code,omitempty"`
	CountryName string `json:"country_name,omitempty"`
	ASOrg       string `json:"as_org,omitempty"`
	ThreatScore uint8  `json:"threat_score,omitempty"`
	ThreatLevel string `json:"threat_level,omitempty"`
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
