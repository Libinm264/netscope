/// Shared data types used across all NetScope components.
use chrono::{DateTime, Utc};
use serde::{Deserialize, Serialize};

// ── Protocol enum ─────────────────────────────────────────────────────────────

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
pub enum Protocol {
    Http,
    Https,
    Http2,
    Grpc,
    Dns,
    Tcp,
    Udp,
    Tls,
    Icmp,
    Arp,
    Unknown,
}

impl std::fmt::Display for Protocol {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            Protocol::Http    => write!(f, "HTTP"),
            Protocol::Https   => write!(f, "HTTPS"),
            Protocol::Http2   => write!(f, "HTTP/2"),
            Protocol::Grpc    => write!(f, "gRPC"),
            Protocol::Dns     => write!(f, "DNS"),
            Protocol::Tcp     => write!(f, "TCP"),
            Protocol::Udp     => write!(f, "UDP"),
            Protocol::Tls     => write!(f, "TLS"),
            Protocol::Icmp    => write!(f, "ICMP"),
            Protocol::Arp     => write!(f, "ARP"),
            Protocol::Unknown => write!(f, "UNKNOWN"),
        }
    }
}

// ── Core flow types ───────────────────────────────────────────────────────────

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Flow {
    pub id: String,
    pub timestamp: DateTime<Utc>,
    pub src_ip: String,
    pub dst_ip: String,
    pub src_port: u16,
    pub dst_port: u16,
    pub protocol: Protocol,
    pub bytes_in: u64,
    pub bytes_out: u64,
    pub payload: Option<FlowPayload>,
    /// TCP retransmission / out-of-order statistics, present for TCP-based flows.
    pub tcp_stats: Option<TcpStats>,
    /// OS process that owns this connection (eBPF mode only).
    pub process: Option<ProcessInfo>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub enum FlowPayload {
    Http(HttpFlow),
    Http2(Http2Flow),
    Dns(DnsFlow),
    Tls(TlsHandshake),
    Icmp(IcmpFlow),
    Arp(ArpFlow),
}

// ── HTTP ──────────────────────────────────────────────────────────────────────

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct HttpFlow {
    pub request: Option<HttpRequest>,
    pub response: Option<HttpResponse>,
    /// Round-trip latency in milliseconds (set when both request and response seen).
    pub latency_ms: Option<u64>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct HttpRequest {
    pub method: String,
    pub path: String,
    pub version: String,
    pub headers: Vec<(String, String)>,
    pub body_preview: Option<String>,
    pub timestamp: DateTime<Utc>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct HttpResponse {
    pub status_code: u16,
    pub status_text: String,
    pub version: String,
    pub headers: Vec<(String, String)>,
    pub body_preview: Option<String>,
    pub timestamp: DateTime<Utc>,
}

// ── HTTP/2 + gRPC ─────────────────────────────────────────────────────────────

/// A decoded HTTP/2 exchange (one request stream + optional response stream).
/// When `grpc_service` is Some, the frame carried a gRPC call.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Http2Flow {
    /// HTTP/2 stream ID (odd = client-initiated).
    pub stream_id: u32,
    /// Decoded request pseudo-headers + headers from the HEADERS frame.
    pub request: Option<Http2Request>,
    /// Decoded response pseudo-headers from the server HEADERS frame.
    pub response: Option<Http2Response>,
    /// Round-trip latency in milliseconds (set when both sides seen).
    pub latency_ms: Option<u64>,
    /// gRPC service extracted from `:path` (`/package.Service/Method` → `package.Service`).
    pub grpc_service: Option<String>,
    /// gRPC method extracted from `:path`.
    pub grpc_method: Option<String>,
    /// gRPC status code from the `grpc-status` trailer (0 = OK).
    pub grpc_status: Option<u32>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Http2Request {
    pub method: String,
    pub path: String,
    pub authority: String,
    pub scheme: String,
    /// All decoded headers (pseudo + regular), in order.
    pub headers: Vec<(String, String)>,
    pub timestamp: DateTime<Utc>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Http2Response {
    pub status_code: u16,
    /// All decoded headers (pseudo + regular), in order.
    pub headers: Vec<(String, String)>,
    pub timestamp: DateTime<Utc>,
}

// ── DNS ───────────────────────────────────────────────────────────────────────

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct DnsFlow {
    pub transaction_id: u16,
    pub query_name: String,
    pub query_type: String,
    pub is_response: bool,
    pub answers: Vec<DnsAnswer>,
    pub rcode: Option<String>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct DnsAnswer {
    pub name: String,
    pub record_type: String,
    pub ttl: u32,
    pub data: String,
}

// ── TLS ───────────────────────────────────────────────────────────────────────

/// Decoded TLS handshake or alert record.
///
/// A single TLS TCP connection will generate multiple TlsHandshake flows:
/// one for ClientHello, one for ServerHello, one for Certificate, etc.
/// The `record_type` field identifies which message this represents.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct TlsHandshake {
    /// Handshake message type: "ClientHello", "ServerHello", "Certificate", "Alert",
    /// "ChangeCipherSpec", "Finished".
    pub record_type: String,

    /// TLS version from the record header (e.g. "TLS 1.2").
    /// For TLS 1.3 ClientHello this reads as "TLS 1.2" (compatibility mode);
    /// the true version appears in the Supported Versions extension.
    pub version: String,

    // ── ClientHello ──────────────────────────────────────────────────────────
    /// Server Name Indication — the hostname the client is connecting to.
    pub sni: Option<String>,
    /// Cipher suites offered by the client (human-readable IANA names).
    pub cipher_suites: Vec<String>,
    /// Whether any of the offered cipher suites are considered weak/deprecated.
    pub has_weak_cipher: bool,

    // ── ServerHello ──────────────────────────────────────────────────────────
    /// Cipher suite chosen by the server.
    pub chosen_cipher: Option<String>,
    /// Negotiated protocol version (may be "TLS 1.3" even when record header says 1.2).
    pub negotiated_version: Option<String>,

    // ── Certificate ──────────────────────────────────────────────────────────
    /// Subject Common Name.
    pub cert_cn: Option<String>,
    /// Subject Alternative Names (DNS names and IPs).
    pub cert_sans: Vec<String>,
    /// Certificate expiry date (ISO 8601: "YYYY-MM-DD").
    pub cert_expiry: Option<String>,
    /// True if the certificate's NotAfter date is in the past.
    pub cert_expired: bool,
    /// Issuer Common Name.
    pub cert_issuer: Option<String>,

    // ── Alert ────────────────────────────────────────────────────────────────
    /// Alert severity level: "warning" or "fatal".
    pub alert_level: Option<String>,
    /// Alert description: "handshake_failure", "certificate_expired", "unknown_ca", etc.
    pub alert_description: Option<String>,
}

// ── ICMP ──────────────────────────────────────────────────────────────────────

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct IcmpFlow {
    pub icmp_type: u8,
    pub icmp_code: u8,
    /// Human-readable type string, e.g. "Echo Request", "Destination Unreachable".
    pub type_str: String,
    /// For Echo Request / Echo Reply: the ICMP identifier field.
    pub echo_id: Option<u16>,
    /// For Echo Request / Echo Reply: the sequence number.
    pub echo_seq: Option<u16>,
    /// Round-trip time in ms — set on Echo Reply when the matching request is found.
    pub rtt_ms: Option<f64>,
}

// ── ARP ───────────────────────────────────────────────────────────────────────

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ArpFlow {
    /// "who-has" (ARP Request) or "is-at" (ARP Reply).
    pub operation: String,
    pub sender_ip: String,
    pub sender_mac: String,
    pub target_ip: String,
    /// For "is-at" replies this is the target's MAC; for "who-has" requests it is
    /// typically 00:00:00:00:00:00.
    pub target_mac: String,
}

// ── Process attribution ───────────────────────────────────────────────────────

/// OS process that owns a network connection.
/// Only populated when the agent runs in eBPF mode (Linux, CAP_BPF).
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ProcessInfo {
    /// OS process ID.
    pub pid: u32,
    /// Process name (up to 15 characters on Linux — from `comm`).
    pub name: String,
}

// ── TCP stats ─────────────────────────────────────────────────────────────────

#[derive(Debug, Clone, Serialize, Deserialize, Default)]
pub struct TcpStats {
    /// Number of retransmitted segments detected on this connection.
    pub retransmissions: u32,
    /// Number of out-of-order segments received (stored in pending buffer).
    pub out_of_order: u32,
}

// ── Raw packet event ──────────────────────────────────────────────────────────

/// A raw packet as seen on the wire, before protocol decoding.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct PacketEvent {
    pub timestamp: DateTime<Utc>,
    pub src_ip: String,
    pub dst_ip: String,
    pub src_port: Option<u16>,
    pub dst_port: Option<u16>,
    pub protocol: Protocol,
    pub length: u32,
    pub raw: Vec<u8>,
}
