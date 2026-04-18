/// Data Transfer Objects — JSON-serialisable versions of the proto types.
/// Rust → TypeScript via Tauri's serde_json bridge.
use chrono::{DateTime, Utc};
use serde::{Deserialize, Serialize};

// ── GeoIP ─────────────────────────────────────────────────────────────────────

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct GeoInfoDto {
    pub country_code: String,
    pub country_name: String,
    pub city: String,
    pub asn: u32,
    pub as_org: String,
}

// ── Threat intelligence ───────────────────────────────────────────────────────

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ThreatInfoDto {
    pub score: u8,
    pub level: String, // "clean" | "low" | "medium" | "high"
    pub reasons: Vec<String>,
}

// ── Core flow ─────────────────────────────────────────────────────────────────

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
    pub tls: Option<TlsFlowDto>,
    pub icmp: Option<IcmpFlowDto>,
    pub arp: Option<ArpFlowDto>,
    pub tcp_stats: Option<TcpStatsDto>,
    /// GeoIP for source IP (None for private/RFC1918 addresses)
    /// `#[serde(default)]` allows loading sessions saved before this field existed.
    #[serde(default)]
    pub geo_src: Option<GeoInfoDto>,
    /// GeoIP for destination IP (None for private/RFC1918 addresses)
    #[serde(default)]
    pub geo_dst: Option<GeoInfoDto>,
    /// Threat intelligence (None when no indicators found — clean traffic)
    #[serde(default)]
    pub threat: Option<ThreatInfoDto>,
    /// "local" | "hub" — defaults to "local" for sessions saved before hub support.
    #[serde(default = "default_source")]
    pub source: String,
    /// Raw packet bytes (hex-encoded for JSON transport)
    #[serde(default)]
    pub raw_hex: String,
}

fn default_source() -> String {
    "local".to_string()
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

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct TlsFlowDto {
    pub record_type: String,
    pub version: String,
    pub sni: Option<String>,
    pub cipher_suites: Vec<String>,
    pub has_weak_cipher: bool,
    pub chosen_cipher: Option<String>,
    pub negotiated_version: Option<String>,
    pub cert_cn: Option<String>,
    pub cert_sans: Vec<String>,
    pub cert_expiry: Option<String>,
    pub cert_expired: bool,
    pub cert_issuer: Option<String>,
    pub alert_level: Option<String>,
    pub alert_description: Option<String>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct IcmpFlowDto {
    pub icmp_type: u8,
    pub icmp_code: u8,
    pub type_str: String,
    pub echo_id: Option<u16>,
    pub echo_seq: Option<u16>,
    pub rtt_ms: Option<f64>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ArpFlowDto {
    pub operation: String,
    pub sender_ip: String,
    pub sender_mac: String,
    pub target_ip: String,
    pub target_mac: String,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct TcpStatsDto {
    pub retransmissions: u32,
    pub out_of_order: u32,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
#[serde(rename_all = "lowercase")]
pub enum CaptureStatus {
    Idle,
    Running,
    Error,
}

/// Convert a proto::Flow into a FlowDto for the frontend.
/// GeoIP and threat fields are left as `None` here and filled in by the
/// capture loop, which has access to the enrichment engines.
pub fn flow_to_dto(flow: &proto::Flow) -> FlowDto {
    use proto::FlowPayload;

    let time_str = flow.timestamp.format("%H:%M:%S%.3f").to_string();

    let mut http: Option<HttpFlowDto> = None;
    let mut dns: Option<DnsFlowDto> = None;
    let mut tls: Option<TlsFlowDto> = None;
    let mut icmp_dto: Option<IcmpFlowDto> = None;
    let mut arp_dto: Option<ArpFlowDto> = None;

    let (protocol, info) = match &flow.payload {
        Some(FlowPayload::Http(h)) => {
            let method = h.request.as_ref().map(|r| r.method.as_str()).unwrap_or("-");
            let path = h.request.as_ref().map(|r| r.path.as_str()).unwrap_or("/");
            let host = h
                .request
                .as_ref()
                .and_then(|r| {
                    r.headers
                        .iter()
                        .find(|(k, _)| k.eq_ignore_ascii_case("host"))
                })
                .map(|(_, v)| v.as_str())
                .unwrap_or("");
            let status_suffix = h
                .response
                .as_ref()
                .map(|r| format!(" → {}", r.status_code))
                .unwrap_or_default();
            let latency_suffix = h
                .latency_ms
                .map(|ms| format!(" ({}ms)", ms))
                .unwrap_or_default();
            let info = format!(
                "{} {}{}{}{}",
                method, host, path, status_suffix, latency_suffix
            );

            let is_error = h
                .response
                .as_ref()
                .map(|r| r.status_code >= 400)
                .unwrap_or(false);
            let proto_str = if is_error {
                format!(
                    "HTTP {}",
                    h.response.as_ref().map(|r| r.status_code).unwrap_or(0)
                )
            } else {
                "HTTP".to_string()
            };

            http = Some(HttpFlowDto {
                method: h.request.as_ref().map(|r| r.method.clone()),
                path: h.request.as_ref().map(|r| r.path.clone()),
                host: h.request.as_ref().and_then(|r| {
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
                req_body_preview: h
                    .request
                    .as_ref()
                    .and_then(|r| r.body_preview.clone()),
                resp_body_preview: h
                    .response
                    .as_ref()
                    .and_then(|r| r.body_preview.clone()),
            });
            (proto_str, info)
        }

        Some(FlowPayload::Dns(d)) => {
            let answers = d
                .answers
                .iter()
                .map(|a| a.data.as_str())
                .collect::<Vec<_>>()
                .join(", ");
            let info = if d.is_response && !d.answers.is_empty() {
                format!("{} {} → {}", d.query_type, d.query_name, answers)
            } else {
                format!("{} {}", d.query_type, d.query_name)
            };
            dns = Some(DnsFlowDto {
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
            });
            ("DNS".to_string(), info)
        }

        Some(FlowPayload::Tls(t)) => {
            let info = match t.record_type.as_str() {
                "ClientHello" => format!(
                    "ClientHello{}{}",
                    t.sni
                        .as_deref()
                        .map(|s| format!(" → {}", s))
                        .unwrap_or_default(),
                    if t.has_weak_cipher { " ⚠ weak cipher" } else { "" }
                ),
                "ServerHello" => format!(
                    "ServerHello {}{}",
                    t.negotiated_version
                        .as_deref()
                        .or(t.chosen_cipher.as_deref())
                        .unwrap_or("?"),
                    t.chosen_cipher
                        .as_deref()
                        .map(|c| format!(" ({})", c))
                        .unwrap_or_default()
                ),
                "Certificate" => format!(
                    "Certificate {}{}",
                    t.cert_cn.as_deref().unwrap_or("?"),
                    if t.cert_expired { " [EXPIRED]" } else { "" }
                ),
                "Alert" => format!(
                    "Alert {} {}",
                    t.alert_level.as_deref().unwrap_or(""),
                    t.alert_description.as_deref().unwrap_or("")
                ),
                other => other.to_string(),
            };
            tls = Some(TlsFlowDto {
                record_type: t.record_type.clone(),
                version: t.version.clone(),
                sni: t.sni.clone(),
                cipher_suites: t.cipher_suites.clone(),
                has_weak_cipher: t.has_weak_cipher,
                chosen_cipher: t.chosen_cipher.clone(),
                negotiated_version: t.negotiated_version.clone(),
                cert_cn: t.cert_cn.clone(),
                cert_sans: t.cert_sans.clone(),
                cert_expiry: t.cert_expiry.clone(),
                cert_expired: t.cert_expired,
                cert_issuer: t.cert_issuer.clone(),
                alert_level: t.alert_level.clone(),
                alert_description: t.alert_description.clone(),
            });
            ("TLS".to_string(), info)
        }

        Some(FlowPayload::Icmp(i)) => {
            let info = if let Some(rtt) = i.rtt_ms {
                format!("{} ({:.1}ms)", i.type_str, rtt)
            } else {
                i.type_str.clone()
            };
            icmp_dto = Some(IcmpFlowDto {
                icmp_type: i.icmp_type,
                icmp_code: i.icmp_code,
                type_str: i.type_str.clone(),
                echo_id: i.echo_id,
                echo_seq: i.echo_seq,
                rtt_ms: i.rtt_ms,
            });
            ("ICMP".to_string(), info)
        }

        Some(FlowPayload::Arp(a)) => {
            let info = format!(
                "{} {} → {} ({})",
                a.operation, a.sender_ip, a.target_ip, a.sender_mac
            );
            arp_dto = Some(ArpFlowDto {
                operation: a.operation.clone(),
                sender_ip: a.sender_ip.clone(),
                sender_mac: a.sender_mac.clone(),
                target_ip: a.target_ip.clone(),
                target_mac: a.target_mac.clone(),
            });
            ("ARP".to_string(), info)
        }

        None => {
            let proto_str = flow.protocol.to_string();
            let info = format!(
                "{}:{} → {}:{}",
                flow.src_ip, flow.src_port, flow.dst_ip, flow.dst_port
            );
            (proto_str, info)
        }
    };

    let tcp_stats = flow.tcp_stats.as_ref().map(|s| TcpStatsDto {
        retransmissions: s.retransmissions,
        out_of_order: s.out_of_order,
    });

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
        tls,
        icmp: icmp_dto,
        arp: arp_dto,
        tcp_stats,
        geo_src: None,
        geo_dst: None,
        threat: None,
        source: "local".to_string(),
        raw_hex: String::new(),
    }
}
