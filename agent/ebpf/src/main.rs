//! BPF kernel programs for NetScope eBPF agent.
//!
//! Compiled to bpfel-unknown-none by `cargo xtask build-ebpf`.
//! The resulting ELF is embedded into the userspace loader via `include_bytes!`.

#![no_std]
#![no_main]
// aya-ebpf maps are declared as `static mut` and accessed exclusively inside
// `unsafe` blocks.  The BPF virtual machine guarantees single-CPU execution
// per probe invocation, so creating a shared reference to a mutable static
// here is safe.  Suppress the Rust 2024 lint for the whole eBPF crate.
#![allow(static_mut_refs)]

mod ssl;
mod tcp;

// Required by the BPF verifier — panic just loops forever.
#[panic_handler]
fn panic(_info: &core::panic::PanicInfo) -> ! {
    loop {}
}
