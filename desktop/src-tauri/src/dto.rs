/// Data Transfer Objects — JSON-serialisable versions of the proto types.
/// Rust → TypeScript via Tauri's serde_json bridge.
use chrono::{DateTime, Utc};
use serde::{Deserialize, Serialize};

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct FlowDto {
    pub id: String,
    pub timestamp: DateTime<Utc>,
    /// Human-readable time string for display (HH:MM:SS.mmm)
    pub time_str: String,
    pub src_ip: String,
    pub dst_ip: String,
    pub src_port: u16,
    pub dst_port: u16,
    pub protocol: String,
    pub length: u64,
    /// One-line summary for the Info column
    pub info: String,
    pub http: Option<HttpFlowDto>,
    pub dns: Option<DnsFlowDto>,
    /// Raw packet bytes (hex-encoded for JSON transport)
    pub raw_hex: String,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct HttpFlowDto {
    pub method: Option<String>,
    pub path: Option<String>,
    pub host: Option<String>,
    pub status_code: Option<u16>,
    pub status_text: Option<String>,
    pub latency_ms: Option<u64>,
    pub req_headers: Vec<[String; 2]>,
    pub resp_headers: Vec<[String; 2]>,
    pub req_body_preview: Option<String>,
    pub resp_body_preview: Option<String>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct DnsFlowDto {
    pub transaction_id: u16,
    pub query_name: String,
    pub query_type: String,
    pub is_response: bool,
    pub rcode: Option<String>,
    pub answers: Vec<DnsAnswerDto>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct DnsAnswerDto {
    pub name: String,
    pub record_type: String,
    pub ttl: u32,
    pub data: String,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct InterfaceDto {
    pub name: String,
    pub description: String,
    pub addresses: Vec<String>,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
#[serde(rename_all = "lowercase")]
pub enum CaptureStatus {
    Idle,
    Running,
    Error,
}

/// Convert a proto::Flow into a FlowDto for the frontend.
pub fn flow_to_dto(flow: &proto::Flow) -> FlowDto {
    use proto::FlowPayload;

    let time_str = flow.timestamp.format("%H:%M:%S%.3f").to_string();

    let (protocol, info, http, dns) = match &flow.payload {
        Some(FlowPayload::Http(h)) => {
            let proto_str = "HTTP".to_string();
            let method = h.request.as_ref().map(|r| r.method.as_str()).unwrap_or("-");
            let path = h.request.as_ref().map(|r| r.path.as_str()).unwrap_or("/");
            let host = h
                .request
                .as_ref()
                .and_then(|r| r.headers.iter().find(|(k, _)| k.eq_ignore_ascii_case("host")))
                .map(|(_, v)| v.as_str())
                .unwrap_or("");
            let status = h
                .response
                .as_ref()
                .map(|r| format!(" → {}", r.status_code))
                .unwrap_or_default();
            let latency = h
                .latency_ms
                .map(|ms| format!(" ({}ms)", ms))
                .unwrap_or_default();

            let info = format!("{} {}{}{}{}", method, host, path, status, latency);

            let http_dto = HttpFlowDto {
                method: h.request.as_ref().map(|r| r.method.clone()),
                path: h.request.as_ref().map(|r| r.path.clone()),
                host: h
                    .request
                    .as_ref()
                    .and_then(|r| {
                        r.headers
                            .iter()
                            .find(|(k, _)| k.eq_ignore_ascii_case("host"))
                            .map(|(_, v)| v.clone())
                    }),
                status_code: h.response.as_ref().map(|r| r.status_code),
                status_text: h.response.as_ref().map(|r| r.status_text.clone()),
                latency_ms: h.latency_ms,
                req_headers: h
                    .request
                    .as_ref()
                    .map(|r| {
                        r.headers
                            .iter()
                            .map(|(k, v)| [k.clone(), v.clone()])
                            .collect()
                    })
                    .unwrap_or_default(),
                resp_headers: h
                    .response
                    .as_ref()
                    .map(|r| {
                        r.headers
                            .iter()
                            .map(|(k, v)| [k.clone(), v.clone()])
                            .collect()
                    })
                    .unwrap_or_default(),
                req_body_preview: h.request.as_ref().and_then(|r| r.body_preview.clone()),
                resp_body_preview: h.response.as_ref().and_then(|r| r.body_preview.clone()),
            };
            (proto_str, info, Some(http_dto), None)
        }
        Some(FlowPayload::Dns(d)) => {
            let answers: String = d
                .answers
                .iter()
                .map(|a| a.data.as_str())
                .collect::<Vec<_>>()
                .join(", ");
            let info = if d.is_response && !d.answers.is_empty() {
                format!(
                    "{} {} → {}",
                    d.query_type, d.query_name, answers
                )
            } else {
                format!("{} {}", d.query_type, d.query_name)
            };

            let dns_dto = DnsFlowDto {
                transaction_id: d.transaction_id,
                query_name: d.query_name.clone(),
                query_type: d.query_type.clone(),
                is_response: d.is_response,
                rcode: d.rcode.clone(),
                answers: d
                    .answers
                    .iter()
                    .map(|a| DnsAnswerDto {
                        name: a.name.clone(),
                        record_type: a.record_type.clone(),
                        ttl: a.ttl,
                        data: a.data.clone(),
                    })
                    .collect(),
            };
            ("DNS".to_string(), info, None, Some(dns_dto))
        }
        None => {
            let proto_str = flow.protocol.to_string();
            let info = format!(
                "{}:{} → {}:{}",
                flow.src_ip, flow.src_port, flow.dst_ip, flow.dst_port
            );
            (proto_str, info, None, None)
        }
    };

    // Check for HTTP errors and override protocol label
    let is_error = http
        .as_ref()
        .and_then(|h| h.status_code)
        .map(|s| s >= 400)
        .unwrap_or(false);
    let protocol = if is_error {
        format!("HTTP {}",
            http.as_ref().and_then(|h| h.status_code).unwrap_or(0))
    } else {
        protocol
    };

    FlowDto {
        id: flow.id.clone(),
        timestamp: flow.timestamp,
        time_str,
        src_ip: flow.src_ip.clone(),
        dst_ip: flow.dst_ip.clone(),
        src_port: flow.src_port,
        dst_port: flow.dst_port,
        protocol,
        length: flow.bytes_in + flow.bytes_out,
        info,
        http,
        dns,
        raw_hex: String::new(), // raw bytes not persisted in phase 2
    }
}
