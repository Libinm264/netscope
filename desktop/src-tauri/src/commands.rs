use crate::db;
use crate::dto::{flow_to_dto, CaptureStatus, FlowDto, InterfaceDto};
use crate::state::SharedState;
use capture::CaptureError;
use config::AgentConfig;
use parser::session::SessionManager;
use tauri::{AppHandle, Emitter, State};
use tokio::sync::oneshot;
use tracing::{error, info};

// ── Interface listing ─────────────────────────────────────────────────────────

#[tauri::command]
pub fn list_interfaces() -> Result<Vec<InterfaceDto>, String> {
    capture::list_interfaces()
        .map(|ifaces| {
            ifaces
                .into_iter()
                .map(|i| InterfaceDto {
                    name: i.name,
                    description: i.description,
                    addresses: i.addresses,
                })
                .collect()
        })
        .map_err(|e| e.to_string())
}

// ── Privilege check ───────────────────────────────────────────────────────────

#[tauri::command]
pub fn check_privileges() -> bool {
    // Attempt to list devices; if it fails with a permission error we lack privileges.
    match capture::list_interfaces() {
        Ok(_) => {
            // A more accurate check: try opening the first interface for capture.
            // For now, list success is a reasonable proxy.
            true
        }
        Err(CaptureError::InsufficientPrivileges) => false,
        Err(_) => true, // other errors are not privilege-related
    }
}

// ── Capture control ───────────────────────────────────────────────────────────

#[tauri::command]
pub async fn start_capture(
    interface: String,
    filter: Option<String>,
    app: AppHandle,
    state: State<'_, SharedState>,
) -> Result<(), String> {
    {
        let s = state.lock().unwrap();
        if s.status == CaptureStatus::Running {
            return Err("Capture already running".into());
        }
    }

    let (stop_tx, mut stop_rx) = oneshot::channel::<()>();

    {
        let mut s = state.lock().unwrap();
        s.status = CaptureStatus::Running;
        s.interface = Some(interface.clone());
        s.filter = filter.clone();
        s.stop_tx = Some(stop_tx);
        s.flows.clear();
    }

    let cfg = AgentConfig {
        interface: interface.clone(),
        bpf_filter: filter,
        ..Default::default()
    };

    let state_clone = state.inner().clone();
    let app_clone = app.clone();

    // Capture runs on a dedicated thread (libpcap is blocking)
    std::thread::spawn(move || {
        let (pkt_tx, pkt_rx) = std::sync::mpsc::channel();

        let cfg_clone = cfg.clone();
        let capture_thread = std::thread::spawn(move || {
            capture::start_capture(&cfg_clone, pkt_tx)
        });

        let mut session_mgr = SessionManager::new();

        loop {
            // Check for stop signal (non-blocking)
            if stop_rx.try_recv().is_ok() {
                info!("Capture stop signal received");
                break;
            }

            // Drain available packets with a short timeout
            match pkt_rx.recv_timeout(std::time::Duration::from_millis(50)) {
                Ok(packet) => {
                    let flows = session_mgr.process(&packet);
                    for flow in flows {
                        let dto = flow_to_dto(&flow);
                        // Store in state
                        {
                            let mut s = state_clone.lock().unwrap();
                            s.flows.push(dto.clone());
                        }
                        // Emit to frontend
                        if let Err(e) = app_clone.emit("flow", &dto) {
                            error!("Failed to emit flow event: {}", e);
                        }
                    }
                }
                Err(std::sync::mpsc::RecvTimeoutError::Timeout) => continue,
                Err(std::sync::mpsc::RecvTimeoutError::Disconnected) => break,
            }
        }

        {
            let mut s = state_clone.lock().unwrap();
            s.status = CaptureStatus::Idle;
            s.stop_tx = None;
        }

        let _ = app_clone.emit("capture-stopped", ());
        let _ = capture_thread.join();
    });

    Ok(())
}

#[tauri::command]
pub fn stop_capture(state: State<'_, SharedState>) -> Result<(), String> {
    let mut s = state.lock().unwrap();
    if let Some(tx) = s.stop_tx.take() {
        let _ = tx.send(());
        Ok(())
    } else {
        Err("No capture running".into())
    }
}

#[tauri::command]
pub fn get_capture_status(state: State<'_, SharedState>) -> CaptureStatus {
    state.lock().unwrap().status.clone()
}

// ── Flow access ───────────────────────────────────────────────────────────────

#[tauri::command]
pub fn get_flows(state: State<'_, SharedState>) -> Vec<FlowDto> {
    state.lock().unwrap().flows.clone()
}

#[tauri::command]
pub fn clear_flows(state: State<'_, SharedState>) {
    state.lock().unwrap().flows.clear();
}

// ── Session persistence ───────────────────────────────────────────────────────

#[tauri::command]
pub async fn save_session(
    path: String,
    name: String,
    state: State<'_, SharedState>,
) -> Result<(), String> {
    let flows = state.lock().unwrap().flows.clone();
    let pool = db::open_or_create(&path)
        .await
        .map_err(|e| e.to_string())?;
    db::save_flows(&pool, &name, &flows)
        .await
        .map_err(|e| e.to_string())?;
    pool.close().await;

    state.lock().unwrap().session_path = Some(path);
    Ok(())
}

#[tauri::command]
pub async fn load_session(
    path: String,
    state: State<'_, SharedState>,
) -> Result<Vec<FlowDto>, String> {
    let pool = db::open_or_create(&path)
        .await
        .map_err(|e| e.to_string())?;
    let flows = db::load_flows(&pool)
        .await
        .map_err(|e| e.to_string())?;
    pool.close().await;

    {
        let mut s = state.lock().unwrap();
        s.flows = flows.clone();
        s.session_path = Some(path);
    }
    Ok(flows)
}
