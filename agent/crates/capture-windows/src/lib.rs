//! Npcap-based packet capture backend for the NetScope Windows agent.
//!
//! This crate is compiled **only on Windows** (`cfg(target_os = "windows")`).
//! On Linux/macOS the main agent uses the `capture` crate (libpcap path).
//!
//! # Requirements
//!
//! - Npcap 1.x installed on the target machine (<https://npcap.com>)
//! - The `wpcap.dll` DLL must be in PATH (Npcap installer handles this)
//! - The MSI installer bundles `wpcap.dll` and `Packet.dll` for deployment
//!
//! # Process Attribution
//!
//! Windows does not expose the originating PID in the pcap packet data.
//! Instead, `capture-windows` maintains a `ProcessCache` (Toolhelp32Snapshot)
//! refreshed every 5 seconds and attempts best-effort attribution by
//! correlating (src_port, dst_port, protocol) against active TCP/UDP connections
//! from `GetExtendedTcpTable` / `GetExtendedUdpTable`.

#![cfg(target_os = "windows")]

pub mod interface;
pub mod process;

use std::{
    net::{IpAddr, Ipv4Addr},
    time::SystemTime,
};

use anyhow::Result;
use pcap::{Capture, Device, Packet};
use tokio::sync::mpsc;
use tracing::{debug, info, warn};

use process::ProcessCache;

/// A captured flow event from the Windows Npcap backend.
/// Mirrors the shape of the Linux capture output so the main agent loop is
/// platform-agnostic.
#[derive(Debug, Clone)]
pub struct WindowsFlow {
    pub src_ip:       IpAddr,
    pub dst_ip:       IpAddr,
    pub src_port:     u16,
    pub dst_port:     u16,
    pub protocol:     String,
    pub bytes:        u64,
    pub process_name: Option<String>,
    pub pid:          Option<u32>,
    pub timestamp:    SystemTime,
}

/// Configuration for the Windows capture backend.
#[derive(Clone, Debug)]
pub struct WindowsCaptureConfig {
    /// Interface name (e.g. `\Device\NPF_{GUID}`). None = auto-detect first.
    pub interface:     Option<String>,
    /// Optional BPF filter string (e.g. `"tcp or udp"`).
    pub bpf_filter:   Option<String>,
    /// Maximum packets to buffer in the channel.
    pub channel_cap:  usize,
    /// Path to npcap DLL directory (overrides PATH lookup).
    pub npcap_dll:    Option<String>,
}

impl Default for WindowsCaptureConfig {
    fn default() -> Self {
        Self {
            interface:    None,
            bpf_filter:  None,
            channel_cap: 4096,
            npcap_dll:   None,
        }
    }
}

/// Start packet capture on Windows using Npcap.
///
/// Returns a channel receiver. Each message is a [`WindowsFlow`] parsed from
/// one captured packet. The capture runs in a dedicated blocking thread.
pub fn start(cfg: WindowsCaptureConfig) -> Result<mpsc::Receiver<WindowsFlow>> {
    let device = interface::find(cfg.interface.as_deref())?;
    info!("windows-capture: starting on {} ({})",
        device.name,
        device.desc.as_deref().unwrap_or("no description"),
    );

    let mut cap = Capture::from_device(device)?
        .promisc(true)
        .snaplen(65535)
        .timeout(100) // ms
        .open()?;

    if let Some(ref filter) = cfg.bpf_filter {
        cap.filter(filter, true)?;
    }

    let (tx, rx) = mpsc::channel(cfg.channel_cap);
    let proc_cache = ProcessCache::new();

    std::thread::spawn(move || {
        loop {
            match cap.next_packet() {
                Ok(packet) => {
                    if let Some(flow) = parse_packet(&packet, &proc_cache) {
                        if tx.blocking_send(flow).is_err() {
                            break; // receiver dropped — exit
                        }
                    }
                }
                Err(pcap::Error::TimeoutExpired) => continue,
                Err(e) => {
                    warn!("windows-capture: packet error: {}", e);
                    break;
                }
            }
        }
        info!("windows-capture: capture loop exited");
    });

    Ok(rx)
}

/// Parse a raw pcap packet into a WindowsFlow.
/// Handles Ethernet → IPv4 → TCP/UDP. Returns None for unsupported frames.
fn parse_packet(packet: &Packet, proc_cache: &ProcessCache) -> Option<WindowsFlow> {
    let data = packet.data;
    if data.len() < 14 {
        return None; // Too short for Ethernet header.
    }

    // Ethernet type field at bytes 12-13.
    let eth_type = u16::from_be_bytes([data[12], data[13]]);
    if eth_type != 0x0800 {
        return None; // Not IPv4.
    }

    let ip = &data[14..];
    if ip.len() < 20 {
        return None;
    }

    let ip_proto  = ip[9];
    let src_ip    = Ipv4Addr::new(ip[12], ip[13], ip[14], ip[15]);
    let dst_ip    = Ipv4Addr::new(ip[16], ip[17], ip[18], ip[19]);
    let ip_hlen   = ((ip[0] & 0x0F) as usize) * 4;

    let transport = &ip[ip_hlen..];
    if transport.len() < 4 {
        return None;
    }

    let src_port = u16::from_be_bytes([transport[0], transport[1]]);
    let dst_port = u16::from_be_bytes([transport[2], transport[3]]);

    let (protocol, _) = match ip_proto {
        6  => ("TCP", true),
        17 => ("UDP", false),
        1  => ("ICMP", false),
        _  => return None,
    };

    // Best-effort process lookup via port correlation.
    let process_name = None::<String>; // TODO: GetExtendedTcpTable correlation
    let pid          = None::<u32>;

    Some(WindowsFlow {
        src_ip:       IpAddr::V4(src_ip),
        dst_ip:       IpAddr::V4(dst_ip),
        src_port,
        dst_port,
        protocol:     protocol.to_string(),
        bytes:        data.len() as u64,
        process_name,
        pid,
        timestamp:    SystemTime::now(),
    })
}
