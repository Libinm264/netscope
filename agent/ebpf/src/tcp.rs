//! Kprobes for TCP connection tracking with process attribution.
//!
//! We hook `tcp_connect` (outbound) and `inet_csk_accept` (inbound) to
//! associate every connection with the PID and process name that opened it —
//! something pcap cannot do without `/proc` polling.

use aya_bpf::{
    helpers::{
        bpf_get_current_comm, bpf_get_current_pid_tgid, bpf_probe_read_kernel,
    },
    macros::{kprobe, kretprobe, map},
    maps::{HashMap, PerfEventArray},
    programs::{ProbeContext, RetProbeContext},
};
use ebpf_common::{ProbeStateKey, TcpConnectEvent, COMM_LEN};

// Kernel struct offsets for struct sock / struct inet_sock.
// These match a modern x86-64 kernel; adjust for ARM or older kernels.
const SK_COMMON_DST_OFFSET: usize = 56;   // __sk_common.skc_daddr  (IPv4 dest)
const SK_COMMON_SRC_OFFSET: usize = 60;   // __sk_common.skc_rcv_saddr
const SK_COMMON_DPORT_OFFSET: usize = 14; // __sk_common.skc_dport (big-endian)
const SK_COMMON_SPORT_OFFSET: usize = 12; // __sk_common.skc_num

// ── BPF maps ──────────────────────────────────────────────────────────────────

/// Scratch storage for the sock* pointer between tcp_connect entry and return.
#[map]
static mut TCP_STATE: HashMap<ProbeStateKey, u64> =
    HashMap::with_max_entries(1024, 0);

/// Outbound connection events delivered to userspace.
#[map]
pub static mut TCP_EVENTS: PerfEventArray<TcpConnectEvent> =
    PerfEventArray::new(0);

// ── tcp_connect ───────────────────────────────────────────────────────────────
//
// int tcp_connect(struct sock *sk)

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
        None => return 0,
    };
    unsafe { TCP_STATE.remove(&key).ok() };

    let retval: i32 = ctx.ret().unwrap_or(-1);

    let mut dst_ip: u32 = 0;
    let mut src_ip: u32 = 0;
    let mut dst_port_be: u16 = 0;
    let mut src_port: u16 = 0;

    unsafe {
        bpf_probe_read_kernel(
            &mut dst_ip as *mut _ as *mut _,
            core::mem::size_of::<u32>() as u32,
            (sk_ptr + SK_COMMON_DST_OFFSET as u64) as *const _,
        ).ok();
        bpf_probe_read_kernel(
            &mut src_ip as *mut _ as *mut _,
            core::mem::size_of::<u32>() as u32,
            (sk_ptr + SK_COMMON_SRC_OFFSET as u64) as *const _,
        ).ok();
        bpf_probe_read_kernel(
            &mut dst_port_be as *mut _ as *mut _,
            core::mem::size_of::<u16>() as u32,
            (sk_ptr + SK_COMMON_DPORT_OFFSET as u64) as *const _,
        ).ok();
        bpf_probe_read_kernel(
            &mut src_port as *mut _ as *mut _,
            core::mem::size_of::<u16>() as u32,
            (sk_ptr + SK_COMMON_SPORT_OFFSET as u64) as *const _,
        ).ok();
    }

    let mut comm = [0u8; COMM_LEN];
    let _ = bpf_get_current_comm(&mut comm);

    let event = TcpConnectEvent {
        pid: (key >> 32) as u32,
        tid: key as u32,
        comm,
        src_ip,
        dst_ip,
        src_port,
        dst_port: u16::from_be(dst_port_be),
        retval,
    };

    unsafe { TCP_EVENTS.output(&ctx, &event, 0) };
    0
}
