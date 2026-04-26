# NetScope Roadmap

This document tracks what has shipped and what is planned for each release.
The goal is to evolve NetScope from a passive network visibility tool into a
full-stack security observability platform.

---

## ✅ v0.1 — Foundation (shipped)

| Area | Feature |
|------|---------|
| Agent | libpcap-based packet capture (HTTP/1.x, DNS, TLS, ICMP, ARP, HTTP/2, gRPC) |
| Agent | Hub output — batch ingest over HTTP with API-key auth |
| Agent | `list-interfaces`, `capture`, `ebpf` CLI subcommands |
| Hub | ClickHouse storage with 90-day TTL |
| Hub | REST API: `/api/v1/flows`, `/api/v1/stats`, `/api/v1/ingest` |
| Hub | Server-Sent Events live feed (`/api/v1/flows/stream`) |
| Hub | TLS certificate fleet tracking (`tls_certs` table) |
| Hub | Geo-IP + threat scoring enrichment at ingest |
| Hub | Compliance endpoints: summary, top-talkers, external connections, geo |
| Hub | Role-based API tokens (admin / viewer) |
| Hub | Agent enrollment tokens + one-line install script |
| UI | Real-time flow table with protocol badges |
| UI | Dashboard: total flows, flows/min, top protocols, active agents |
| UI | Flows explorer with filters (protocol, IP, time range) |
| UI | TLS certificate audit page |
| UI | Compliance reporting (external connections, geo breakdown) |
| UI | Alert rules + alert event history |
| Desktop | Tauri + Rust desktop GUI (macOS / Linux) |
| CI/CD | GitHub Actions: 4-platform agent binary builds |
| Docs | Quick-start README, one-line install |

---

## 🔄 v0.2 — eBPF Engine (in progress)

> **Theme**: See through TLS, know which process owns every byte.

The eBPF engine intercepts plaintext data *before* OpenSSL encrypts it and
*after* it decrypts it, giving you full HTTP visibility without a CA cert or
a proxy.

| Area | Feature | Status |
|------|---------|--------|
| Agent | `ProcessInfo { pid, name }` added to proto `Flow` | ✅ done |
| Agent | eBPF → Hub bridge: `ssl_event_to_flow`, `tcp_connect_to_flow` | ✅ done |
| Agent | HTTP-from-SSL parser (`parse_http_plaintext`) | ✅ done |
| Agent | Per-process terminal output (`[process:pid]` prefix) | ✅ done |
| Agent | Hub sender thread (avoids blocking inside async eBPF loop) | ✅ done |
| Hub | `process_name` + `pid` columns in ClickHouse `flows` table | ✅ done |
| Hub | Ingest, Query, Compliance endpoints surface process fields | ✅ done |
| UI | Process column in FlowTable (green name + PID, "—" for pcap) | ✅ done |
| Docs | eBPF section in README | 🔲 todo |
| Agent | eBPF kernel programs (BPF C): SSL uprobes + TCP kprobes | 🔲 kernel side |
| Agent | eBPF binary build pipeline (CI) | 🔲 todo |

### Running the eBPF engine

```bash
# Linux only, requires kernel ≥ 5.8 and CAP_BPF / root
# Step 1 — compile the BPF C programs
cargo xtask build-ebpf --release

# Step 2 — run the agent in eBPF mode
sudo netscope-agent ebpf \
  --hub-url http://your-hub:8080 \
  --api-key YOUR_API_KEY
```

The agent will auto-detect `libssl.so` via `ldconfig`.
Pass `--libssl-path /usr/lib/.../libssl.so.3` to override.

---

## ✅ v0.3 — Intelligence Layer (shipped)

> **Theme**: Move from observation to detection.

| Area | Feature | Status |
|------|---------|--------|
| Agent | Kubernetes pod enrichment — reads `/proc/self/cgroup`, tags flows with `pod_name` + `k8s_namespace` | ✅ done |
| Agent | OS detection — reports `os` + `capture_mode` + `ebpf_enabled` in heartbeat | ✅ done |
| Agent | Heartbeat thread — periodic heartbeat (30s) in both pcap and eBPF modes | ✅ done |
| Hub | K8s columns in flows table (`pod_name`, `k8s_namespace`) | ✅ done |
| Hub | Agent fleet columns (`os`, `capture_mode`, `ebpf_enabled`) | ✅ done |
| Hub | Per-process network policy engine — `process_policies` table, CRUD API, violation log | ✅ done |
| Hub | Policy evaluation — checks eBPF flows against enabled policies at ingest, records violations | ✅ done |
| Hub | Threat Intel API — `GET /api/v1/threats` aggregates threat-scored IPs with process info | ✅ done |
| Hub | Alert test delivery — `POST /api/v1/alerts/:id/test` fires a test notification immediately | ✅ done |
| Hub | Flows Query includes `pod_name`, `k8s_namespace`, `threat_score`, `threat_level` | ✅ done |
| UI | Agent Fleet page — eBPF/pcap badge, OS badge, flow count/1h, upgrade callout | ✅ done |
| UI | Threat Intelligence page — threat IP table with score bars, severity badges, process column | ✅ done |
| UI | Process Policies page — rule builder, toggle, violations log | ✅ done |
| UI | Alert test delivery button — fires inline, shows ✓/✗ feedback per rule | ✅ done |
| UI | Flow table — pod name in Process column, threat level badge in Info column | ✅ done |
| UI | Service topology — threat score coloring on external nodes, process column (eBPF) | ✅ done |
| CI | Build workflow updated — all new features covered by existing test/build pipelines | ✅ done |

---

## 🔄 v0.4 — Enterprise Readiness (in progress)

> **Theme**: Deploy at scale, integrate with your stack.
>
> **Licensing**: Community edition stays MIT. Enterprise features
> (hub/enterprise/) are licensed under BSL-1.1 — free to self-host,
> not for resale as a competing managed service.

### Priority 1 — Identity & Access (shipped in this release)

| Area | Feature | Status |
|------|---------|--------|
| Hub | Multi-tenant `organisations` table + org isolation | ✅ done |
| Hub | `org_members` table — role assignments (owner/admin/analyst/viewer) | ✅ done |
| Hub | `teams` table + team membership scoping | ✅ done |
| Hub | `sso_configs` table — SAML/OIDC IdP metadata storage | ✅ done |
| Hub | JWT-based license engine — plan/feature/quota gating | ✅ done |
| Hub | Enterprise API: org, members, teams, SSO config, license endpoints | ✅ done |
| Hub | Phase 12 ClickHouse migrations (idempotent) | ✅ done |
| UI | Settings sub-nav layout (Tokens, Org, Members, Teams, SSO, License) | ✅ done |
| UI | Organisation settings page — name, agent quota, retention | ✅ done |
| UI | Members & Roles page — invite, role picker, remove | ✅ done |
| UI | Teams page — create, expand, add/remove members | ✅ done |
| UI | SSO configuration page — OIDC (Dex) + SAML 2.0 forms | ✅ done |
| UI | License & Plan page — feature matrix, plan badge, apply key | ✅ done |
| UI | EnterpriseGate component — upgrade prompt for locked features | ✅ done |
| UI | Sidebar Settings group — collapsible with Enterprise badges | ✅ done |
| Docs | BSL-1.1 license file (hub/enterprise/LICENSE) | ✅ done |

### Priority 2 — Auth Flows & Session Management (shipped in this release)

| Area | Feature | Status |
|------|---------|--------|
| Hub | OIDC authorisation-code flow — Dex / Okta / Azure AD / Google | ✅ done |
| Hub | SAML 2.0 SP handler — ACS, metadata endpoint, IdP-initiated redirect | ✅ done |
| Hub | Session store — httpOnly `ns_session` cookie, 24h TTL, in-memory store | ✅ done |
| Hub | Session-aware RBAC middleware — session role takes priority over API key | ✅ done |
| Hub | Local email/password login — bcrypt, `POST /enterprise/auth/login` | ✅ done |
| Hub | SCIM 2.0 user provisioning — full CRUD, filter, deprovision | ✅ done |
| Hub | Invite token flow — single-use 7-day tokens, `invite_tokens` table | ✅ done |
| Hub | Password reset flow — single-use 1-hour tokens, anti-enumeration 200 | ✅ done |
| Hub | Auth event audit logging — login/logout/invite written to `audit_events` | ✅ done |
| Hub | `GET /enterprise/auth/me` — current session identity endpoint | ✅ done |
| UI | Login page — OIDC + SAML SSO buttons + email/password form | ✅ done |
| UI | Accept-invite page — password set on first login | ✅ done |
| UI | Forgot-password page — request reset link | ✅ done |
| UI | Reset-password page — set new password via token | ✅ done |
| UI | UserMenu logout — `POST /enterprise/auth/logout` + redirect to `/login` | ✅ done |
| UI | Members page — invite link copy (no-SMTP), "you" badge, self-remove guard | ✅ done |

### Priority 3 — Integrations & Export (next)

| Area | Feature | Status |
|------|---------|--------|
| Hub | Audit log export — CEF, LEEF, JSON download endpoint | 🔲 next |
| Hub | Splunk HEC output — batch POST to Splunk HTTP Event Collector | 🔲 next |
| Hub | Elastic / ECS output — JSON lines to Logstash / Elasticsearch | 🔲 next |
| Hub | Datadog Logs output — POST to intake API | 🔲 next |
| Hub | Grafana Loki push — `/loki/api/v1/push` | 🔲 next |
| Hub | S3 / GCS long-term storage tier — hourly Parquet dumps | 🔲 planned |
| UI | Integrations settings page — enable/configure each SIEM sink | 🔲 next |
| UI | Audit log export button — date-range picker + format selector | 🔲 next |

### Priority 4 — Detection & Analytics (after P3)

| Area | Feature | Status |
|------|---------|--------|
| Hub | Sigma rule engine — parse YAML Sigma rules, evaluate at ingest | 🔲 planned |
| Hub | OpenTelemetry trace correlation — link flows to OTel trace IDs | 🔲 planned |
| Hub | Kafka consumer group scaling — horizontal ingest workers | 🔲 planned |
| UI | Saved queries + query history | 🔲 planned |
| UI | Sigma rule manager — upload, edit, trigger history | 🔲 planned |
| UI | OTel trace links — click flow → open Jaeger/Tempo side panel | 🔲 planned |

### Priority 5 — Platform Expansion (future)

| Area | Feature | Status |
|------|---------|--------|
| UI | Multi-cluster fleet overview | 🔲 planned |
| UI | Custom dashboard builder | 🔲 planned |
| Agent | Windows support — pcap via Npcap | 🔲 planned |
| Agent | eBPF for Go and Python native TLS libs | 🔲 planned |
| Integrations | Splunk HEC, Elastic Beats, Datadog Logs, Grafana Loki | 🔲 planned |

---

## Contributing

Bug reports and pull requests are welcome.
See [CONTRIBUTING.md](CONTRIBUTING.md) if it exists, or open an issue to discuss.
