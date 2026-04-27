//! Python ssl.SSLSocket uprobe attachment.
//!
//! Python's ssl module is implemented in `_ssl.cpython-*.so`. We locate the
//! shared library for each running Python process by scanning `/proc/<pid>/maps`,
//! then attach uprobes to `_ssl_read` and `_ssl_write` within that library.
//!
//! The BPF programs (`python_ssl_read_ret`, `python_ssl_write_entry`) live in
//! the same BPF ELF and write into the shared `SSL_EVENTS` perf array.

#![cfg(target_os = "linux")]

use std::{
    collections::HashSet,
    fs,
    io::{BufRead, BufReader},
    path::PathBuf,
};

use anyhow::{Context, Result};
use aya::{
    programs::UProbe,
    Ebpf,
};
use tokio::sync::mpsc;
use tracing::{info, warn};

use crate::EbpfEvent;
use super::ssl;

const PYTHON_WRITE_SYMBOL: &str = "_ssl_write";
const PYTHON_READ_SYMBOL:  &str = "_ssl_read";

/// Enumerate Python ssl modules from running processes and attach uprobes.
///
/// Returns the count of libraries successfully instrumented.
pub async fn attach_all(
    ebpf: &mut Ebpf,
    tx: mpsc::Sender<EbpfEvent>,
) -> Result<usize> {
    let ssl_libs = discover_python_ssl_libs();
    if ssl_libs.is_empty() {
        info!("python-ssl: no _ssl.cpython-*.so found — skipping");
        return Ok(0);
    }

    let write_prog: &mut UProbe = ebpf
        .program_mut("python_ssl_write_entry")
        .context("python_ssl_write_entry program not found in BPF ELF")?
        .try_into()?;
    write_prog.load()?;

    let read_prog: &mut UProbe = ebpf
        .program_mut("python_ssl_read_ret")
        .context("python_ssl_read_ret program not found in BPF ELF")?
        .try_into()?;
    read_prog.load()?;

    let mut attached = 0;
    for lib in &ssl_libs {
        let lib_str = lib.to_str().unwrap_or_default();

        if let Err(e) = write_prog.attach(Some(PYTHON_WRITE_SYMBOL), 0, lib_str, None) {
            warn!("python-ssl: attach write to {} failed: {}", lib.display(), e);
            continue;
        }
        if let Err(e) = read_prog.attach(Some(PYTHON_READ_SYMBOL), 0, lib_str, None) {
            warn!("python-ssl: attach read to {} failed: {}", lib.display(), e);
            continue;
        }

        info!("python-ssl: attached to {}", lib.display());
        attached += 1;
    }

    // Pump events from the shared SSL_EVENTS perf array.
    ssl::pump_perf_events(ebpf, tx).await?;

    Ok(attached)
}

/// Scan /proc/*/maps for _ssl.cpython-*.so paths across all Python processes.
fn discover_python_ssl_libs() -> Vec<PathBuf> {
    let mut seen: HashSet<PathBuf> = HashSet::new();

    let Ok(proc) = fs::read_dir("/proc") else { return Vec::new(); };

    for entry in proc.flatten() {
        if !entry.file_name().to_string_lossy().chars().all(|c| c.is_ascii_digit()) {
            continue;
        }
        let maps_path = entry.path().join("maps");
        let Ok(file) = fs::File::open(&maps_path) else { continue; };
        let reader = BufReader::new(file);

        for line in reader.lines().flatten() {
            // /proc/maps format:
            // address perms offset dev inode pathname
            if let Some(path) = line.split_whitespace().last() {
                if path.contains("_ssl.cpython") && path.ends_with(".so") {
                    let pb = PathBuf::from(path);
                    if pb.exists() && !seen.contains(&pb) {
                        seen.insert(pb.clone());
                    }
                }
            }
        }
    }

    seen.into_iter().collect()
}
