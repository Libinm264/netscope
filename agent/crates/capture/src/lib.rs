pub mod interface;
pub mod tcp_stream;

use anyhow::Result;
use chrono::Utc;
use config::AgentConfig;
use etherparse::{InternetSlice, SlicedPacket, TransportSlice};
use pcap::{Capture, Device};
use proto::PacketEvent;
use proto::Protocol;
use std::sync::mpsc;
use thiserror::Error;
use tracing::{debug, error};

#[derive(Error, Debug)]
pub enum CaptureError {
    #[error("Insufficient privileges for packet capture. Try running with sudo or grant CAP_NET_RAW.")]
    InsufficientPrivileges,

    #[error("Interface '{0}' not found. Use 'netscope-agent list-interfaces' to see available interfaces.")]
    InterfaceNotFound(String),

    #[error("pcap error: {0}")]
    Pcap(#[from] pcap::Error),

    #[error("Capture error: {0}")]
    Other(#[from] anyhow::Error),
}

/// List all available network interfaces on this machine.
pub fn list_interfaces() -> Result<Vec<interface::InterfaceInfo>, CaptureError> {
    let devices = Device::list()?;
    let infos = devices
        .into_iter()
        .map(|d| interface::InterfaceInfo {
            name: d.name.clone(),
            description: d.desc.unwrap_or_default(),
            addresses: d.addresses.iter().map(|a| a.addr.to_string()).collect(),
        })
        .collect();
    Ok(infos)
}

/// Start a blocking packet capture loop, sending raw PacketEvents to the given sender.
/// This should be called from a dedicated thread, not the async runtime.
pub fn start_capture(
    cfg: &AgentConfig,
    tx: mpsc::Sender<PacketEvent>,
) -> Result<(), CaptureError> {
    let device = Device::list()
        .map_err(CaptureError::Pcap)?
        .into_iter()
        .find(|d| d.name == cfg.interface)
        .ok_or_else(|| CaptureError::InterfaceNotFound(cfg.interface.clone()))?;

    let mut cap = Capture::from_device(device)
        .map_err(|e| {
            if e.to_string().contains("permission") || e.to_string().contains("Operation not permitted") {
                CaptureError::InsufficientPrivileges
            } else {
                CaptureError::Pcap(e)
            }
        })?
        .promisc(cfg.promiscuous)
        .snaplen(cfg.snaplen)
        .timeout(cfg.buffer_timeout_ms)
        .open()
        .map_err(|e| {
            if e.to_string().contains("permission") || e.to_string().contains("Operation not permitted") {
                CaptureError::InsufficientPrivileges
            } else {
                CaptureError::Pcap(e)
            }
        })?;

    if let Some(ref filter) = cfg.bpf_filter {
        cap.filter(filter, true).map_err(CaptureError::Pcap)?;
    }

    loop {
        match cap.next_packet() {
            Ok(packet) => {
                let ts = Utc::now();
                let data = packet.data.to_vec();

                match parse_packet(&data, ts) {
                    Some(event) => {
                        if tx.send(event).is_err() {
                            break;
                        }
                    }
                    None => {
                        debug!("Skipped non-IP packet ({} bytes)", data.len());
                    }
                }
            }
            Err(pcap::Error::TimeoutExpired) => continue,
            Err(e) => {
                error!("Capture error: {}", e);
                return Err(CaptureError::Pcap(e));
            }
        }
    }

    Ok(())
}

/// Parse raw packet bytes into a PacketEvent. Returns None for non-IP packets.
fn parse_packet(data: &[u8], timestamp: chrono::DateTime<Utc>) -> Option<PacketEvent> {
    let sliced = SlicedPacket::from_ethernet(data)
        .or_else(|_| SlicedPacket::from_ip(data))
        .ok()?;

    // etherparse 0.15: InternetSlice variants hold a single Slice type
    let (src_ip, dst_ip): (String, String) = match &sliced.net {
        Some(InternetSlice::Ipv4(ipv4)) => (
            ipv4.header().source_addr().to_string(),
            ipv4.header().destination_addr().to_string(),
        ),
        Some(InternetSlice::Ipv6(ipv6)) => (
            ipv6.header().source_addr().to_string(),
            ipv6.header().destination_addr().to_string(),
        ),
        None => return None,
    };

    let (src_port, dst_port, protocol) = match &sliced.transport {
        Some(TransportSlice::Tcp(tcp)) => (
            Some(tcp.source_port()),
            Some(tcp.destination_port()),
            Protocol::Tcp,
        ),
        Some(TransportSlice::Udp(udp)) => (
            Some(udp.source_port()),
            Some(udp.destination_port()),
            Protocol::Udp,
        ),
        _ => (None, None, Protocol::Unknown),
    };

    Some(PacketEvent {
        timestamp,
        src_ip,
        dst_ip,
        src_port,
        dst_port,
        protocol,
        length: data.len() as u32,
        raw: data.to_vec(),
    })
}
