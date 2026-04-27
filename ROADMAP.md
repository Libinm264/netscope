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

### Priority 3 — Integrations & Export ✅ shipped

| Area | Feature | Status |
|------|---------|--------|
| Hub | Audit log export — CEF, LEEF, JSON download endpoint | ✅ done |
| Hub | Splunk HEC output — batch POST to Splunk HTTP Event Collector | ✅ done |
| Hub | Elastic / ECS output — JSON lines to Logstash / Elasticsearch | ✅ done |
| Hub | Datadog Logs output — POST to intake API | ✅ done |
| Hub | Grafana Loki push — `/loki/api/v1/push` | ✅ done |
| Hub | SIEM dispatcher — background goroutine, 30s poll, `last_shipped` watermark per sink | ✅ done |
| Hub | `integrations_config` ClickHouse table (Phase 13) | ✅ done |
| Hub | Secret redaction on List — "token", "api_key", "password" → `***` | ✅ done |
| Hub | Integration test endpoint — lightweight health-check with latency | ✅ done |
| Hub | S3 / GCS long-term storage tier — hourly Parquet dumps | 🔲 planned |
| UI | Integrations settings page — collapsible sink cards, toggle, test/save | ✅ done |
| UI | Audit log export toolbar — format picker (JSON/CEF/LEEF) + quick range + Download | ✅ done |

### Priority 4 — Detection & Analytics 🔄 in progress

> **Tier decisions**: Sigma rules → Enterprise (detection budget); OTel correlation → Community
> (CNCF ecosystem adoption driver); Saved queries → Community (QoL, increases stickiness);
> Kafka group scaling → Enterprise (horizontal scale deployments).

| Area | Feature | Tier | Status |
|------|---------|------|--------|
| Hub | `trace_id` column on `flows` (Phase 14 ALTER TABLE) | Community | ✅ done |
| Hub | OTel trace correlation — accept `trace_id` on ingest, filter in GET /flows | Community | ✅ done |
| Hub | Kafka consumer group ID — `KAFKA_GROUP_ID` env var, `KafkaGroupID` in config | Enterprise | ✅ done |
| Hub | `saved_queries` table (Phase 15) — ReplacingMergeTree, soft-delete | Community | ✅ done |
| Hub | Saved queries CRUD API — `GET/POST/PATCH/DELETE /api/v1/saved-queries` | Community | ✅ done |
| Hub | Community quota enforcement — max 10 saved queries on Community plan | Community | ✅ done |
| Hub | `sigma_rules` table (Phase 16) + `sigma_matches` table (Phase 17) | Community/Enterprise | ✅ done |
| Hub | 5 built-in detection rules seeded at startup (Phase 17b) | Community | ✅ done |
| Hub | Sigma engine — evaluates enabled rules every 5 min, records matches | Community/Enterprise | ✅ done |
| Hub | Sigma CRUD API — `GET/POST/PATCH/DELETE /enterprise/sigma/rules` | Enterprise CRUD | ✅ done |
| Hub | Sigma matches API — `GET /enterprise/sigma/matches` | Community | ✅ done |
| Hub | Enterprise gate — built-in rules readable by all; custom rules Enterprise-only | ✅ done | ✅ done |
| UI | OTel trace ID filter in Flow Explorer — `trace_id` input field | Community | ✅ done |
| UI | Saved queries — Save button dropdown, recall, delete in Flow Explorer | Community | ✅ done |
| UI | Detection Rules page `/sigma` — rules list, match history, new-rule form | Community/Enterprise | ✅ done |
| UI | Sidebar — "Detection" nav item (ScanSearch icon) | ✅ done | ✅ done |

---

## 🔄 v0.5 — Fleet Intelligence (in progress)

> **Theme**: Own the fleet. Every OS, every cloud, one pane of glass.
>
> **Build order**: Hub (F1–F4) → Agent (F5–F6) → Desktop (F7–F8)

### F1 — Cloud VPC Flow Log Ingestion

| Area | Feature | Tier | Status |
|------|---------|------|--------|
| Hub | Cloud source config CRUD — `cloud_flow_sources` table (Phase 22) | Community | 🔄 in progress |
| Hub | Pull audit log — `cloud_flow_pull_log` table (Phase 23) | Community | 🔄 in progress |
| Hub | `flows.source` column — distinguishes agent-push vs cloud-pull (Phase 24) | Community | 🔄 in progress |
| Hub | AWS VPC Flow Logs ingestion — S3 + CloudWatch Logs poll | Community | 🔄 in progress |
| Hub | GCP VPC Flow Logs ingestion — Pub/Sub pull | Enterprise | 🔄 in progress |
| Hub | Azure NSG Flow Logs ingestion — Blob Storage / Event Hub | Enterprise | 🔄 in progress |
| Hub | Cloud source handler — CRUD + manual trigger + pull log API | Community | 🔄 in progress |
| UI | Cloud Sources management page `/cloud` | Community | 🔲 todo |

### F2 — Multi-Cluster Fleet Overview

| Area | Feature | Tier | Status |
|------|---------|------|--------|
| Hub | `agent_configs` table — remote config push (Phase 25) | Community | 🔄 in progress |
| Hub | `agents.config_version` column (Phase 26) | Community | 🔄 in progress |
| Hub | Fleet cluster grid API — aggregated per-cluster health | Community | 🔄 in progress |
| Hub | Cross-cluster flow search API | Community | 🔄 in progress |
| Hub | Remote config push + agent config poll + ack API | Community | 🔄 in progress |
| Agent | `poll_config` + `ack_config` on heartbeat cycle | Community | 🔲 todo |
| UI | Fleet health page `/fleet` — cluster cards + agent grid | Community | 🔲 todo |

### F3 — Compliance Dashboard & Scheduled Reports

| Area | Feature | Tier | Status |
|------|---------|------|--------|
| Hub | `compliance_report_schedules` table (Phase 27) | Enterprise | 🔄 in progress |
| Hub | `compliance_report_runs` audit log (Phase 28) | Enterprise | 🔄 in progress |
| Hub | SOC 2 / PCI-DSS / HIPAA query bundles | Enterprise | 🔄 in progress |
| Hub | PDF + CSV renderer (gofpdf) | Enterprise | 🔄 in progress |
| Hub | Compliance scheduler — 5-min cron loop | Enterprise | 🔄 in progress |
| Hub | Compliance report CRUD + manual run + preview API | Enterprise | 🔄 in progress |
| UI | Compliance report schedules page `/compliance/reports` | Enterprise | 🔲 todo |

### F4 — Incident Workflow Integration

| Area | Feature | Tier | Status |
|------|---------|------|--------|
| Hub | `incidents` table — in-hub incident timeline (Phase 29) | Enterprise | 🔄 in progress |
| Hub | `incident_workflow_config` table (Phase 30) | Enterprise | 🔄 in progress |
| Hub | Incident dispatcher — hooks Sigma engine on match | Enterprise | 🔄 in progress |
| Hub | Jira REST API v3 ticket creation | Enterprise | 🔄 in progress |
| Hub | Linear GraphQL ticket creation | Enterprise | 🔄 in progress |
| Hub | PagerDuty + OpsGenie routing from Sigma matches | Enterprise | 🔄 in progress |
| Hub | Incident CRUD + ack/resolve/notes API | Enterprise | 🔄 in progress |
| UI | Incidents timeline page `/incidents` | Enterprise | 🔲 todo |

### F5 — Windows Agent (Npcap)

| Area | Feature | Tier | Status |
|------|---------|------|--------|
| Agent | `capture-windows` crate — Npcap backend | Enterprise | 🔲 todo |
| Agent | Windows process enrichment (Toolhelp32Snapshot) | Enterprise | 🔲 todo |
| Agent | `#[cfg(windows)]` capture dispatch in main agent | Enterprise | 🔲 todo |
| Agent | MSI installer (WiX) bundling wpcap.dll | Enterprise | 🔲 todo |

### F6 — eBPF for Go + Python TLS

| Area | Feature | Tier | Status |
|------|---------|------|--------|
| Agent | eBPF uprobes for Go `crypto/tls` (Write + Read) | Community | 🔲 todo |
| Agent | eBPF uprobes for Python `ssl.SSLSocket` | Community | 🔲 todo |
| Agent | Go binary symbol resolver (iterate /proc/*/exe) | Community | 🔲 todo |
| Agent | `--enable-go-tls` + `--enable-python-ssl` CLI flags | Community | 🔲 todo |

### F7 — Fleet View in Desktop

| Area | Feature | Tier | Status |
|------|---------|------|--------|
| Desktop | `FleetPane` component — cluster grid (requires hub connection) | Community | 🔲 todo |
| Desktop | `get_fleet_summary` + `get_agent_list` Tauri commands | Community | 🔲 todo |
| Desktop | Fleet tab in bottom tab bar | Community | 🔲 todo |

### F8 — OTel Trace Side Panel in Desktop

| Area | Feature | Tier | Status |
|------|---------|------|--------|
| Desktop | `OtelTracePanel` — slide-in drawer with Jaeger/Tempo webview | Community | 🔲 todo |
| Desktop | `trace_id` dot indicator on flow rows | Community | 🔲 todo |
| Desktop | `get_otel_backend_url` Tauri command | Community | 🔲 todo |

---

## DB Migration Summary (v0.5 Phases 22–30)

| Phase | Change | Feature |
|-------|--------|---------|
| 22 | `cloud_flow_sources` CREATE | F1 |
| 23 | `cloud_flow_pull_log` CREATE | F1 |
| 24 | `flows ADD COLUMN source` | F1 |
| 25 | `agent_configs` CREATE | F2 |
| 26 | `agents ADD COLUMN config_version` | F2 |
| 27 | `compliance_report_schedules` CREATE | F3 |
| 28 | `compliance_report_runs` CREATE | F3 |
| 29 | `incidents` CREATE | F4 |
| 30 | `incident_workflow_config` CREATE | F4 |

---

## Contributing

Bug reports and pull requests are welcome.
See [CONTRIBUTING.md](CONTRIBUTING.md) if it exists, or open an issue to discuss.
