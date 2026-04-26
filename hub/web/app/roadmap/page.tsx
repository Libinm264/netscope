"use client";

import { CheckCircle2, Circle, Clock, Cpu, Globe, Lock, Map, Server, Shield, Zap } from "lucide-react";
import { clsx } from "clsx";

// ── Data model ────────────────────────────────────────────────────────────────

type Status = "done" | "in-progress" | "next" | "planned";

interface RoadmapItem {
  area:   string;
  label:  string;
  status: Status;
}

interface Phase {
  version:     string;
  title:       string;
  theme:       string;
  status:      "shipped" | "in-progress" | "planned";
  icon:        React.ReactNode;
  groups:      { heading: string; items: RoadmapItem[] }[];
}

// ── Roadmap data ──────────────────────────────────────────────────────────────

const PHASES: Phase[] = [
  {
    version: "v0.1",
    title:   "Foundation",
    theme:   "Passive network visibility with ClickHouse storage and a real-time UI.",
    status:  "shipped",
    icon:    <Server size={16} />,
    groups: [
      {
        heading: "Agent",
        items: [
          { area: "Agent", label: "libpcap packet capture (HTTP, DNS, TLS, ICMP, ARP, HTTP/2, gRPC)", status: "done" },
          { area: "Agent", label: "Hub output — batch ingest over HTTP with API-key auth", status: "done" },
          { area: "Agent", label: "list-interfaces, capture, ebpf CLI subcommands", status: "done" },
        ],
      },
      {
        heading: "Hub",
        items: [
          { area: "Hub", label: "ClickHouse storage with 90-day TTL", status: "done" },
          { area: "Hub", label: "REST API — flows, stats, ingest endpoints", status: "done" },
          { area: "Hub", label: "Server-Sent Events live feed", status: "done" },
          { area: "Hub", label: "TLS certificate fleet tracking", status: "done" },
          { area: "Hub", label: "Geo-IP + threat scoring enrichment at ingest", status: "done" },
          { area: "Hub", label: "Compliance endpoints — summary, top-talkers, external, geo", status: "done" },
          { area: "Hub", label: "Role-based API tokens (admin / viewer)", status: "done" },
          { area: "Hub", label: "Agent enrollment tokens + one-line install script", status: "done" },
        ],
      },
      {
        heading: "UI",
        items: [
          { area: "UI", label: "Real-time flow table with protocol badges", status: "done" },
          { area: "UI", label: "Dashboard — total flows, flows/min, top protocols, active agents", status: "done" },
          { area: "UI", label: "Flows explorer with filters", status: "done" },
          { area: "UI", label: "TLS certificate audit page", status: "done" },
          { area: "UI", label: "Compliance reporting", status: "done" },
          { area: "UI", label: "Alert rules + alert event history", status: "done" },
        ],
      },
    ],
  },
  {
    version: "v0.2",
    title:   "eBPF Engine",
    theme:   "See through TLS — intercept plaintext before OpenSSL encrypts it, attribute every byte to a process.",
    status:  "in-progress",
    icon:    <Cpu size={16} />,
    groups: [
      {
        heading: "Agent",
        items: [
          { area: "Agent", label: "ProcessInfo { pid, name } added to flow proto", status: "done" },
          { area: "Agent", label: "eBPF → Hub bridge (ssl_event_to_flow, tcp_connect_to_flow)", status: "done" },
          { area: "Agent", label: "HTTP-from-SSL parser (parse_http_plaintext)", status: "done" },
          { area: "Agent", label: "Per-process terminal output prefix", status: "done" },
          { area: "Agent", label: "Hub sender thread (non-blocking eBPF loop)", status: "done" },
          { area: "Agent", label: "eBPF kernel programs — SSL uprobes + TCP kprobes", status: "in-progress" },
          { area: "Agent", label: "eBPF binary build pipeline (CI)", status: "planned" },
        ],
      },
      {
        heading: "Hub & UI",
        items: [
          { area: "Hub", label: "process_name + pid columns in flows table", status: "done" },
          { area: "Hub", label: "Ingest, Query, Compliance expose process fields", status: "done" },
          { area: "UI",  label: "Process column in FlowTable (green name + PID)", status: "done" },
        ],
      },
    ],
  },
  {
    version: "v0.3",
    title:   "Intelligence Layer",
    theme:   "Move from observation to detection — K8s enrichment, threat intel, process policies.",
    status:  "shipped",
    icon:    <Shield size={16} />,
    groups: [
      {
        heading: "Agent",
        items: [
          { area: "Agent", label: "Kubernetes pod enrichment (pod_name, k8s_namespace)", status: "done" },
          { area: "Agent", label: "OS detection + capture_mode in heartbeat", status: "done" },
          { area: "Agent", label: "Heartbeat thread (30s, pcap & eBPF)", status: "done" },
        ],
      },
      {
        heading: "Hub",
        items: [
          { area: "Hub", label: "Per-process network policy engine + violation log", status: "done" },
          { area: "Hub", label: "Policy evaluation at ingest", status: "done" },
          { area: "Hub", label: "Threat Intel API — threat-scored IPs with process info", status: "done" },
          { area: "Hub", label: "Alert test delivery (POST /alerts/:id/test)", status: "done" },
        ],
      },
      {
        heading: "UI",
        items: [
          { area: "UI", label: "Agent Fleet page — eBPF/pcap badge, OS, upgrade callout", status: "done" },
          { area: "UI", label: "Threat Intelligence page — score bars, severity badges", status: "done" },
          { area: "UI", label: "Process Policies page — rule builder, toggle, violations log", status: "done" },
          { area: "UI", label: "Alert test delivery button — inline ✓/✗ feedback", status: "done" },
          { area: "UI", label: "Service topology — threat score colouring on external nodes", status: "done" },
        ],
      },
    ],
  },
  {
    version: "v0.4",
    title:   "Enterprise Readiness",
    theme:   "Deploy at scale, integrate with your stack. Hub enterprise hardening.",
    status:  "in-progress",
    icon:    <Lock size={16} />,
    groups: [
      {
        heading: "Priority 1 — Identity & Access  ✅",
        items: [
          { area: "Hub", label: "Multi-tenant organisations table + org isolation", status: "done" },
          { area: "Hub", label: "org_members — owner / admin / analyst / viewer roles", status: "done" },
          { area: "Hub", label: "Teams table + team membership scoping", status: "done" },
          { area: "Hub", label: "SSO config table — SAML / OIDC IdP metadata", status: "done" },
          { area: "Hub", label: "JWT license engine — plan / feature / quota gating", status: "done" },
          { area: "UI",  label: "Settings sub-nav (Tokens, Org, Members, Teams, SSO, License)", status: "done" },
          { area: "UI",  label: "Organisation, Members, Teams, SSO, License settings pages", status: "done" },
          { area: "UI",  label: "EnterpriseGate component — upgrade prompt for locked features", status: "done" },
        ],
      },
      {
        heading: "Priority 2 — Auth Flows & Sessions  ✅",
        items: [
          { area: "Hub", label: "OIDC authorisation-code flow (Dex / Okta / Azure AD / Google)", status: "done" },
          { area: "Hub", label: "SAML 2.0 SP — ACS, metadata endpoint, IdP-initiated redirect", status: "done" },
          { area: "Hub", label: "Session store — httpOnly ns_session cookie, 24h TTL", status: "done" },
          { area: "Hub", label: "Session-aware RBAC middleware", status: "done" },
          { area: "Hub", label: "Local email/password login + bcrypt", status: "done" },
          { area: "Hub", label: "SCIM 2.0 user provisioning (Okta, Azure AD, OneLogin)", status: "done" },
          { area: "Hub", label: "Invite token flow — single-use 7-day tokens", status: "done" },
          { area: "Hub", label: "Password reset flow — anti-enumeration 200 response", status: "done" },
          { area: "Hub", label: "Auth event audit logging", status: "done" },
          { area: "UI",  label: "Login page — OIDC + SAML + email/password", status: "done" },
          { area: "UI",  label: "Accept-invite, forgot-password, reset-password pages", status: "done" },
          { area: "UI",  label: "Members page — invite link copy, 'you' badge, self-remove guard", status: "done" },
        ],
      },
      {
        heading: "Priority 3 — Integrations & Export  ✅",
        items: [
          { area: "Hub", label: "Audit log export — JSON, CEF (ArcSight), LEEF (QRadar)", status: "done" },
          { area: "Hub", label: "SIEM sink dispatcher — Splunk HEC, Elastic/ECS, Datadog, Loki", status: "done" },
          { area: "Hub", label: "Integration config CRUD API — upsert, delete, test connection", status: "done" },
          { area: "Hub", label: "integrations_config ClickHouse table with last_shipped tracking", status: "done" },
          { area: "UI",  label: "Integrations settings page — four sink cards with test buttons", status: "done" },
          { area: "UI",  label: "Audit export toolbar — format + date range + download button", status: "done" },
        ],
      },
      {
        heading: "Priority 4 — Detection & Analytics",
        items: [
          { area: "Hub", label: "Sigma rule engine — parse YAML rules, evaluate at ingest", status: "next" },
          { area: "Hub", label: "OpenTelemetry trace correlation — link flows to OTel trace IDs", status: "next" },
          { area: "Hub", label: "Kafka consumer group scaling — horizontal ingest workers", status: "next" },
          { area: "UI",  label: "Saved queries + query history", status: "next" },
          { area: "UI",  label: "Sigma rule manager — upload, edit, trigger history", status: "planned" },
          { area: "UI",  label: "OTel trace links — click flow → Jaeger/Tempo side panel", status: "planned" },
        ],
      },
      {
        heading: "Priority 5 — Platform Expansion",
        items: [
          { area: "UI",    label: "Multi-cluster fleet overview", status: "planned" },
          { area: "UI",    label: "Custom dashboard builder", status: "planned" },
          { area: "Agent", label: "Windows support — pcap via Npcap", status: "planned" },
          { area: "Agent", label: "eBPF for Go and Python native TLS libs", status: "planned" },
        ],
      },
    ],
  },
  {
    version: "v0.5",
    title:   "Agent Enterprise",
    theme:   "Make the agent enterprise-grade — Windows, signed binaries, fleet management.",
    status:  "planned",
    icon:    <Globe size={16} />,
    groups: [
      {
        heading: "Agent",
        items: [
          { area: "Agent", label: "Windows support (Npcap driver, installer MSI)", status: "planned" },
          { area: "Agent", label: "Signed binaries (code signing for macOS + Windows)", status: "planned" },
          { area: "Agent", label: "eBPF for Go crypto/tls and Python ssl module", status: "planned" },
          { area: "Agent", label: "Remote configuration push from hub", status: "planned" },
          { area: "Agent", label: "Auto-update mechanism", status: "planned" },
        ],
      },
      {
        heading: "Hub & UI",
        items: [
          { area: "Hub", label: "Fleet management API — push config, trigger updates", status: "planned" },
          { area: "Hub", label: "S3 / GCS long-term storage tier — hourly Parquet dumps", status: "planned" },
          { area: "UI",  label: "Multi-cluster fleet overview", status: "planned" },
          { area: "UI",  label: "Agent version management page", status: "planned" },
        ],
      },
    ],
  },
];

// ── Status helpers ────────────────────────────────────────────────────────────

const STATUS_CONFIG: Record<Status, { icon: React.ReactNode; color: string; dot: string }> = {
  "done":        { icon: <CheckCircle2 size={13} />, color: "text-emerald-400", dot: "bg-emerald-500" },
  "in-progress": { icon: <Clock        size={13} />, color: "text-indigo-400",  dot: "bg-indigo-500"  },
  "next":        { icon: <Zap          size={13} />, color: "text-amber-400",   dot: "bg-amber-500"   },
  "planned":     { icon: <Circle       size={13} />, color: "text-slate-600",   dot: "bg-slate-700"   },
};

const PHASE_STATUS_CONFIG = {
  shipped:    { label: "Shipped",     badge: "text-emerald-400 bg-emerald-500/10 border-emerald-500/20" },
  "in-progress": { label: "In Progress", badge: "text-indigo-400 bg-indigo-500/10 border-indigo-500/20" },
  planned:    { label: "Planned",     badge: "text-slate-400  bg-slate-700/40  border-white/10"        },
};

const AREA_COLORS: Record<string, string> = {
  Hub:   "text-indigo-400 bg-indigo-500/10",
  UI:    "text-sky-400    bg-sky-500/10",
  Agent: "text-emerald-400 bg-emerald-500/10",
};

// ── Components ────────────────────────────────────────────────────────────────

function AreaBadge({ area }: { area: string }) {
  return (
    <span className={clsx(
      "shrink-0 text-[9px] font-semibold px-1.5 py-0.5 rounded uppercase tracking-wide",
      AREA_COLORS[area] ?? "text-slate-400 bg-slate-700/40",
    )}>
      {area}
    </span>
  );
}

function StatusIcon({ status }: { status: Status }) {
  const cfg = STATUS_CONFIG[status];
  return <span className={cfg.color}>{cfg.icon}</span>;
}

function PhaseBadge({ status }: { status: Phase["status"] }) {
  const cfg = PHASE_STATUS_CONFIG[status];
  return (
    <span className={clsx(
      "text-[10px] font-semibold px-2 py-0.5 rounded border leading-none",
      cfg.badge,
    )}>
      {cfg.label}
    </span>
  );
}

function PhaseCard({ phase }: { phase: Phase }) {
  const isCurrent = phase.status === "in-progress";
  return (
    <div className={clsx(
      "rounded-2xl border overflow-hidden",
      isCurrent
        ? "border-indigo-500/25 bg-[#0d0d1a]"
        : phase.status === "shipped"
          ? "border-emerald-500/15 bg-[#0d0d1a]"
          : "border-white/[0.05] bg-[#0b0b17]",
    )}>
      {/* Phase header */}
      <div className="flex items-start justify-between gap-4 px-5 py-4 border-b border-white/[0.05]">
        <div className="flex items-center gap-3 min-w-0">
          <div className={clsx(
            "p-2 rounded-lg shrink-0",
            phase.status === "shipped"     ? "bg-emerald-500/10 text-emerald-400" :
            phase.status === "in-progress" ? "bg-indigo-500/10  text-indigo-400"  :
                                              "bg-slate-700/40   text-slate-500",
          )}>
            {phase.icon}
          </div>
          <div>
            <div className="flex items-center gap-2 flex-wrap">
              <span className="text-xs font-mono text-slate-500">{phase.version}</span>
              <span className="text-sm font-bold text-white">{phase.title}</span>
            </div>
            <p className="text-xs text-slate-500 mt-0.5 leading-snug">{phase.theme}</p>
          </div>
        </div>
        <PhaseBadge status={phase.status} />
      </div>

      {/* Groups */}
      <div className="divide-y divide-white/[0.04]">
        {phase.groups.map(group => (
          <div key={group.heading} className="px-5 py-3 space-y-1.5">
            <p className="text-[10px] font-semibold text-slate-500 uppercase tracking-wider mb-2">
              {group.heading}
            </p>
            {group.items.map((item, i) => (
              <div key={i} className="flex items-start gap-2.5">
                <StatusIcon status={item.status} />
                <div className="flex items-start gap-2 flex-1 min-w-0">
                  <p className={clsx(
                    "text-xs leading-snug flex-1",
                    item.status === "done"    ? "text-slate-400" :
                    item.status === "next"    ? "text-white"     :
                    item.status === "in-progress" ? "text-indigo-200" :
                                                "text-slate-600",
                  )}>
                    {item.label}
                  </p>
                  <AreaBadge area={item.area} />
                </div>
              </div>
            ))}
          </div>
        ))}
      </div>
    </div>
  );
}

// ── Legend ────────────────────────────────────────────────────────────────────

function Legend() {
  const items: { status: Status; label: string }[] = [
    { status: "done",        label: "Shipped" },
    { status: "in-progress", label: "In Progress" },
    { status: "next",        label: "Up Next" },
    { status: "planned",     label: "Planned" },
  ];
  return (
    <div className="flex items-center gap-4 flex-wrap">
      {items.map(({ status, label }) => {
        const cfg = STATUS_CONFIG[status];
        return (
          <div key={status} className="flex items-center gap-1.5">
            <span className={cfg.color}>{cfg.icon}</span>
            <span className="text-[11px] text-slate-500">{label}</span>
          </div>
        );
      })}
      <div className="ml-4 flex items-center gap-3">
        {Object.entries(AREA_COLORS).map(([area, cls]) => (
          <span key={area} className={clsx("text-[9px] font-semibold px-1.5 py-0.5 rounded uppercase tracking-wide", cls)}>
            {area}
          </span>
        ))}
      </div>
    </div>
  );
}

// ── Page ──────────────────────────────────────────────────────────────────────

export default function RoadmapPage() {
  const currentPhase = PHASES.find(p => p.status === "in-progress");
  const doneCount    = PHASES.filter(p => p.status === "shipped").length;

  return (
    <div className="p-6 space-y-6 max-w-4xl">
      {/* Header */}
      <div className="flex items-start justify-between gap-4 flex-wrap">
        <div className="flex items-center gap-3">
          <div className="p-2 rounded-lg bg-indigo-500/10">
            <Map size={20} className="text-indigo-400" />
          </div>
          <div>
            <h1 className="text-lg font-semibold text-white">Product Roadmap</h1>
            <p className="text-xs text-slate-400 mt-0.5">
              NetScope — from passive network visibility to a full-stack security observability platform
            </p>
          </div>
        </div>
        <div className="flex items-center gap-3 text-xs text-slate-500">
          <span className="text-emerald-400 font-semibold">{doneCount} versions shipped</span>
          {currentPhase && (
            <>
              <span>·</span>
              <span>Now on <span className="text-indigo-300 font-semibold">{currentPhase.version} {currentPhase.title}</span></span>
            </>
          )}
        </div>
      </div>

      {/* Legend */}
      <Legend />

      {/* Timeline */}
      <div className="relative space-y-4">
        {/* Vertical line */}
        <div className="absolute left-[27px] top-8 bottom-8 w-px bg-white/[0.06] -z-0" />

        {PHASES.map(phase => (
          <div key={phase.version} className="relative flex gap-4">
            {/* Timeline dot */}
            <div className="shrink-0 mt-4 ml-3.5">
              <div className={clsx(
                "w-3.5 h-3.5 rounded-full border-2 z-10 relative",
                phase.status === "shipped"      ? "bg-emerald-500 border-emerald-400"  :
                phase.status === "in-progress"  ? "bg-indigo-500  border-indigo-300 ring-4 ring-indigo-500/20" :
                                                   "bg-slate-800   border-slate-600",
              )} />
            </div>
            {/* Card */}
            <div className="flex-1 min-w-0">
              <PhaseCard phase={phase} />
            </div>
          </div>
        ))}
      </div>

      {/* Footer note */}
      <p className="text-[11px] text-slate-600 text-center pb-4">
        Community edition is MIT licensed. Enterprise features (hub/enterprise/) are BSL-1.1 — free to self-host, not for resale as a managed service.
      </p>
    </div>
  );
}
