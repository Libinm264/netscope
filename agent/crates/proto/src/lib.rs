/// Shared data types used across all NetScope components.
/// These mirror the protobuf schema defined in /proto/netscope.proto.
use chrono::{DateTime, Utc};
use serde::{Deserialize, Serialize};

#[derive(Debug, Clone, Serialize, Deserialize)]
pub enum Protocol {
    Http,
    Https,
    Dns,
    Tcp,
    Udp,
    Unknown,
}

impl std::fmt::Display for Protocol {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            Protocol::Http => write!(f, "HTTP"),
            Protocol::Https => write!(f, "HTTPS"),
            Protocol::Dns => write!(f, "DNS"),
            Protocol::Tcp => write!(f, "TCP"),
            Protocol::Udp => write!(f, "UDP"),
            Protocol::Unknown => write!(f, "UNKNOWN"),
        }
    }
}

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
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub enum FlowPayload {
    Http(HttpFlow),
    Dns(DnsFlow),
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct HttpFlow {
    pub request: Option<HttpRequest>,
    pub response: Option<HttpResponse>,
    /// Round-trip latency in milliseconds (set when both request and response are seen)
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

/// A raw packet event as seen on the wire, before protocol decoding.
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
