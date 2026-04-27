//! Windows process enumeration using Toolhelp32Snapshot.
//!
//! Builds a `pid → process_name` map refreshed every 5 seconds.
//! The agent calls `lookup(pid)` to attribute flows to processes.

#![cfg(target_os = "windows")]

use std::{
    collections::HashMap,
    sync::{Arc, RwLock},
    time::{Duration, Instant},
};

use anyhow::Result;
use windows::Win32::{
    Foundation::CloseHandle,
    System::Diagnostics::ToolHelp::{
        CreateToolhelp32Snapshot, Process32FirstW, Process32NextW,
        PROCESSENTRY32W, TH32CS_SNAPPROCESS,
    },
};

/// Thread-safe process name cache refreshed periodically.
#[derive(Clone)]
pub struct ProcessCache {
    inner: Arc<RwLock<CacheInner>>,
}

struct CacheInner {
    map:         HashMap<u32, String>,
    last_refresh: Instant,
    ttl:         Duration,
}

impl ProcessCache {
    /// Create a new cache with a 5-second TTL.
    pub fn new() -> Self {
        let mut inner = CacheInner {
            map:          HashMap::new(),
            last_refresh: Instant::now() - Duration::from_secs(10),
            ttl:          Duration::from_secs(5),
        };
        let _ = inner.refresh();
        Self { inner: Arc::new(RwLock::new(inner)) }
    }

    /// Look up the process name for a given PID.
    /// Refreshes the cache if the TTL has elapsed.
    pub fn lookup(&self, pid: u32) -> Option<String> {
        {
            let r = self.inner.read().unwrap();
            if r.last_refresh.elapsed() < r.ttl {
                return r.map.get(&pid).cloned();
            }
        }
        // Refresh needed — take write lock.
        let mut w = self.inner.write().unwrap();
        if w.last_refresh.elapsed() >= w.ttl {
            let _ = w.refresh();
        }
        w.map.get(&pid).cloned()
    }
}

impl CacheInner {
    fn refresh(&mut self) -> Result<()> {
        self.map.clear();
        unsafe {
            let snapshot = CreateToolhelp32Snapshot(TH32CS_SNAPPROCESS, 0)?;
            let mut entry = PROCESSENTRY32W {
                dwSize: std::mem::size_of::<PROCESSENTRY32W>() as u32,
                ..Default::default()
            };

            if Process32FirstW(snapshot, &mut entry).is_ok() {
                loop {
                    let name = String::from_utf16_lossy(
                        &entry.szExeFile[..entry.szExeFile
                            .iter()
                            .position(|&c| c == 0)
                            .unwrap_or(entry.szExeFile.len())],
                    );
                    self.map.insert(entry.th32ProcessID, name);

                    if Process32NextW(snapshot, &mut entry).is_err() {
                        break;
                    }
                }
            }
            let _ = CloseHandle(snapshot);
        }
        self.last_refresh = Instant::now();
        Ok(())
    }
}

impl Default for ProcessCache {
    fn default() -> Self {
        Self::new()
    }
}
