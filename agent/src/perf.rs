/// Agent performance sampler.
///
/// Collects CPU and memory metrics for the current process using `sysinfo`.
/// Call `PerfSampler::new()` once at startup, then `sample()` before each
/// heartbeat. The first sample may return 0 CPU (no prior measurement);
/// subsequent calls over the 30-second heartbeat window are accurate.
use sysinfo::{Pid, System};

/// A single performance snapshot.
#[derive(Debug, Clone, Default)]
pub struct PerfSnapshot {
    /// CPU usage of this process as a percentage (0.0 – 100.0 × num_cores).
    /// Normalised to a single-core equivalent so values stay in 0–100.
    pub cpu_pct: f32,
    /// Resident memory (RSS) in megabytes.
    pub mem_mb: u64,
    /// Cumulative count of flows dropped because the Hub send channel was full.
    pub packets_dropped: u64,
}

/// Stateful sampler — keeps a `System` handle across calls so that CPU delta
/// is computed over the heartbeat interval rather than process lifetime.
pub struct PerfSampler {
    sys:     System,
    pid:     Pid,
    dropped: u64,
}

impl PerfSampler {
    pub fn new() -> Self {
        let pid = Pid::from_u32(std::process::id());
        let mut sys = System::new();
        sys.refresh_process(pid);
        Self { sys, pid, dropped: 0 }
    }

    /// Increment the dropped-flow counter (call when a send fails due to
    /// back-pressure).
    #[allow(dead_code)]
    pub fn record_drop(&mut self) {
        self.dropped += 1;
    }

    /// Take a snapshot. `cpu_pct` reflects usage since the previous call.
    pub fn sample(&mut self) -> PerfSnapshot {
        self.sys.refresh_process(self.pid);

        let (cpu_pct, mem_mb) = self
            .sys
            .process(self.pid)
            .map(|p| {
                let num_cpus = self.sys.cpus().len().max(1) as f32;
                let cpu = p.cpu_usage() / num_cpus;
                let mem = p.memory() / 1024 / 1024;
                (cpu, mem)
            })
            .unwrap_or((0.0, 0));

        PerfSnapshot {
            cpu_pct,
            mem_mb,
            packets_dropped: self.dropped,
        }
    }
}
