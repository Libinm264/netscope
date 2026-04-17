//! NetScope xtask — build helper for BPF programs.
//!
//! Usage (from the `agent/` directory):
//!   cargo xtask build-ebpf            # debug build
//!   cargo xtask build-ebpf --release  # release build
//!
//! The compiled BPF ELF is written to:
//!   agent/target/bpfel-unknown-none/{debug|release}/netscope-ebpf
//!
//! The ebpf-loader crate then embeds it with include_bytes!.

use std::{
    env,
    path::{Path, PathBuf},
    process::{Command, ExitStatus},
};

use anyhow::{bail, Context, Result};

fn main() -> Result<()> {
    let mut args = env::args().skip(1); // skip "xtask"
    match args.next().as_deref() {
        Some("build-ebpf") => {
            let release = args.any(|a| a == "--release");
            build_ebpf(release)
        }
        Some("help") | None => {
            println!("Usage: cargo xtask <command>");
            println!();
            println!("Commands:");
            println!("  build-ebpf [--release]   Compile BPF kernel programs");
            Ok(())
        }
        Some(cmd) => bail!("Unknown command: {}", cmd),
    }
}

fn build_ebpf(release: bool) -> Result<()> {
    // The agent workspace root is two levels above xtask/
    let workspace_root = workspace_root()?;
    let ebpf_dir = workspace_root.join("ebpf");

    println!("Building BPF programs in {}", ebpf_dir.display());

    // Ensure the nightly toolchain and bpf target are present
    run(
        Command::new("rustup")
            .args(["target", "add", "bpfel-unknown-none"])
            .current_dir(&ebpf_dir),
    )
    .context("Failed to add bpfel-unknown-none target")?;

    let mut cmd = Command::new("cargo");
    cmd.current_dir(&ebpf_dir)
        .env("CARGO_CFG_BPF", "1")
        .args(["+nightly", "build", "--target", "bpfel-unknown-none", "-Z", "build-std=core"]);

    if release {
        cmd.arg("--release");
    }

    run(&mut cmd).context("BPF build failed")?;

    let profile = if release { "release" } else { "debug" };
    let artifact = workspace_root
        .join("target")
        .join("bpfel-unknown-none")
        .join(profile)
        .join("netscope-ebpf");

    println!("BPF artifact: {}", artifact.display());
    Ok(())
}

fn workspace_root() -> Result<PathBuf> {
    // xtask lives at agent/xtask, so workspace root is agent/
    let manifest = env!("CARGO_MANIFEST_DIR");
    let p = Path::new(manifest)
        .parent()
        .context("Cannot determine workspace root")?
        .to_path_buf();
    Ok(p)
}

fn run(cmd: &mut Command) -> Result<ExitStatus> {
    let status = cmd.status().context("Failed to spawn process")?;
    if !status.success() {
        bail!("Command exited with {}", status);
    }
    Ok(status)
}
