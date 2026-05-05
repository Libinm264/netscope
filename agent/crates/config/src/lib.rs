use serde::{Deserialize, Serialize};

/// Controls how much of each HTTP flow the agent captures and forwards.
///
/// `Metadata` (default) — captures headers, timing, status codes, DNS, TLS
/// metadata, and protocol attribution, but strips request/response body
/// content. This keeps overhead near zero at any traffic volume.
///
/// `Full` — captures everything including body previews. Switched to
/// automatically on 4xx/5xx responses, or pushed from the Hub Fleet UI.
#[derive(Debug, Clone, Copy, PartialEq, Serialize, Deserialize, Default)]
#[serde(rename_all = "lowercase")]
pub enum SamplingMode {
    #[default]
    Metadata,
    Full,
}

impl std::fmt::Display for SamplingMode {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            SamplingMode::Metadata => write!(f, "metadata"),
            SamplingMode::Full     => write!(f, "full"),
        }
    }
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct AgentConfig {
    /// Network interface to capture on (e.g. "en0", "eth0", "lo")
    pub interface: String,

    /// BPF filter expression (e.g. "tcp port 80")
    pub bpf_filter: Option<String>,

    /// Output mode for Phase 1
    pub output: OutputMode,

    /// Maximum bytes to capture per packet (snaplen)
    pub snaplen: i32,

    /// Promiscuous mode
    pub promiscuous: bool,

    /// Capture buffer timeout in milliseconds
    pub buffer_timeout_ms: i32,

    /// Max body bytes to preview in decoded flows
    pub body_preview_bytes: usize,

    /// Hub WebSocket URL (Phase 4+)
    pub hub_url: Option<String>,

    /// Agent API key for Hub authentication (Phase 4+)
    pub api_key: Option<String>,
}

impl Default for AgentConfig {
    fn default() -> Self {
        Self {
            interface: "en0".to_string(),
            bpf_filter: None,
            output: OutputMode::Stdout,
            snaplen: 65535,
            promiscuous: true,
            buffer_timeout_ms: 100,
            body_preview_bytes: 512,
            hub_url: None,
            api_key: None,
        }
    }
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "lowercase")]
pub enum OutputMode {
    Stdout,
    Hub,
}
