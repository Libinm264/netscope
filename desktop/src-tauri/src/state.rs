use crate::dto::{CaptureStatus, FlowDto};
use crate::geoip::GeoIpReader;
use crate::hub::HubConfig;
use crate::threat::ThreatScorer;
use std::sync::{Arc, Mutex};
use tokio::sync::oneshot;

pub struct AppState {
    pub flows: Vec<FlowDto>,
    pub status: CaptureStatus,
    pub interface: Option<String>,
    pub filter: Option<String>,
    /// Sender to signal the capture thread to stop
    pub stop_tx: Option<oneshot::Sender<()>>,
    /// Path to the open session DB file
    pub session_path: Option<String>,
    /// MaxMind GeoLite2 reader — None until databases are loaded
    pub geoip: Option<Arc<GeoIpReader>>,
    /// Offline threat scorer — always present (no external DB required)
    pub threat_scorer: Arc<ThreatScorer>,
    /// Hub connection config — None until user configures a connection
    pub hub_config: Option<HubConfig>,
    /// OTel backend base URL for trace linking (e.g. "http://localhost:16686" for Jaeger)
    pub otel_backend_url: Option<String>,
}

impl AppState {
    pub fn new() -> Self {
        // Try to auto-load GeoIP DBs from ~/.netscope/
        let geoip = GeoIpReader::try_default().map(Arc::new);

        AppState {
            flows: Vec::new(),
            status: CaptureStatus::Idle,
            interface: None,
            filter: None,
            stop_tx: None,
            session_path: None,
            geoip,
            threat_scorer: Arc::new(ThreatScorer::new()),
            hub_config: None,
            otel_backend_url: None,
        }
    }
}

pub type SharedState = Arc<Mutex<AppState>>;

pub fn new_shared_state() -> SharedState {
    Arc::new(Mutex::new(AppState::new()))
}
