/// HubClient sends decoded flows to a remote NetScope Hub API over HTTP.
///
/// Flows are buffered locally and flushed in batches of up to 20, or whenever
/// the caller explicitly calls `flush()`. This keeps the ingest latency low
/// while reducing the number of HTTP round-trips.
use anyhow::{Context, Result};
use chrono::SecondsFormat;
use gethostname::gethostname;
use proto::{Flow, FlowPayload};
use reqwest::blocking::Client;
use serde::Serialize;
use std::time::Duration;
use uuid::Uuid;

const BATCH_SIZE: usize = 20;
const TIMEOUT_SECS: u64 = 10;

// ── Wire types (match the Go API's models/flow.go) ─────────────────────────

#[derive(Serialize)]
struct HubHttpFlow {
    method:     String,
    path:       String,
    status:     u32,
    latency_ms: u64,
}

#[derive(Serialize)]
struct HubDnsFlow {
    query_name:  String,
    query_type:  String,
    is_response: bool,
    answers:     Vec<String>,
    rcode:       i32,
}

#[derive(Serialize)]
struct HubTlsFlow {
    record_type:        String,
    version:            String,
    #[serde(skip_serializing_if = "Option::is_none")]
    sni:                Option<String>,
    cipher_suites:      Vec<String>,
    has_weak_cipher:    bool,
    #[serde(skip_serializing_if = "Option::is_none")]
    chosen_cipher:      Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    negotiated_version: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    cert_cn:            Option<String>,
    cert_sans:          Vec<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    cert_expiry:        Option<String>,
    cert_expired:       bool,
    #[serde(skip_serializing_if = "Option::is_none")]
    cert_issuer:        Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    alert_level:        Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    alert_description:  Option<String>,
}

#[derive(Serialize)]
struct HubIcmpFlow {
    icmp_type: u8,
    icmp_code: u8,
    type_str:  String,
    #[serde(skip_serializing_if = "Option::is_none")]
    echo_id:   Option<u16>,
    #[serde(skip_serializing_if = "Option::is_none")]
    echo_seq:  Option<u16>,
    #[serde(skip_serializing_if = "Option::is_none")]
    rtt_ms:    Option<f64>,
}

#[derive(Serialize)]
struct HubArpFlow {
    operation:  String,
    sender_ip:  String,
    sender_mac: String,
    target_ip:  String,
    target_mac: String,
}

#[derive(Serialize)]
struct HubTcpStats {
    retransmissions: u32,
    out_of_order:    u32,
}

#[derive(Serialize)]
struct HubFlow {
    id:          String,
    agent_id:    String,
    hostname:    String,
    timestamp:   String,
    protocol:    String,
    src_ip:      String,
    src_port:    u16,
    dst_ip:      String,
    dst_port:    u16,
    bytes_in:    u64,
    bytes_out:   u64,
    duration_ms: u32,
    info:        String,
    #[serde(skip_serializing_if = "Option::is_none")]
    http:        Option<HubHttpFlow>,
    #[serde(skip_serializing_if = "Option::is_none")]
    dns:         Option<HubDnsFlow>,
    #[serde(skip_serializing_if = "Option::is_none")]
    tls:         Option<HubTlsFlow>,
    #[serde(skip_serializing_if = "Option::is_none")]
    icmp:        Option<HubIcmpFlow>,
    #[serde(skip_serializing_if = "Option::is_none")]
    arp:         Option<HubArpFlow>,
    #[serde(skip_serializing_if = "Option::is_none")]
    tcp_stats:   Option<HubTcpStats>,
    /// Process name from eBPF attribution (empty string when unavailable).
    process_name: String,
    /// Process PID from eBPF attribution (0 when unavailable).
    pid:          u32,
}

#[derive(Serialize)]
struct IngestRequest<'a> {
    agent_id: &'a str,
    hostname: &'a str,
    flows:    &'a [HubFlow],
}

// ── Client ─────────────────────────────────────────────────────────────────

pub struct HubClient {
    client:   Client,
    ingest_url: String,
    api_key:  String,
    agent_id: String,
    hostname: String,
    buffer:   Vec<HubFlow>,
}

impl HubClient {
    pub fn new(hub_url: &str, api_key: &str) -> Result<Self> {
        let client = Client::builder()
            .timeout(Duration::from_secs(TIMEOUT_SECS))
            .build()
            .context("build HTTP client")?;

        let hostname = gethostname()
            .to_string_lossy()
            .into_owned();

        let ingest_url = format!(
            "{}/api/v1/ingest",
            hub_url.trim_end_matches('/')
        );

        Ok(Self {
            client,
            ingest_url,
            api_key: api_key.to_string(),
            agent_id: Uuid::new_v4().to_string(),
            hostname,
            buffer: Vec::with_capacity(BATCH_SIZE),
        })
    }

    /// Enqueue a flow. Automatically flushes when the buffer reaches BATCH_SIZE.
    pub fn send_flow(&mut self, flow: &Flow) -> Result<()> {
        self.buffer.push(flow_to_wire(flow, &self.agent_id, &self.hostname));
        if self.buffer.len() >= BATCH_SIZE {
            self.flush()?;
        }
        Ok(())
    }

    /// Flush the buffered flows to the Hub API immediately.
    pub fn flush(&mut self) -> Result<()> {
        if self.buffer.is_empty() {
            return Ok(());
        }

        let body = IngestRequest {
            agent_id: &self.agent_id,
            hostname: &self.hostname,
            flows: &self.buffer,
        };

        let response = self
            .client
            .post(&self.ingest_url)
            .header("X-Api-Key", &self.api_key)
            .json(&body)
            .send()
            .context("POST to hub /api/v1/ingest")?;

        if !response.status().is_success() {
            let status = response.status();
            let body = response.text().unwrap_or_default();
            anyhow::bail!("hub ingest returned {}: {}", status, body);
        }

        self.buffer.clear();
        Ok(())
    }

    pub fn agent_id(&self) -> &str {
        &self.agent_id
    }

    pub fn hostname(&self) -> &str {
        &self.hostname
    }
}

// ── Conversion ─────────────────────────────────────────────────────────────

fn flow_to_wire(flow: &Flow, agent_id: &str, hostname: &str) -> HubFlow {
    let protocol = flow.protocol.to_string();

    let mut http:     Option<HubHttpFlow> = None;
    let mut dns:      Option<HubDnsFlow>  = None;
    let mut tls:      Option<HubTlsFlow>  = None;
    let mut icmp_w:   Option<HubIcmpFlow> = None;
    let mut arp_w:    Option<HubArpFlow>  = None;
    let mut duration_ms: u32 = 0;

    let info = match &flow.payload {
        Some(FlowPayload::Http(h)) => {
            let method  = h.request.as_ref().map(|r| r.method.clone()).unwrap_or_default();
            let path    = h.request.as_ref().map(|r| r.path.clone()).unwrap_or_default();
            let status  = h.response.as_ref().map(|r| r.status_code as u32).unwrap_or(0);
            let latency = h.latency_ms.unwrap_or(0);
            duration_ms = latency as u32;
            let info = format!("{} {}", method, path);
            http = Some(HubHttpFlow { method, path, status, latency_ms: latency });
            info
        }

        Some(FlowPayload::Dns(d)) => {
            let answers = d.answers.iter().map(|a| a.data.clone()).collect();
            let rcode   = rcode_to_int(d.rcode.as_deref());
            let info    = format!("{} {}", d.query_name, d.query_type);
            dns = Some(HubDnsFlow {
                query_name: d.query_name.clone(),
                query_type: d.query_type.clone(),
                is_response: d.is_response,
                answers,
                rcode,
            });
            info
        }

        Some(FlowPayload::Tls(t)) => {
            let info = match t.record_type.as_str() {
                "ClientHello" => format!(
                    "ClientHello{}",
                    t.sni.as_deref().map(|s| format!(" → {}", s)).unwrap_or_default()
                ),
                "ServerHello" => format!(
                    "ServerHello {}",
                    t.chosen_cipher.as_deref().unwrap_or("?")
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
            tls = Some(HubTlsFlow {
                record_type:        t.record_type.clone(),
                version:            t.version.clone(),
                sni:                t.sni.clone(),
                cipher_suites:      t.cipher_suites.clone(),
                has_weak_cipher:    t.has_weak_cipher,
                chosen_cipher:      t.chosen_cipher.clone(),
                negotiated_version: t.negotiated_version.clone(),
                cert_cn:            t.cert_cn.clone(),
                cert_sans:          t.cert_sans.clone(),
                cert_expiry:        t.cert_expiry.clone(),
                cert_expired:       t.cert_expired,
                cert_issuer:        t.cert_issuer.clone(),
                alert_level:        t.alert_level.clone(),
                alert_description:  t.alert_description.clone(),
            });
            info
        }

        Some(FlowPayload::Icmp(i)) => {
            let info = if let Some(rtt) = i.rtt_ms {
                format!("{} ({:.1}ms)", i.type_str, rtt)
            } else {
                i.type_str.clone()
            };
            icmp_w = Some(HubIcmpFlow {
                icmp_type: i.icmp_type,
                icmp_code: i.icmp_code,
                type_str:  i.type_str.clone(),
                echo_id:   i.echo_id,
                echo_seq:  i.echo_seq,
                rtt_ms:    i.rtt_ms,
            });
            info
        }

        Some(FlowPayload::Arp(a)) => {
            let info = format!("{} {} → {}", a.operation, a.sender_ip, a.target_ip);
            arp_w = Some(HubArpFlow {
                operation:  a.operation.clone(),
                sender_ip:  a.sender_ip.clone(),
                sender_mac: a.sender_mac.clone(),
                target_ip:  a.target_ip.clone(),
                target_mac: a.target_mac.clone(),
            });
            info
        }

        Some(FlowPayload::Http2(h2)) => {
            let method  = h2.request.as_ref().map(|r| r.method.clone()).unwrap_or_default();
            let path    = h2.request.as_ref().map(|r| r.path.clone()).unwrap_or_default();
            let status  = h2.response.as_ref().map(|r| r.status_code as u32).unwrap_or(0);
            let latency = h2.latency_ms.unwrap_or(0);
            duration_ms = latency as u32;
            let info = match (&h2.grpc_service, &h2.grpc_method) {
                (Some(svc), Some(meth)) => format!("gRPC {}/{}", svc, meth),
                _ => format!("{} {}", method, path),
            };
            http = Some(HubHttpFlow { method, path, status, latency_ms: latency });
            info
        }

        None => String::new(),
    };

    let tcp_stats = flow.tcp_stats.as_ref().map(|s| HubTcpStats {
        retransmissions: s.retransmissions,
        out_of_order:    s.out_of_order,
    });

    let (process_name, pid) = flow.process
        .as_ref()
        .map(|p| (p.name.clone(), p.pid))
        .unwrap_or_else(|| (String::new(), 0));

    HubFlow {
        id:          flow.id.clone(),
        agent_id:    agent_id.to_string(),
        hostname:    hostname.to_string(),
        timestamp:   flow.timestamp.to_rfc3339_opts(SecondsFormat::Millis, true),
        protocol,
        src_ip:      flow.src_ip.clone(),
        src_port:    flow.src_port,
        dst_ip:      flow.dst_ip.clone(),
        dst_port:    flow.dst_port,
        bytes_in:    flow.bytes_in,
        bytes_out:   flow.bytes_out,
        duration_ms,
        info,
        http,
        dns,
        tls,
        icmp: icmp_w,
        arp:  arp_w,
        tcp_stats,
        process_name,
        pid,
    }
}

/// Convert a DNS RCODE string to its numeric value (RFC 1035 / 2136).
fn rcode_to_int(rcode: Option<&str>) -> i32 {
    match rcode {
        Some("NOERROR")  => 0,
        Some("FORMERR")  => 1,
        Some("SERVFAIL") => 2,
        Some("NXDOMAIN") => 3,
        Some("NOTIMP")   => 4,
        Some("REFUSED")  => 5,
        _                => 0,
    }
}
