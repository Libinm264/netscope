//! BPF kernel programs for NetScope eBPF agent.
//!
//! Compiled to bpfel-unknown-none by `cargo xtask build-ebpf`.
//! The resulting ELF is embedded into the userspace loader via `include_bytes!`.

#![no_std]
#![no_main]

mod ssl;
mod tcp;

// Required by the BPF verifier — panic just loops forever.
#[panic_handler]
fn panic(_info: &core::panic::PanicInfo) -> ! {
    loop {}
}
