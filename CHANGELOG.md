# NetScope Changelog

All notable changes to NetScope are documented here.  
Format: user-facing features first, internals second.  
Community features are marked **✅ Community**; Enterprise-only features are marked **🔒 Enterprise**.

---

## v0.7 — Incident Intelligence Release

**Released:** 2026-05-05

The v0.7 release makes NetScope the first network observability platform that lets you *replay an incident* as it happened, guard privacy with zero-config PII masking, and prove sub-1% CPU overhead with live sparklines in the dashboard.

---

### 🎬 Incident Replay Timeline (V1) — *New*

**The headline feature.** When an anomaly fires, click the new **Replay** button on any anomaly row. NetScope pulls every flow captured in a ±5-minute window around the incident and renders them as a parallel protocol lane timeline.

- **Five protocol lanes in parallel:** HTTP (blue), HTTP/2 & gRPC (indigo), DNS (magenta), TLS (cyan), TCP/UDP (slate)
- **Red trigger line** marks the exact moment the anomaly was detected across every lane
- **Minute tick marks** on a sticky time axis so you can orient yourself instantly
- **Click any flow block** to expand a detail panel — src/dst, bytes, duration, and protocol info, all without leaving the page
- **Deep-linkable** — send a replay URL directly to a teammate from your alerting tool:  
  `https://your-hub/replay?agent_id=…&around=<ISO8601>&window_mins=10`
- **API-first** — `GET /api/v1/replay` returns up to 500 flows sorted chronologically; fully scriptable

**Entry points:**
- Anomalies page → **Replay** button on every row (pre-centered on `detected_at`)
- Agent fleet cards → **View recent timeline** link (centered on `last_seen`)

> ✅ Community · ✅ Enterprise

---

### 🛡️ PII Masking Engine (T1) — *New*

Credentials, tokens, and payment data are redacted **inside the Rust agent** before they ever reach the network or ClickHouse. Nothing sensitive touches disk.

**Redacted automatically:**
| Category | Examples |
|---|---|
| Auth headers | `Authorization`, `X-Api-Key`, `Cookie`, `Set-Cookie` |
| Tokens in bodies | `access_token`, `refresh_token`, `id_token`, `secret` |
| Passwords | `password`, `passwd`, `new_password`, `current_password` |
| Payment data | `card_number`, `cvv`, `ssn`, `credit_card` |
| API credentials | `api_key`, `api_secret`, `private_key`, `client_secret` |

Values are replaced with `[REDACTED]` inline. The field name is preserved for debugging; only the value is masked.

**Configurable:** Add your own regex patterns to `~/.config/netscope/agent.toml`:
```toml
[privacy]
extra_patterns = ["x-internal-token", "x-session-id"]
```

> ✅ Community · ✅ Enterprise

---

### 📈 Agent Performance Telemetry (T2) — *New*

Every agent heartbeat now reports `cpu_pct`, `mem_mb`, and `packets_dropped`. The Fleet dashboard renders these as per-agent sparklines with live colour coding:

- **Indigo** — CPU < 50% (healthy)
- **Amber** — CPU 50–80% (moderate load)
- **Red** — CPU > 80% (investigate)
- **Drop warning badge** — shown when `packets_dropped > 0`

Typical overhead: **< 1% CPU, < 15 MB RSS** at 1 Gbps on a 2-core VM.

**History API** — query any agent's last N samples:
```
GET /api/v1/agents/{agent_id}/perf?limit=20
```

**ClickHouse** stores samples in `agent_perf_samples` with a 30-day TTL. Query it directly for trending, capacity planning, or exporting to Grafana.

> ✅ Community · ✅ Enterprise

---

### ⚡ Adaptive Sampling (T4) — *New*

Stop burning storage on routine traffic. NetScope now operates in two modes, switchable per-agent from the Fleet UI with **zero restart**.

| | Metadata mode (default) | Full capture |
|---|---|---|
| Headers & timing | ✅ | ✅ |
| DNS / TLS / ICMP | ✅ | ✅ |
| HTTP request body | ❌ stripped | ✅ |
| HTTP response body | ❌ stripped | ✅ |
| 4xx / 5xx bodies | ✅ kept | ✅ |
| Storage cost | ~40 % of full | 100 % |

**Error-first guarantee:** Even in metadata mode, bodies are always preserved on HTTP 4xx and 5xx responses. You never lose the payload that explains the error.

**Config push:** Flip the toggle in the Fleet dashboard → Hub writes the new mode to `agent_configs` → agent picks it up within 30 seconds via the existing config poll. No restart, no reconnect.

**API:**
```
GET  /api/v1/agents/{id}/sampling       → { mode: "metadata" | "full" }
POST /api/v1/agents/{id}/sampling       → { mode: "full" }   (admin only)
```

> ✅ Community · ✅ Enterprise

---

### 🍎 macOS Code Signing (T3) — *Done*

The macOS `.dmg` is now signed and notarised with an Apple Developer ID certificate. Users open NetScope with a standard double-click — no "unidentified developer" bypass required.

- **Identity:** Developer ID Application (validated against Apple's notarisation service in CI)
- **CI:** All six Apple environment variables are wired in `build-desktop.yml`; signing runs automatically on every tagged release build

> ✅ Community · ✅ Enterprise

---

## v0.6 — Security & Intelligence Release

### 🤖 AI Security Copilot — *New in v0.6*

Ask your network anything in plain English. The Copilot translates natural-language questions into ClickHouse SQL, executes them against your live flow data, streams the results back, and explains what it found.

**Example queries:**
- *"Which processes made outbound connections to port 443 in the last hour?"*
- *"Show me DNS lookups that returned NXDOMAIN today"*
- *"What are the top 10 external destinations by bytes sent?"*
- *"Are there any flows with threat scores above 80?"*

Powered by Claude. The API key lives on your server — it never passes through Anthropic's servers or ours. Accessible from the slide-out panel (⌘K / Ctrl+K) on any page.

> ✅ Community · ✅ Enterprise

---

### 📊 Behavioural Anomaly Detection — *New in v0.6*

NetScope builds a **7-day rolling baseline** for every agent/protocol pair, broken down by hour-of-week (168 buckets). When observed traffic deviates significantly from the expected mean, an anomaly event fires.

- **Z-score thresholding** — flags spikes and drops (e.g. "HTTP traffic 4.3σ above baseline")
- **Three severity levels** — High / Medium / Low, colour-coded in the dashboard
- **Anomaly types** — `spike` (sudden surge) and `drop` (unexpected silence)
- **Auto-refresh** — the Anomalies page polls every 30 seconds; stat cards update live
- **Baseline viewer** — inspect the learned baseline per agent and protocol at `/anomalies`
- **v0.7 integration** — every anomaly row now has a **Replay** button → protocol lane timeline

> ✅ Community · ✅ Enterprise

---

### 📋 Custom Dashboards — *New in v0.6*

Build and save bespoke dashboards combining any metric NetScope tracks. Each dashboard is a named, persistent layout of widgets:

- **Widget types:** timeseries chart, protocol breakdown donut, top-N table, stat card
- **Saved to Hub** — accessible from any browser; shared across your team
- **Python SDK support** — create and update dashboards programmatically

> ✅ Community (up to 5) · 🔒 Enterprise (unlimited)

---

### 🐍 Python SDK — *New in v0.6*

`pip install netscope-sdk` gives you a full Python client for NetScope:

```python
from netscope import NetScopeClient

client = NetScopeClient(hub_url="https://hub.example.com", api_key="...")

# Query flows
flows = client.flows.list(protocol="HTTP", hours=1)

# Create an alert rule
client.alerts.create(metric="http_error_rate", threshold=0.05, ...)

# Ask the Copilot
result = client.copilot.ask("Which IPs had more than 100 failed connections today?")
```

Full async support via `AsyncNetScopeClient`. Runnable examples in `sdk/examples/`.

> ✅ Community · ✅ Enterprise

---

### 📄 Compliance Reports — *New in v0.6*

One-click reports for auditors and security teams:

- **Connection log** — every src/dst with timestamps, filterable by internal/external
- **TLS audit** — expired, expiring-soon, and self-signed certificates across the fleet
- **Top talkers** — highest-bandwidth processes and IPs
- **External destinations** — all outbound connections with byte counts and unique source IPs
- **GeoIP heat map** — connections by country with max threat score per country
- **Export** — JSON, CEF (ArcSight), or LEEF (QRadar) via `GET /api/v1/enterprise/audit/export`

> 🔒 Enterprise

---

## v0.5 — Process & Cloud Release

### 🔬 eBPF Agent — Process Attribution & TLS Plaintext

The Linux eBPF agent attaches to the kernel network stack and maps every connection back to the originating process:

- **Process name + PID** on every flow — see exactly which binary made the connection
- **TLS plaintext capture** — decrypts TLS 1.2/1.3 sessions without a CA cert by hooking `SSL_read`/`SSL_write` in the process memory
- **Kubernetes enrichment** — pod name and namespace injected from the k8s downward API
- **Process policy engine** — define allow/deny rules per process, CIDR, and port; violations are logged and can trigger alerts
- **Zero kernel module** — uses CO-RE BPF programs; works on any kernel ≥ 5.8 with BTF

```bash
sudo netscope-agent-ebpf \
  --hub-url https://hub.example.com \
  --api-key <key>
```

> ✅ Community · ✅ Enterprise

---

### ☁️ Cloud Flow Integration

Ingest VPC Flow Logs from AWS, GCP, and Azure alongside your on-prem pcap/eBPF flows:

- **Unified view** — cloud and on-prem flows in the same query interface
- **Cross-env service map** — edges between on-prem agents and cloud VPCs
- **Compliance** — cloud external connections appear in the TLS and connection audit reports

> 🔒 Enterprise

---

### 🔗 OpenTelemetry Trace Correlation

NetScope parses `traceparent` headers from HTTP and HTTP/2 flows and indexes the trace ID. Filter flows by trace ID to jump from an anomalous span in Jaeger or Tempo directly to the underlying network activity.

```
GET /api/v1/flows?trace_id=4bf92f3577b34da6a3ce929d0e0e4736
```

Works with any OTel-instrumented application — no agent changes required.

> ✅ Community · ✅ Enterprise

---

## v0.4 — Enterprise Readiness Release

### 🔍 Sigma Detection Rules

Five built-in rules cover the most common attack patterns out of the box:

| Rule | What it detects |
|---|---|
| Port scan | Single source scanning ≥ 20 distinct ports in 60 s |
| Beaconing | Periodic connections at suspiciously regular intervals |
| Data exfiltration | Outbound byte spike to an external IP |
| DNS tunnelling | Unusually long DNS query names (> 50 chars) |
| Lateral movement | Internal source connecting to many distinct internal hosts |

Enterprise users can write custom Sigma rules in YAML with full ClickHouse SQL expressions, view 30-day match history, and enable/disable rules per environment.

> ✅ Community (built-in rules) · 🔒 Enterprise (custom rules)

---

### 🔔 Alert Rules & Multi-Channel Delivery

Define threshold-based alert rules on any metric and deliver them to your existing tooling:

**Metrics:** `flows_per_minute`, `http_error_rate`, `dns_nxdomain_rate`, `anomaly_flow_rate`, `anomaly_http_latency`

**Channels:** Slack, PagerDuty, OpsGenie, Microsoft Teams, generic webhook, email

**Features:**
- Cooldown period — prevent alert storms
- Test delivery button — verify routing before production
- Webhook HMAC signing — authenticate payloads in your receiving endpoint
- Alert event history — every fired rule with value, threshold, and delivery status

> ✅ Community · ✅ Enterprise

---

### 🏢 Enterprise Identity & Access

| Feature | Details |
|---|---|
| **SSO — SAML 2.0** | Azure AD, Okta, Google Workspace |
| **SSO — OIDC** | Auth0, Keycloak, any OIDC provider |
| **RBAC** | `admin` (full write) and `viewer` (read-only) API tokens |
| **Multi-org** | Isolated data planes per organisation, slug-based routing |
| **Teams** | Group members into teams; use in alert routing and policy scoping |
| **Audit log** | Every API call with actor, method, path, status, latency, IP |
| **Invite flow** | Email invite with short-lived token; no password set by admin |

> 🔒 Enterprise

---

### 📤 SIEM Integration (Enterprise)

Push flows and alerts into your existing security stack in real time:

| Sink | Protocol |
|---|---|
| Splunk | HEC (HTTP Event Collector) |
| Elasticsearch / OpenSearch | REST bulk index |
| Datadog | Logs API |
| Grafana Loki | Push API |
| Generic webhook | JSON POST with HMAC signature |

Configure at `Settings → Integrations`. Test connectivity with one click before enabling live shipping.

> 🔒 Enterprise

---

### 🗄️ Long-Term Cold Storage (Enterprise)

Export flows directly from ClickHouse to object storage on a schedule — no bytes flow through the Hub process:

| Provider | Notes |
|---|---|
| AWS S3 | IAM role or key-based |
| Google Cloud Storage | Service account JSON |
| MinIO / Cloudflare R2 | S3-compatible endpoint |

Cadences: hourly or daily. Format: Parquet (default) or JSON lines. Configure at `Settings → Storage`.

> 🔒 Enterprise

---

## Core Platform (all versions)

These capabilities ship in every release:

### 🌊 Real-Time Flow Capture

- **pcap mode** — works on macOS, Linux, and Windows; no root on macOS (libpcap)
- **eBPF mode** — Linux only; process attribution, TLS plaintext, K8s enrichment
- **Desktop app** — Tauri 2 + React; live flow table, sparkline graphs, session save/load
- **CLI agent** — headless capture for servers; hub-connect or standalone JSONL output
- Capture any interface or `any` (all interfaces); Berkeley Packet Filter expressions supported

---

### 🔬 Deep Protocol Decode

Every captured packet is parsed into a structured flow with protocol-specific fields:

| Protocol | Fields decoded |
|---|---|
| **HTTP/1.1** | Method, path, status, latency, host, user-agent, body preview |
| **HTTP/2 & gRPC** | Stream ID, method, path, status, HPACK headers |
| **DNS** | Query name, type (A/AAAA/MX/…), answers, RCODE, latency |
| **TLS** | SNI, cipher suite, TLS version, cert CN/SANs/expiry/issuer |
| **TCP** | Retransmissions, out-of-order segments |
| **ICMP** | Type/code, echo ID/seq, RTT |
| **ARP** | Operation, sender/target IP+MAC |
| **UDP** | Port-based heuristics for DNS-over-UDP and QUIC detection |

---

### 🌍 GeoIP & Threat Intelligence

Every external IP is enriched with:
- **Country** (MaxMind GeoLite2 — updated weekly in CI)
- **AS organisation** — ISP or cloud provider
- **AbuseIPDB threat score** (0–100) + threat level (high/medium/low/clean)
- Flows with `threat_score ≥ 70` surface in the **Threats** dashboard with process attribution

---

### 🔐 TLS Certificate Fleet Monitor

NetScope tracks every unique TLS certificate seen across the fleet and alerts before they expire:

- **Certificate inventory** — CN, issuer, SANs, fingerprint, first/last seen
- **Expiry dashboard** — expired / critical (< 7 days) / warning (< 30 days) / OK
- **Weak cipher detection** — flags RC4, 3DES, NULL, EXPORT cipher suites
- **Alert integration** — pipe expiring-cert events into your alert channels

---

### 📡 Service Dependency Graph

Automatic service map built from observed traffic — no instrumentation required:

- Nodes are IPs/services; edges are protocol + flow count + avg latency
- Filter by time window (15m / 1h / 6h / 24h)
- Identifies which services talk to which, at what rate, and how healthy
- eBPF enrichment adds process names to nodes

---

### 📊 HTTP Endpoint Analytics

Aggregate HTTP metrics per `(method, path)` pair across the fleet:

- Request count, error count, **error rate %**
- **p50 / p95 / p99 latency** — all computed in ClickHouse; no sampling
- Window selector: 15m, 1h, 6h, 24h
- Sortable table; exportable via the API

---

### 🤝 Fleet Management

Manage dozens of agents from a single Hub dashboard:

| Capability | Details |
|---|---|
| **Enrollment tokens** | Short-lived tokens with one-line install command; no admin key exposure |
| **Remote config push** | Sampling mode, log level, and custom settings pushed to agents live |
| **Status tracking** | Online (< 5 min), Idle (< 30 min), Offline; last-seen timestamp |
| **eBPF badge** | Agents in eBPF mode highlighted; upgrade nudge shown when all are in pcap |
| **Per-agent perf sparklines** | CPU%, memory, drop counter — updated every 30 s |
| **Sampling toggle** | Switch metadata ↔ full capture per agent; zero restart |

---

### 💾 Saved Flow Queries

Save any combination of filters (protocol, src IP, dst IP, hostname, trace ID, time range) as a named query and recall it instantly. Community: up to 10 saved queries. Enterprise: unlimited.

---

### 🗓️ Incident Timeline

The Incidents page (`/incidents`) surfaces the chronological sequence of alert fires, anomaly events, and policy violations in a unified feed — one place to reconstruct what happened and when.

---

## Planned — v0.8

| ID | Feature | Description |
|---|---|---|
| V2 | Natural Language Flow Search | Type a question; get filtered flows — no query language needed |
| V3 | Passive API Inventory | Auto-discover every internal API endpoint from observed HTTP(S) traffic |
| V4 | Slack/Teams Alert Threads | Each alert opens a dedicated thread; updates post as the incident evolves |
| V5 | One-Command Docker Compose | `curl ... | sh` spins up Hub + ClickHouse + Agent in a single step |

---

*For the full technical reference, see [docs.html](website/docs.html).  
For the public roadmap, see [ROADMAP.md](ROADMAP.md).*
