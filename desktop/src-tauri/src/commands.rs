use crate::db;
use crate::dto::{flow_to_dto, CaptureStatus, FlowDto, GeoInfoDto, InterfaceDto, ThreatInfoDto};
use crate::geoip::GeoIpReader;
use crate::hub::{hub_record_to_dto, HubClient, HubConfig, HubFlowFilters};
use crate::state::SharedState;
use crate::threat::ThreatScorer;
use capture::CaptureError;
use config::AgentConfig;
use parser::session::SessionManager;
use std::path::Path;
use std::sync::Arc;
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
    match capture::list_interfaces() {
        Ok(_) => true,
        Err(CaptureError::InsufficientPrivileges) => false,
        Err(_) => true,
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
    let (stop_tx, mut stop_rx) = oneshot::channel::<()>();

    // Check-and-set status + clone enrichment engines in one atomic lock acquisition.
    // Splitting the check and set across separate locks creates a TOCTOU race where
    // two concurrent start_capture calls both pass the check before either sets Running.
    let (geoip_reader, threat_scorer): (Option<Arc<GeoIpReader>>, Arc<ThreatScorer>) = {
        let mut s = state.lock().unwrap();
        if s.status == CaptureStatus::Running {
            return Err("Capture already running".into());
        }
        s.status = CaptureStatus::Running;
        s.interface = Some(interface.clone());
        s.filter = filter.clone();
        s.stop_tx = Some(stop_tx);
        s.flows.clear();
        (s.geoip.clone(), s.threat_scorer.clone())
    };

    let cfg = AgentConfig {
        interface: interface.clone(),
        bpf_filter: filter,
        ..Default::default()
    };

    let state_clone = state.inner().clone();
    let app_clone = app.clone();

    std::thread::spawn(move || {
        let (pkt_tx, pkt_rx) = std::sync::mpsc::channel();

        let cfg_clone = cfg.clone();
        let capture_thread = std::thread::spawn(move || capture::start_capture(&cfg_clone, pkt_tx));

        let mut session_mgr = SessionManager::new();

        loop {
            if stop_rx.try_recv().is_ok() {
                info!("Capture stop signal received");
                break;
            }

            match pkt_rx.recv_timeout(std::time::Duration::from_millis(50)) {
                Ok(packet) => {
                    let flows = session_mgr.process(&packet);
                    for flow in flows {
                        let mut dto = flow_to_dto(&flow);

                        // ── GeoIP enrichment ──────────────────────────────
                        if let Some(geo) = &geoip_reader {
                            dto.geo_src = geo.lookup(&dto.src_ip).map(|g| GeoInfoDto {
                                country_code: g.country_code,
                                country_name: g.country_name,
                                city: g.city,
                                asn: g.asn,
                                as_org: g.as_org,
                            });
                            dto.geo_dst = geo.lookup(&dto.dst_ip).map(|g| GeoInfoDto {
                                country_code: g.country_code,
                                country_name: g.country_name,
                                city: g.city,
                                asn: g.asn,
                                as_org: g.as_org,
                            });
                        }

                        // ── Threat scoring ────────────────────────────────
                        dto.threat = threat_scorer
                            .score_flow(&dto.src_ip, &dto.dst_ip, dto.dst_port)
                            .map(|t| ThreatInfoDto {
                                score: t.score,
                                level: t.level.as_str().to_string(),
                                reasons: t.reasons,
                            });

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

// ── GeoIP management ──────────────────────────────────────────────────────────

/// Returns true if GeoIP databases are loaded and ready.
#[tauri::command]
pub fn get_geoip_status(state: State<'_, SharedState>) -> bool {
    state.lock().unwrap().geoip.is_some()
}

/// Load GeoIP databases from the given paths (or use defaults if empty).
#[tauri::command]
pub fn load_geoip_db(
    city_path: String,
    asn_path: String,
    state: State<'_, SharedState>,
) -> Result<(), String> {
    let reader = if city_path.is_empty() && asn_path.is_empty() {
        GeoIpReader::try_default().ok_or("No GeoIP databases found in ~/.netscope/")?
    } else {
        GeoIpReader::open(Path::new(&city_path), Path::new(&asn_path))?
    };
    state.lock().unwrap().geoip = Some(Arc::new(reader));
    Ok(())
}

// ── Hub connection ────────────────────────────────────────────────────────────

/// Store hub connection config. Pass null/empty url to disconnect.
#[tauri::command]
pub fn set_hub_config(
    url: String,
    token: String,
    state: State<'_, SharedState>,
) {
    let mut s = state.lock().unwrap();
    if url.is_empty() {
        s.hub_config = None;
    } else {
        s.hub_config = Some(HubConfig { url, token });
    }
}

#[tauri::command]
pub fn get_hub_config(state: State<'_, SharedState>) -> Option<HubConfig> {
    state.lock().unwrap().hub_config.clone()
}

/// Test that the configured hub is reachable.
#[tauri::command]
pub async fn test_hub_connection(state: State<'_, SharedState>) -> Result<(), String> {
    let config = state
        .lock()
        .unwrap()
        .hub_config
        .clone()
        .ok_or("No hub configured")?;
    HubClient::new(config).test_connection().await
}

/// Query flows from the hub and merge them into local state.
#[tauri::command]
pub async fn query_hub_flows(
    filters: HubFlowFilters,
    state: State<'_, SharedState>,
) -> Result<Vec<FlowDto>, String> {
    let config = state
        .lock()
        .unwrap()
        .hub_config
        .clone()
        .ok_or("No hub configured")?;

    let records = HubClient::new(config).query_flows(&filters).await?;
    let dtos: Vec<FlowDto> = records.into_iter().map(hub_record_to_dto).collect();

    // Merge into local flow store so they appear in the packet list
    {
        let mut s = state.lock().unwrap();
        // Remove any previously loaded hub flows before replacing
        s.flows.retain(|f| f.source != "hub");
        s.flows.extend(dtos.clone());
        s.flows.sort_by_key(|f| f.timestamp);
    }

    Ok(dtos)
}
