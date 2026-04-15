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

    let (http, dns, info) = match &flow.payload {
        Some(FlowPayload::Http(h)) => {
            let method = h
                .request
                .as_ref()
                .map(|r| r.method.clone())
                .unwrap_or_default();
            let path = h
                .request
                .as_ref()
                .map(|r| r.path.clone())
                .unwrap_or_default();
            let status = h
                .response
                .as_ref()
                .map(|r| r.status_code as u32)
                .unwrap_or(0);
            let latency = h.latency_ms.unwrap_or(0);
            let info = format!("{} {}", method, path);

            (
                Some(HubHttpFlow { method, path, status, latency_ms: latency }),
                None,
                info,
            )
        }

        Some(FlowPayload::Dns(d)) => {
            let answers = d.answers.iter().map(|a| a.data.clone()).collect();
            let rcode = rcode_to_int(d.rcode.as_deref());
            let info = format!("{} {}", d.query_name, d.query_type);

            (
                None,
                Some(HubDnsFlow {
                    query_name:  d.query_name.clone(),
                    query_type:  d.query_type.clone(),
                    is_response: d.is_response,
                    answers,
                    rcode,
                }),
                info,
            )
        }

        None => (None, None, String::new()),
    };

    HubFlow {
        id:          Uuid::new_v4().to_string(),
        agent_id:    agent_id.to_string(),
        hostname:    hostname.to_string(),
        timestamp:   flow
            .timestamp
            .to_rfc3339_opts(SecondsFormat::Millis, true),
        protocol,
        src_ip:      flow.src_ip.clone(),
        src_port:    flow.src_port,
        dst_ip:      flow.dst_ip.clone(),
        dst_port:    flow.dst_port,
        bytes_in:    flow.bytes_in,
        bytes_out:   flow.bytes_out,
        duration_ms: 0, // computed if latency available
        info,
        http,
        dns,
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
