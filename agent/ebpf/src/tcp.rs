//! Kprobes for TCP connection tracking with process attribution.
//!
//! We hook `tcp_connect` (outbound SYN) to record the connection's 5-tuple
//! keyed by PID so that SSL uprobes can look it up when they fire.
//!
//! Kernel struct offsets
//! ─────────────────────
//! The `struct sock` (and `struct inet_sock`) fields we need live inside
//! `__sk_common`:
//!
//!   offsetof(__sk_common, skc_daddr)      = 0  (dst IPv4, network byte order)
//!   offsetof(__sk_common, skc_rcv_saddr)  = 4  (src IPv4, network byte order)
//!   offsetof(__sk_common, skc_dport)      = 12 (dst port, network byte order, u16)
//!   offsetof(__sk_common, skc_num)        = 14 (src port, host byte order, u16)
//!
//! These have been stable since Linux 4.x for x86-64 / aarch64.
//! See: include/net/sock.h → struct sock_common.

use aya_bpf::{
    helpers::{bpf_get_current_comm, bpf_get_current_pid_tgid, bpf_probe_read_kernel},
    macros::{kprobe, kretprobe, map},
    maps::{HashMap, PerfEventArray},
    programs::{ProbeContext, RetProbeContext},
};
use ebpf_common::{ProbeStateKey, TcpConnectEvent, COMM_LEN};

// ── Kernel struct sock field offsets ──────────────────────────────────────────
// __sk_common starts at the very beginning of struct sock.

/// skc_daddr — destination IPv4 (network byte order)
const SK_SKC_DADDR: u64 = 0;
/// skc_rcv_saddr — source IPv4 (network byte order, set after connect returns)
const SK_SKC_RCVSADDR: u64 = 4;
/// skc_dport — destination port (network byte order / big-endian, u16)
const SK_SKC_DPORT: u64 = 12;
/// skc_num — source port (host byte order, u16)
const SK_SKC_NUM: u64 = 14;

// ── Connection entry shared with SSL uprobes ──────────────────────────────────

/// Minimal connection record stored per-PID so SSL probes can tag their events
/// with network-layer info without needing to know the SSL* internals.
#[repr(C)]
pub struct ConnEntry {
    pub src_ip:   u32,
    pub dst_ip:   u32,
    pub src_port: u16,
    pub dst_port: u16,
}

// ── BPF maps ──────────────────────────────────────────────────────────────────

/// Scratch storage for the sock* pointer between tcp_connect entry and return.
#[map]
static mut TCP_STATE: HashMap<ProbeStateKey, u64> =
    HashMap::with_max_entries(1024, 0);

/// Most-recent successful TCP connection keyed by PID.
/// SSL uprobes read this to tag plaintext events with IP/port info.
/// The map is intentionally small — we only need a recent snapshot per process.
#[map]
pub static mut PID_CONN: HashMap<u32, ConnEntry> =
    HashMap::with_max_entries(4096, 0);

/// Outbound TCP connection events delivered to userspace for flow tracking.
#[map]
pub static mut TCP_EVENTS: PerfEventArray<TcpConnectEvent> =
    PerfEventArray::new(0);

// ── tcp_connect — entry ───────────────────────────────────────────────────────
//
// Kernel signature: int tcp_connect(struct sock *sk)
// arg0 = struct sock *sk

#[kprobe]
pub fn tcp_connect_entry(ctx: ProbeContext) -> u32 {
    let sk_ptr: u64 = match ctx.arg(0) {
        Some(v) => v,
        None => return 0,
    };
    let key = bpf_get_current_pid_tgid();
    unsafe { TCP_STATE.insert(&key, &sk_ptr, 0).ok() };
    0
}

// ── tcp_connect — return ──────────────────────────────────────────────────────

#[kretprobe]
pub fn tcp_connect_return(ctx: RetProbeContext) -> u32 {
    let key = bpf_get_current_pid_tgid();

    let sk_ptr = match unsafe { TCP_STATE.get(&key) } {
        Some(p) => *p,
        None    => return 0,
    };
    unsafe { TCP_STATE.remove(&key).ok() };

    let retval: i32 = ctx.ret().unwrap_or(-1);

    // Read the 5-tuple from struct sock via bpf_probe_read_kernel.
    let mut dst_ip: u32    = 0;
    let mut src_ip: u32    = 0;
    let mut dst_port_be: u16 = 0;
    let mut src_port: u16  = 0;

    unsafe {
        bpf_probe_read_kernel(
            &mut dst_ip as *mut _ as *mut _,
            core::mem::size_of::<u32>() as u32,
            (sk_ptr + SK_SKC_DADDR) as *const _,
        ).ok();
        bpf_probe_read_kernel(
            &mut src_ip as *mut _ as *mut _,
            core::mem::size_of::<u32>() as u32,
            (sk_ptr + SK_SKC_RCVSADDR) as *const _,
        ).ok();
        bpf_probe_read_kernel(
            &mut dst_port_be as *mut _ as *mut _,
            core::mem::size_of::<u16>() as u32,
            (sk_ptr + SK_SKC_DPORT) as *const _,
        ).ok();
        bpf_probe_read_kernel(
            &mut src_port as *mut _ as *mut _,
            core::mem::size_of::<u16>() as u32,
            (sk_ptr + SK_SKC_NUM) as *const _,
        ).ok();
    }

    let dst_port = u16::from_be(dst_port_be);
    let pid = (key >> 32) as u32;

    // Update PID_CONN so SSL probes can tag their events, regardless of
    // whether the connect succeeded (SSL may already be in flight).
    let conn = ConnEntry { src_ip, dst_ip, src_port, dst_port };
    unsafe { PID_CONN.insert(&pid, &conn, 0).ok() };

    let mut comm = [0u8; COMM_LEN];
    let _ = bpf_get_current_comm(&mut comm);

    let event = TcpConnectEvent {
        pid,
        tid: key as u32,
        comm,
        src_ip,
        dst_ip,
        src_port,
        dst_port,
        retval,
    };

    unsafe { TCP_EVENTS.output(&ctx, &event, 0) };
    0
}
