# NetScope v0.7.0 — Incident Intelligence Release

**Date:** 2026-05-05  
**Tags:** `v0.7.0` · Community (MIT) + Enterprise (BSL-1.1)

---

## Overview

v0.7 is the **Incident Intelligence** release. Every major workflow gap in incident response is now closed: replay any anomaly as a scrollable timeline, search flows with plain English, auto-discover every API endpoint your services call, receive rich Slack/Teams alert threads with inline flow context, and spin up the full stack with a single `docker compose up`.

Nine features shipped across two tracks — Trust & Compliance (T1–T4) and Value (V1–V5).

---

## What's New

### 🎬 V1 — Incident Replay Timeline

Click **Replay** on any anomaly row to instantly see every network flow captured in a ±5-minute window around the incident, rendered as parallel protocol lanes:

- **HTTP** (blue) · **HTTP/2 & gRPC** (indigo) · **DNS** (magenta) · **TLS** (cyan) · **TCP/UDP** (slate)
- Red trigger line marks the exact anomaly moment across all lanes
- Minute-tick time axis; click any flow block for src/dst/bytes/duration detail
- Deep-linkable URL — paste into Slack/PagerDuty tickets
- API: `GET /api/v1/replay?agent_id=…&around=<ISO8601>&window_mins=10`

### 🔍 V2 — Natural Language Flow Search

A plain-English search bar lives at the top of the **Flows** page. Type a query and the AI extracts protocol, IP, and time-range filters that update the flow table in place — no modal, no separate screen.

```
"DNS failures in the last 30 minutes"
"HTTP errors from 10.0.0.5 today"
"TLS flows to external IPs"
```

Requires `ANTHROPIC_API_KEY`. Explanation chip shows what was applied; × resets.

### 🗂️ V3 — Passive API Inventory

Auto-discovers every HTTP, HTTP/2, and gRPC endpoint called across your fleet with zero instrumentation. **Sidebar → API Inventory** shows call volume, p95 latency, error rate, agent count, and a **"New today"** badge for endpoints first seen in the last 24 hours.

### 🧵 V4 — Rich Alert Threads (Slack · Teams · PagerDuty · OpsGenie)

Alert notifications now carry structured context:

- **Slack Block Kit** — rule name, metric/threshold, up to 5 inline flows, "View in NetScope" + "▶ Replay" action buttons
- **Microsoft Teams Adaptive Card** — same facts rendered natively in Teams (no connector needed)
- **PagerDuty Events v2** and **OpsGenie** integrations added
- `POST /api/v1/alerts/{id}/resolve` for runbook-style resolution notes

### 🐳 V5 — One-Command Docker Compose

```bash
cp .env.example .env   # fill in secrets
docker compose up -d   # starts ClickHouse + Hub API + Web UI
```

Health-check ordering ensures ClickHouse is ready before the API, and the API is ready before the dashboard. Clean first boot < 60 seconds.

---

## Trust & Compliance Track

### 🛡️ T1 — PII Masking Engine

Credentials, tokens, and payment fields are redacted **inside the Rust agent** before reaching ClickHouse:

| Category | Fields |
|---|---|
| Auth headers | `Authorization`, `X-Api-Key`, `Cookie`, `Set-Cookie` |
| Token fields | `access_token`, `refresh_token`, `id_token`, `secret` |
| Passwords | `password`, `passwd`, `new_password`, `current_password` |
| Payment data | `card_number`, `cvv`, `ssn`, `credit_card` |

Custom patterns via `~/.config/netscope/agent.toml` `[privacy] extra_patterns`.

### 📈 T2 — Agent Performance Telemetry

Each heartbeat now reports `cpu_pct`, `mem_mb`, `packets_dropped`. Fleet dashboard shows per-agent sparklines colour-coded indigo/amber/red. Overhead: **< 1% CPU, < 15 MB RSS** at 1 Gbps.

History API: `GET /api/v1/agents/{id}/perf?limit=20`

### 🍎 T3 — macOS Code Signing

`.dmg` signed and notarised with Apple Developer ID. Standard double-click to open — no "unidentified developer" bypass.

### ⚡ T4 — Adaptive Sampling

Two capture modes, switchable live from Fleet UI:

| | Metadata (default) | Full capture |
|---|---|---|
| Headers & timing | ✅ | ✅ |
| HTTP bodies | ❌ stripped | ✅ |
| 4xx/5xx bodies | ✅ always kept | ✅ |
| Storage | ~40% | 100% |

API: `GET/POST /api/v1/agents/{id}/sampling`

---

## Version Bumps

| Component | Old | New |
|---|---|---|
| Desktop (Tauri) | 0.6.1 | 0.7.0 |
| Agent (Rust) | 0.5.0 | 0.7.0 |
| Hub Web (Next.js) | 0.5.0 | 0.7.0 |
| Python SDK | 0.6.0 | 0.7.0 |
| Helm chart | 0.1.0 | 0.7.0 |

---

## Release Assets

| Asset | Platform |
|---|---|
| `netscope-agent-v0.7.0-aarch64-apple-darwin.tar.gz` | macOS ARM64 |
| `netscope-agent-v0.7.0-x86_64-apple-darwin.tar.gz` | macOS Intel |
| `netscope-agent-v0.7.0-x86_64-unknown-linux-gnu.tar.gz` | Linux x86_64 |
| `netscope-agent-v0.7.0-aarch64-unknown-linux-gnu.tar.gz` | Linux ARM64 |
| `NetScope_0.7.0_aarch64.dmg` | Desktop macOS ARM64 |
| `NetScope_0.7.0_x64.dmg` | Desktop macOS Intel |
| `NetScope_0.7.0_x64-setup.exe` | Desktop Windows x64 |
| `NetScope_0.7.0_amd64.AppImage` | Desktop Linux |
| `latest.json` | Auto-updater manifest |
| `ghcr.io/libinm264/netscope-hub-api:0.7.0` | Docker Hub API |
| `ghcr.io/libinm264/netscope-hub-web:0.7.0` | Docker Hub Web |

Docker images also tagged `:latest` and `:0.7`.

---

## Upgrade Guide

### Docker Compose (new in v0.7)

```bash
git pull
cp .env.example .env        # first time only — fill in your secrets
docker compose pull          # pull latest images (if using GHCR tags)
docker compose up -d         # rolling restart
```

### Hub API (binary / k8s)

Standard ClickHouse schema migration runs automatically on startup via `main.go` DDL. No manual migration step required.

New optional env vars:
```
ANTHROPIC_API_KEY   — enables NL Flow Search and AI Copilot
SMTP_HOST / SMTP_PORT / SMTP_USER / SMTP_PASSWORD  — email alerts
```

### Agent

Replace the binary and restart. The new `[sampling]` and `[privacy]` config sections are optional — existing `agent.toml` files continue to work unchanged.

---

## Full Changelog

See [CHANGELOG.md](./CHANGELOG.md) for complete per-feature detail.
