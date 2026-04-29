//! Userspace eBPF loader for the NetScope agent.
//!
//! Loads the pre-compiled BPF ELF (embedded at compile time), attaches all
//! probes, and pumps events out through a channel for the main agent loop.
//!
//! # Requirements
//! - Linux kernel ≥ 5.8 (BPF ring buffers, BTF)
//! - `/sys/kernel/btf/vmlinux` (BTF for CO-RE relocations)
//! - `CAP_BPF`, `CAP_NET_ADMIN`, `CAP_PERFMON` — or run as root

#![cfg(target_os = "linux")]

mod go_tls;
mod python_ssl;
mod ssl;
mod tcp;

use anyhow::{Context, Result};
use aya::Ebpf;
use tokio::sync::mpsc;
use tracing::{info, warn};

pub use ebpf_common::SslDirection;
pub use ssl::SslFlowEvent;
pub use tcp::TcpFlowEvent;

/// A decoded event from any eBPF probe.
#[derive(Debug, Clone)]
pub enum EbpfEvent {
    /// Plaintext data intercepted from an SSL_read / SSL_write call.
    Ssl(SslFlowEvent),
    /// Outbound TCP connection with process attribution.
    TcpConnect(TcpFlowEvent),
}

/// Configuration for the eBPF loader.
#[derive(Clone, Debug)]
pub struct EbpfConfig {
    /// Path to the libssl shared library.
    /// If None, the loader will attempt to discover it automatically.
    pub libssl_path: Option<String>,
    /// Capacity of the event channel.
    pub channel_capacity: usize,
    /// Attach uprobes to Go crypto/tls (Write + Read) in running Go binaries.
    /// Requires scanning /proc — adds ~100ms startup overhead per Go process.
    pub enable_go_tls: bool,
    /// Attach uprobes to Python _ssl.cpython-*.so (ssl.SSLSocket send/recv).
    pub enable_python_ssl: bool,
}

impl Default for EbpfConfig {
    fn default() -> Self {
        Self {
            libssl_path: None,
            channel_capacity: 8192,
            enable_go_tls: false,
            enable_python_ssl: false,
        }
    }
}

/// Start the eBPF loader.  Returns a receiver that yields decoded events.
///
/// This is the main entry point.  Call once at startup; the background tasks
/// run until the returned handle is dropped or `shutdown_tx` fires.
pub async fn start(
    cfg: EbpfConfig,
) -> Result<mpsc::Receiver<EbpfEvent>> {
    let (tx, rx) = mpsc::channel(cfg.channel_capacity);

    // Load the BPF ELF compiled by `cargo xtask build-ebpf`.
    // The BPF ELF is built by `cargo xtask build-ebpf --release` which writes
    // to agent/target/bpfel-unknown-none/release/netscope-ebpf.
    // CARGO_MANIFEST_DIR = agent/crates/ebpf-loader  → ../../ = agent/
    let mut ebpf = Ebpf::load(include_bytes!(concat!(
        env!("CARGO_MANIFEST_DIR"),
        "/../../target/bpfel-unknown-none/release/netscope-ebpf"
    )))
    .context("Failed to load BPF object — run `cargo xtask build-ebpf --release` first")?;

    // Attach aya logger so BPF trace_printk output appears via tracing.
    if let Err(e) = aya_log::EbpfLogger::init(&mut ebpf) {
        warn!("BPF logger init failed: {}", e);
    }

    // Discover libssl path if not provided.
    let libssl = match &cfg.libssl_path {
        Some(p) => p.clone(),
        None => ssl::find_libssl()
            .context("Could not find libssl — install libssl-dev or set --libssl-path")?,
    };
    info!("Attaching SSL probes to {}", libssl);

    // Attach SSL uprobes.
    ssl::attach(&mut ebpf, &libssl, tx.clone())
        .await
        .context("SSL uprobe attach failed")?;

    // Attach TCP kprobes.
    tcp::attach(&mut ebpf, tx.clone())
        .await
        .context("TCP kprobe attach failed")?;

    // Optional: Go crypto/tls uprobes (Community feature).
    if cfg.enable_go_tls {
        match go_tls::attach_all(&mut ebpf, tx.clone()).await {
            Ok(n) => info!("Go TLS uprobes attached to {} binaries", n),
            Err(e) => warn!("Go TLS uprobe attach failed: {}", e),
        }
    }

    // Optional: Python ssl uprobes (Community feature).
    if cfg.enable_python_ssl {
        match python_ssl::attach_all(&mut ebpf, tx.clone()).await {
            Ok(n) => info!("Python SSL uprobes attached to {} libraries", n),
            Err(e) => warn!("Python SSL uprobe attach failed: {}", e),
        }
    }

    // Keep the ebpf handle alive by moving it into a background task.
    tokio::spawn(async move {
        let _ebpf = ebpf; // dropped when this task exits
        // Park forever — probes keep running as long as this handle is alive.
        tokio::signal::ctrl_c().await.ok();
        info!("eBPF loader shutting down");
    });

    Ok(rx)
}
