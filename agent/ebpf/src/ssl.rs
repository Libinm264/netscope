//! Uprobes on SSL_write and SSL_read to capture plaintext before encryption
//! and after decryption respectively.
//!
//! Strategy
//! ────────
//!   SSL_write entry  → save buf_ptr in WRITE_STATE keyed by pid_tgid
//!   SSL_write return → read plaintext from buf_ptr, tag with IP/port from
//!                      PID_CONN (populated by tcp_connect kprobe), emit event
//!
//!   SSL_read  entry  → save buf_ptr in READ_STATE
//!   SSL_read  return → return value is byte count; read buf, emit event
//!
//! Stack space
//! ──────────
//! SslEvent is ~4140 bytes — far beyond the 512-byte BPF stack limit.
//! We use a per-CPU scratch array (one slot per CPU, no lock needed in
//! BPF context) so the event lives in map memory, not on the stack.

use aya_ebpf::{
    helpers::{bpf_get_current_comm, bpf_get_current_pid_tgid, bpf_probe_read_user},
    macros::{map, uprobe, uretprobe},
    maps::{HashMap, PerCpuArray, PerfEventArray},
    programs::{ProbeContext, RetProbeContext},
};
use ebpf_common::{
    ProbeStateKey, SslDirection, SslEvent, SslProbeState, COMM_LEN, SSL_DATA_MAX,
};

// ── BPF maps ──────────────────────────────────────────────────────────────────

/// Per-CPU scratch buffer for assembling SslEvents before output.
#[map]
static mut SSL_EVENT_SCRATCH: PerCpuArray<SslEvent> =
    PerCpuArray::with_max_entries(1, 0);

/// Entry state for in-flight SSL_write calls: pid_tgid → buf_ptr
#[map]
static mut WRITE_STATE: HashMap<ProbeStateKey, SslProbeState> =
    HashMap::with_max_entries(1024, 0);

/// Entry state for in-flight SSL_read calls: pid_tgid → buf_ptr
#[map]
static mut READ_STATE: HashMap<ProbeStateKey, SslProbeState> =
    HashMap::with_max_entries(1024, 0);

/// SSL plaintext events delivered to userspace via perf ring.
#[map]
pub static mut SSL_EVENTS: PerfEventArray<SslEvent> = PerfEventArray::new(0);

// ── Helpers ───────────────────────────────────────────────────────────────────

#[inline(always)]
fn pid_tgid_key() -> ProbeStateKey {
    bpf_get_current_pid_tgid()
}

#[inline(always)]
fn current_comm() -> [u8; COMM_LEN] {
    // New aya-ebpf API: bpf_get_current_comm() takes no argument, returns Result.
    bpf_get_current_comm().unwrap_or([0u8; COMM_LEN])
}

#[inline(always)]
fn conn_for_pid(pid: u32) -> (u32, u32, u16, u16) {
    match unsafe { crate::tcp::PID_CONN.get(&pid) } {
        Some(c) => (c.src_ip, c.dst_ip, c.src_port, c.dst_port),
        None    => (0, 0, 0, 0),
    }
}

// ── SSL_write ─────────────────────────────────────────────────────────────────

#[uprobe]
pub fn ssl_write_entry(ctx: ProbeContext) -> u32 {
    let buf_ptr: u64 = match ctx.arg(1) {
        Some(v) => v,
        None => return 0,
    };
    let key = pid_tgid_key();
    let state = SslProbeState { buf_ptr, fd: 0, _pad: 0 };
    unsafe { WRITE_STATE.insert(&key, &state, 0).ok() };
    0
}

#[uretprobe]
pub fn ssl_write_return(ctx: RetProbeContext) -> u32 {
    let key = pid_tgid_key();

    let state = match unsafe { WRITE_STATE.get(&key) } {
        Some(s) => *s,
        None    => return 0,
    };
    unsafe { WRITE_STATE.remove(&key).ok() };

    // New aya-ebpf API: ret() requires an explicit type parameter.
    let ret: i64 = ctx.ret::<i64>().unwrap_or(0);
    if ret <= 0 {
        return 0;
    }
    let data_len = (ret as usize).min(SSL_DATA_MAX) as u32;

    let event_ptr = match unsafe { SSL_EVENT_SCRATCH.get_ptr_mut(0) } {
        Some(p) => p,
        None    => return 0,
    };

    let pid = (key >> 32) as u32;
    let (src_ip, dst_ip, src_port, dst_port) = conn_for_pid(pid);

    unsafe {
        let e = &mut *event_ptr;
        e.pid       = pid;
        e.tid       = key as u32;
        e.comm      = current_comm();
        e.direction = SslDirection::Write;
        e.src_ip    = src_ip;
        e.dst_ip    = dst_ip;
        e.src_port  = src_port;
        e.dst_port  = dst_port;
        e.data_len  = data_len;

        // New aya-ebpf API: bpf_probe_read_user<T>(src) reads sizeof(T) bytes.
        // Read the full SSL_DATA_MAX-byte array; the recipient uses data_len to
        // know how many bytes are meaningful.
        if let Ok(buf) = bpf_probe_read_user::<[u8; SSL_DATA_MAX]>(
            state.buf_ptr as *const [u8; SSL_DATA_MAX],
        ) {
            e.data = buf;
        }

        SSL_EVENTS.output(&ctx, &*event_ptr, 0);
    }
    0
}

// ── SSL_read ──────────────────────────────────────────────────────────────────

#[uprobe]
pub fn ssl_read_entry(ctx: ProbeContext) -> u32 {
    let buf_ptr: u64 = match ctx.arg(1) {
        Some(v) => v,
        None => return 0,
    };
    let key = pid_tgid_key();
    let state = SslProbeState { buf_ptr, fd: 0, _pad: 0 };
    unsafe { READ_STATE.insert(&key, &state, 0).ok() };
    0
}

#[uretprobe]
pub fn ssl_read_return(ctx: RetProbeContext) -> u32 {
    let key = pid_tgid_key();

    let state = match unsafe { READ_STATE.get(&key) } {
        Some(s) => *s,
        None    => return 0,
    };
    unsafe { READ_STATE.remove(&key).ok() };

    let ret: i64 = ctx.ret::<i64>().unwrap_or(0);
    if ret <= 0 {
        return 0;
    }
    let data_len = (ret as usize).min(SSL_DATA_MAX) as u32;

    let event_ptr = match unsafe { SSL_EVENT_SCRATCH.get_ptr_mut(0) } {
        Some(p) => p,
        None    => return 0,
    };

    let pid = (key >> 32) as u32;
    let (src_ip, dst_ip, src_port, dst_port) = conn_for_pid(pid);

    unsafe {
        let e = &mut *event_ptr;
        e.pid       = pid;
        e.tid       = key as u32;
        e.comm      = current_comm();
        e.direction = SslDirection::Read;
        e.src_ip    = src_ip;
        e.dst_ip    = dst_ip;
        e.src_port  = src_port;
        e.dst_port  = dst_port;
        e.data_len  = data_len;

        if let Ok(buf) = bpf_probe_read_user::<[u8; SSL_DATA_MAX]>(
            state.buf_ptr as *const [u8; SSL_DATA_MAX],
        ) {
            e.data = buf;
        }

        SSL_EVENTS.output(&ctx, &*event_ptr, 0);
    }
    0
}
