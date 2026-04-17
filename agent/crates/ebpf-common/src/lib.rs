//! Shared types between the BPF kernel programs and the userspace aya loader.
//!
//! Must stay `no_std` + `no alloc` so the BPF target can compile it.

#![cfg_attr(feature = "bpf", no_std)]

// ── Constants ─────────────────────────────────────────────────────────────────

/// Maximum bytes of plaintext captured per SSL_read / SSL_write call.
pub const SSL_DATA_MAX: usize = 4096;
/// Maximum length of a process name (comm) from the kernel.
pub const COMM_LEN: usize = 16;

// ── SSL / TLS events ──────────────────────────────────────────────────────────

/// Direction of an SSL data event relative to the process that owns it.
#[repr(u32)]
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum SslDirection {
    Read  = 0,   // data arriving from the network (SSL_read return)
    Write = 1,   // data leaving the process (SSL_write entry)
}

/// Perf event emitted once per SSL_read / SSL_write call with plaintext payload.
#[repr(C)]
#[derive(Clone, Copy)]
pub struct SslEvent {
    /// PID of the process that made the call.
    pub pid: u32,
    /// Thread ID.
    pub tid: u32,
    /// Process name, NUL-padded.
    pub comm: [u8; COMM_LEN],
    /// Read or Write.
    pub direction: SslDirection,
    /// Source IPv4 address (network byte order).
    pub src_ip: u32,
    /// Destination IPv4 address (network byte order).
    pub dst_ip: u32,
    pub src_port: u16,
    pub dst_port: u16,
    /// Actual number of bytes captured (≤ SSL_DATA_MAX).
    pub data_len: u32,
    /// Plaintext bytes. Only the first `data_len` bytes are valid.
    pub data: [u8; SSL_DATA_MAX],
}

#[cfg(feature = "userspace")]
unsafe impl aya::Pod for SslEvent {}

// ── TCP connection events ─────────────────────────────────────────────────────

/// Emitted on every outbound TCP connection attempt (kprobe tcp_connect).
#[repr(C)]
#[derive(Clone, Copy, Debug)]
pub struct TcpConnectEvent {
    pub pid: u32,
    pub tid: u32,
    pub comm: [u8; COMM_LEN],
    /// Source IPv4 (may be 0 before the kernel assigns a local port).
    pub src_ip: u32,
    /// Destination IPv4.
    pub dst_ip: u32,
    pub src_port: u16,
    pub dst_port: u16,
    /// Return value of tcp_connect (0 = success, negative = errno).
    pub retval: i32,
}

#[cfg(feature = "userspace")]
unsafe impl aya::Pod for TcpConnectEvent {}

// ── Scratch state stored in BPF maps between entry and return probes ──────────

/// Key for the uprobe-state map: (pid << 32 | tid).
pub type ProbeStateKey = u64;

/// Value stored on SSL_write/SSL_read entry to retrieve on return.
#[repr(C)]
#[derive(Clone, Copy)]
pub struct SslProbeState {
    /// Pointer to the plaintext buffer argument (cast to u64 for portability).
    pub buf_ptr: u64,
    pub fd: u32,
    pub _pad: u32,
}
