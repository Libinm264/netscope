mod commands;
mod db;
mod dto;
mod geoip;
mod hub;
mod state;
mod threat;

use commands::*;
use state::new_shared_state;

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    tracing_subscriber::fmt()
        .with_env_filter(
            tracing_subscriber::EnvFilter::try_from_default_env()
                .unwrap_or_else(|_| tracing_subscriber::EnvFilter::new("info")),
        )
        .init();

    tauri::Builder::default()
        .plugin(tauri_plugin_dialog::init())
        .manage(new_shared_state())
        .invoke_handler(tauri::generate_handler![
            // Capture
            list_interfaces,
            check_privileges,
            start_capture,
            stop_capture,
            get_capture_status,
            get_flows,
            clear_flows,
            // Session
            save_session,
            load_session,
            // GeoIP
            get_geoip_status,
            load_geoip_db,
            // Hub
            set_hub_config,
            get_hub_config,
            test_hub_connection,
            query_hub_flows,
            // Export
            export_pcap,
        ])
        .run(tauri::generate_context!())
        .expect("error while running NetScope desktop");
}
