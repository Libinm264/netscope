//! Go crypto/tls uprobe attachment.
//!
//! Go TLS differs from OpenSSL: each Go binary statically links the crypto/tls
//! package, so we cannot attach to a shared library. Instead, we iterate
//! `/proc/*/exe`, identify Go binaries by checking for the "go build" symbol
//! (`runtime.main` is always present in Go binaries), resolve the offsets of
//! `crypto/tls.(*Conn).Write` and `crypto/tls.(*Conn).Read`, and attach
//! uprobes to each binary individually.
//!
//! The BPF programs (`go_tls_write_entry`, `go_tls_read_ret`) live in the
//! same BPF ELF as the OpenSSL probes and share the `SSL_EVENTS` perf array.

#![cfg(target_os = "linux")]

use std::{
    collections::HashSet,
    fs,
    io::{BufRead, BufReader},
    path::{Path, PathBuf},
    process::Command,
};

use anyhow::{Context, Result};
use aya::{
    programs::UProbe,
    Ebpf,
};
use tokio::sync::mpsc;
use tracing::{debug, info, warn};

use crate::EbpfEvent;

const GO_WRITE_SYMBOL: &str = "crypto/tls.(*Conn).Write";
const GO_READ_SYMBOL:  &str = "crypto/tls.(*Conn).Read";

/// Enumerate all running Go processes and attach uprobes to crypto/tls Write/Read.
///
/// Returns the count of processes successfully instrumented.
pub async fn attach_all(
    ebpf: &mut Ebpf,
    _tx: mpsc::Sender<EbpfEvent>,
) -> Result<usize> {
    let go_binaries = discover_go_binaries();
    if go_binaries.is_empty() {
        info!("go-tls: no Go binaries found in /proc — skipping");
        return Ok(0);
    }

    let write_prog: &mut UProbe = ebpf
        .program_mut("go_tls_write_entry")
        .context("go_tls_write_entry program not found in BPF ELF")?
        .try_into()?;
    write_prog.load()?;

    let read_prog: &mut UProbe = ebpf
        .program_mut("go_tls_read_ret")
        .context("go_tls_read_ret program not found in BPF ELF")?
        .try_into()?;
    read_prog.load()?;

    let mut attached = 0;
    for binary in &go_binaries {
        // Resolve symbol offsets for this binary.
        let Some(write_offset) = resolve_symbol(binary, GO_WRITE_SYMBOL) else {
            debug!("go-tls: {} has no Write symbol — skipping", binary.display());
            continue;
        };
        let Some(read_offset) = resolve_symbol(binary, GO_READ_SYMBOL) else {
            debug!("go-tls: {} has no Read symbol — skipping", binary.display());
            continue;
        };

        if let Err(e) = write_prog.attach(
            None,
            write_offset,
            binary,
            None,
        ) {
            warn!("go-tls: attach Write to {} failed: {}", binary.display(), e);
            continue;
        }
        if let Err(e) = read_prog.attach(
            None,
            read_offset,
            binary,
            None,
        ) {
            warn!("go-tls: attach Read to {} failed: {}", binary.display(), e);
            continue;
        }

        info!("go-tls: attached to {}", binary.display());
        attached += 1;
    }

    // Note: SSL_EVENTS is shared with the OpenSSL probes. ssl::attach() already
    // took the map and started pumping it — Go TLS events arrive on the same
    // perf ring and are delivered through that existing pump.

    Ok(attached)
}

/// Discover unique Go binary paths from /proc/*/exe.
fn discover_go_binaries() -> Vec<PathBuf> {
    let mut seen: HashSet<PathBuf> = HashSet::new();
    let mut result = Vec::new();

    let Ok(proc) = fs::read_dir("/proc") else { return result; };

    for entry in proc.flatten() {
        let pid_dir = entry.path();
        // Only process numeric PID directories.
        if !entry.file_name().to_string_lossy().chars().all(|c| c.is_ascii_digit()) {
            continue;
        }
        let exe_link = pid_dir.join("exe");
        let Ok(binary) = fs::read_link(&exe_link) else { continue; };
        if seen.contains(&binary) {
            continue;
        }
        if is_go_binary(&binary) {
            seen.insert(binary.clone());
            result.push(binary);
        }
    }
    result
}

/// Check whether a binary is a Go binary by looking for the `runtime.main`
/// symbol using `nm --dynamic` or `objdump -t`.
fn is_go_binary(path: &Path) -> bool {
    if !path.exists() {
        return false;
    }
    // Fast heuristic: read the first 512 bytes and look for "go build".
    if let Ok(mut f) = fs::File::open(path) {
        use std::io::Read;
        let mut buf = [0u8; 512];
        if f.read(&mut buf).is_ok() {
            if buf.windows(8).any(|w| w == b"go build") {
                return true;
            }
        }
    }
    // Slower: run `nm` and look for runtime.main.
    Command::new("nm")
        .args(["--dynamic", path.to_str().unwrap_or_default()])
        .output()
        .map(|o| String::from_utf8_lossy(&o.stdout).contains("runtime.main"))
        .unwrap_or(false)
}

/// Resolve the file offset of a symbol using `nm`.
fn resolve_symbol(binary: &Path, symbol: &str) -> Option<u64> {
    let output = Command::new("nm")
        .args([binary.to_str()?, "--format=posix"])
        .output()
        .ok()?;
    let stdout = String::from_utf8_lossy(&output.stdout);
    for line in stdout.lines() {
        // nm posix format: name type value size
        let parts: Vec<&str> = line.split_whitespace().collect();
        if parts.len() >= 3 && parts[0] == symbol {
            return u64::from_str_radix(parts[2], 16).ok();
        }
    }
    None
}
