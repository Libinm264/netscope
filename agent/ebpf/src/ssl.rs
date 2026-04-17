//! Uprobes on SSL_write and SSL_read to capture plaintext before encryption
//! and after decryption respectively.
//!
//! Strategy:
//!   SSL_write entry  → save (buf_ptr, pid_tgid) in WRITE_STATE map
//!   SSL_write return → read state, bpf_probe_read buf, emit SslEvent{Write}
//!
//!   SSL_read  entry  → save buf_ptr in READ_STATE map
//!   SSL_read  return → return value is byte count; read buf, emit SslEvent{Read}

use aya_bpf::{
    helpers::{bpf_get_current_comm, bpf_get_current_pid_tgid, bpf_probe_read_user},
    macros::{map, uprobe, uretprobe},
    maps::{HashMap, PerfEventArray},
    programs::{ProbeContext, RetProbeContext},
    BpfContext,
};
use ebpf_common::{
    CommLen, ProbeStateKey, SslDirection, SslEvent, SslProbeState, SSL_DATA_MAX,
};

// ── BPF maps ──────────────────────────────────────────────────────────────────

/// Stores SSL_write / SSL_read entry state keyed by (pid << 32 | tid).
#[map]
static mut WRITE_STATE: HashMap<ProbeStateKey, SslProbeState> =
    HashMap::with_max_entries(1024, 0);

#[map]
static mut READ_STATE: HashMap<ProbeStateKey, SslProbeState> =
    HashMap::with_max_entries(1024, 0);

/// Ring buffer for SSL plaintext events delivered to userspace.
#[map]
pub static mut SSL_EVENTS: PerfEventArray<SslEvent> = PerfEventArray::new(0);

// ── Helpers ───────────────────────────────────────────────────────────────────

#[inline(always)]
fn pid_tgid_key() -> ProbeStateKey {
    bpf_get_current_pid_tgid()
}

#[inline(always)]
fn current_comm() -> [u8; CommLen::COMM_LEN] {
    let mut comm = [0u8; CommLen::COMM_LEN];
    let _ = bpf_get_current_comm(&mut comm);
    comm
}

// ── SSL_write ─────────────────────────────────────────────────────────────────
//
// Signature: int SSL_write(SSL *ssl, const void *buf, int num)
//   arg0 = ssl*, arg1 = buf*, arg2 = num

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
        None => return 0,
    };
    unsafe { WRITE_STATE.remove(&key).ok() };

    let ret: i64 = ctx.ret().unwrap_or(0);
    if ret <= 0 {
        return 0;
    }
    let data_len = (ret as usize).min(SSL_DATA_MAX) as u32;

    let mut event = SslEvent {
        pid: (key >> 32) as u32,
        tid: key as u32,
        comm: current_comm(),
        direction: SslDirection::Write,
        src_ip: 0,
        dst_ip: 0,
        src_port: 0,
        dst_port: 0,
        data_len,
        data: [0u8; SSL_DATA_MAX],
    };

    unsafe {
        bpf_probe_read_user(
            event.data.as_mut_ptr(),
            data_len,
            state.buf_ptr as *const _,
        )
        .ok()
    };

    unsafe { SSL_EVENTS.output(&ctx, &event, 0) };
    0
}

// ── SSL_read ──────────────────────────────────────────────────────────────────
//
// Signature: int SSL_read(SSL *ssl, void *buf, int num)
// The buffer is filled on return, so we capture buf_ptr on entry and
// read it on return when we know the actual byte count.

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
        None => return 0,
    };
    unsafe { READ_STATE.remove(&key).ok() };

    let ret: i64 = ctx.ret().unwrap_or(0);
    if ret <= 0 {
        return 0;
    }
    let data_len = (ret as usize).min(SSL_DATA_MAX) as u32;

    let mut event = SslEvent {
        pid: (key >> 32) as u32,
        tid: key as u32,
        comm: current_comm(),
        direction: SslDirection::Read,
        src_ip: 0,
        dst_ip: 0,
        src_port: 0,
        dst_port: 0,
        data_len,
        data: [0u8; SSL_DATA_MAX],
    };

    unsafe {
        bpf_probe_read_user(
            event.data.as_mut_ptr(),
            data_len,
            state.buf_ptr as *const _,
        )
        .ok()
    };

    unsafe { SSL_EVENTS.output(&ctx, &event, 0) };
    0
}
