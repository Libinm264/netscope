<div align="center">

# NetScope

**A unified, cross-platform network observability platform**

Real-time packet capture · Deep protocol inspection · GeoIP & threat intelligence · SaaS hub · eBPF agent

[![Build NetScope Desktop](https://github.com/Libinm264/netscope/actions/workflows/build-desktop.yml/badge.svg)](https://github.com/Libinm264/netscope/actions/workflows/build-desktop.yml)
![Rust](https://img.shields.io/badge/Rust-2021_edition-orange?logo=rust)
![Tauri](https://img.shields.io/badge/Tauri-2.x-blue?logo=tauri)
![React](https://img.shields.io/badge/React-18.3-61dafb?logo=react)
![Go](https://img.shields.io/badge/Go-1.22-00add8?logo=go)
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
- [Installation](#installation)
- [Running NetScope](#running-netscope)
  - [Desktop Application](#desktop-application)
  - [CLI Agent](#cli-agent)
  - [Hub API Server](#hub-api-server)
- [Usage Guide](#usage-guide)
  - [Starting a Capture](#starting-a-capture)
  - [Filtering Traffic](#filtering-traffic)
  - [GeoIP Enrichment](#geoip-enrichment)
  - [Threat Intelligence](#threat-intelligence)
  - [Hub Connect Mode](#hub-connect-mode)
  - [TLS Certificate Fleet](#tls-certificate-fleet)
  - [HTTP Analytics](#http-analytics)
  - [Service Dependency Map](#service-dependency-map)
  - [Saving and Loading Sessions](#saving-and-loading-sessions)
- [Development](#development)
- [CI / CD](#ci--cd)
- [Roadmap](#roadmap)
- [Contributing](#contributing)
- [License](#license)

---

## Overview

NetScope is a full-stack network observability platform built for developers, security engineers, and network operators. It captures live traffic, decodes it into human-readable flows with deep protocol inspection, enriches every connection with geographic and threat data, and streams everything to a centralised hub for fleet-wide visibility.

It ships as three complementary pieces:

| Component | What it does |
|---|---|
| **Desktop app** (macOS / Windows / Linux) | Wireshark-inspired 3-pane UI with live capture, GeoIP flags, threat badges, certificate audit, analytics, and service map |
| **CLI agent** (`netscope-agent`) | Headless capture for servers, containers, and CI environments; streams flows to the hub |
| **Hub** (Go API + Next.js dashboard) | Multi-tenant SaaS backend — Kafka ingestion, ClickHouse analytics, real-time SSE streaming, alerting, compliance, RBAC |

---

## Features

### Capture & Decoding

| Feature | Description |
|---|---|
| **Live packet capture** | libpcap / Npcap with BPF filter support; promiscuous mode toggle |
| **HTTP/1.1 decoding** | Full request/response pairing: method, path, host, status, headers, body preview, latency |
| **HTTP/2 + gRPC decoding** | Binary frame walking, HPACK header decompression, stream pairing, gRPC service/method/status extraction |
| **DNS decoding** | Query/response pairing with record types (A, AAAA, CNAME, MX, TXT, PTR), RCODE, TTLs |
| **TLS handshake inspection** | ClientHello SNI, cipher suites, negotiated version, weak cipher detection, certificate CN/SANs/expiry, alert decoding |
| **TCP reassembly** | Out-of-order segment handling, retransmission counting, stream fragmentation |
| **ICMP decoding** | Echo request/reply with RTT, type/code human-readable strings |
| **ARP decoding** | Who-has / is-at with sender and target IP/MAC |
| **eBPF capture** (Linux) | Kernel-side eBPF program for high-throughput capture without root; complements libpcap path |

### Desktop UI

| Feature | Description |
|---|---|
| **3-pane layout** | Packet list → protocol detail tree → raw hex dump; all panes resizable |
| **Virtualised packet list** | Handles 100k+ rows without jank (TanStack Virtual) |
| **GeoIP enrichment** | MaxMind GeoLite2-City + ASN; country flag emoji + ASN sub-line on every row; geo fields in detail tree |
| **Threat score badges** | Offline CIDR blocklist + port heuristics; color-coded ⚠ HIGH / MED / LOW badges; row highlighting |
| **Hub Connect Mode** | Query historical flows from a running hub instance directly inside the desktop UI |
| **TLS certificate sidebar** | Fleet-view of all certs seen in session — expired → critical → warning → valid; red badge count |
| **HTTP analytics pane** | Per-endpoint p50 / p95 / p99 latency bars, error rates, sortable table |
| **Service dependency map** | SVG force-directed graph of IP-to-IP flows; pan + zoom; hover tooltips |
| **Live filtering** | Protocol shortcuts (`http`, `dns`, `tls`, `errors`, `threats`, `hub`) plus free-text IP/port/keyword search |
| **BPF expressions** | Pre-filter at the kernel level (`tcp port 443`, `host 8.8.8.8`, etc.) |
| **Session persistence** | Save / load `.nscope` files (SQLite); sessions survive app restart |
| **Privilege helper** | Onboarding modal with platform-specific fix instructions when raw socket access is missing |
| **Error boundary** | React `ErrorBoundary` catches render crashes and shows a recoverable error screen |

### Hub (SaaS Backend)

| Feature | Description |
|---|---|
| **Flow ingestion** | Agents POST batches via `POST /api/v1/ingest`; Kafka path for high-throughput, direct ClickHouse write as fallback |
| **Real-time SSE stream** | `GET /api/v1/flows/stream` — fan-out to all connected dashboard clients |
| **ClickHouse analytics** | Time-series queries, per-endpoint stats, p50/p95/p99 latency, DNS NXDOMAIN rates |
| **GeoIP + threat enrichment** | Hub-side enrichment on ingest; data available to all downstream consumers |
| **Alert rules** | Configurable thresholds (flows/min, HTTP error rate, DNS NXDOMAIN rate, anomaly σ); webhook delivery with exponential back-off retry (1 s → 5 s → 30 s, 3 attempts) |
| **TLS certificate fleet** | Certs extracted from ingested TLS flows; expiry dashboard across entire agent fleet |
| **RBAC** | `admin` / `viewer` roles on API tokens; `RequireAdmin` middleware on write endpoints |
| **Audit log** | Every authenticated API call recorded to `audit_events` (ClickHouse): token ID, role, method, path, status, latency, client IP. Queryable via `GET /api/v1/audit` (admin only). 90-day TTL. |
| **Per-agent scoped tokens** | Enrolled agents receive a unique `viewer`-role token — never the global bootstrap admin key |
| **Compliance reporting** | PCI-DSS, HIPAA, CIS benchmark report generation |
| **OpenTelemetry export** | Forward flow metrics to any OTLP-compatible backend |
| **Kubernetes** | Helm chart + manifests for deploying hub + ClickHouse + Kafka in-cluster |

---

## Architecture

### System Architecture

```
┌─────────────────────────────── NetScope Desktop ──────────────────────────────────┐
│                                                                                    │
│  ┌──────────────────────────────┐       ┌───────────────────────────────────────┐ │
│  │    React Frontend (UI)       │       │       Tauri Backend (Rust)            │ │
│  │                              │       │                                       │ │
│  │  PacketListPane (TanStack)   │◄──────│  Tauri Commands (IPC)                 │ │
│  │  PacketDetailPane (tree)     │       │  SharedState  Arc<Mutex<AppState>>    │ │
│  │  HexDumpPane                 │       │  SQLite (sqlx)  →  .nscope files      │ │
│  │  AnalyticsPane               │       │  GeoIP reader   (maxminddb)           │ │
│  │  ServiceMapPane              │       │  Threat scorer  (ipnet CIDR)          │ │
│  │  CertSidebar                 │       │  Hub client     (reqwest)             │ │
│  │  HubConnectModal             │       │                                       │ │
│  │  Zustand store               │       └──────────────┬────────────────────────┘ │
│  └──────────────────────────────┘                      │ spawns OS thread         │
└───────────────────────────────────────────────────────────────────────────────────┘
                                                         │
                          ┌──────────────────────────────▼──────────────────────────┐
                          │              Rust Agent Crates                           │
                          │                                                          │
                          │   capture::start_capture  ──►  PacketEvent channel      │
                          │   tcp_stream::Reassembler ──►  byte stream              │
                          │   parser::SessionManager  ──►  proto::Flow              │
                          │   ebpf_loader             ──►  eBPF program (Linux)     │
                          └──────────────────────────────┬──────────────────────────┘
                                                         │
                              ┌──────────────────────────▼───────────────────────┐
                              │          libpcap / Npcap / eBPF                  │
                              └──────────────────────────┬───────────────────────┘
                                                         │
                              ┌──────────────────────────▼───────────────────────┐
                              │         Network Interface (eth0 / en0 / Wi-Fi)   │
                              └──────────────────────────────────────────────────┘

                                              ║  HTTPS  (agents push flows)
                          ┌───────────────────╨───────────────────────────────────┐
                          │                NetScope Hub                            │
                          │                                                        │
                          │  Go/Fiber API  ──►  Kafka  ──►  ClickHouse consumer  │
                          │                 ──►  ClickHouse  (direct write)       │
                          │                 ──►  SSE fan-out (dashboard clients)  │
                          │  Next.js 14 Dashboard  (hub/web/)                     │
                          │  Alerting evaluator  ──►  Webhooks                    │
                          └────────────────────────────────────────────────────────┘
```

### Data Flow

```
Raw network packets
        │
        ▼
libpcap / eBPF raw frame
        │
        ▼
etherparse — Ethernet / IP header slicing
        │
        ├── EtherType 0x0806 ──► try_parse_arp()  ──────────────────────► ArpFlow
        │
        ├── UDP port 53/5353 ──► Protocol::Dns  ──► dns::parse_dns()  ──► DnsFlow
        │
        ├── UDP (other)  ──────────────────────────────────────────────► UdpFlow
        │
        ├── ICMP  ───────────────────────────────────────────────────► IcmpFlow
        │
        └── TCP ──► TcpReassembler ──► reassembled byte stream
                          │
                          ▼
                   SessionManager
                          │
                          ├── TLS? ──► tls::parse_handshake() ──────────► TlsFlow
                          │
                          ├── HTTP/2? ──► http2::H2Session ──► HPACK ──► Http2Flow / gRPCFlow
                          │
                          ├── HTTP? ──► http::parse_request/response ──► HttpFlow
                          │
                          └── raw TCP ──────────────────────────────────► TcpFlow
                                           │
                                           ▼
                                 proto::Flow (shared type)
                                           │
                               ┌───────────┴───────────┐
                               │                       │
                        Tauri event "flow"         stdout (CLI)
                           → JSON                colored output
                               │
                               ▼
                    GeoIP enrichment (maxminddb)
                    Threat scoring  (CIDR + port heuristics)
                               │
                               ▼
                        Zustand store
                               │
                    ┌──────────┴──────────┐
                    │                     │
             React components       Hub ingest API
             (PacketListPane        (optional, via
              etc.)                  hub.rs client)
```

### Component Breakdown

#### Rust Agent Crates (`agent/crates/`)

| Crate | Responsibility |
|---|---|
| `capture` | libpcap wrapper: interface enumeration, packet capture loop, BPF filter application, raw `PacketEvent` emission; ARP / DNS protocol promotion |
| `parser` | Protocol parsers: `http.rs` (HTTP/1.1 via httparse), `dns.rs` (DNS wire format), `tls.rs` (TLS 1.2/1.3 handshake), `icmp.rs`, `arp.rs`, `session.rs` (flow assembly + TCP reassembly) |
| `proto` | Shared Rust types: `Flow`, `FlowPayload`, `HttpFlow`, `DnsFlow`, `TlsFlow`, `IcmpFlow`, `ArpFlow`, `PacketEvent` |
| `config` | Shared config structs (`AgentConfig`) used by both the CLI and Tauri backend |
| `ebpf-loader` | Loads and attaches eBPF programs for kernel-side capture (Linux only) |
| `ebpf-common` | Types shared between userspace and eBPF kernel code |

#### TCP Reassembler (`capture/src/tcp_stream.rs`)

Tracks active TCP connections keyed by `(src_ip, dst_ip, src_port, dst_port)`. Buffers out-of-order segments in a `pending: HashMap<u32, Vec<u8>>` and flushes them in-sequence. Handles `SYN`, `FIN`, and `RST` state transitions, and counts retransmissions and out-of-order segments per stream.

#### Protocol Parsers (`parser/src/`)

- **HTTP/1.1** — `httparse` for request/response parsing. Extracts all headers, detects `Content-Length` / `Transfer-Encoding` for body boundaries, stores a 512-byte body preview, measures latency by matching request/response pairs in the same stream.
- **HTTP/2** — Binary frame parser (`http2.rs`). Detects the 24-byte client connection preface, walks SETTINGS/DATA/HEADERS/CONTINUATION frames, decodes HPACK-compressed headers using the `hpack` crate, pairs client/server streams by stream ID, measures per-stream latency.
- **gRPC** — Detected when `content-type: application/grpc*` is present in HTTP/2 HEADERS frame. Extracts service and method from `:path` (`/package.Service/Method`) and `grpc-status` from response trailers.
- **DNS** — Manual wire-format parsing. Reads question section (QNAME, QTYPE), iterates resource records (A, AAAA, CNAME, MX, TXT, PTR), extracts RCODE from flags.
- **TLS** — Handshake record parsing: ClientHello (SNI, cipher suites, extensions), ServerHello (chosen cipher, negotiated version), Certificate (CN, SANs, expiry, issuer), Alert (level + description). Detects weak cipher suites (RC4, 3DES, NULL, EXPORT, MD5).
- **ICMP** — Type/code decoding with human-readable strings; echo request/reply RTT calculation.
- **ARP** — Ethernet ARP (HW type 1, proto 0x0800); extracts sender and target IP/MAC.

#### Tauri Backend (`desktop/src-tauri/src/`)

| File | Responsibility |
|---|---|
| `commands.rs` | All `#[tauri::command]` handlers: start/stop capture, list interfaces, privilege check, session save/load, GeoIP management, hub connection, flow query |
| `state.rs` | `SharedState` (Arc<Mutex<AppState>>): flows ring-buffer, capture status, stop channel, GeoIP reader, threat scorer, hub config |
| `dto.rs` | All serde-serializable JSON types sent to frontend: `FlowDto`, `GeoInfoDto`, `ThreatInfoDto`, all protocol sub-DTOs |
| `db.rs` | SQLite schema, session serialization, load/replay of stored flows |
| `geoip.rs` | MaxMind GeoLite2-City + ASN reader; skips private/RFC1918 IPs; auto-loads from `~/.netscope/` |
| `threat.rs` | Offline threat scorer: CIDR blocklist, port heuristics (C2, Tor, Telnet); returns `ThreatResult` with score + level + reasons |
| `hub.rs` | Hub API client (reqwest): `test_connection`, `query_flows` with filters; converts `HubFlowRecord` → `FlowDto` |

#### React Frontend (`desktop/src/`)

| Component | Responsibility |
|---|---|
| `PacketListPane` | Virtualised flow table (TanStack Virtual) — 100k+ rows; two-line rows with IP + geo/ASN sub-line; threat badges; hub badges; auto-scroll during capture |
| `PacketDetailPane` | Expandable protocol tree for the selected flow: HTTP headers, DNS answers, TLS handshake fields, ICMP, ARP, GeoIP, threat intelligence |
| `HexDumpPane` | Raw bytes as 16-byte hex rows with ASCII panel |
| `AnalyticsPane` | Per-endpoint p50/p95/p99 latency bars, error rate, sortable by count/latency/errors |
| `ServiceMapPane` | SVG force-directed graph (spring simulation); IP nodes sized by flow count; edges coloured by protocol; pan + zoom; positions cached across packets |
| `CertSidebar` | TLS cert fleet panel: all certs seen in session, sorted by expiry severity, count badge on toolbar |
| `GeoIpBanner` | Amber warning bar when GeoIP databases are absent; auto-detects `~/.netscope/` |
| `HubConnectModal` | Hub URL + API token form; test connection; protocol/IP/limit query filters; load hub flows |
| `ErrorBoundary` | React class component: catches render crashes and shows error + stack instead of a blank screen |
| `FilterBar` | Real-time filter with protocol shortcuts and free-text search |
| `InterfaceSelector` | Dropdown from `list_interfaces` Tauri command |
| `StatusBar` | Live flow count, capture state, elapsed time |
| `PrivilegeModal` | Platform-specific privilege fix instructions |

#### Hub API (`hub/api/`)

| Package | Responsibility |
|---|---|
| `handlers/flows.go` | `POST /ingest`, `GET /flows`, `GET /flows/stream` (SSE); Kafka or direct ClickHouse write |
| `handlers/analytics.go` | `GET /analytics/endpoints` — per-endpoint latency percentiles and error rates |
| `handlers/certs.go` | `GET /certs` — TLS cert fleet listing with expiry summary |
| `handlers/services.go` | `GET /services` — service dependency graph data |
| `handlers/alerts.go` | CRUD for alert rules; `GET /alert-events` |
| `handlers/compliance.go` | PCI-DSS, HIPAA, CIS report generation |
| `handlers/fleet.go` | Agent registration and heartbeat |
| `alerting/evaluator.go` | Background rule evaluator; metric computation; webhook firing |
| `middleware/auth.go` | `TokenAuth` — API key validation against ClickHouse `api_tokens`; stores `role` + `token_id` in context; `RequireAdmin` role gate |
| `middleware/audit.go` | `AuditLog` — fires after every authenticated handler; writes `audit_events` row asynchronously |
| `handlers/audit.go` | `GET /audit` — queryable audit log (admin only); filter by token, status, limit |
| `geoip/` | MaxMind GeoLite2 reader for hub-side enrichment |
| `threat/` | Threat scorer for hub-side enrichment on ingest |
| `kafka/` | Franz-go producer; `Publish` method with fallback to direct write |
| `clickhouse/` | ClickHouse client wrapper; async batch `Writer` |

---

### Project Structure

```
netscope/
├── .github/
│   └── workflows/
│       └── build-desktop.yml       # CI: matrix build macOS / Windows / Linux
│
├── agent/                          # Rust CLI agent (standalone binary)
│   ├── Cargo.toml
│   ├── src/main.rs                 # CLI entrypoint (clap subcommands)
│   └── crates/
│       ├── capture/src/
│       │   ├── lib.rs              # libpcap loop, DNS/ARP promotion
│       │   ├── interface.rs
│       │   └── tcp_stream.rs       # TCP reassembler + retransmission tracking
│       ├── parser/src/
│       │   ├── lib.rs
│       │   ├── http.rs             # HTTP/1.1 request+response pairing
│       │   ├── http2.rs            # HTTP/2 frame parser + HPACK + gRPC extraction
│       │   ├── dns.rs              # DNS wire-format decoder
│       │   ├── tls.rs              # TLS 1.2/1.3 handshake + cert parsing
│       │   ├── icmp.rs
│       │   ├── arp.rs
│       │   └── session.rs          # SessionManager: flow assembly + parser dispatch
│       ├── proto/src/lib.rs        # Shared Flow types
│       ├── config/src/lib.rs       # AgentConfig
│       ├── ebpf-common/            # Shared eBPF/userspace types
│       └── ebpf-loader/            # eBPF program loader (Linux)
│
├── agent/ebpf/                     # eBPF kernel programs (BPF bytecode)
│
├── desktop/                        # Tauri desktop application
│   ├── package.json
│   ├── vite.config.ts
│   ├── tailwind.config.js
│   ├── src/                        # React frontend
│   │   ├── App.tsx                 # Main layout, toolbar, pane splitter
│   │   ├── main.tsx                # Entry point + ErrorBoundary
│   │   ├── types/flow.ts           # TypeScript DTOs
│   │   ├── store/captureStore.ts   # Zustand store + cert inventory + endpoint stats
│   │   ├── lib/utils.ts            # cn(), rowBgColor(), formatBytes()
│   │   └── components/
│   │       ├── PacketListPane.tsx
│   │       ├── PacketDetailPane.tsx
│   │       ├── HexDumpPane.tsx
│   │       ├── AnalyticsPane.tsx
│   │       ├── ServiceMapPane.tsx
│   │       ├── CertSidebar.tsx
│   │       ├── GeoIpBanner.tsx
│   │       ├── HubConnectModal.tsx
│   │       ├── ErrorBoundary.tsx
│   │       ├── FilterBar.tsx
│   │       ├── InterfaceSelector.tsx
│   │       ├── StatusBar.tsx
│   │       ├── PrivilegeModal.tsx
│   │       └── ui/                 # shadcn-style primitives
│   └── src-tauri/
│       ├── Cargo.toml
│       ├── tauri.conf.json
│       └── src/
│           ├── lib.rs              # Tauri app setup + command registration
│           ├── main.rs
│           ├── commands.rs         # All Tauri IPC commands
│           ├── state.rs            # SharedState definition
│           ├── dto.rs              # Rust → TS JSON types
│           ├── db.rs               # SQLite session persistence
│           ├── geoip.rs            # MaxMind GeoLite2 reader
│           ├── threat.rs           # Offline threat scorer
│           └── hub.rs              # Hub API client
│
└── hub/
    ├── api/                        # Go/Fiber REST API
    │   ├── main.go
    │   ├── config/
    │   ├── handlers/               # HTTP handlers (flows, analytics, certs, alerts…)
    │   ├── alerting/               # Rule evaluator + webhook delivery
    │   ├── middleware/             # Auth (TokenAuth, RequireAdmin), rate limit, audit log
    │   ├── models/                 # Go structs: Flow, AlertRule, TlsCert…
    │   ├── clickhouse/             # ClickHouse client + async batch writer
    │   ├── kafka/                  # Franz-go producer
    │   ├── geoip/                  # Hub-side MaxMind reader
    │   ├── threat/                 # Hub-side threat scorer
    │   ├── metrics/                # Prometheus counters
    │   └── k8s/                    # Kubernetes manifests + Helm chart
    └── web/                        # Next.js 14 dashboard
        ├── app/                    # App Router pages
        ├── components/
        └── lib/
```

---

## Technology Stack

### Agent / Desktop Backend (Rust)

| Library | Version | Purpose |
|---|---|---|
| [tokio](https://tokio.rs) | 1.x | Async runtime |
| [pcap](https://crates.io/crates/pcap) | 2.x | libpcap / Npcap bindings |
| [etherparse](https://crates.io/crates/etherparse) | 0.15 | Ethernet / IP / TCP / UDP header parsing |
| [httparse](https://crates.io/crates/httparse) | 1.x | HTTP/1.1 request & response parsing |
| [hpack](https://crates.io/crates/hpack) | 0.3 | HPACK header decompression for HTTP/2 |
| [maxminddb](https://crates.io/crates/maxminddb) | 0.24 | MaxMind GeoLite2 binary database reader |
| [ipnet](https://crates.io/crates/ipnet) | 2.x | CIDR range matching for threat scoring |
| [reqwest](https://crates.io/crates/reqwest) | 0.12 | Async HTTP client for hub API |
| [dirs](https://crates.io/crates/dirs) | 5.x | Platform home directory resolution |
| [clap](https://crates.io/crates/clap) | 4.x | CLI argument parsing |
| [serde](https://serde.rs) + serde_json | 1.x | Serialization |
| [chrono](https://crates.io/crates/chrono) | 0.4 | Timestamps |
| [tracing](https://crates.io/crates/tracing) | 0.1 | Structured logging |
| [anyhow](https://crates.io/crates/anyhow) | 1.x | Error handling |
| [thiserror](https://crates.io/crates/thiserror) | 1.x | Custom error types |

### Desktop App Framework (Tauri)

| Library | Version | Purpose |
|---|---|---|
| [tauri](https://tauri.app) | 2.x | Desktop app framework (Rust core + WebView) |
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

### Hub (Go + ClickHouse + Kafka)

| Library / Service | Purpose |
|---|---|
| [Go](https://go.dev) 1.22 | API server language |
| [Fiber v2](https://gofiber.io) | HTTP framework |
| [ClickHouse](https://clickhouse.com) | Time-series storage for flows, certs, alert events |
| [clickhouse-go v2](https://github.com/ClickHouse/clickhouse-go) | ClickHouse Go client |
| [Apache Kafka](https://kafka.apache.org) | Flow ingestion queue (franz-go client) |
| [MaxMind GeoLite2](https://dev.maxmind.com) | Hub-side geo enrichment |
| [oschwald/geoip2-golang](https://github.com/oschwald/geoip2-golang) | Go GeoIP reader |
| [google/uuid](https://github.com/google/uuid) | UUID generation for alert events |
| [Next.js 14](https://nextjs.org) | Web dashboard (App Router) |

---

## Prerequisites

### macOS

| Requirement | Version | How to install |
|---|---|---|
| Xcode Command Line Tools | Latest | `xcode-select --install` |
| Rust toolchain | 1.75+ | `curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs \| sh` |
| Node.js | 18 LTS+ | [nodejs.org](https://nodejs.org) or `brew install node` |

> **libpcap** ships with macOS — no extra install needed.  
> **Permissions:** Go to **System Settings → Privacy & Security → Local Network** and enable NetScope if prompted.

---

### Windows

| Requirement | Version | How to install |
|---|---|---|
| Visual Studio Build Tools | 2019 / 2022 | [visualstudio.microsoft.com](https://visualstudio.microsoft.com/visual-cpp-build-tools/) — select **"Desktop development with C++"** |
| Rust toolchain | 1.75+ | [rustup.rs](https://rustup.rs) — choose `x86_64-pc-windows-msvc` |
| Node.js | 18 LTS+ | [nodejs.org](https://nodejs.org) |
| **Npcap** | 1.75+ | [npcap.com](https://npcap.com) — check **"WinPcap API-compatible mode"** |
| Npcap SDK | 1.13 | [npcap-sdk-1.13.zip](https://npcap.com/dist/npcap-sdk-1.13.zip) — extract to `C:\npcap-sdk` |
| WebView2 runtime | Latest | Pre-installed on Windows 11; [download from Microsoft](https://developer.microsoft.com/en-us/microsoft-edge/webview2/) if missing |

```powershell
# Set Npcap SDK path once (PowerShell)
[System.Environment]::SetEnvironmentVariable("LIB", "C:\npcap-sdk\Lib\x64", "User")
```

> Packet capture requires Administrator rights — run as Administrator or use the Npcap service.

---

### Linux

```bash
# Debian / Ubuntu
sudo apt-get install -y build-essential libpcap-dev libgtk-3-dev \
    libwebkit2gtk-4.1-dev libayatana-appindicator3-dev librsvg2-dev patchelf curl

# Fedora / RHEL
sudo dnf install -y gcc gcc-c++ libpcap-devel gtk3-devel \
    webkit2gtk4.1-devel libayatana-appindicator-gtk3-devel librsvg2-devel patchelf

# Arch
sudo pacman -S --needed base-devel libpcap gtk3 webkit2gtk-4.1 \
    libayatana-appindicator librsvg patchelf
```

```bash
# Rust + Node.js
curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh
source "$HOME/.cargo/env"
curl -o- https://raw.githubusercontent.com/nvm-sh/nvm/v0.39.7/install.sh | bash
source ~/.bashrc && nvm install 20 && nvm use 20
```

```bash
# Capture privileges (pick one)
sudo setcap cap_net_raw+ep /path/to/netscope-agent  # preferred: no root at runtime
sudo netscope-agent capture --interface eth0         # or: run with sudo
```

---

## Installation

### Option A — Pre-built Releases

Download from the [**Releases page**](https://github.com/Libinm264/netscope/releases):

| Platform | File |
|---|---|
| macOS (Apple Silicon) | `NetScope_aarch64.dmg` |
| macOS (Intel) | `NetScope_x86_64.dmg` |
| Windows | `NetScope_x64_en-US.msi` |
| Linux portable | `netscope_amd64.AppImage` |
| Linux Debian | `netscope_amd64.deb` |

```bash
# macOS: clear quarantine if Gatekeeper blocks it
xattr -cr /Applications/NetScope.app

# Linux AppImage
chmod +x NetScope_*.AppImage && ./NetScope_*.AppImage
```

### Option B — Build from Source

```bash
git clone https://github.com/Libinm264/netscope.git
cd netscope

# CLI agent
cd agent && cargo build --release
# → agent/target/release/netscope-agent

# Desktop app
cd ../desktop && npm install && npm run tauri build
# → desktop/src-tauri/target/release/bundle/{dmg,msi,appimage}/
```

> First build takes 5–10 minutes (full Rust dependency tree). Subsequent builds are incremental.

---

## Running NetScope

### Desktop Application

Launch from your Applications folder / Start Menu / AppImage. On first launch:

1. If privileges are missing, the **Privilege Helper** modal guides through the fix.
2. If GeoIP databases are absent, an amber banner prompts you to set them up (see [GeoIP Enrichment](#geoip-enrichment)).

### CLI Agent

```bash
# List available interfaces
netscope-agent list-interfaces

# Basic capture
netscope-agent capture --interface en0

# With BPF filter
netscope-agent capture --interface eth0 --filter "tcp port 80 or udp port 53"

# Save to JSONL
netscope-agent capture --interface en0 --output flows.jsonl

# Stream to hub
netscope-agent capture --interface en0 \
    --hub-url https://hub.example.com \
    --api-key ns_your_token_here
```

**Environment variables:**

| Variable | Default | Description |
|---|---|---|
| `RUST_LOG` | `info` | Log verbosity (`error`, `warn`, `info`, `debug`, `trace`) |
| `NETSCOPE_INTERFACE` | — | Default interface (overridden by `--interface`) |

### Hub API Server

```bash
cd hub/api

# Copy and edit the environment file
cp .env.example .env
# Set: CLICKHOUSE_ADDR, KAFKA_BROKERS, BOOTSTRAP_API_KEY, GEOIP_DB_PATH, etc.

go run ./...

# With Docker Compose (includes ClickHouse + Kafka)
docker compose up
```

The API listens on `:8080` by default. The Next.js dashboard:

```bash
cd hub/web && npm install && npm run dev   # dev: http://localhost:3000
npm run build && npm start                  # production
```

---

## Usage Guide

### Starting a Capture

1. Open NetScope desktop.
2. Select an interface from the **Interface** dropdown.
3. _(Optional)_ Enter a BPF filter: `host 8.8.8.8`, `tcp port 443`, etc.
4. Click **▶ Start** — flows appear in real time.
5. Click any row to inspect it in the detail tree and hex panes.

### Filtering Traffic

The filter bar applies instantly without stopping the capture:

| Input | Effect |
|---|---|
| `http` | HTTP flows only |
| `dns` | DNS flows only |
| `tls` | TLS flows only |
| `errors` | HTTP 4xx / 5xx only |
| `threats` | Flows with a non-clean threat score |
| `hub` | Flows loaded from the hub |
| `192.168.1.1` | Any flow matching that IP |
| `443` | Any flow on that port |
| `POST /api` | Text search in the Info column |

### GeoIP Enrichment

NetScope uses the free **MaxMind GeoLite2** databases to show country flags and ASN info on every row.

**Setup:**

1. Create a free account at [dev.maxmind.com](https://dev.maxmind.com/geoip/geolite2-free-geolocation-data).
2. Download `GeoLite2-City.mmdb` and `GeoLite2-ASN.mmdb`.
3. Place them in `~/.netscope/`:

```bash
mkdir -p ~/.netscope
cp GeoLite2-City.mmdb ~/.netscope/
cp GeoLite2-ASN.mmdb  ~/.netscope/
```

The app auto-loads the databases on startup. If they are missing, an amber banner appears in the toolbar with a link to download them.

> Private/RFC1918 IPs (10.x, 172.16–31.x, 192.168.x, loopback) are skipped — geo enrichment only applies to routable public IPs.

### Threat Intelligence

NetScope scores every flow against a built-in offline threat database — no external service required:

- **CIDR blocklist** — Known Tor exit nodes, C2 infrastructure, and malicious ranges.
- **Port heuristics** — C2 ports (4444, 1337, 31337), Tor (9001, 9030), Telnet (23).

Flows are tagged `HIGH` / `MED` / `LOW` with colored row highlights and `⚠` badges. The detail pane shows full reasons. Use the `threats` quick-filter to isolate suspicious traffic.

### Hub Connect Mode

Connect the desktop app to a running NetScope Hub to query historical and fleet-wide flows:

1. Click the **⛓ Hub** button in the toolbar.
2. Enter your Hub URL and API token.
3. Click **Test connection** — green ✓ indicates success.
4. Set optional filters (protocol, source IP, result limit).
5. Click **Load hub flows** — hub flows are merged into the local view, tagged with a blue `hub` badge.

### TLS Certificate Fleet

Click the **🛡 Shield** button in the toolbar to open the certificate sidebar. It shows every TLS certificate seen in the current session, sorted by urgency:

| Severity | Condition |
|---|---|
| 🔴 Expired | `certExpired = true` |
| 🔴 Critical | Expires within 7 days |
| 🟡 Warning | Expires within 30 days |
| 🟢 Valid | More than 30 days remaining |

A red badge on the Shield button shows the count of critical/expired certs.

### HTTP Analytics

Switch to the **Analytics** tab in the bottom pane to see per-endpoint performance:

- **p50 / p95 / p99** latency bars for each `METHOD /path` endpoint
- **Error rate** (HTTP 4xx/5xx percentage)
- Sortable by request count, p95 latency, or error rate

Data updates live as new flows arrive during capture.

### Service Dependency Map

Switch to the **Service Map** tab to see a force-directed graph of all IP-to-IP connections in the session:

- **Nodes** = unique IP addresses, sized proportionally by flow count
- **Edges** = connections, coloured by protocol (blue = HTTP, violet = DNS, indigo = TLS, cyan = ICMP, gray = other)
- **Hover** a node for IP, country, ASN, flow count, and protocol list
- **Pan** by dragging; **zoom** with scroll wheel; **Reset** button to re-centre

Node positions are stable — the graph layout only runs when new IPs appear, so the map doesn't jump on every new packet.

### Saving and Loading Sessions

- **Save:** Click 💾 in the toolbar → choose a `.nscope` filename.
- **Load:** Click 📂 in the toolbar → select a `.nscope` file.

`.nscope` files are standard SQLite databases you can inspect with [DB Browser for SQLite](https://sqlitebrowser.org). Sessions are forward-compatible — new fields (GeoIP, threat, source) default gracefully when loading older files.

---

## Development

### Dev Mode

```bash
cd desktop
npm install
npm run tauri dev
```

Starts the Vite dev server on `http://localhost:1420` and the Tauri window pointing at it. React changes hot-reload instantly; Rust backend changes require a restart.

### Useful Commands

```bash
# Frontend
cd desktop
npx tsc --noEmit              # Type-check without building
npm run build                 # Production Vite build

# Rust agent
cd agent
cargo test                    # Unit tests
cargo check                   # Fast compile check
cargo fmt                     # Format code
cargo clippy -- -D warnings   # Lint

# Rust desktop backend
cd desktop/src-tauri
cargo check                   # Fast check
cargo clippy -- -D warnings   # Lint

# Hub API
cd hub/api
go build ./...                # Build check
go vet ./...                  # Lint
go test ./...                 # Tests
gofmt -w .                    # Format
```

### Adding a New Protocol Decoder

1. Create `agent/crates/parser/src/myproto.rs` with `parse_myproto(buf: &[u8]) -> Option<MyProtoFlow>`.
2. Add `MyProtoFlow` to `agent/crates/proto/src/lib.rs` and a variant to `FlowPayload`.
3. Wire the parser into `session.rs` `SessionManager::handle_tcp_data()`.
4. Add `MyProtoFlowDto` to `desktop/src-tauri/src/dto.rs` and update `flow_to_dto()`.
5. Render it in `PacketDetailPane.tsx` with a new tree section.

---

## CI / CD

GitHub Actions builds and publishes release bundles on every version tag:

```bash
git tag v1.0.0 && git push origin v1.0.0
```

The workflow (`.github/workflows/build-desktop.yml`) runs on four runners in parallel:

| Runner | Target |
|---|---|
| `macos-latest` | `aarch64-apple-darwin` |
| `macos-latest` | `x86_64-apple-darwin` |
| `windows-latest` | `x86_64-pc-windows-msvc` |
| `ubuntu-22.04` | `x86_64-unknown-linux-gnu` |

All four bundles are uploaded as GitHub Release assets via `tauri-apps/tauri-action`. Rust build caching (`swatinem/rust-cache`) keeps CI builds under 5 minutes on cache hits.

---

## Roadmap

- [x] **Phase 1** — Rust CLI agent: libpcap capture, HTTP/1.1 + DNS parsers, TCP reassembler
- [x] **Phase 2** — Tauri desktop GUI: virtualised packet list, hex dump, detail tree, session persistence, GitHub Actions CI
- [x] **Phase 3** — SaaS Hub: Go/Fiber REST API, Kafka ingestion, ClickHouse analytics, Next.js 14 dashboard, JWT + API key auth
- [x] **Phase 4** — Protocol expansion: TLS handshake parsing, TCP retransmissions, ICMP, ARP
- [x] **Phase 5** — Analytics & export: service dependency map, HTTP endpoint analytics, OpenTelemetry export
- [x] **Phase 6** — Enterprise features: fleet management, TLS cert viewer, anomaly detection, onboarding, RBAC
- [x] **Phase 7** — Alerting & Kubernetes: webhook alert rules, compliance reporting (PCI-DSS, HIPAA, CIS), Helm chart
- [x] **Phase 8** — eBPF agent + enrichment: kernel-side eBPF capture, GeoIP + threat intelligence (hub and desktop)
- [x] **Desktop upgrade** — GeoIP enrichment, threat badges, Hub Connect Mode, cert sidebar, HTTP analytics, service mini-map; full bug audit
- [x] **Production hardening** — API key proxy (no browser exposure), SSRF prevention, security headers, startup production gate, error sanitisation
- [x] **Phase 9** — Production foundation: HTTP/2 + gRPC decoder, per-agent scoped tokens, audit log, alert delivery retry, SSE rate limit, hub URL fix
- [ ] **Phase 10** — K8s metadata enrichment (pod/namespace/labels from eBPF), WASM plugin SDK for custom protocol parsers, OTel trace → packet drill-down

---

## Contributing

Contributions are welcome!

1. Fork the repo and create a feature branch: `git checkout -b feature/my-feature`
2. Make your changes with tests where applicable.
3. Run `cargo fmt && cargo clippy -- -D warnings` (Rust) and `npx tsc --noEmit` (TypeScript).
4. Open a Pull Request with a clear description of what changed and why.

For larger features or protocol additions, please open an issue first to discuss the approach.

---

## License

MIT License — see [LICENSE](LICENSE) for details.

---

<div align="center">
  Built with Rust · Go · React · ClickHouse
</div>
