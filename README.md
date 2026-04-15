<div align="center">

# NetScope

**A unified, cross-platform network observability platform**

Real-time packet capture · Deep protocol inspection · Desktop GUI · Headless CLI agent

[![Build NetScope Desktop](https://github.com/Libinm264/netscope/actions/workflows/build-desktop.yml/badge.svg)](https://github.com/Libinm264/netscope/actions/workflows/build-desktop.yml)
![Rust](https://img.shields.io/badge/Rust-2021_edition-orange?logo=rust)
![Tauri](https://img.shields.io/badge/Tauri-2.x-blue?logo=tauri)
![React](https://img.shields.io/badge/React-18.3-61dafb?logo=react)
![License](https://img.shields.io/badge/license-MIT-green)

</div>

---

## Table of Contents

- [Overview](#overview)
- [Features](#features)
- [Architecture](#architecture)
  - [System Architecture](#system-architecture)
  - [Data Flow](#data-flow)
  - [Component Breakdown](#component-breakdown)
  - [Project Structure](#project-structure)
- [Technology Stack](#technology-stack)
- [Prerequisites](#prerequisites)
  - [macOS](#macos)
  - [Windows](#windows)
  - [Linux](#linux)
- [Installation](#installation)
  - [Option A — Pre-built Releases](#option-a--pre-built-releases)
  - [Option B — Build from Source](#option-b--build-from-source)
- [Running NetScope](#running-netscope)
  - [Desktop Application](#desktop-application)
  - [CLI Agent](#cli-agent)
- [Usage Guide](#usage-guide)
  - [Starting a Capture](#starting-a-capture)
  - [Filtering Traffic](#filtering-traffic)
  - [Saving and Loading Sessions](#saving-and-loading-sessions)
- [Development](#development)
  - [Dev Mode](#dev-mode)
  - [Project Layout for Developers](#project-layout-for-developers)
- [CI / CD](#ci--cd)
- [Roadmap](#roadmap)
- [Contributing](#contributing)
- [License](#license)

---

## Overview

NetScope is a network observability tool built for developers and network engineers. It captures live network traffic directly from your machine's interfaces and decodes it into human-readable flows — showing HTTP request/response pairs, DNS queries, TCP connections, and raw packet bytes — all inside a clean, Wireshark-inspired desktop UI.

Unlike heavyweight tools like Wireshark, NetScope is purpose-built for fast inspection of application-layer traffic. It ships as both:

- **A desktop app** (macOS, Windows, Linux) with a 3-pane UI and session persistence.
- **A headless CLI agent** for servers, containers, or CI environments.

---

## Features

| Feature | Description |
|---|---|
| **Live packet capture** | Captures traffic via libpcap/Npcap with BPF filter support |
| **HTTP/1.1 decoding** | Full request/response pairing with headers, method, status, body preview |
| **DNS decoding** | Query/response pairing with record types, answers, RCODE |
| **TCP reassembly** | Handles out-of-order segments and stream fragmentation |
| **3-pane UI** | Packet list → detail tree → raw hex dump |
| **Live filtering** | Filter by IP, port, protocol, or keyword in real time |
| **Session persistence** | Save/load capture sessions as `.nscope` files (SQLite) |
| **BPF expressions** | Use standard tcpdump filter syntax (e.g. `tcp port 443`) |
| **Privilege helper** | Onboarding modal detects and guides through privilege setup |
| **Cross-platform** | Ships as `.dmg`, `.msi`, and `.AppImage` |
| **Headless CLI** | `netscope-agent` runs without a GUI for server environments |

---

## Architecture

### System Architecture

```
┌──────────────────────────────────────────────────────────────────┐
│                        NetScope Desktop                          │
│                                                                  │
│  ┌─────────────────────────────┐   ┌──────────────────────────┐ │
│  │     React Frontend (UI)     │   │   Tauri Backend (Rust)   │ │
│  │                             │   │                          │ │
│  │  PacketListPane (virtual)   │◄──│  Tauri Commands (IPC)    │ │
│  │  PacketDetailPane           │   │  AppState (Arc<Mutex>)   │ │
│  │  HexDumpPane                │   │  SQLite (sqlx)           │ │
│  │  FilterBar                  │   │  Tauri Events            │ │
│  │  InterfaceSelector          │   │                          │ │
│  │  StatusBar                  │   └──────────┬───────────────┘ │
│  │                             │              │ spawns thread    │
│  │  Zustand Store              │              ▼                  │
│  └─────────────────────────────┘   ┌──────────────────────────┐ │
│                                    │   Rust Agent (crates)    │ │
│                                    │                          │ │
│                                    │  capture::start_capture  │ │
│                                    │  tcp_stream::Reassembler │ │
│                                    │  parser::SessionManager  │ │
│                                    │  http::parse_request     │ │
│                                    │  dns::parse_dns          │ │
│                                    └──────────┬───────────────┘ │
└─────────────────────────────────────────────────────────────────┘
                                                │
                                                ▼
                                    ┌───────────────────────┐
                                    │  libpcap / Npcap      │
                                    │  (OS packet capture)  │
                                    └───────────────────────┘
                                                │
                                                ▼
                                    ┌───────────────────────┐
                                    │    Network Interface  │
                                    │  (eth0 / en0 / Wi-Fi) │
                                    └───────────────────────┘
```

The desktop app embeds the Rust agent crates directly — no external process needed. The same crates also power the standalone `netscope-agent` CLI.

---

### Data Flow

```
Network packets
      │
      ▼
libpcap raw frame
      │
      ▼
etherparse — Ethernet/IP/TCP/UDP header slicing
      │
      ├─── UDP? ──► dns::parse_dns() ──────────────────────────► DnsFlow
      │
      └─── TCP? ──► TcpReassembler ──► reassembled byte stream
                         │
                         ▼
                  SessionManager
                         │
                         ├── looks_like_http? ──► http::parse_request/response ──► HttpFlow
                         │
                         └── raw TCP ──────────────────────────────────────────► TcpFlow
                                              │
                                              ▼
                                    proto::Flow (shared type)
                                              │
                                    ┌─────────┴─────────┐
                                    │                   │
                               Tauri event          stdout (CLI)
                              "flow" → JSON        colored output
                                    │
                                    ▼
                             Zustand store
                                    │
                                    ▼
                             React components
```

---

### Component Breakdown

#### Rust Agent Crates (`agent/crates/`)

| Crate | Responsibility |
|---|---|
| `capture` | Wraps libpcap: interface enumeration, packet capture loop, raw `PacketEvent` emission |
| `parser` | Protocol parsers: `http.rs` (HTTP/1.1 via httparse), `dns.rs` (DNS wire format), `session.rs` (flow assembly and TCP reassembly orchestration) |
| `proto` | Shared Rust types: `Flow`, `HttpFlow`, `DnsFlow`, `PacketEvent` — the lingua franca between all layers |
| `config` | Shared configuration structs used by both the agent CLI and the Tauri backend |

#### TCP Reassembler (`capture/src/tcp_stream.rs`)

Tracks active TCP connections keyed by `(src_ip, dst_ip, src_port, dst_port)`. Buffers out-of-order segments in a `pending: HashMap<u32, Vec<u8>>` and flushes them in-sequence. Handles `SYN`, `FIN`, and `RST` state transitions correctly.

#### Protocol Parsers (`parser/src/`)

- **HTTP** — Uses `httparse` to parse request lines and response status lines. Extracts all headers, detects `Content-Length` / `Transfer-Encoding` to determine body boundaries, and stores a 512-byte body preview. Applies heuristics to identify HTTP streams (ports 80, 8080, 3000, 5000 and first-byte content sniffing).
- **DNS** — Parses DNS wire format manually. Reads the question section (QNAME, QTYPE), iterates resource record answers (A, AAAA, CNAME, MX, TXT), and extracts RCODE from flags.

#### Tauri Backend (`desktop/src-tauri/src/`)

| File | Responsibility |
|---|---|
| `commands.rs` | All `#[tauri::command]` handlers: start/stop capture, list interfaces, privilege check, session save/load, flow query |
| `state.rs` | `AppState` with capture thread handle, flow ring-buffer, and running flag |
| `dto.rs` | `FlowDto`, `HttpFlowDto`, `DnsFlowDto` — serde-serializable JSON types sent to the frontend |
| `db.rs` | SQLite schema (`sessions`, `flows` tables), session serialization, load/replay of stored flows |

#### React Frontend (`desktop/src/`)

| Component | Responsibility |
|---|---|
| `PacketListPane` | Virtualised flow table ([@tanstack/react-virtual](https://tanstack.com/virtual)) — handles 100k+ rows without jank; auto-scrolls during live capture, pauses on manual scroll-up |
| `PacketDetailPane` | Expandable field tree for the selected flow: HTTP headers, DNS answers, timing info |
| `HexDumpPane` | Raw bytes rendered as 16-byte hex rows with ASCII side-panel |
| `FilterBar` | Real-time text filter with protocol shortcuts (`http`, `dns`, `errors`) |
| `InterfaceSelector` | Dropdown populated at startup from the `list_interfaces` Tauri command |
| `StatusBar` | Live packet count, capture state indicator, elapsed time |
| `PrivilegeModal` | Shown when `check_privileges` returns false; provides platform-specific fix instructions |

---

### Project Structure

```
netscope/
├── .github/
│   └── workflows/
│       └── build-desktop.yml     # CI: matrix build for macOS / Windows / Linux
│
├── proto/
│   └── netscope.proto            # Shared Protobuf schema (Flow, HttpFlow, DnsFlow)
│
├── agent/                        # Rust CLI agent (standalone binary)
│   ├── Cargo.toml                # Workspace root + binary definition
│   ├── src/
│   │   └── main.rs               # CLI entrypoint (clap subcommands)
│   └── crates/
│       ├── capture/              # libpcap wrapper + TCP reassembler
│       │   └── src/
│       │       ├── lib.rs
│       │       ├── interface.rs
│       │       └── tcp_stream.rs
│       ├── parser/               # Protocol decoders
│       │   └── src/
│       │       ├── lib.rs
│       │       ├── http.rs
│       │       ├── dns.rs
│       │       └── session.rs
│       ├── proto/                # Shared types (Flow, PacketEvent, …)
│       │   └── src/lib.rs
│       └── config/               # Shared config structs
│           └── src/lib.rs
│
└── desktop/                      # Tauri desktop application
    ├── package.json
    ├── vite.config.ts
    ├── tailwind.config.js
    ├── src/                      # React frontend
    │   ├── App.tsx
    │   ├── types/flow.ts
    │   ├── store/captureStore.ts
    │   ├── lib/utils.ts
    │   └── components/
    │       ├── PacketListPane.tsx
    │       ├── PacketDetailPane.tsx
    │       ├── HexDumpPane.tsx
    │       ├── FilterBar.tsx
    │       ├── InterfaceSelector.tsx
    │       ├── StatusBar.tsx
    │       ├── PrivilegeModal.tsx
    │       └── ui/               # shadcn-style UI primitives
    └── src-tauri/                # Tauri / Rust backend
        ├── Cargo.toml
        ├── tauri.conf.json
        ├── build.rs
        └── src/
            ├── lib.rs
            ├── main.rs
            ├── commands.rs
            ├── state.rs
            ├── dto.rs
            └── db.rs
```

---

## Technology Stack

### Agent / Backend (Rust)

| Library | Version | Purpose |
|---|---|---|
| [tokio](https://tokio.rs) | 1.x | Async runtime |
| [pcap](https://crates.io/crates/pcap) | 2.x | libpcap / Npcap bindings |
| [etherparse](https://crates.io/crates/etherparse) | 0.15 | Ethernet / IP / TCP / UDP header parsing |
| [httparse](https://crates.io/crates/httparse) | 1.x | HTTP/1.1 request & response parsing |
| [clap](https://crates.io/crates/clap) | 4.x | CLI argument parsing |
| [serde](https://serde.rs) + serde_json | 1.x | Serialization |
| [chrono](https://crates.io/crates/chrono) | 0.4 | Timestamps |
| [tracing](https://crates.io/crates/tracing) | 0.1 | Structured logging |

### Desktop Backend (Tauri)

| Library | Version | Purpose |
|---|---|---|
| [tauri](https://tauri.app) | 2.x | Desktop app framework (Rust core) |
| [tauri-plugin-dialog](https://crates.io/crates/tauri-plugin-dialog) | 2.x | Native file open/save dialogs |
| [sqlx](https://crates.io/crates/sqlx) | 0.7 | Async SQLite (session persistence) |

### Frontend (React + TypeScript)

| Library | Version | Purpose |
|---|---|---|
| [React](https://react.dev) | 18.3 | UI framework |
| [TypeScript](https://www.typescriptlang.org) | 5.6 | Type safety |
| [Zustand](https://github.com/pmndrs/zustand) | 5.x | Lightweight state management |
| [@tanstack/react-virtual](https://tanstack.com/virtual) | 3.x | Virtualised list (100k+ rows) |
| [Tailwind CSS](https://tailwindcss.com) | 3.4 | Utility-first styling |
| [Lucide React](https://lucide.dev) | 0.468 | Icon library |
| [Vite](https://vitejs.dev) | 6.x | Build tool and dev server |

---

## Prerequisites

### macOS

| Requirement | Version | How to install |
|---|---|---|
| Xcode Command Line Tools | Latest | `xcode-select --install` |
| Rust toolchain | 1.70+ | `curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs \| sh` |
| Node.js | 18 LTS+ | [nodejs.org](https://nodejs.org) or `brew install node` |
| Homebrew _(optional)_ | Latest | [brew.sh](https://brew.sh) |

> **libpcap** ships with macOS via Xcode — no extra install needed.

> **Permissions:** macOS may require you to grant the app Local Network access.  
> Go to **System Settings → Privacy & Security → Local Network** and enable NetScope if prompted.

---

### Windows

| Requirement | Version | How to install |
|---|---|---|
| Visual Studio Build Tools | 2019 / 2022 | [visualstudio.microsoft.com](https://visualstudio.microsoft.com/visual-cpp-build-tools/) — select **"Desktop development with C++"** workload |
| Rust toolchain | 1.70+ | [rustup.rs](https://rustup.rs) — choose the `x86_64-pc-windows-msvc` target |
| Node.js | 18 LTS+ | [nodejs.org](https://nodejs.org) |
| **Npcap** | 1.75+ | [npcap.com](https://npcap.com) — installer must have **"WinPcap API-compatible mode"** checked |
| Npcap SDK | 1.13 | [npcap-sdk-1.13.zip](https://npcap.com/dist/npcap-sdk-1.13.zip) — extract to `C:\npcap-sdk` |
| WebView2 runtime | Latest | Pre-installed on Windows 11; download from [Microsoft](https://developer.microsoft.com/en-us/microsoft-edge/webview2/) if missing |

**Set the Npcap SDK library path before building (run once in PowerShell):**

```powershell
[System.Environment]::SetEnvironmentVariable("LIB", "C:\npcap-sdk\Lib\x64", "User")
```

> **Privileges:** Packet capture on Windows requires Administrator rights.  
> Right-click NetScope and choose **"Run as administrator"**, or open your terminal as Administrator before running `netscope-agent`.

---

### Linux

Choose the commands for your distribution:

**Debian / Ubuntu / Linux Mint:**

```bash
sudo apt-get update
sudo apt-get install -y \
    build-essential \
    libpcap-dev \
    libgtk-3-dev \
    libwebkit2gtk-4.1-dev \
    libayatana-appindicator3-dev \
    librsvg2-dev \
    patchelf \
    curl
```

**Fedora / RHEL / CentOS:**

```bash
sudo dnf install -y \
    gcc gcc-c++ make \
    libpcap-devel \
    gtk3-devel \
    webkit2gtk4.1-devel \
    libayatana-appindicator-gtk3-devel \
    librsvg2-devel \
    patchelf \
    curl
```

**Arch Linux / Manjaro:**

```bash
sudo pacman -S --needed \
    base-devel \
    libpcap \
    gtk3 \
    webkit2gtk-4.1 \
    libayatana-appindicator \
    librsvg \
    patchelf \
    curl
```

**Then install Rust and Node.js on any distro:**

```bash
# Rust
curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh
source "$HOME/.cargo/env"

# Node.js via nvm (recommended)
curl -o- https://raw.githubusercontent.com/nvm-sh/nvm/v0.39.7/install.sh | bash
source ~/.bashrc
nvm install 20
nvm use 20
```

**Granting packet capture privileges on Linux (pick one option):**

```bash
# Option 1 — Grant CAP_NET_RAW to the binary (no root required at runtime)
sudo setcap cap_net_raw+ep /path/to/netscope-agent

# Option 2 — Run with sudo
sudo netscope-agent capture --interface eth0

# Option 3 — Add your user to the pcap group (distro-dependent)
sudo groupadd pcap
sudo usermod -aG pcap $USER
# Log out and back in for the group change to take effect
```

---

## Installation

### Option A — Pre-built Releases

Download the latest installer for your platform from the [**Releases page**](https://github.com/Libinm264/netscope/releases):

| Platform | File | Notes |
|---|---|---|
| macOS (Apple Silicon) | `NetScope_aarch64.dmg` | Requires macOS 11+ |
| macOS (Intel) | `NetScope_x86_64.dmg` | Requires macOS 10.15+ |
| Windows | `NetScope_x64_en-US.msi` | Requires Windows 10+ with WebView2 |
| Linux (portable) | `netscope_amd64.AppImage` | No install needed — just make executable and run |
| Linux (Debian) | `netscope_amd64.deb` | `sudo dpkg -i netscope_amd64.deb` |

**macOS first-launch (Gatekeeper):**

If macOS says the app is from an unidentified developer, run:

```bash
xattr -cr /Applications/NetScope.app
```

Then open it normally from Finder.

**Linux AppImage:**

```bash
chmod +x NetScope_*.AppImage
./NetScope_*.AppImage
```

---

### Option B — Build from Source

#### 1. Clone the repository

```bash
git clone https://github.com/Libinm264/netscope.git
cd netscope
```

#### 2. Build the CLI agent

```bash
cd agent
cargo build --release
# Binary output: agent/target/release/netscope-agent
```

> **Windows only:** Ensure the `LIB` environment variable is set to the Npcap SDK before running this command (see [Windows prerequisites](#windows)).

#### 3. Build the desktop application

```bash
cd desktop
npm install
npm run tauri build
```

Bundled output locations:

| Platform | Output path |
|---|---|
| macOS | `desktop/src-tauri/target/release/bundle/dmg/` |
| Windows | `desktop/src-tauri/target/release/bundle/msi/` |
| Linux | `desktop/src-tauri/target/release/bundle/appimage/` and `deb/` |

> **First-build note:** Rust compiles the full dependency tree on the first run. This typically takes 5–10 minutes. Subsequent incremental builds are much faster.

---

## Running NetScope

### Desktop Application

Launch the installed app from:
- **macOS:** Applications folder or Spotlight (`Cmd+Space` → "NetScope")
- **Windows:** Start Menu or desktop shortcut
- **Linux:** Application launcher or `./NetScope_*.AppImage`

On first launch, NetScope checks for sufficient privileges to open a raw socket. If privileges are missing, the **Privilege Helper** dialog appears with platform-specific instructions.

### CLI Agent

```bash
# List available network interfaces
netscope-agent list-interfaces

# Capture all traffic on en0 (macOS Wi-Fi)
netscope-agent capture --interface en0

# Capture with a BPF filter expression
netscope-agent capture --interface eth0 --filter "tcp port 80 or udp port 53"

# Save decoded flows to a JSONL file
netscope-agent capture --interface en0 --output flows.jsonl

# Stream flows to a NetScope Hub (Phase 3)
netscope-agent capture --interface en0 \
    --hub-url https://hub.example.com \
    --api-key YOUR_API_KEY
```

**Environment variables:**

| Variable | Default | Description |
|---|---|---|
| `RUST_LOG` | `info` | Log verbosity: `error`, `warn`, `info`, `debug`, `trace` |
| `NETSCOPE_INTERFACE` | — | Default capture interface (overridden by `--interface`) |

---

## Usage Guide

### Starting a Capture

1. Open the NetScope desktop app.
2. Select a network interface from the **Interface** dropdown in the toolbar.
3. _(Optional)_ Enter a BPF filter expression in the filter field — e.g. `host 8.8.8.8` or `tcp port 443`.
4. Click **Start** (▶) to begin capturing.
5. Decoded flows appear in real time. Click any row to inspect it in the detail and hex panes.

### Filtering Traffic

The **Filter** bar accepts plain text and is applied instantly without interrupting the capture:

| Input | Effect |
|---|---|
| `http` | Show only HTTP flows |
| `dns` | Show only DNS flows |
| `errors` | Show flows with HTTP error status codes (4xx / 5xx) |
| `192.168.1.1` | Match any flow with that source or destination IP |
| `443` | Match any flow with that source or destination port |
| `POST /api` | Match any text appearing in the info column |

### Saving and Loading Sessions

- **Save:** `File → Save Session` (or `Cmd/Ctrl+S`) — writes all captured flows to a `.nscope` file.
- **Load:** `File → Open Session` — replays a previously saved `.nscope` file back into the UI.

`.nscope` files are standard SQLite databases. You can inspect them with any SQLite browser (e.g. [DB Browser for SQLite](https://sqlitebrowser.org)).

---

## Development

### Dev Mode

Run the desktop app with hot-reload:

```bash
cd desktop
npm install
npm run tauri dev
```

This starts the Vite dev server on `http://localhost:1420` and the Tauri window pointing at it. React component changes hot-reload instantly; Rust backend changes require a restart.

**Useful commands:**

```bash
# Type-check the frontend without building
cd desktop && npx tsc --noEmit

# Run all Rust unit tests
cd agent && cargo test

# Check Rust code without producing binaries
cd agent && cargo check

# Format Rust code
cd agent && cargo fmt

# Lint Rust code
cd agent && cargo clippy -- -D warnings
```

### Project Layout for Developers

```
agent/crates/capture/src/
├── lib.rs          ← Modify CaptureError types or the start_capture() loop
├── interface.rs    ← Add interface metadata (speed, type, hardware address)
└── tcp_stream.rs   ← TCP reassembler — extend for TLS fingerprinting or QUIC

agent/crates/parser/src/
├── http.rs         ← Extend for HTTP/2, WebSocket upgrade detection
├── dns.rs          ← Add DNS record types: SRV, NAPTR, DNSKEY, DS
└── session.rs      ← Wire in new protocol parsers here

desktop/src/components/
├── PacketListPane.tsx   ← Add new columns to the flow table
├── PacketDetailPane.tsx ← Add detail rendering for new protocol types
└── HexDumpPane.tsx      ← Add byte-range highlighting for field offsets

desktop/src-tauri/src/
├── commands.rs     ← Add new Tauri IPC commands
├── dto.rs          ← Add new JSON-serializable types for the frontend
└── db.rs           ← Add SQLite migrations / new tables
```

**Adding a new protocol decoder — step by step:**

1. Create `agent/crates/parser/src/myproto.rs` with a `parse_myproto(buf: &[u8]) -> Option<MyProtoFlow>` function.
2. Add a `MyProtoFlow` struct to `agent/crates/proto/src/lib.rs`.
3. Add a variant to the `Flow` enum and wire the parser into `session.rs`'s `SessionManager::handle_tcp_data()`.
4. Add a `MyProtoFlowDto` to `desktop/src-tauri/src/dto.rs` and update `flow_to_dto()`.
5. Render it in `PacketDetailPane.tsx` by matching on the new `flowType`.

---

## CI / CD

GitHub Actions automatically builds and publishes release bundles when a version tag is pushed:

```bash
git tag v0.2.0
git push origin v0.2.0
```

The workflow (`.github/workflows/build-desktop.yml`) runs on four runners in parallel:

| Runner | Target triple |
|---|---|
| `macos-latest` | `aarch64-apple-darwin` (Apple Silicon) |
| `macos-latest` | `x86_64-apple-darwin` (Intel Mac) |
| `windows-latest` | `x86_64-pc-windows-msvc` |
| `ubuntu-22.04` | `x86_64-unknown-linux-gnu` |

All four bundles are uploaded as GitHub Release assets automatically via `tauri-apps/tauri-action`.

**Rust build caching** is provided by `swatinem/rust-cache` (keyed on `Cargo.lock` + target), keeping CI build times under 5 minutes on cache hits.

---

## Roadmap

- [x] **Phase 1** — Rust CLI agent: libpcap capture, HTTP/1.1 parser, DNS decoder, TCP reassembler
- [x] **Phase 2** — Tauri desktop GUI: virtualised packet list, hex dump, detail tree, session persistence, GitHub Actions CI
- [ ] **Phase 3** — SaaS Hub: Go/Fiber REST API, Kafka flow ingestion, ClickHouse time-series analytics, Next.js 14 dashboard, Auth0 multi-tenant authentication
- [ ] **Phase 4** — TLS/HTTPS decoding via eBPF uprobes (Linux) and mitmproxy integration
- [ ] **Phase 5** — Kubernetes/container network visibility using eBPF (cilium/ebpf)
- [ ] **Phase 6** — AI-assisted anomaly detection and natural-language flow summarization

---

## Contributing

Contributions are welcome!

1. Fork the repo and create a feature branch: `git checkout -b feature/my-feature`
2. Make your changes with tests where applicable.
3. Run `cargo fmt && cargo clippy -- -D warnings` and fix any warnings.
4. Open a Pull Request with a clear description of what changed and why.

For larger features or protocol additions, please open an issue first to discuss the approach.

---

## License

MIT License — see [LICENSE](LICENSE) for details.

---

<div align="center">
  Built with Rust and React
</div>
