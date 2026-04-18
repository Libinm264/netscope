/// NetScope Hub API client.
///
/// Connects to a running hub instance so the desktop can query historical
/// flows, push local captures, and stream live events from remote agents.
use crate::dto::{FlowDto, GeoInfoDto, ThreatInfoDto};
use reqwest::Client;
use serde::{Deserialize, Serialize};

// ── Config ────────────────────────────────────────────────────────────────────

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct HubConfig {
    pub url: String,   // e.g. "https://hub.example.com"
    pub token: String, // X-API-Key value
}

// ── Hub API response types ────────────────────────────────────────────────────

#[derive(Debug, Deserialize)]
pub struct HubFlowsResponse {
    pub flows: Vec<HubFlowRecord>,
    pub total: u64,
}

#[derive(Debug, Clone, Deserialize)]
pub struct HubFlowRecord {
    pub id: String,
    pub ts: String,
    pub src_ip: String,
    pub dst_ip: String,
    pub src_port: u16,
    pub dst_port: u16,
    pub protocol: String,
    pub bytes: u64,
    // Hub-generated info string
    pub info: Option<String>,
    // GeoIP (already enriched by hub)
    pub country_code: Option<String>,
    pub country_name: Option<String>,
    pub asn_org: Option<String>,
    pub asn: Option<u32>,
    // Threat intelligence
    pub threat_score: Option<u8>,
    pub threat_level: Option<String>,
}

#[derive(Debug, Serialize, Deserialize, Default)]
#[serde(rename_all = "camelCase")]
pub struct HubFlowFilters {
    pub protocol: Option<String>,
    pub src_ip: Option<String>,
    pub dst_ip: Option<String>,
    pub limit: Option<u32>,
}

// ── Client ────────────────────────────────────────────────────────────────────

pub struct HubClient {
    http: Client,
    pub config: HubConfig,
}

impl HubClient {
    pub fn new(config: HubConfig) -> Self {
        HubClient {
            http: Client::new(),
            config,
        }
    }

    /// Ping the hub stats endpoint to verify connectivity.
    pub async fn test_connection(&self) -> Result<(), String> {
        let url = format!("{}/api/v1/stats", self.config.url);
        self.http
            .get(&url)
            .header("X-API-Key", &self.config.token)
            .send()
            .await
            .map_err(|e| format!("Connection failed: {e}"))?
            .error_for_status()
            .map_err(|e| format!("Hub returned error: {e}"))?;
        Ok(())
    }

    /// Query flows with optional filters.
    pub async fn query_flows(
        &self,
        filters: &HubFlowFilters,
    ) -> Result<Vec<HubFlowRecord>, String> {
        let url = format!("{}/api/v1/flows", self.config.url);
        let mut req = self
            .http
            .get(&url)
            .header("X-API-Key", &self.config.token);

        if let Some(proto) = &filters.protocol {
            req = req.query(&[("protocol", proto.as_str())]);
        }
        if let Some(src) = &filters.src_ip {
            req = req.query(&[("src_ip", src.as_str())]);
        }
        if let Some(dst) = &filters.dst_ip {
            req = req.query(&[("dst_ip", dst.as_str())]);
        }
        let limit = filters.limit.unwrap_or(500);
        req = req.query(&[("limit", limit)]);

        let resp: HubFlowsResponse = req
            .send()
            .await
            .map_err(|e| format!("Request failed: {e}"))?
            .error_for_status()
            .map_err(|e| format!("Hub error: {e}"))?
            .json()
            .await
            .map_err(|e| format!("JSON parse error: {e}"))?;

        Ok(resp.flows)
    }
}

// ── Conversion: HubFlowRecord → FlowDto ──────────────────────────────────────

pub fn hub_record_to_dto(rec: HubFlowRecord) -> FlowDto {
    use chrono::DateTime;

    let timestamp = rec
        .ts
        .parse::<DateTime<chrono::Utc>>()
        .unwrap_or_else(|_| chrono::Utc::now());

    let time_str = timestamp.format("%H:%M:%S%.3f").to_string();
    let info = rec
        .info
        .unwrap_or_else(|| format!("{}:{} → {}:{}", rec.src_ip, rec.src_port, rec.dst_ip, rec.dst_port));

    let geo_src = None; // hub flows don't have per-direction geo on src
    let geo_dst = match (rec.country_code, rec.country_name) {
        (Some(cc), Some(cn)) => Some(GeoInfoDto {
            country_code: cc,
            country_name: cn,
            city: String::new(),
            asn: rec.asn.unwrap_or(0),
            as_org: rec.asn_org.unwrap_or_default(),
        }),
        _ => None,
    };

    let threat = match (rec.threat_score, rec.threat_level) {
        (Some(score), Some(level)) if score > 0 => Some(ThreatInfoDto {
            score,
            level,
            reasons: vec!["From hub threat intelligence".to_string()],
        }),
        _ => None,
    };

    FlowDto {
        id: rec.id,
        timestamp,
        time_str,
        src_ip: rec.src_ip,
        dst_ip: rec.dst_ip,
        src_port: rec.src_port,
        dst_port: rec.dst_port,
        protocol: rec.protocol,
        length: rec.bytes,
        info,
        http: None,
        dns: None,
        tls: None,
        icmp: None,
        arp: None,
        tcp_stats: None,
        geo_src,
        geo_dst,
        threat,
        source: "hub".to_string(),
        raw_hex: String::new(),
    }
}
