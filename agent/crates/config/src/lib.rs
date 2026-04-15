use serde::{Deserialize, Serialize};

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
