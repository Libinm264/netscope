/// Offline IP / port threat-scoring engine.
///
/// Scores range from 0–100.  A score is only produced when indicators are
/// found; clean connections return `None` so we don't bloat the DTO stream.
use ipnet::Ipv4Net;
use std::net::IpAddr;

// ── Severity levels ───────────────────────────────────────────────────────────

#[derive(Debug, Clone, PartialEq)]
pub enum ThreatLevel {
    Clean,
    Low,
    Medium,
    High,
}

impl ThreatLevel {
    pub fn as_str(&self) -> &'static str {
        match self {
            ThreatLevel::Clean => "clean",
            ThreatLevel::Low => "low",
            ThreatLevel::Medium => "medium",
            ThreatLevel::High => "high",
        }
    }
}

#[derive(Debug, Clone)]
pub struct ThreatResult {
    pub score: u8,
    pub level: ThreatLevel,
    pub reasons: Vec<String>,
}

// ── Static threat data ────────────────────────────────────────────────────────

/// Known-bad CIDR ranges: Tor exits, botnet C2 infra, mass-scanners.
const BUILTIN_CIDRS: &[&str] = &[
    "185.220.0.0/15",  // Large Tor exit relay pool
    "199.87.154.0/24", // Tor / botnet C2
    "5.188.206.0/24",  // Known botnet C2
    "45.142.212.0/24", // Botnet C2
    "194.165.16.0/24", // Botnet C2
    "80.82.77.0/24",   // Shodan mass-scanner
    "198.20.69.0/24",  // Shodan mass-scanner
    "89.248.167.0/24", // Known malicious hosting
    "91.92.109.0/24",  // Bulletproof hosting
];

/// Ports associated with common RAT / C2 frameworks (Metasploit, CobaltStrike, etc.).
const C2_PORTS: &[u16] = &[4444, 1337, 31337, 6666, 6667, 4899, 8888, 9999, 50050];

/// Tor relay ports.
const TOR_PORTS: &[u16] = &[9001, 9030, 9150];

const TELNET_PORT: u16 = 23;

// ── Scorer ────────────────────────────────────────────────────────────────────

pub struct ThreatScorer {
    blocklist: Vec<Ipv4Net>,
}

impl ThreatScorer {
    pub fn new() -> Self {
        let blocklist = BUILTIN_CIDRS
            .iter()
            .filter_map(|cidr| cidr.parse::<Ipv4Net>().ok())
            .collect();
        ThreatScorer { blocklist }
    }

    /// Score a single flow direction. Returns `None` when there are no
    /// indicators (avoids adding `threat: null` noise to every DTO).
    pub fn score_flow(
        &self,
        src_ip: &str,
        dst_ip: &str,
        dst_port: u16,
    ) -> Option<ThreatResult> {
        let mut score: u8 = 0;
        let mut reasons: Vec<String> = Vec::new();

        // Check both IPs against the blocklist
        for (label, ip_str) in [("Source", src_ip), ("Destination", dst_ip)] {
            if let Ok(IpAddr::V4(v4)) = ip_str.parse::<IpAddr>() {
                for net in &self.blocklist {
                    if net.contains(&v4) {
                        score = score.saturating_add(75);
                        reasons.push(format!("{label} IP in threat blocklist ({net})"));
                        break;
                    }
                }
            }
        }

        // Port-based heuristics
        if C2_PORTS.contains(&dst_port) {
            score = score.saturating_add(40);
            reasons.push(format!("Known C2/RAT port ({dst_port})"));
        } else if TOR_PORTS.contains(&dst_port) {
            score = score.saturating_add(25);
            reasons.push(format!("Tor relay port ({dst_port})"));
        } else if dst_port == TELNET_PORT {
            score = score.saturating_add(20);
            reasons.push("Cleartext Telnet (port 23)".to_string());
        }

        if score == 0 {
            return None;
        }

        let score = score.min(100);
        let level = if score >= 70 {
            ThreatLevel::High
        } else if score >= 40 {
            ThreatLevel::Medium
        } else if score >= 20 {
            ThreatLevel::Low
        } else {
            ThreatLevel::Clean
        };

        Some(ThreatResult {
            score,
            level,
            reasons,
        })
    }
}
