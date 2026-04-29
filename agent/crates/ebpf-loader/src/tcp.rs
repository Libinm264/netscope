//! TCP kprobe attachment and connection event decoding.

#![cfg(target_os = "linux")]

use anyhow::{Context, Result};
use aya::{
    maps::{AsyncPerfEventArray, MapData},
    programs::KProbe,
    util::online_cpus,
    Ebpf,
};
use bytes::BytesMut;
use chrono::Utc;
use ebpf_common::TcpConnectEvent;
use tokio::sync::mpsc;
use tracing::{debug, warn};

use crate::EbpfEvent;

/// Decoded TCP connect event with process attribution.
#[derive(Debug, Clone)]
pub struct TcpFlowEvent {
    pub pid: u32,
    pub comm: String,
    pub src_ip: std::net::Ipv4Addr,
    pub dst_ip: std::net::Ipv4Addr,
    pub src_port: u16,
    pub dst_port: u16,
    pub success: bool,
    pub timestamp: chrono::DateTime<Utc>,
}

/// Attach tcp_connect kprobe/kretprobe and start pumping events into `tx`.
pub async fn attach(ebpf: &mut Ebpf, tx: mpsc::Sender<EbpfEvent>) -> Result<()> {
    // Entry probe — saves the sock pointer
    {
        let prog: &mut KProbe = ebpf
            .program_mut("tcp_connect_entry")
            .context("tcp_connect_entry not found")?
            .try_into()
            .context("Expected KProbe")?;
        prog.load().context("Failed to load tcp_connect_entry")?;
        prog.attach("tcp_connect", 0)
            .context("Failed to attach tcp_connect_entry")?;
    }

    // Return probe — reads sock fields and emits event
    {
        let prog: &mut KProbe = ebpf
            .program_mut("tcp_connect_return")
            .context("tcp_connect_return not found")?
            .try_into()
            .context("Expected KProbe")?;
        prog.load().context("Failed to load tcp_connect_return")?;
        prog.attach("tcp_connect", 0)
            .context("Failed to attach tcp_connect_return")?;
    }

    // Pump perf events
    let tcp_events: AsyncPerfEventArray<MapData> =
        AsyncPerfEventArray::try_from(
            ebpf.take_map("TCP_EVENTS").context("TCP_EVENTS map missing")?,
        )
        .context("TCP_EVENTS is not a PerfEventArray")?;

    let cpus = online_cpus()
        .map_err(|(msg, e)| anyhow::anyhow!("online_cpus: {}: {}", msg, e))?;
    for cpu_id in cpus {
        let mut buf = tcp_events.open(cpu_id, None)?;
        let tx2 = tx.clone();

        tokio::spawn(async move {
            let mut bufs = (0..16)
                .map(|_| BytesMut::with_capacity(core::mem::size_of::<TcpConnectEvent>()))
                .collect::<Vec<_>>();

            loop {
                let events = match buf.read_events(&mut bufs).await {
                    Ok(e) => e,
                    Err(e) => {
                        warn!("TCP perf read error cpu={}: {}", cpu_id, e);
                        break;
                    }
                };

                for i in 0..events.read {
                    let raw = &bufs[i];
                    if raw.len() < core::mem::size_of::<TcpConnectEvent>() {
                        continue;
                    }
                    let evt: TcpConnectEvent = unsafe {
                        core::ptr::read_unaligned(raw.as_ptr() as *const TcpConnectEvent)
                    };

                    let comm = String::from_utf8_lossy(
                        evt.comm.split(|&b| b == 0).next().unwrap_or(&[]),
                    ).into_owned();

                    let flow = TcpFlowEvent {
                        pid: evt.pid,
                        comm,
                        src_ip: std::net::Ipv4Addr::from(u32::from_be(evt.src_ip)),
                        dst_ip: std::net::Ipv4Addr::from(u32::from_be(evt.dst_ip)),
                        src_port: evt.src_port,
                        dst_port: evt.dst_port,
                        success: evt.retval == 0,
                        timestamp: Utc::now(),
                    };

                    debug!(
                        pid = flow.pid,
                        comm = %flow.comm,
                        dst = %flow.dst_ip,
                        port = flow.dst_port,
                        success = flow.success,
                        "TCP connect"
                    );

                    if tx2.send(EbpfEvent::TcpConnect(flow)).await.is_err() {
                        break;
                    }
                }
            }
        });
    }

    Ok(())
}
