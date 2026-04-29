//! Kprobes for TCP connection tracking with process attribution.
//!
//! We hook `tcp_connect` (outbound SYN) to record the connection's 5-tuple
//! keyed by PID so that SSL uprobes can look it up when they fire.
//!
//! Kernel struct sock field offsets
//! ─────────────────────────────────
//! __sk_common starts at the very beginning of struct sock.
//!   skc_daddr      = 0   dst IPv4 (network byte order)
//!   skc_rcv_saddr  = 4   src IPv4 (network byte order, set after connect)
//!   skc_dport      = 12  dst port (network byte order, u16)
//!   skc_num        = 14  src port (host byte order, u16)
//! Stable since Linux 4.x on x86-64 / aarch64.

use aya_ebpf::{
    helpers::{bpf_get_current_comm, bpf_get_current_pid_tgid, bpf_probe_read_kernel},
    macros::{kprobe, kretprobe, map},
    maps::{HashMap, PerfEventArray},
    programs::{ProbeContext, RetProbeContext},
};
use ebpf_common::{ProbeStateKey, TcpConnectEvent, COMM_LEN};

const SK_SKC_DADDR:    u64 = 0;
const SK_SKC_RCVSADDR: u64 = 4;
const SK_SKC_DPORT:    u64 = 12;
const SK_SKC_NUM:      u64 = 14;

#[repr(C)]
pub struct ConnEntry {
    pub src_ip:   u32,
    pub dst_ip:   u32,
    pub src_port: u16,
    pub dst_port: u16,
}

#[map]
static mut TCP_STATE: HashMap<ProbeStateKey, u64> =
    HashMap::with_max_entries(1024, 0);

#[map]
pub static mut PID_CONN: HashMap<u32, ConnEntry> =
    HashMap::with_max_entries(4096, 0);

#[map]
pub static mut TCP_EVENTS: PerfEventArray<TcpConnectEvent> =
    PerfEventArray::new(0);

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

#[kretprobe]
pub fn tcp_connect_return(ctx: RetProbeContext) -> u32 {
    let key = bpf_get_current_pid_tgid();

    let sk_ptr = match unsafe { TCP_STATE.get(&key) } {
        Some(p) => *p,
        None    => return 0,
    };
    unsafe { TCP_STATE.remove(&key).ok() };

    // New aya-ebpf API: ret() requires an explicit type parameter.
    let retval: i32 = ctx.ret::<i32>().unwrap_or(-1);

    // New aya-ebpf API: bpf_probe_read_kernel<T>(src) reads exactly sizeof(T) bytes.
    let dst_ip: u32 = unsafe {
        bpf_probe_read_kernel::<u32>((sk_ptr + SK_SKC_DADDR) as *const u32).unwrap_or(0)
    };
    let src_ip: u32 = unsafe {
        bpf_probe_read_kernel::<u32>((sk_ptr + SK_SKC_RCVSADDR) as *const u32).unwrap_or(0)
    };
    let dst_port_be: u16 = unsafe {
        bpf_probe_read_kernel::<u16>((sk_ptr + SK_SKC_DPORT) as *const u16).unwrap_or(0)
    };
    let src_port: u16 = unsafe {
        bpf_probe_read_kernel::<u16>((sk_ptr + SK_SKC_NUM) as *const u16).unwrap_or(0)
    };

    let dst_port = u16::from_be(dst_port_be);
    let pid = (key >> 32) as u32;

    let conn = ConnEntry { src_ip, dst_ip, src_port, dst_port };
    unsafe { PID_CONN.insert(&pid, &conn, 0).ok() };

    // New aya-ebpf API: bpf_get_current_comm() takes no argument, returns Result.
    let comm: [u8; COMM_LEN] = bpf_get_current_comm().unwrap_or([0u8; COMM_LEN]);

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
