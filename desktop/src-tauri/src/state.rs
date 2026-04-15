use crate::dto::{CaptureStatus, FlowDto};
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
}

impl AppState {
    pub fn new() -> Self {
        AppState {
            flows: Vec::new(),
            status: CaptureStatus::Idle,
            interface: None,
            filter: None,
            stop_tx: None,
            session_path: None,
        }
    }
}

pub type SharedState = Arc<Mutex<AppState>>;

pub fn new_shared_state() -> SharedState {
    Arc::new(Mutex::new(AppState::new()))
}
