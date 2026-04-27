use crate::db;
use crate::dto::{flow_to_dto, CaptureStatus, FlowDto, GeoInfoDto, InterfaceDto, ThreatInfoDto};
use crate::geoip::GeoIpReader;
use crate::hub::{hub_record_to_dto, AgentInfo, ClusterSummary, HubClient, HubConfig, HubFlowFilters};
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

// ── PCAP export ───────────────────────────────────────────────────────────────

/// Export captured flows as a libpcap (.pcap) file with synthetic IP packets.
/// Each flow record becomes one minimal Ethernet + IP + TCP/UDP packet so the
/// file opens in Wireshark and other packet-analysis tools.
#[tauri::command]
pub async fn export_pcap(
    path: String,
    state: State<'_, SharedState>,
) -> Result<usize, String> {
    let flows = state.lock().unwrap().flows.clone();
    let count = flows.len();
    write_pcap(&path, &flows).map_err(|e| e.to_string())?;
    Ok(count)
}

fn write_pcap(path: &str, flows: &[FlowDto]) -> std::io::Result<()> {
    use std::io::Write;
    let mut f = std::io::BufWriter::new(std::fs::File::create(path)?);

    // Global pcap header (little-endian, LINKTYPE_ETHERNET = 1)
    f.write_all(&0xa1b2c3d4_u32.to_le_bytes())?;
    f.write_all(&2_u16.to_le_bytes())?;
    f.write_all(&4_u16.to_le_bytes())?;
    f.write_all(&0_i32.to_le_bytes())?;
    f.write_all(&0_u32.to_le_bytes())?;
    f.write_all(&65535_u32.to_le_bytes())?;
    f.write_all(&1_u32.to_le_bytes())?;

    for flow in flows {
        let ip_proto: u8 = match flow.protocol.as_str() {
            "UDP" | "DNS" => 17,
            "ICMP"        =>  1,
            _             =>  6,
        };

        // Payload: flow summary as UTF-8 text
        let payload = format!(
            "{} {}:{} -> {}:{} len={}",
            flow.protocol, flow.src_ip, flow.src_port,
            flow.dst_ip, flow.dst_port, flow.length,
        );
        let payload = payload.as_bytes();

        let transport_len: usize = match ip_proto { 17 => 8, 6 => 20, _ => 8 };
        let ip_total = 20 + transport_len + payload.len();

        // Ethernet header (14 bytes, zero MACs, ethertype 0x0800)
        let mut eth = [0u8; 14];
        eth[12] = 0x08;

        // IPv4 header (20 bytes)
        let mut ip = [0u8; 20];
        ip[0]  = 0x45;
        ip[2]  = ((ip_total >> 8) & 0xFF) as u8;
        ip[3]  = (ip_total & 0xFF) as u8;
        ip[5]  = 1;
        ip[6]  = 0x40;
        ip[8]  = 64;
        ip[9]  = ip_proto;
        if let Some(a) = parse_ipv4(&flow.src_ip) { ip[12..16].copy_from_slice(&a); }
        if let Some(a) = parse_ipv4(&flow.dst_ip) { ip[16..20].copy_from_slice(&a); }

        // Transport header
        let mut transport = vec![0u8; transport_len];
        transport[0] = ((flow.src_port >> 8) & 0xFF) as u8;
        transport[1] = (flow.src_port & 0xFF) as u8;
        transport[2] = ((flow.dst_port >> 8) & 0xFF) as u8;
        transport[3] = (flow.dst_port & 0xFF) as u8;
        if ip_proto == 17 {
            let udp_len = (8 + payload.len()) as u16;
            transport[4] = (udp_len >> 8) as u8;
            transport[5] = (udp_len & 0xFF) as u8;
        } else if ip_proto == 6 {
            transport[12] = 0x50;
            transport[13] = 0x02;
            transport[14] = 0xFF;
            transport[15] = 0xFF;
        }

        let pkt_len = (14 + 20 + transport_len + payload.len()) as u32;
        let ts_sec  = flow.timestamp.timestamp() as u32;
        let ts_usec = flow.timestamp.timestamp_subsec_micros();

        f.write_all(&ts_sec.to_le_bytes())?;
        f.write_all(&ts_usec.to_le_bytes())?;
        f.write_all(&pkt_len.to_le_bytes())?;
        f.write_all(&pkt_len.to_le_bytes())?;
        f.write_all(&eth)?;
        f.write_all(&ip)?;
        f.write_all(&transport)?;
        f.write_all(payload)?;
    }
    f.flush()
}

fn parse_ipv4(s: &str) -> Option<[u8; 4]> {
    let parts: Vec<u8> = s.split('.').filter_map(|p| p.parse().ok()).collect();
    if parts.len() == 4 { Some([parts[0], parts[1], parts[2], parts[3]]) } else { None }
}

// ── Fleet commands ────────────────────────────────────────────────────────────

/// Fetch cluster summaries from the connected hub (requires hub to be configured).
#[tauri::command]
pub async fn get_fleet_clusters(
    state: State<'_, SharedState>,
) -> Result<Vec<ClusterSummary>, String> {
    let config = state
        .lock()
        .unwrap()
        .hub_config
        .clone()
        .ok_or("No hub configured. Connect to a hub first.")?;
    HubClient::new(config).get_fleet_clusters().await
}

/// Fetch agent list from the connected hub, optionally filtered by cluster name.
#[tauri::command]
pub async fn get_fleet_agents(
    cluster: Option<String>,
    state: State<'_, SharedState>,
) -> Result<Vec<AgentInfo>, String> {
    let config = state
        .lock()
        .unwrap()
        .hub_config
        .clone()
        .ok_or("No hub configured. Connect to a hub first.")?;
    HubClient::new(config)
        .get_fleet_agents(cluster.as_deref())
        .await
}

// ── OTel backend commands ─────────────────────────────────────────────────────

/// Return the configured OTel backend URL (e.g. Jaeger / Zipkin / Grafana Tempo).
#[tauri::command]
pub fn get_otel_backend_url(state: State<'_, SharedState>) -> Option<String> {
    state.lock().unwrap().otel_backend_url.clone()
}

/// Set (or clear) the OTel backend URL. Pass an empty string to clear.
#[tauri::command]
pub fn set_otel_backend_url(url: String, state: State<'_, SharedState>) {
    let mut s = state.lock().unwrap();
    s.otel_backend_url = if url.is_empty() { None } else { Some(url) };
}

/// Open a URL in the system's default browser.
/// Used by OtelTracePanel to deep-link into Jaeger / Zipkin / Tempo.
#[tauri::command]
pub fn open_url(url: String) -> Result<(), String> {
    #[cfg(target_os = "macos")]
    std::process::Command::new("open")
        .arg(&url)
        .spawn()
        .map_err(|e| e.to_string())?;

    #[cfg(target_os = "linux")]
    std::process::Command::new("xdg-open")
        .arg(&url)
        .spawn()
        .map_err(|e| e.to_string())?;

    #[cfg(target_os = "windows")]
    std::process::Command::new("cmd")
        .args(["/c", "start", "", &url])
        .spawn()
        .map_err(|e| e.to_string())?;

    Ok(())
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
