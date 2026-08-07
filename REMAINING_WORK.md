# Nexor — Remaining Work (post-hiatus resume plan)

> Written 2026-08-06, after a ~3-month pause. Last active commit: `6587237` on
> **2026-05-12** (v0.7.0 shipped). This document is the single source of truth for
> what still needs to happen; the historical `ROADMAP.md` stays intact for
> record-keeping and is now partially out of date.
>
> Reconcile [ROADMAP.md](./ROADMAP.md) with this file as items complete.

---

## Legend

| Icon | Meaning |
|---|---|
| ✅ | Actually shipped in code today (regardless of what ROADMAP says) |
| 🟡 | Partially implemented — scaffolding exists but is not runtime-complete |
| 🔲 | Not started |
| ⚠️ | Blocking or high-risk — do this first |

Every item lists the concrete files to touch. Effort is `S = < 1 day`,
`M = 1–3 days`, `L = 1–2 weeks`.

---

## 0. Reconciliation with ROADMAP.md

The `ROADMAP.md` written in April/May 2026 marks several items as `🔲 todo`
that are, in fact, already implemented. Reality check before starting anything
new:

| ROADMAP claim | Actual status | Evidence |
|---|---|---|
| `poll_config` + `ack_config` on heartbeat cycle | ✅ done | [agent/src/main.rs](agent/src/main.rs) L346 & [hub_client.rs](agent/src/hub_client.rs) L275 |
| Fleet health page `/fleet` | ✅ done | [hub/web/app/fleet/page.tsx](hub/web/app/fleet/page.tsx) (251 lines) |
| Cloud Sources page `/cloud` | ✅ done | [hub/web/app/cloud/page.tsx](hub/web/app/cloud/page.tsx) (334 lines) |
| Incidents timeline page `/incidents` | ✅ done | [hub/web/app/incidents/page.tsx](hub/web/app/incidents/page.tsx) (306 lines) |
| Compliance report schedules page | ✅ done | [hub/web/app/compliance/reports/page.tsx](hub/web/app/compliance/reports/page.tsx) (376 lines) |
| `FleetPane` in desktop (F7) | ✅ done | [desktop/src/components/FleetPane.tsx](desktop/src/components/FleetPane.tsx) |
| `OtelTracePanel` in desktop (F8) | ✅ done | [desktop/src/components/OtelTracePanel.tsx](desktop/src/components/OtelTracePanel.tsx) |
| `get_fleet_clusters`/`get_fleet_agents` Tauri cmds | ✅ done | [desktop/src-tauri/src/commands.rs](desktop/src-tauri/src/commands.rs) L388–L413 |
| `get_otel_backend_url`/`set_otel_backend_url` | ✅ done | [desktop/src-tauri/src/commands.rs](desktop/src-tauri/src/commands.rs) L421 |
| eBPF CI build pipeline | ✅ done | [.github/workflows/build-ebpf.yml](.github/workflows/build-ebpf.yml) |
| eBPF section in README | ✅ done | ROADMAP itself + [CHANGELOG.md](CHANGELOG.md) v0.5 section |
| Hub-side F1–F4 (cloud pull, fleet config API, compliance scheduler, incidents/Jira/Linear dispatch) | ✅ done | [hub/api/cloud/](hub/api/cloud), [hub/api/handlers/fleet.go](hub/api/handlers/fleet.go), [hub/api/enterprise/compliance/scheduler.go](hub/api/enterprise/compliance/scheduler.go), [hub/api/enterprise/incidents/dispatcher.go](hub/api/enterprise/incidents/dispatcher.go) — verified real SDK calls (AWS/GCP/Azure), not stubs |

**Status: DONE (2026-08-06).** [ROADMAP.md](ROADMAP.md) has been updated in place —
all confirmed-shipped rows above now read `✅ done` there, and the F5/F8 rows
were corrected to reflect the two genuinely partial items found below.

**New gaps found during this reconciliation pass** (not previously tracked):

| # | Item | Effort | Files |
|---|---|---|---|
| N1 | **`desktop/src-tauri/Cargo.lock` name/version fossil** — fixed cosmetically (`netscope-desktop`/`0.1.0` → `nexor-desktop`/`0.7.0`) via direct text edit since it's a local path-package entry (no registry checksum to invalidate). **Still needs a real `cargo build` once Rust is installed on a dev machine** — the lockfile is also missing two newer deps (`tauri-plugin-updater`, `tauri-plugin-process`) that are in [Cargo.toml](desktop/src-tauri/Cargo.toml) but never got resolved into the lock. This self-heals automatically on the next `cargo build` / `cargo tauri dev` — no manual lockfile surgery needed beyond what's already been done. | S | [desktop/src-tauri/Cargo.lock](desktop/src-tauri/Cargo.lock) — ✅ cosmetic part done 2026-08-06 |
| N2 | **F8's `trace_id` dot indicator on flow rows was never built** — the OTel drawer toggle and backend-URL command exist, but `PacketListPane` rows carry no per-row visual indicator that a flow has trace headers (only the toolbar toggle button glows). Genuinely a small remaining task. | S | [desktop/src/components/PacketListPane.tsx](desktop/src/components/PacketListPane.tsx), reuse `hasTraceHeaders()` logic already in [desktop/src/App.tsx](desktop/src/App.tsx) L45 |
| N3 | **The original founding spec (recovered from [SPEC.docx](SPEC.docx)) envisioned DB wire-protocol decoding** — PostgreSQL, MySQL, Redis, Kafka, AMQP — as a core desktop feature. **This was never built.** The shipped [parser/](agent/crates/parser/src) crate only decodes HTTP/1.1, HTTP/2, gRPC, DNS, TLS, ICMP, ARP. This is a genuinely differentiated, unbuilt feature worth considering for the wedge/roadmap discussion — no other cheap self-hosted tool does passive DB query visibility via network capture. | L | new crate under [agent/crates/parser/src/](agent/crates/parser/src), see [SPEC.md](SPEC.md) §4.2.3 for original scope |
| N4 | **Go toolchain + testcontainers-go added to `hub/api`** (2026-08-07) to write real §4 V1–V4 integration tests. Go 1.25 was not pre-installed on this dev machine — installed locally to `~/go-toolchain` (no sudo). `hub/api/go.mod`/`go.sum` now include `github.com/testcontainers/testcontainers-go`. **Dev-environment note:** on a JNJ/Zscaler-restricted machine, `testcontainers-go`'s Docker registry pull for `clickhouse/clickhouse-server:24.3-alpine` is blocked — the tests compile and link cleanly but must actually be *run* on an unrestricted machine (see §4 below for the exact commands). | — | done, no further action |

---

## 1. ⚠️ Housekeeping (do this in day one back)

| # | Item | Effort | Status |
|---|---|---|---|
| H1 | **[SPEC.md](SPEC.md) is a Word `.docx`, not Markdown.** | S | ✅ **done (2026-08-06)** — original binary preserved via `git mv` as [SPEC.docx](SPEC.docx); XML content extracted (including 11 embedded tables), product name normalised `NetScope` → `Nexor`, and a new [SPEC.md](SPEC.md) written with an editorial preamble explaining it's the original founding vision doc, not the current shipped spec, plus a call-out list of what was envisioned but never built (see N3 below — DB protocol decoding is the standout). |
| H2 | **Version drift.** | S | ✅ **done** — [desktop/package.json](desktop/package.json), [desktop/src-tauri/Cargo.toml](desktop/src-tauri/Cargo.toml) bumped to `0.7.0`; [agent/src/main.rs](agent/src/main.rs) now reads `env!("CARGO_PKG_VERSION")`; [hub/api/main.go](hub/api/main.go) has a build-time-overridable `var version = "dev"` wired through [hub/api/Dockerfile](hub/api/Dockerfile) `ARG VERSION` and [.github/workflows/publish-hub-images.yml](.github/workflows/publish-hub-images.yml) `extra-build-args`. |
| H3 | **Domain decision.** `nexor.com` is not owned. Current install/quick-start URLs use `nexor.ie`. | S–M | 🔲 **still open — business decision, not code.** Needs you to decide before any bulk find/replace across [README.md](README.md), [website/install.sh](website/install.sh), [website/hub-quickstart.sh](website/hub-quickstart.sh), [website/docs.html](website/docs.html), [website/index.html](website/index.html). Note: the recovered [SPEC.md](SPEC.md) original vision doc used yet a *third* placeholder domain (`nexor.io`) — three different TLDs floating around (`.io`, `.ie`, `.com`) across the repo's history. Pick one. |
| H4 | **Rename artefact cleanup.** Confirm no stray `NetScope` string remains. | S | ✅ **done (2026-08-06)** — ran `grep -Ril netscope` repo-wide with terminal access. Fixed: [.claude/launch.json](.claude/launch.json) (three hardcoded macOS absolute paths from the original author's machine, changed to workspace-relative `cwd` paths); [desktop/src-tauri/Cargo.lock](desktop/src-tauri/Cargo.lock) (N1 above). Remaining hits are intentional historical references inside [REMAINING_WORK.md](REMAINING_WORK.md) itself and [.github/agents/nexor.agent.md](.github/agents/nexor.agent.md) (documenting the rename) — not code, left as-is. |
| H5 | **CI green run.** Re-run every workflow on `main` to make sure nothing rotted during the pause. | S | 🔲 **still needs a real CI run on GitHub** (this assistant has local terminal access but not GitHub Actions runners). Push a branch or open a PR to trigger the matrix and confirm green — particularly worth re-checking [.github/workflows/publish-hub-images.yml](.github/workflows/publish-hub-images.yml) end-to-end now that the `VERSION` build-arg fix has landed. |

---

## 2. 🟡 Finish v0.5 F5 — Windows agent (biggest single gap)

The [capture-windows](agent/crates/capture-windows) crate compiles standalone but
is **not wired into the agent** and its process attribution is a stub.

### 2.1 Wire Windows capture into `main.rs`

- **File:** [agent/src/main.rs](agent/src/main.rs)
- **Effort:** M
- Add a `#[cfg(target_os = "windows")]` branch inside `run_capture` (or an
  entirely separate `run_capture_windows(cfg)` picked at compile time) that
  calls `capture_windows::start(WindowsCaptureConfig { … })`, receives
  `WindowsFlow`s, and converts them into `proto::Flow` for the same
  `HubClient::send_flow` pipeline used by libpcap.
- Mirror the Linux capture in shape: heartbeat, config poll, K8s stub returns
  empty strings on Windows, adaptive sampling still applies.

### 2.2 Finish process attribution

- **File:** [agent/crates/capture-windows/src/lib.rs](agent/crates/capture-windows/src/lib.rs) L164 (`TODO: GetExtendedTcpTable correlation`).
- **Effort:** M
- Add a `conn_table.rs` module using the `windows` crate's
  `Win32::NetworkManagement::IpHelper::GetExtendedTcpTable` / `GetExtendedUdpTable`
  with `TCP_TABLE_OWNER_PID_ALL`. Refresh every ~500 ms into a
  `HashMap<(u16 src_port, u16 dst_port, Protocol), u32 pid>` behind an
  `RwLock` (same TTL pattern as `ProcessCache`).
- In `parse_packet`, do `conn_table.lookup(src_port, dst_port, protocol)` →
  PID → `ProcessCache::lookup(pid)` → `process_name`.
- Add unit tests behind `#[cfg(all(test, target_os = "windows"))]` on a mock
  cache seeded with fixture rows.

### 2.3 CI: build Windows binary

- **File:** [.github/workflows/build-agent.yml](.github/workflows/build-agent.yml)
- **Effort:** S
- Add matrix rows:
  ```yaml
  - target: x86_64-pc-windows-msvc
    os: windows-latest
  - target: aarch64-pc-windows-msvc
    os: windows-11-arm    # or windows-latest with cross build
  ```
- Install Npcap SDK in the runner (download from `npcap.com`, unzip, set
  `LIB` env var). The `pcap` v2 crate links via build.rs so the DLL is not
  needed for compilation, only for runtime.
- Publish `nexor-agent-vX.Y.Z-x86_64-pc-windows-msvc.zip` alongside the
  existing tarballs.

### 2.4 WiX MSI installer

- **New folder:** `installer/windows/`
- **Effort:** M
- Author a `Product.wxs` that installs `nexor-agent.exe` + `wpcap.dll` +
  `Packet.dll` (bundled from Npcap redistributable) to
  `C:\Program Files\Nexor\`, registers a Windows service via `sc.exe` in a
  custom action, and creates an entry in Add/Remove Programs.
- CI: add a Windows job that runs `dotnet tool install --global wix` and
  produces `Nexor-Agent-vX.Y.Z-x64.msi`.
- Reference: existing macOS notarisation flow in [.github/workflows/build-desktop.yml](.github/workflows/build-desktop.yml)
  is the closest prior art.

### 2.5 Docs

- Update [README.md](README.md) Quick Start with a Windows section
  (`msiexec /i Nexor-Agent-x64.msi HUB_URL=... HUB_API_KEY=...`).
- Update [website/docs.html](website/docs.html) install matrix.

---

## 3. 🟡 Finish v0.5 F6 — eBPF Go crypto/tls + Python `ssl`

User-space loaders are complete; the **BPF kernel programs are missing**.

### 3.1 Author BPF programs

- **Files:** create `agent/ebpf/src/go_tls.rs` and `agent/ebpf/src/python_ssl.rs`; register them from [agent/ebpf/src/main.rs](agent/ebpf/src/main.rs).
- **Effort:** L (this is the deepest technical item on the list)
- Required program symbols (referenced by the loaders):
    | Loader expects | Kind | Notes |
    |---|---|---|
    | `go_tls_write_entry` | `#[uprobe]` | reads Go ABI: `AX = *tls.Conn`, `BX = *data`, `CX = len`. Reuse `SSL_EVENTS` perf array, tag with `SslDirection::Egress`. |
    | `go_tls_read_ret` | `#[uretprobe]` | pair with an entry probe that stashes the buffer pointer keyed by `pid_tgid`. |
    | `python_ssl_write_entry` | `#[uprobe]` | CPython C API — first arg is `PySSLSocket*`; second/third are `char* buf, ssize_t len`. |
    | `python_ssl_read_ret` | `#[uretprobe]` | same pairing as OpenSSL. |
- Share the existing `PerCpuArray<SslEvent>` scratch buffer, `WRITE_STATE`,
  and `READ_STATE` maps from [ssl.rs](agent/ebpf/src/ssl.rs) — do not
  duplicate them.
- Populate `SslEvent.comm` from `bpf_get_current_comm()`; leave IP/port
  fields empty when the connection isn't in `PID_CONN` yet (Go often does
  the TCP connect from a different goroutine).

### 3.2 Loader integration

- **File:** [agent/crates/ebpf-loader/src/go_tls.rs](agent/crates/ebpf-loader/src/go_tls.rs) and [python_ssl.rs](agent/crates/ebpf-loader/src/python_ssl.rs) — already reference the symbols above; once the BPF crate exports them, these compile as-is.
- Verify `discover_go_binaries()` handles PIE binaries (offset resolution
  must add the base address from `/proc/<pid>/maps`).
- **Effort:** S once §3.1 lands.

### 3.3 Runtime tests

- **New folder:** `agent/tests/ebpf_integration/`
- **Effort:** M
- Fixtures: tiny Go server (`hello.go`) and Python server
  (`hello.py`) that make an outbound HTTPS request. Assert that
  `nexor-agent ebpf --enable-go-tls --enable-python-ssl` emits a flow with
  `process_name` matching the binary and plaintext preview containing the
  request URL.
- Run in the [build-ebpf.yml](.github/workflows/build-ebpf.yml) workflow
  on `ubuntu-24.04` (kernel ≥ 6.8 has BTF pre-shipped).

---

## 4. � v0.5 F1–F4 — end-to-end verification & mark shipped

All backend + UI code exists; the roadmap marks the whole cluster
`🔄 in progress` out of caution. Real remaining work:

| # | Verification target | Effort | Status |
|---|---|---|---|
| V1 | AWS/GCP/Azure cloud pull actually ingests a fixture VPC log → visible in `/flows` | M | 🟡 test written, **not yet run** |
| V2 | Fleet cluster grid returns non-zero data on a hub with three agents in two clusters | S | 🟡 test written, **not yet run** |
| V3 | Compliance report scheduler fires a PDF at the configured cron and stores a `compliance_report_runs` row | M | 🟡 test written, **not yet run** |
| V4 | Sigma match triggers `incidents.Dispatcher.Dispatch`, creates an incident, and lands on the `/incidents` timeline | M | 🟡 test written, **not yet run** |

**Status (2026-08-07):** Real integration tests for all four items are
written and fully compile + link (`go test -run '^$' ./cloud/... ./handlers/...
./enterprise/compliance/... ./enterprise/sigma/...` — zero errors), but have
**not actually been executed to a pass/fail result**. This dev machine is a
JNJ corporate box behind Zscaler, which blocks the Docker registry pull
`testcontainers-go` needs to fetch `clickhouse/clickhouse-server:24.3-alpine`
— the run hangs/times out on image pull, not on test logic. Docker itself
is healthy here; it's specifically registry access that's blocked.

**Run these on an unrestricted machine (e.g. your MacBook)** to get the
actual pass/fail signal and flip these rows to ✅:

```bash
cd hub/api
go test ./cloud/...                 -run TestCloudPull_FixtureVPCLog_VisibleInFlows          -v
go test ./handlers/...              -run TestFleetClusters_ThreeAgentsTwoClusters_ReturnsNonZeroGrid -v
go test ./enterprise/compliance/... -run TestScheduler_FiresDueSchedule_RecordsRun            -v
go test ./enterprise/sigma/...      -run TestEngine_RuleMatch_DispatchesIncident              -v
```

Each test spins up its own throwaway ClickHouse container via
`testutil.StartClickHouse` (see [hub/api/testutil/clickhouse.go](hub/api/testutil/clickhouse.go)),
applies the real schema via the newly-extracted [hub/api/clickhouse/migrate.go](hub/api/clickhouse/migrate.go),
and exercises the real handler/engine/scheduler code paths — no mocks. If a
test fails on the Mac, that's a real bug to fix, not an environment artifact.

New test files (all internal-package tests where needed to reach unexported
`tick()`/`evaluate()` synchronously, avoiding goroutine/ticker race conditions):
- [hub/api/cloud/ingest_integration_test.go](hub/api/cloud/ingest_integration_test.go) (V1)
- [hub/api/handlers/fleet_integration_test.go](hub/api/handlers/fleet_integration_test.go) (V2)
- [hub/api/enterprise/compliance/scheduler_integration_test.go](hub/api/enterprise/compliance/scheduler_integration_test.go) (V3)
- [hub/api/enterprise/sigma/engine_integration_test.go](hub/api/enterprise/sigma/engine_integration_test.go) (V4)

Once all four pass on your Mac, flip the `🔄` rows in ROADMAP.md's v0.5 F1–F4
section to `✅` and update the status column above.

---

## 5. 🔲 Post-v0.7 net-new features (candidate v0.8 scope)

These are opportunities the code enables but has not shipped. Prioritise by
strategic value before committing.

### 5.1 macOS eBPF alternative — Endpoint Security framework
- Currently the eBPF pipeline is Linux-only. On macOS the equivalent is
  `EndpointSecurity.framework` for process attribution + a system extension
  for packet interception. Non-trivial (system-extension provisioning
  profile required) but the only path to feature parity with Linux.
- **Effort:** L–XL. Ship in v0.9+.

### 5.2 Long-term storage tier (S3 / GCS Parquet)
- `hub/api/enterprise/storage/` is already scaffolded and marked
  `🔲 planned` in ROADMAP. An hourly job that pulls
  `SELECT * FROM flows WHERE ts BETWEEN … AND …` and writes Parquet to a
  configurable object store, with a corresponding "cold read" path in the
  Flow Explorer.
- **Effort:** M.

### 5.3 Copilot autonomous investigation loop
- Today the copilot answers a single question. Next step: given an anomaly
  event, run a chain of NL-to-SQL queries automatically (top talkers →
  affected processes → external destinations → threat scores) and produce
  an incident summary attached to the `incidents` row.
- **Effort:** M–L. Very high demo value.

### 5.4 Alert channel: Discord + Google Chat
- Two channels users keep asking for. Both are webhook-based → additive
  case in `alerting/delivery.go`.
- **Effort:** S each.

### 5.5 Desktop: `.nscope` schema migration
- SQLite session files pinned to v0.6 schema. Add a schema version column
  and an upgrade path so users can open old captures after v0.8.
- **Effort:** S.

### 5.6 Mobile-friendly hub dashboard
- The Next.js UI is desktop-first. A responsive pass on
  [hub/web/app/](hub/web/app) — especially the SSE Live Feed and Sigma
  detection pages — unlocks on-call use.
- **Effort:** M.

### 5.7 Positioning strategy: one front door, three audiences — not three products

The "wedge" candidates discussed (Wireshark-killer desktop app / self-hosted
fleet observability for small teams / incident-replay+security-first) are
**not mutually exclusive features you have to choose between and abandon**.
They already coexist in the shipped v0.7 code — the single shared `Flow`
substrate simultaneously powers the desktop packet inspector, the hub fleet
dashboards, and the Sigma/incidents/compliance pipeline today. Nothing here
requires ripping anything out.

The actual constraint for a solo maintainer is **narrative, not code**: you
cannot pitch "Wireshark killer + fleet SIEM + incident-replay platform" in
one README hero line without diluting who it's for. A stranger who lands on
the repo needs to decide "is this for me?" in about ten seconds.

Recommended structure — a **layered wedge**, not an exclusive pick:

1. **Pick one front-door story** for the README hero line, landing page, and
   first-run onboarding flow. This is a marketing/positioning decision, not
   an architecture decision, and it's revisitable every release cycle as
   adoption data comes in.
2. **Let the other two ride along as growth paths** that are already built —
   e.g. "start with the desktop inspector → discover you can point it at a
   self-hosted hub → discover the hub does fleet dashboards, Sigma
   detections, and compliance reports out of the box." Each surface is a
   natural upsell/expansion off the one that hooked the user, not a
   competing pitch.
3. **New features (§5.1–§5.6, N3) should be prioritised by which front-door
   story is currently live**, not built in a vacuum. Example: if the
   front door is "Wireshark killer," §5.5 (`.nscope` schema migration) and
   the N3 DB-protocol-decoding feature (below) matter more than §5.6
   (mobile dashboard). If the front door is "fleet observability for small
   teams," it's the reverse.

**N3 (DB wire-protocol decoding — Postgres/MySQL/Redis/Kafka/AMQP) is worth
prioritising regardless of which front door is chosen.** It's technically
adjacent to the eBPF SSL/TLS work already shipped (same uprobe/perf-array
plumbing, see §3), and it's a genuine differentiator: no cheap self-hosted
tool today gives passive DB query visibility from network capture alone.
It strengthens all three audiences simultaneously — a debugging aid for the
desktop-app audience, a DB observability feature for the fleet audience,
and a DB access-audit trail for the security audience.

### 🔒 Locked decision (2026-08-07)

Front door: **self-hosted fleet observability for small teams** — a
Datadog/New Relic-style self-hosted alternative. README.md hero line,
Overview section, and component table have been updated to lead with this
(Hub + agent first, desktop repositioned as the companion inspector).

Priority order for everything downstream of this file:

1. **Fleet observability** (front door, ships now) — §4 (V1–V4 verification),
   plus H3 domain decision and H5 CI green run, since a self-host pitch lives
   or dies on "does the quickstart actually work end-to-end."
2. **Incident-replay / security** (next differentiator) — §3 (eBPF Go/Python
   TLS probes) and N3 (DB protocol decoding) both feed this directly: Sigma
   detections and DB-access audit trails are far more compelling once
   plaintext visibility covers Go/Python services and DB wire protocols,
   not just OpenSSL/libpcap traffic.
3. **Wireshark-killer desktop app** (companion, not the pitch) — §2 (Windows
   agent) matters here insofar as it makes the *agent* fleet-wide on Windows
   hosts too, but the desktop app itself gets feature work (§5.5 `.nscope`
   schema migration) only after 1 and 2 are solid.

---

## 6. Suggested execution order (single-developer, 4 weeks back)

Re-sequenced around the locked front-door decision (§5.7) — fleet
observability ships first, security/incident-replay is the v0.8
differentiator, desktop stays a companion.

| Week | Focus |
|---|---|
| **Week 1** | §1 housekeeping (H1–H5, incl. the domain call — H3 — since the self-host pitch needs one URL everywhere). Reconcile ROADMAP (§0, done). Cut `v0.7.1` with SPEC fix + version drift fixes. |
| **Week 2** | §4 v0.5 F1–F4 **end-to-end verification** (V1–V4: cloud pull, fleet grid, compliance scheduler, Sigma→incident dispatch) — this is the fleet-observability front door, so it needs to be provably solid, not just "code exists." Flip ROADMAP `🔄` rows to `✅` once each integration test is green. |
| **Week 3** | §3 eBPF Go/Python TLS probes (the deep BPF work) **+ start N3** (DB wire-protocol decoding scaffolding) — both feed directly into the incident-replay/security differentiator that ships next. |
| **Week 4** | Finish N3 for at least one DB protocol (Postgres is the highest-value first target — most common self-hosted stack). Open a v0.8 milestone: fleet-observability verification (week 2) + security differentiator (week 3–4) is a coherent "self-hosted observability + DB/TLS visibility" release story. §2 (Windows agent) and desktop polish (§5.5) move to v0.9, scoped as companion-app improvements once the front door has real users. |

Ship `v0.8.0` when §4's V1–V4 are green in CI **and** N3 covers at least one
DB protocol end-to-end (capture → decode → hub ingest → dashboard). That is
the actual "small team self-hosts Nexor and gets more value than Datadog for
free" release, not a Windows/BPF completeness milestone.

---

## 7. Business-side (not code)

- ⚠️ **Buy the domain**. Whichever you choose (`nexor.com`, `nexor.dev`,
  standardise on `nexor.ie`), do it before you tell anyone the project
  restarted — domain squatters follow public GitHub activity on niche
  observability tools.
- **Trademark / company alignment**: the Go module path is
  `github.com/klyzar/hub-api`; product is "Nexor". Decide whether Klyzar
  is the parent company (fine) or if the module path should also rename
  to `github.com/nexor-io/hub-api`. A module rename is a one-off, but
  it invalidates every downstream fork's import path — do it early or
  never.
- **License clarity**: [hub/enterprise/LICENSE](hub/enterprise/LICENSE) is
  BSL-1.1. Add a top-level `LICENSING.md` that spells out the split so
  contributors don't push enterprise code as MIT PRs by accident.
- **Public roadmap page**: [hub/web/app/roadmap/](hub/web/app/roadmap)
  already exists — populate it from this file so users see the plan.

---

## Cross-references

- Historical/full roadmap: [ROADMAP.md](ROADMAP.md)
- Shipped features log: [CHANGELOG.md](CHANGELOG.md), [RELEASE_NOTES_v0.7.md](RELEASE_NOTES_v0.7.md)
- Project agent (context for chat): [.github/agents/nexor.agent.md](.github/agents/nexor.agent.md)
- Product architecture: [README.md](README.md#architecture)
