//! SSL uprobe attachment and event decoding.

#![cfg(target_os = "linux")]

use std::path::Path;

use anyhow::{bail, Context, Result};
use aya::{
    maps::AsyncPerfEventArray,
    programs::UProbe,
    util::online_cpus,
    Ebpf,
};
use bytes::BytesMut;
use chrono::Utc;
use ebpf_common::{SslDirection, SslEvent, SSL_DATA_MAX};
use tokio::sync::mpsc;
use tracing::{debug, warn};

use crate::EbpfEvent;

/// Decoded SSL plaintext event.
#[derive(Debug, Clone)]
pub struct SslFlowEvent {
    pub pid: u32,
    pub comm: String,
    pub direction: SslDirection,
    pub src_ip: std::net::Ipv4Addr,
    pub dst_ip: std::net::Ipv4Addr,
    pub src_port: u16,
    pub dst_port: u16,
    /// Decoded plaintext (UTF-8 lossy).
    pub data: String,
    pub timestamp: chrono::DateTime<Utc>,
}

/// Discover the path to the OpenSSL shared library via `/proc/*/maps`
/// scanning or `ldconfig -p`.
pub fn find_libssl() -> Result<String> {
    // Try ldconfig first (works on Debian/Ubuntu/RHEL)
    if let Ok(out) = std::process::Command::new("ldconfig")
        .args(["-p"])
        .output()
    {
        let output = String::from_utf8_lossy(&out.stdout);
        for line in output.lines() {
            if line.contains("libssl.so") {
                if let Some(path) = line.split("=>").nth(1) {
                    let p = path.trim().to_string();
                    if Path::new(&p).exists() {
                        return Ok(p);
                    }
                }
            }
        }
    }

    // Fallback: common paths
    for path in &[
        "/usr/lib/x86_64-linux-gnu/libssl.so.3",
        "/usr/lib/x86_64-linux-gnu/libssl.so.1.1",
        "/usr/lib64/libssl.so.3",
        "/usr/lib64/libssl.so.1.1",
        "/lib/x86_64-linux-gnu/libssl.so.3",
    ] {
        if Path::new(path).exists() {
            return Ok(path.to_string());
        }
    }

    bail!("libssl not found on this system");
}

/// Attach SSL_write and SSL_read probes and start pumping events into `tx`.
pub async fn attach(
    ebpf: &mut Ebpf,
    libssl_path: &str,
    tx: mpsc::Sender<EbpfEvent>,
) -> Result<()> {
    // Attach SSL_write entry / return
    attach_uprobe(ebpf, "ssl_write_entry",  libssl_path, "SSL_write", false)?;
    attach_uprobe(ebpf, "ssl_write_return", libssl_path, "SSL_write", true)?;

    // Attach SSL_read entry / return
    attach_uprobe(ebpf, "ssl_read_entry",   libssl_path, "SSL_read",  false)?;
    attach_uprobe(ebpf, "ssl_read_return",  libssl_path, "SSL_read",  true)?;

    // Pump events from the perf ring into the channel
    let ssl_events: AsyncPerfEventArray<SslEvent> =
        AsyncPerfEventArray::try_from(ebpf.take_map("SSL_EVENTS").context("SSL_EVENTS map missing")?)
            .context("SSL_EVENTS is not a PerfEventArray")?;

    let cpus = online_cpus().context("Failed to enumerate online CPUs")?;
    for cpu_id in cpus {
        let mut buf = ssl_events.open(cpu_id, None)?;
        let tx2 = tx.clone();

        tokio::spawn(async move {
            let mut bufs = (0..16)
                .map(|_| BytesMut::with_capacity(core::mem::size_of::<SslEvent>() + 4096))
                .collect::<Vec<_>>();

            loop {
                let events = match buf.read_events(&mut bufs).await {
                    Ok(e) => e,
                    Err(e) => {
                        warn!("SSL perf read error cpu={}: {}", cpu_id, e);
                        break;
                    }
                };

                for i in 0..events.read {
                    let raw = &bufs[i];
                    if raw.len() < core::mem::size_of::<SslEvent>() {
                        continue;
                    }

                    // SAFETY: SslEvent is repr(C) and we checked the length.
                    let evt: SslEvent = unsafe {
                        core::ptr::read_unaligned(raw.as_ptr() as *const SslEvent)
                    };

                    let data_len = evt.data_len.min(SSL_DATA_MAX as u32) as usize;
                    let data = String::from_utf8_lossy(&evt.data[..data_len]).into_owned();
                    let comm = String::from_utf8_lossy(
                        evt.comm.split(|&b| b == 0).next().unwrap_or(&[]),
                    ).into_owned();

                    let flow = SslFlowEvent {
                        pid: evt.pid,
                        comm,
                        direction: evt.direction,
                        src_ip: std::net::Ipv4Addr::from(u32::from_be(evt.src_ip)),
                        dst_ip: std::net::Ipv4Addr::from(u32::from_be(evt.dst_ip)),
                        src_port: evt.src_port,
                        dst_port: evt.dst_port,
                        data,
                        timestamp: Utc::now(),
                    };

                    debug!(
                        pid = flow.pid,
                        comm = %flow.comm,
                        direction = ?flow.direction,
                        bytes = data_len,
                        "SSL event"
                    );

                    if tx2.send(EbpfEvent::Ssl(flow)).await.is_err() {
                        break;
                    }
                }
            }
        });
    }

    Ok(())
}

fn attach_uprobe(
    ebpf: &mut Ebpf,
    prog_name: &str,
    lib_path: &str,
    fn_name: &str,
    is_ret: bool,
) -> Result<()> {
    let prog: &mut UProbe = ebpf
        .program_mut(prog_name)
        .with_context(|| format!("BPF program '{}' not found", prog_name))?
        .try_into()
        .context("Expected UProbe program type")?;

    prog.load().with_context(|| format!("Failed to load {}", prog_name))?;

    prog.attach(Some(fn_name), 0, lib_path, None)
        .with_context(|| {
            format!(
                "Failed to attach {} to {}:{}",
                prog_name, lib_path, fn_name
            )
        })?;

    Ok(())
}
