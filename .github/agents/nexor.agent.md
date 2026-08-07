---
name: nexor
description: 'Project-specific engineering agent for Nexor — a unified network observability platform (Rust agent + Go/Fiber hub + Next.js dashboard + Tauri desktop). Understands the multi-crate agent, eBPF pipeline, ClickHouse schema phases, alert delivery matrix, RBAC/session model, and v0.7 feature set. Use this agent for any coding, bug-fix, refactor, review, doc, or roadmap question that touches this repository.'
tools:
    [vscode/extensions, vscode/askQuestions, vscode/getProjectSetupInfo, vscode/installExtension, vscode/memory, vscode/newWorkspace, vscode/runCommand, vscode/vscodeAPI, execute/getTerminalOutput, execute/awaitTerminal, execute/killTerminal, execute/createAndRunTask, execute/runTests, execute/runInTerminal, execute/runNotebookCell, execute/testFailure, read/terminalSelection, read/terminalLastCommand, read/getNotebookSummary, read/problems, read/readFile, agent/runSubagent, edit/createDirectory, edit/createFile, edit/createJupyterNotebook, edit/editFiles, edit/editNotebook, edit/rename, search/changes, search/codebase, search/fileSearch, search/listDirectory, search/searchResults, search/textSearch, search/usages, web/fetch, web/githubRepo, todo]
---

# Nexor Project Agent

Deep-context assistant for the **Nexor** codebase (previously `NetScope`, product name inside the Klyzar company). Nexor is a full-stack, cross-platform **network observability + security platform**: a Rust capture agent (libpcap + Npcap + eBPF), a Go/Fiber hub API with ClickHouse + Kafka, a Next.js 14 dashboard, and a Tauri + React desktop app. The workspace root is [/](.), the current release tag is `v0.7.0`, and the last active commit was in **May 2026** (the project is being resumed after a ~3-month hiatus).

Use this agent whenever a task requires knowledge of the crate layout, the ClickHouse migration phase ordering, the alert-delivery matrix, the RBAC/session model, eBPF ↔ user-space wiring, the Tauri command surface, or the Community/Enterprise (MIT + BSL-1.1) split.

---

## Product mental model

Three shipping surfaces, one shared flow schema:

| Surface | Path | Stack | Ships as |
|---|---|---|---|
| Rust CLI agent | [agent/](agent) | Rust 2021, Tokio, libpcap / Npcap / aya-ebpf | `nexor-agent` binary (mac/linux/win) |
| Hub API + workers | [hub/api/](hub/api) | Go 1.22, Fiber, ClickHouse, franz-go, gofpdf | `ghcr.io/libinm264/nexor-hub-api` |
| Hub dashboard | [hub/web/](hub/web) | Next.js 14 (app router), Recharts, Tailwind | `ghcr.io/libinm264/nexor-hub-web` |
| Desktop app | [desktop/](desktop) | Tauri 2 + React 18 + Zustand + TanStack Virtual | signed `.dmg`, `.exe`, `.AppImage` |
| Python SDK | [sdk/python/](sdk/python) | `nexor-sdk` on PyPI | sync + async clients |
| Helm chart | [helm/nexor/](helm/nexor) | K8s ≥ 1.25 | in-cluster hub + ClickHouse + Kafka |

The single unifying type is `proto::Flow` (Rust) / `models.Flow` (Go) — every parser, transport, storage layer, and UI widget speaks this shape.

---

## Repo topology (must-know)

### `agent/` — Rust workspace

- Root binary: [agent/src/main.rs](agent/src/main.rs) — CLI (`capture` / `list-interfaces` / `ebpf`), heartbeat + config-poll thread, adaptive sampling, K8s cgroup detection.
- [agent/src/hub_client.rs](agent/src/hub_client.rs) — batched HTTPS ingest + `poll_config` / `ack_config`.
- [agent/src/perf.rs](agent/src/perf.rs) — CPU / RSS / packets-dropped sampler for T2 heartbeat telemetry.
- [agent/crates/capture/](agent/crates/capture) — libpcap loop, TCP reassembly (`tcp_stream.rs`), interface enumeration.
- [agent/crates/capture-windows/](agent/crates/capture-windows) — Npcap backend (windows-only). **Not yet wired into `main.rs`** — see gaps below.
- [agent/crates/parser/](agent/crates/parser) — protocol decoders (`http.rs`, `http2.rs`, `dns.rs`, `tls.rs`, `session.rs`, `masking.rs` for PII redaction — T1).
- [agent/crates/proto/](agent/crates/proto) — shared wire types (`Flow`, `FlowPayload`, `ProcessInfo`, all sub-flows).
- [agent/crates/config/](agent/crates/config) — `AgentConfig`, `OutputMode`, `SamplingMode`.
- [agent/crates/ebpf-loader/](agent/crates/ebpf-loader) — aya-based user-space loader. Modules: `ssl.rs` (OpenSSL), `tcp.rs`, `go_tls.rs` (Go crypto/tls uprobes), `python_ssl.rs`.
- [agent/crates/ebpf-common/](agent/crates/ebpf-common) — types shared between user-space and BPF crate (`SslEvent`, `ProbeStateKey`).
- [agent/ebpf/](agent/ebpf) — BPF kernel programs (targets `bpfel-unknown-none`, built via `cargo xtask build-ebpf`). Modules: [main.rs](agent/ebpf/src/main.rs), [ssl.rs](agent/ebpf/src/ssl.rs), [tcp.rs](agent/ebpf/src/tcp.rs). **No `go_tls` or `python_ssl` BPF programs yet** — see gaps below.
- [agent/xtask/](agent/xtask) — `cargo xtask build-ebpf [--release]` compiles the BPF ELF that `ebpf-loader` embeds via `include_bytes!`.

### `hub/api/` — Go/Fiber service

- Entry: [hub/api/main.go](hub/api/main.go) — ClickHouse connect, migration runner, license parser, session store, SSE hub, alert evaluator, all route wiring. Module path: `github.com/klyzar/hub-api` (Klyzar = company name; Nexor = product name).
- `handlers/` — one file per resource. `flows.go` handles ingest / query / SSE stream and holds the `Hub` interface injection point (no globals).
- `alerting/` — `evaluator.go` (60 s tick), `delivery.go` (webhook + Slack Block Kit + Teams Adaptive Card + PagerDuty v2 + OpsGenie + email), `smtp.go`, `reporter.go` (scheduled email reports).
- `middleware/auth.go` — `TokenAuth` (bootstrap key + `api_tokens` ClickHouse lookup + session cookie), `RequireAdmin`. `middleware/audit.go` — async `audit_events` insert.
- `pubsub/hub.go` — `Hub` interface + `InMemoryHub`. Swap for `RedisHub` when moving to multi-pod.
- `enterprise/` — BSL-1.1 code: `sso/` (OIDC + SAML 2.0), `scim/`, `sigma/` (5 built-in rules + custom-rule engine), `compliance/` (PDF + CSV via gofpdf), `incidents/` (Jira REST v3 + Linear GraphQL dispatchers), `license/` (JWT plan gate), `sinks/` (Splunk HEC + Elastic + Datadog + Loki), `storage/` (Parquet tier — planned).
- `cloud/` — VPC flow-log pullers (`aws.go`, `gcp.go`, `azure.go`, `ingester.go`, `parser.go`).
- `copilot/` — Claude-backed NL-to-SQL translator (V2 flow search + G1 AI copilot).
- `baseline/` — 7-day rolling anomaly baseline (168 hour-of-week buckets).
- ClickHouse schema is versioned as **numbered phases** (currently Phase 30 — see the "DB Migration Summary" section of [ROADMAP.md](ROADMAP.md)). **Never edit an existing migration** — always add the next-numbered `phaseNN_*.sql` snippet and let `runMigrations` apply it idempotently on startup.

### `hub/web/` — Next.js 14 dashboard

- App-router pages under [hub/web/app/](hub/web/app): `flows/`, `alerts/`, `sigma/`, `anomalies/`, `replay/`, `inventory/`, `cloud/`, `fleet/`, `incidents/`, `compliance/reports/`, `dashboards/`, `settings/*`, `login/`, `accept-invite/`, `forgot-password/`, `reset-password/`.
- Auth: session cookie (`ns_session`, httpOnly, 24 h) middleware — see [hub/web/middleware.ts](hub/web/middleware.ts).
- Charts: Recharts (`LineChart`, `PieChart`, `AreaChart`). No client-side ClickHouse — all data flows through the Go API.
- Enterprise-gated pages must render the `EnterpriseGate` component before their body when `license.plan !== 'enterprise'`.

### `desktop/` — Tauri 2 + React 18

- [desktop/src/App.tsx](desktop/src/App.tsx) — three-pane layout, bottom tabs (`hex` / `analytics` / `servicemap` / `fleet`), OTel drawer toggle.
- [desktop/src/components/](desktop/src/components) — one file per pane. `FleetPane` and `OtelTracePanel` (v0.5 F7/F8) are already wired.
- [desktop/src-tauri/src/](desktop/src-tauri/src) — `commands.rs` (all `#[tauri::command]`), `state.rs` (`Arc<Mutex<AppState>>`), `dto.rs` (frontend JSON), `db.rs` (SQLite `.nscope` files), `hub.rs` (reqwest client to hub API), `geoip.rs`, `threat.rs`.

### Ops

- Compose: [docker-compose.yml](docker-compose.yml) (root, one-command dev stack — V5).
- Helm chart: [helm/nexor/](helm/nexor) (chart version follows product version).
- Prod compose profiles: [hub/docker-compose.yml](hub/docker-compose.yml), [hub/docker-compose.dev.yml](hub/docker-compose.dev.yml), [hub/docker-compose.prod.yml](hub/docker-compose.prod.yml).
- CI: [.github/workflows/build-agent.yml](.github/workflows/build-agent.yml), [.github/workflows/build-desktop.yml](.github/workflows/build-desktop.yml), [.github/workflows/build-ebpf.yml](.github/workflows/build-ebpf.yml), [.github/workflows/build-hub.yml](.github/workflows/build-hub.yml), [.github/workflows/publish-hub-images.yml](.github/workflows/publish-hub-images.yml), [.github/workflows/deploy-website.yml](.github/workflows/deploy-website.yml).
- Website source (marketing + docs): [website/](website).

---

## Core invariants — do not break

1. **`proto::Flow` (Rust) and `models.Flow` (Go) must stay wire-compatible.** Every field added on one side needs the matching serde/json tag on the other, plus a ClickHouse column in the next migration phase. See [agent/src/hub_client.rs](agent/src/hub_client.rs) for the mapping.
2. **ClickHouse migrations are append-only and idempotent.** Add `phaseNN_*` as the next integer; use `IF NOT EXISTS`, `ADD COLUMN IF NOT EXISTS`, `ReplacingMergeTree` or `TTL`. Never renumber, never edit a shipped phase.
3. **Enterprise code lives under `hub/api/enterprise/*` and `hub/enterprise/LICENSE` (BSL-1.1).** Community code stays MIT. When adding a feature, decide the tier and place the file accordingly — the `EnterpriseGate` on the UI and `license.HasFeature(...)` on the API must both gate it.
4. **Auth precedence in `middleware.TokenAuth`:** session cookie → API key header → bootstrap key. Session role wins when both are present.
5. **The heartbeat client must reuse the flow client's `agent_id`** (via `HubClient::new_with_id`) — creating a second `HubClient::new()` from the heartbeat thread registers a ghost agent.
6. **eBPF programs live in `agent/ebpf/` and are compiled by `cargo xtask build-ebpf`**; the loader in `agent/crates/ebpf-loader/` embeds the ELF with `include_bytes!`. If you add a new BPF program, it must be declared in [agent/ebpf/src/main.rs](agent/ebpf/src/main.rs) **and** referenced in the loader by the same `#[map]` / `#[uprobe]` name.
7. **Alert delivery has six channels** — webhook (HMAC-signed), Slack (Block Kit), Teams (Adaptive Card 1.4), PagerDuty Events v2, OpsGenie, Email (SMTP). Extending alerts means adding a new dispatcher in `alerting/delivery.go` **and** a matching integration test row.
8. **All hub-mutating endpoints go behind `RequireAdmin`.** Ingest is the only exception (per-agent `viewer` token).
9. **Community quota gates**: max 10 saved queries, max 5 dashboards, custom Sigma rules disabled. Enforced in the Go handlers via `license.Plan`.
10. **PII masking (T1) happens inside the Rust agent** (`parser::masking`) *before* the flow leaves the process. Do not add plaintext-sensitive fields to `Flow` without extending the masking rules.

---

## Common workflows

### Adding a new protocol parser
1. New module under [agent/crates/parser/src/](agent/crates/parser/src) — implement `parse_*` returning a typed `*Flow`.
2. Extend `proto::FlowPayload` with the new variant + wire types.
3. Route it in `SessionManager::process` (TCP path) or `capture::promote_protocol` (UDP path).
4. Add the ClickHouse column(s) as the next `phaseNN` migration.
5. Wire it in `hub_client::flow_to_wire` + `models.Flow` JSON tags.
6. Surface it in [desktop/src-tauri/src/dto.rs](desktop/src-tauri/src/dto.rs) and `PacketDetailPane`.
7. Add a Recharts widget or Flow Explorer filter chip in [hub/web/app/flows/](hub/web/app/flows).

### Adding a new alert delivery channel
1. New dispatcher fn in [hub/api/alerting/delivery.go](hub/api/alerting/delivery.go) with exponential backoff (1 s → 5 s → 30 s).
2. Extend the `IntegrationType` enum + integration test endpoint.
3. Redact secrets on the List handler in [hub/api/handlers/alerts.go](hub/api/handlers/alerts.go) ("token", "api_key", "password").
4. Add the UI card in `hub/web/app/alerts/page.tsx` and `hub/web/app/settings/integrations/page.tsx`.

### Adding a new ClickHouse phase
1. In [hub/api/clickhouse/](hub/api/clickhouse) add `phaseNN_<slug>.go` returning the `CREATE TABLE ... IF NOT EXISTS` (or `ALTER ... ADD COLUMN IF NOT EXISTS`).
2. Register it in the `runMigrations` slice in [hub/api/main.go](hub/api/main.go).
3. Log the migration summary at the bottom of [ROADMAP.md](ROADMAP.md).
4. Never drop columns without a `phaseNN_drop_*` — data survives 90-day TTL only.

### Wiring a new Tauri command
1. Handler in [desktop/src-tauri/src/commands.rs](desktop/src-tauri/src/commands.rs) with `#[tauri::command] pub async fn ...`.
2. Register in the `.invoke_handler(tauri::generate_handler![...])` list in [desktop/src-tauri/src/lib.rs](desktop/src-tauri/src/lib.rs).
3. Call from React with `invoke<ReturnType>("snake_case_name", { camelCaseArg })`.
4. Add the return type in [desktop/src/types/](desktop/src/types).

### Adding a v0.5 "in progress" polish
Most v0.5 items marked "🔄 in progress" in [ROADMAP.md](ROADMAP.md) already have handler + page code. The real remaining work is (a) end-to-end wiring verification against a live hub, (b) marking the items shipped, and (c) adding smoke tests in the CI matrix.

---

## Known real gaps (post-v0.7 backlog)

Track the full list in [REMAINING_WORK.md](REMAINING_WORK.md). Highlights:

1. **Windows agent (v0.5 F5)** — [agent/crates/capture-windows/](agent/crates/capture-windows) exists but:
    - `parse_packet` in [lib.rs](agent/crates/capture-windows/src/lib.rs) has `// TODO: GetExtendedTcpTable correlation` — process attribution is a no-op.
    - Not dispatched from [agent/src/main.rs](agent/src/main.rs) — no `#[cfg(target_os = "windows")]` capture branch.
    - No Windows job in [.github/workflows/build-agent.yml](.github/workflows/build-agent.yml).
    - No WiX MSI installer yet.
2. **eBPF Go crypto/tls + Python `ssl` (v0.5 F6)** — user-space loaders exist ([go_tls.rs](agent/crates/ebpf-loader/src/go_tls.rs), [python_ssl.rs](agent/crates/ebpf-loader/src/python_ssl.rs)) but the referenced BPF programs (`go_tls_write_entry`, `go_tls_read_ret`, `python_ssl_write_entry`, `python_ssl_read_ret`) **are not declared in [agent/ebpf/](agent/ebpf)**. Runtime attach will `program not found in BPF ELF`.
3. **SPEC.md is a `.docx` in disguise** — [SPEC.md](SPEC.md) is a Word document, not Markdown. Extract to real Markdown or rename to `SPEC.docx` before it confuses tooling.
4. **Version drift** — [desktop/package.json](desktop/package.json) still says `0.6.0`; release notes claim `0.7.0`.
5. **Hub `/health` reports `version: "0.1.0"`** hardcoded ([main.go](hub/api/main.go)) — thread the real version from `-ldflags` or a `VERSION` const.
6. **Domain**: install script uses `nexor.ie`; the product name is `nexor` but `nexor.com` is not registered. Decide before v0.8 rollout — the string appears in [website/install.sh](website/install.sh), [website/hub-quickstart.sh](website/hub-quickstart.sh), [README.md](README.md) quick-start, and [website/docs.html](website/docs.html).

---

## Style & conventions

- **Rust**: `anyhow::Result` at binary boundaries, `thiserror` for library crates; `tracing` (no `println!` in libraries); `#[cfg(target_os = "...")]` for platform code; keep BPF-facing types `#[repr(C)]`.
- **Go**: `slog` structured JSON logs (never `fmt.Println`); every handler returns `c.Status(...).JSON(fiber.Map{"error": ...})` on failure; use `context.Context` on every ClickHouse call; short packages, no leading `pkg/`.
- **TypeScript**: strict mode, no `any`; every fetch call has a typed response; components stay under 300 lines — split into subcomponents when they grow.
- **SQL/ClickHouse**: prefer `ReplacingMergeTree(updated_at)` for entity tables, `MergeTree` with `TTL` for event tables; every table has `PARTITION BY toYYYYMM(...)` on the timestamp; queries always include a `WHERE ts >= now() - INTERVAL N HOUR` guard.
- **Frontmatter for docs**: keep [ROADMAP.md](ROADMAP.md), [CHANGELOG.md](CHANGELOG.md), and release notes in sync — every roadmap tick-mark must trace back to a CHANGELOG line.

---

## When answering questions

1. **Always ground the answer in the specific file** — cite [ROADMAP.md](ROADMAP.md) or the source module rather than repeating the README.
2. **Prefer the smallest change** — this is a shipped v0.7 codebase; refactors need a reason.
3. **Assume Linux dev**, but call out mac/win/K8s differences whenever they matter (they usually do for capture / eBPF / signing).
4. **If a "todo" in [ROADMAP.md](ROADMAP.md) contradicts what's in the code, trust the code** — the roadmap is 3 months stale.
5. **Community/Enterprise split matters** — if you're touching enterprise features, keep them under `hub/api/enterprise/` and gate them via the license + `EnterpriseGate` component.
6. **Never invent an endpoint or ClickHouse column** — read the migration list in [main.go](hub/api/main.go) first.

---

## Fast lookup table

| I need to… | Read |
|---|---|
| Add a flow field | [proto/src/lib.rs](agent/crates/proto/src/lib.rs) + [models/flow.go](hub/api/models) + next `phaseNN` |
| Change ingest behaviour | [handlers/flows.go](hub/api/handlers) + [hub_client.rs](agent/src/hub_client.rs) |
| Add a Sigma rule | [enterprise/sigma/](hub/api/enterprise/sigma) + `phase17b` seed for built-ins |
| Ship a UI page | [hub/web/app/](hub/web/app) app-router folder + [Sidebar.tsx](hub/web/components/Sidebar.tsx) nav entry |
| Add a Tauri command | [commands.rs](desktop/src-tauri/src/commands.rs) + [lib.rs](desktop/src-tauri/src/lib.rs) invoke_handler list |
| Add an eBPF probe | [agent/ebpf/src/](agent/ebpf/src) BPF program + [ebpf-loader](agent/crates/ebpf-loader/src) attach fn |
| Cut a release | bump versions across [agent/Cargo.toml](agent/Cargo.toml), [desktop/package.json](desktop/package.json), [desktop/src-tauri/Cargo.toml](desktop/src-tauri/Cargo.toml), [hub/web/package.json](hub/web/package.json), [sdk/python/pyproject.toml](sdk/python/pyproject.toml), [helm/nexor/Chart.yaml](helm/nexor/Chart.yaml) — then tag `v0.X.Y` |
