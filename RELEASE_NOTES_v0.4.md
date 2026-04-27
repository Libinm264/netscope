# NetScope v0.4 — Enterprise Readiness Release

**Date:** 2026-04-27  
**Tags:** `v0.4.0` · Community (MIT) + Enterprise (BSL-1.1)

---

## What's New

v0.4 is the **Enterprise Readiness** milestone — four shipped priority phases that bring
production-grade security, integrations, and analytics to NetScope while keeping the
Community edition genuinely powerful and MIT-licensed.

---

### 🔍 P4 — Detection & Analytics

| Feature | Community | Enterprise |
|---|:---:|:---:|
| 5 built-in Sigma rules (port scans, beaconing, data exfil, DNS tunnelling, lateral movement) | ✅ | ✅ |
| Custom Sigma rule CRUD + 30-day match history | — | ✅ |
| Saved flow-filter queries (up to 10) | ✅ | ✅ (unlimited) |
| OTel trace ID filter — link flows → Jaeger/Tempo | ✅ | ✅ |
| Long-term S3/GCS cold-storage export (hourly/daily) | — | ✅ |
| Agent cluster labels (`prod-eu`, `staging-us` …) | ✅ | ✅ |

**Long-term storage** exports flows directly from ClickHouse to S3-compatible object storage
(AWS S3, GCS, MinIO, Cloudflare R2) — zero bytes flow through the hub process.
Configure at `Settings → Storage`.

---

### 🔗 P3 — Integrations & Export

| Feature | Community | Enterprise |
|---|:---:|:---:|
| SIEM sinks — Splunk HEC, Elasticsearch, generic webhook | — | ✅ |
| Audit log CSV/JSON export | — | ✅ |
| Kafka consumer group ID (`KAFKA_GROUP_ID`) | — | ✅ |

---

### 🏢 P1–P2 — Multi-Tenancy, RBAC & SSO

| Feature | Community | Enterprise |
|---|:---:|:---:|
| Organisations + invite links | — | ✅ |
| RBAC — Owner / Admin / Viewer | — | ✅ |
| OIDC & SAML SSO (Okta, Azure AD, Google Workspace) | — | ✅ |
| SCIM 2.0 automated provisioning | — | ✅ |
| "You" badge + self-remove guard in member list | ✅ | ✅ |

---

## Upgrading

```bash
# Hub (Docker)
docker pull ghcr.io/libinm264/netscope-hub:v0.4.0
docker compose up -d hub

# Agent (Linux)
curl -sSL https://netscope.ie/install.sh | sudo sh

# macOS desktop
# Download the .dmg from the Releases page
```

Database migrations run automatically on hub startup (phases 1–21).
No manual schema changes required.

---

## Breaking Changes

None. The API is fully backward-compatible with v0.3 agents.

---

## What's Next — v0.5

- **Windows support** — Npcap-based capture agent (Enterprise); free agent for
  Windows Server fleet monitoring.
- **Fleet management** — remote config push, agent version pinning, one-command rollouts.
- **AI anomaly baseline** — unsupervised ML model trained on your own traffic to surface
  novel threats that Sigma rules miss.

Full roadmap: [netscope.ie/#roadmap](https://netscope.ie/#roadmap)

---

## Customer Email Template

> Subject: **NetScope v0.4 is live — Detection, Storage Export & More**
>
> Hi [Name],
>
> NetScope v0.4 shipped today. Here's what's new for you:
>
> **If you're on Community (free):**
> - 🔍 Five built-in Sigma detection rules are now active on your hub — catching port
>   scans, DNS tunnelling, and lateral movement out of the box.
> - 💾 Save up to 10 flow filter presets so your most-used searches are one click away.
> - 🔗 If you run OpenTelemetry tracing, set your Jaeger/Tempo URL in Org Settings and
>   every `trace_id` in the flow table becomes a clickable link.
>
> **If you're on Enterprise:**
> Everything above, plus:
> - 📦 **Cold storage export** — push flows to S3, GCS, MinIO, or Cloudflare R2 on an
>   hourly or daily schedule. Configure it under Settings → Storage.
> - 🚨 **Custom Sigma rules** — write your own YAML detection rules and review 30 days
>   of match history in the new Detection page.
> - 📡 **SIEM integration** — forward flows to Splunk, Elasticsearch, or a webhook in
>   real time.
>
> **Upgrading is zero-downtime.** Pull the new hub image and restart — migrations run
> automatically. Your existing agents need no changes.
>
> ```
> docker pull ghcr.io/libinm264/netscope-hub:v0.4.0 && docker compose up -d hub
> ```
>
> Full release notes: https://github.com/Libinm264/netscope/releases/tag/v0.4.0  
> What's new on the website: https://netscope.ie/#changelog
>
> As always, ping us on GitHub Discussions or reply to this email with any questions.
>
> — The NetScope team
