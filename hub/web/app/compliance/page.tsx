"use client";

import { useState, useEffect, useCallback } from "react";
import {
  fetchComplianceSummary,
  fetchTopTalkers,
  fetchExternalConnections,
  fetchTLSAudit,
  fetchComplianceConnections,
  fetchGeoSummary,
  type ComplianceSummary,
  type TopTalker,
  type ExternalDest,
  type TLSAuditRecord,
  type ConnectionRecord,
  type GeoCountry,
} from "@/lib/api";
import {
  ClipboardList,
  RefreshCw,
  Globe,
  ShieldAlert,
  Activity,
  ArrowUpRight,
  AlertTriangle,
  CheckCircle2,
  XCircle,
  Clock,
  Search,
  ChevronDown,
} from "lucide-react";
import { clsx } from "clsx";

// ── Helpers ────────────────────────────────────────────────────────────────────

function fmtBytes(n: number): string {
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
  if (n < 1024 * 1024 * 1024) return `${(n / 1024 / 1024).toFixed(1)} MB`;
  return `${(n / 1024 / 1024 / 1024).toFixed(2)} GB`;
}

function fmtNum(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}K`;
  return String(n);
}

function fmtDate(iso: string): string {
  try { return new Date(iso).toLocaleString(); } catch { return iso; }
}

const WINDOWS = ["1h", "6h", "24h", "7d", "30d"] as const;
type Window = (typeof WINDOWS)[number];

type Tab = "talkers" | "external" | "tls" | "connections" | "geo";

const ISSUE_CONFIG: Record<
  TLSAuditRecord["issue"],
  { label: string; color: string; icon: React.ReactNode }
> = {
  expired:           { label: "Expired",          color: "text-red-400 bg-red-500/10 border-red-500/20",      icon: <XCircle size={12} /> },
  expiring_critical: { label: "< 7 days",          color: "text-orange-400 bg-orange-500/10 border-orange-500/20", icon: <AlertTriangle size={12} /> },
  expiring_soon:     { label: "< 30 days",         color: "text-yellow-400 bg-yellow-500/10 border-yellow-500/20", icon: <Clock size={12} /> },
  self_signed:       { label: "Self-signed",       color: "text-purple-400 bg-purple-500/10 border-purple-500/20", icon: <ShieldAlert size={12} /> },
  ok:                { label: "Valid",             color: "text-emerald-400 bg-emerald-500/10 border-emerald-500/20", icon: <CheckCircle2 size={12} /> },
};

// ── Threat badge ───────────────────────────────────────────────────────────────

function ThreatBadge({ score }: { score: number }) {
  if (score === 0) return null;
  const cfg =
    score >= 75 ? { label: "HIGH",   cls: "bg-red-500/10 text-red-400 border-red-500/20" } :
    score >= 50 ? { label: "MED",    cls: "bg-orange-500/10 text-orange-400 border-orange-500/20" } :
                  { label: "LOW",    cls: "bg-yellow-500/10 text-yellow-400 border-yellow-500/20" };
  return (
    <span className={clsx(
      "inline-flex items-center gap-1 text-[10px] px-1.5 py-0.5 rounded border font-medium",
      cfg.cls,
    )}>
      <AlertTriangle size={9} />
      {cfg.label}
    </span>
  );
}

// ── Country flag helper ────────────────────────────────────────────────────────

function CountryFlag({ code }: { code: string }) {
  // Convert ISO 3166-1 alpha-2 to regional indicator symbols (emoji flag)
  if (!code || code.length !== 2) return null;
  const flag = code
    .toUpperCase()
    .split("")
    .map((c) => String.fromCodePoint(0x1f1e6 + c.charCodeAt(0) - 65))
    .join("");
  return <span className="text-base leading-none">{flag}</span>;
}

// ── Geo panel ──────────────────────────────────────────────────────────────────

function GeoPanel({ countries, window: win }: { countries: GeoCountry[]; window: string }) {
  const maxConns = Math.max(...countries.map((c) => c.connections), 1);

  if (countries.length === 0) {
    return (
      <div className="py-16 text-center space-y-2">
        <Globe size={24} className="mx-auto text-slate-700" />
        <p className="text-sm text-slate-600">
          No geo data yet — set <code className="text-slate-500">GEOIP_CITY_DB</code> and <code className="text-slate-500">GEOIP_ASN_DB</code> to enable enrichment
        </p>
      </div>
    );
  }

  return (
    <div className="overflow-x-auto">
      <table className="w-full text-left">
        <thead>
          <tr className="border-b border-white/[0.06]">
            {["Country", "Connections", "Bytes Out", "Unique Sources", "Threat", "Share"].map((h) => (
              <th key={h} className="px-4 py-2.5 text-xs font-medium text-slate-500 uppercase tracking-wider">
                {h}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {countries.map((c) => (
            <tr key={c.code} className="border-b border-white/[0.04] hover:bg-white/[0.02]">
              <td className="px-4 py-3">
                <div className="flex items-center gap-2">
                  <CountryFlag code={c.code} />
                  <div>
                    <p className="text-sm text-white">{c.name || c.code}</p>
                    <p className="text-xs text-slate-600 font-mono">{c.code}</p>
                  </div>
                </div>
              </td>
              <td className="px-4 py-3 text-sm text-slate-300">{fmtNum(c.connections)}</td>
              <td className="px-4 py-3 text-sm text-slate-400 font-mono">{fmtBytes(c.bytes_out)}</td>
              <td className="px-4 py-3 text-sm text-slate-500">{c.unique_sources}</td>
              <td className="px-4 py-3">
                <ThreatBadge score={c.max_threat_score} />
              </td>
              <td className="px-4 py-3 w-40">
                <div className="flex items-center gap-2">
                  <div className="flex-1 h-1.5 rounded-full bg-white/[0.06] overflow-hidden">
                    <div
                      className="h-full rounded-full bg-indigo-500"
                      style={{ width: `${(c.connections / maxConns) * 100}%` }}
                    />
                  </div>
                  <span className="text-xs text-slate-600 w-8 text-right">
                    {((c.connections / maxConns) * 100).toFixed(0)}%
                  </span>
                </div>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

// ── Summary card ───────────────────────────────────────────────────────────────

function StatCard({
  label,
  value,
  sub,
  icon,
  accent,
}: {
  label: string;
  value: string | number;
  sub?: string;
  icon: React.ReactNode;
  accent: string;
}) {
  return (
    <div className="rounded-xl bg-[#0d0d1a] border border-white/[0.06] p-4 flex items-start gap-3">
      <div className={clsx("p-2 rounded-lg", accent)}>{icon}</div>
      <div>
        <p className="text-xs text-slate-500 uppercase tracking-wider">{label}</p>
        <p className="text-2xl font-semibold text-white mt-0.5">{value}</p>
        {sub && <p className="text-xs text-slate-600 mt-0.5">{sub}</p>}
      </div>
    </div>
  );
}

// ── Tab bar ────────────────────────────────────────────────────────────────────

function TabBar({
  active,
  onChange,
  counts,
}: {
  active: Tab;
  onChange: (t: Tab) => void;
  counts: Record<Tab, number>;
}) {
  const tabs: { id: Tab; label: string }[] = [
    { id: "talkers",     label: "Top Talkers" },
    { id: "external",    label: "External Destinations" },
    { id: "tls",         label: "TLS Audit" },
    { id: "connections", label: "Connection Log" },
    { id: "geo",         label: "Geo Map" },
  ];

  return (
    <div className="flex gap-1 border-b border-white/[0.06] px-2">
      {tabs.map((t) => (
        <button
          key={t.id}
          onClick={() => onChange(t.id)}
          className={clsx(
            "px-3 py-2.5 text-sm font-medium border-b-2 transition-colors -mb-px",
            active === t.id
              ? "border-indigo-500 text-white"
              : "border-transparent text-slate-500 hover:text-slate-300",
          )}
        >
          {t.label}
          {counts[t.id] > 0 && (
            <span className="ml-2 text-xs px-1.5 py-0.5 rounded-full bg-white/[0.06] text-slate-400">
              {counts[t.id]}
            </span>
          )}
        </button>
      ))}
    </div>
  );
}

// ── Top Talkers panel ──────────────────────────────────────────────────────────

function TopTalkersPanel({ talkers }: { talkers: TopTalker[] }) {
  const maxOut = Math.max(...talkers.map((t) => t.bytes_out), 1);

  if (talkers.length === 0) {
    return (
      <div className="py-16 text-center text-sm text-slate-600">
        No traffic data for this window
      </div>
    );
  }

  return (
    <div className="overflow-x-auto">
      <table className="w-full text-left">
        <thead>
          <tr className="border-b border-white/[0.06]">
            {["IP / Host", "Outbound", "Inbound", "Flows", "Unique Dests", "Outbound bar"].map((h) => (
              <th key={h} className="px-4 py-2.5 text-xs font-medium text-slate-500 uppercase tracking-wider">
                {h}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {talkers.map((t) => (
            <tr key={t.ip} className="border-b border-white/[0.04] hover:bg-white/[0.02]">
              <td className="px-4 py-3">
                <p className="text-sm text-white font-mono">{t.ip}</p>
                {t.hostname && t.hostname !== t.ip && (
                  <p className="text-xs text-slate-600 mt-0.5">{t.hostname}</p>
                )}
              </td>
              <td className="px-4 py-3 text-sm text-slate-300 font-mono">{fmtBytes(t.bytes_out)}</td>
              <td className="px-4 py-3 text-sm text-slate-500 font-mono">{fmtBytes(t.bytes_in)}</td>
              <td className="px-4 py-3 text-sm text-slate-400">{fmtNum(t.flow_count)}</td>
              <td className="px-4 py-3 text-sm text-slate-400">{t.unique_destinations}</td>
              <td className="px-4 py-3 w-40">
                <div className="h-1.5 rounded-full bg-white/[0.06] overflow-hidden">
                  <div
                    className="h-full rounded-full bg-indigo-500"
                    style={{ width: `${(t.bytes_out / maxOut) * 100}%` }}
                  />
                </div>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

// ── External Connections panel ─────────────────────────────────────────────────

function ExternalPanel({ destinations }: { destinations: ExternalDest[] }) {
  if (destinations.length === 0) {
    return (
      <div className="py-16 text-center text-sm text-slate-600">
        No external connections in this window
      </div>
    );
  }

  return (
    <div className="overflow-x-auto">
      <table className="w-full text-left">
        <thead>
          <tr className="border-b border-white/[0.06]">
            {["Destination IP", "Port", "Protocol", "Flows", "Bytes Out", "Sources", "Threat", "Last Seen"].map((h) => (
              <th key={h} className="px-4 py-2.5 text-xs font-medium text-slate-500 uppercase tracking-wider">
                {h}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {destinations.map((d) => (
            <tr key={d.dst_ip} className="border-b border-white/[0.04] hover:bg-white/[0.02]">
              <td className="px-4 py-3">
                <div className="flex items-center gap-1.5">
                  <Globe size={12} className="text-orange-400 shrink-0" />
                  <span className="text-sm text-white font-mono">{d.dst_ip}</span>
                </div>
              </td>
              <td className="px-4 py-3 text-sm text-slate-400 font-mono">{d.dst_port}</td>
              <td className="px-4 py-3">
                <span className="text-xs px-2 py-0.5 rounded bg-slate-700/40 text-slate-400 border border-white/[0.06]">
                  {d.protocol}
                </span>
              </td>
              <td className="px-4 py-3 text-sm text-slate-400">{fmtNum(d.flow_count)}</td>
              <td className="px-4 py-3 text-sm text-slate-300 font-mono">{fmtBytes(d.bytes_out)}</td>
              <td className="px-4 py-3 text-xs text-slate-500 font-mono">
                {(d.src_ips ?? []).slice(0, 3).join(", ")}
                {(d.src_ips ?? []).length > 3 && (
                  <span className="text-slate-600"> +{d.src_ips.length - 3}</span>
                )}
              </td>
              <td className="px-4 py-3">
                <ThreatBadge score={(d as ExternalDest & { threat_score?: number }).threat_score ?? 0} />
              </td>
              <td className="px-4 py-3 text-xs text-slate-600 whitespace-nowrap">
                {fmtDate(d.last_seen)}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

// ── TLS Audit panel ────────────────────────────────────────────────────────────

function TLSPanel({ certs }: { certs: TLSAuditRecord[] }) {
  const [filter, setFilter] = useState<TLSAuditRecord["issue"] | "all">("all");

  const shown = filter === "all" ? certs : certs.filter((c) => c.issue === filter);

  const counts = {
    expired:           certs.filter((c) => c.issue === "expired").length,
    expiring_critical: certs.filter((c) => c.issue === "expiring_critical").length,
    expiring_soon:     certs.filter((c) => c.issue === "expiring_soon").length,
    self_signed:       certs.filter((c) => c.issue === "self_signed").length,
    ok:                certs.filter((c) => c.issue === "ok").length,
  };

  if (certs.length === 0) {
    return (
      <div className="py-16 text-center text-sm text-slate-600">
        No TLS certificate data yet
      </div>
    );
  }

  return (
    <div>
      {/* Filter chips */}
      <div className="flex gap-2 p-4 flex-wrap">
        <button
          onClick={() => setFilter("all")}
          className={clsx(
            "px-3 py-1 rounded-full text-xs border transition-colors",
            filter === "all"
              ? "bg-indigo-500/10 text-indigo-400 border-indigo-500/20"
              : "bg-transparent text-slate-500 border-white/[0.06] hover:text-slate-300",
          )}
        >
          All ({certs.length})
        </button>
        {(Object.keys(ISSUE_CONFIG) as TLSAuditRecord["issue"][]).map((issue) => {
          const cfg = ISSUE_CONFIG[issue];
          const cnt = counts[issue];
          if (cnt === 0) return null;
          return (
            <button
              key={issue}
              onClick={() => setFilter(issue)}
              className={clsx(
                "flex items-center gap-1.5 px-3 py-1 rounded-full text-xs border transition-colors",
                filter === issue ? cfg.color : "bg-transparent text-slate-500 border-white/[0.06] hover:text-slate-300",
              )}
            >
              {cfg.icon} {cfg.label} ({cnt})
            </button>
          );
        })}
      </div>

      <div className="overflow-x-auto">
        <table className="w-full text-left">
          <thead>
            <tr className="border-b border-white/[0.06]">
              {["Status", "Common Name", "Issuer", "Expiry", "Days Left", "Host / IP", "Last Seen"].map((h) => (
                <th key={h} className="px-4 py-2.5 text-xs font-medium text-slate-500 uppercase tracking-wider">
                  {h}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {shown.map((cert) => {
              const cfg = ISSUE_CONFIG[cert.issue];
              return (
                <tr key={cert.fingerprint} className="border-b border-white/[0.04] hover:bg-white/[0.02]">
                  <td className="px-4 py-3">
                    <span className={clsx("inline-flex items-center gap-1.5 px-2 py-0.5 rounded-full text-xs border", cfg.color)}>
                      {cfg.icon} {cfg.label}
                    </span>
                  </td>
                  <td className="px-4 py-3 text-sm text-white font-mono">{cert.cn || "—"}</td>
                  <td className="px-4 py-3 text-xs text-slate-500 max-w-[140px] truncate">{cert.issuer || "—"}</td>
                  <td className="px-4 py-3 text-sm text-slate-400 font-mono whitespace-nowrap">{cert.expiry || "—"}</td>
                  <td className="px-4 py-3 text-sm">
                    <span className={clsx(
                      cert.days_left < 0 ? "text-red-400" :
                      cert.days_left <= 7 ? "text-orange-400" :
                      cert.days_left <= 30 ? "text-yellow-400" : "text-slate-400"
                    )}>
                      {cert.days_left < 0 ? `${Math.abs(cert.days_left)}d ago` : `${cert.days_left}d`}
                    </span>
                  </td>
                  <td className="px-4 py-3 text-xs text-slate-500">
                    {cert.hostname || cert.dst_ip || "—"}
                  </td>
                  <td className="px-4 py-3 text-xs text-slate-600 whitespace-nowrap">
                    {fmtDate(cert.last_seen)}
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>
    </div>
  );
}

// ── Connection Log panel ───────────────────────────────────────────────────────

function ConnectionLogPanel({ window }: { window: Window }) {
  const [connections, setConnections] = useState<ConnectionRecord[]>([]);
  const [loading, setLoading] = useState(true);
  const [srcIP, setSrcIP] = useState("");
  const [dstIP, setDstIP] = useState("");
  const [externalOnly, setExternalOnly] = useState(false);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const res = await fetchComplianceConnections({
        window,
        src_ip: srcIP || undefined,
        dst_ip: dstIP || undefined,
        external_only: externalOnly ? "true" : undefined,
        limit: 200,
      });
      setConnections(res.connections ?? []);
    } finally {
      setLoading(false);
    }
  }, [window, srcIP, dstIP, externalOnly]);

  useEffect(() => { load(); }, [load]);

  return (
    <div>
      {/* Filters */}
      <div className="flex gap-3 p-4 flex-wrap items-center border-b border-white/[0.04]">
        <div className="relative">
          <Search size={12} className="absolute left-2.5 top-1/2 -translate-y-1/2 text-slate-600 pointer-events-none" />
          <input
            type="text"
            placeholder="Source IP"
            value={srcIP}
            onChange={(e) => setSrcIP(e.target.value)}
            className="pl-7 pr-3 py-1.5 rounded-lg bg-[#12121f] border border-white/[0.08]
                       text-sm text-slate-300 placeholder-slate-600 focus:outline-none
                       focus:border-indigo-500/50 w-36"
          />
        </div>
        <div className="relative">
          <Search size={12} className="absolute left-2.5 top-1/2 -translate-y-1/2 text-slate-600 pointer-events-none" />
          <input
            type="text"
            placeholder="Dest IP"
            value={dstIP}
            onChange={(e) => setDstIP(e.target.value)}
            className="pl-7 pr-3 py-1.5 rounded-lg bg-[#12121f] border border-white/[0.08]
                       text-sm text-slate-300 placeholder-slate-600 focus:outline-none
                       focus:border-indigo-500/50 w-36"
          />
        </div>
        <label className="flex items-center gap-2 text-sm text-slate-400 cursor-pointer select-none">
          <input
            type="checkbox"
            checked={externalOnly}
            onChange={(e) => setExternalOnly(e.target.checked)}
            className="rounded border-white/20 bg-[#12121f] accent-indigo-500"
          />
          External only
        </label>
        <button
          onClick={load}
          className="flex items-center gap-1.5 px-3 py-1.5 rounded-lg bg-[#12121f]
                     border border-white/[0.08] text-sm text-slate-400 hover:text-white transition-colors"
        >
          <RefreshCw size={12} className={loading ? "animate-spin" : ""} />
          Apply
        </button>
        <span className="text-xs text-slate-600 ml-auto">{connections.length} rows</span>
      </div>

      {loading ? (
        <div className="py-12 text-center text-sm text-slate-600">Loading…</div>
      ) : connections.length === 0 ? (
        <div className="py-12 text-center text-sm text-slate-600">No connections match the current filters</div>
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full text-left">
            <thead>
              <tr className="border-b border-white/[0.06]">
                {["Time", "Host", "Proto", "Source", "Destination", "Bytes ↑↓", "Ext"].map((h) => (
                  <th key={h} className="px-4 py-2.5 text-xs font-medium text-slate-500 uppercase tracking-wider">
                    {h}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {connections.map((c) => (
                <tr key={c.id} className={clsx(
                  "border-b border-white/[0.04] hover:bg-white/[0.02]",
                  c.is_external && "bg-orange-500/[0.02]",
                )}>
                  <td className="px-4 py-2.5 text-xs text-slate-600 whitespace-nowrap">
                    {fmtDate(c.timestamp)}
                  </td>
                  <td className="px-4 py-2.5 text-xs text-slate-500">{c.hostname || "—"}</td>
                  <td className="px-4 py-2.5">
                    <span className="text-xs px-1.5 py-0.5 rounded bg-slate-700/40 text-slate-400 border border-white/[0.06]">
                      {c.protocol}
                    </span>
                  </td>
                  <td className="px-4 py-2.5 text-xs text-slate-400 font-mono whitespace-nowrap">
                    {c.src_ip}:{c.src_port}
                  </td>
                  <td className="px-4 py-2.5 text-xs font-mono whitespace-nowrap">
                    <span className={c.is_external ? "text-orange-300" : "text-slate-400"}>
                      {c.dst_ip}:{c.dst_port}
                    </span>
                  </td>
                  <td className="px-4 py-2.5 text-xs text-slate-500 font-mono whitespace-nowrap">
                    {fmtBytes(c.bytes_out)} / {fmtBytes(c.bytes_in)}
                  </td>
                  <td className="px-4 py-2.5">
                    {c.is_external && (
                      <ArrowUpRight size={12} className="text-orange-400" />
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}

// ── Page ───────────────────────────────────────────────────────────────────────

export default function CompliancePage() {
  const [window, setWindow] = useState<Window>("24h");
  const [tab, setTab] = useState<Tab>("talkers");
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const [summary, setSummary] = useState<ComplianceSummary | null>(null);
  const [talkers, setTalkers] = useState<TopTalker[]>([]);
  const [external, setExternal] = useState<ExternalDest[]>([]);
  const [certs, setCerts] = useState<TLSAuditRecord[]>([]);
  const [countries, setCountries] = useState<GeoCountry[]>([]);

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const [sumRes, talkRes, extRes, tlsRes, geoRes] = await Promise.all([
        fetchComplianceSummary(window),
        fetchTopTalkers(window),
        fetchExternalConnections(window),
        fetchTLSAudit(),
        fetchGeoSummary(window),
      ]);
      setSummary(sumRes);
      setTalkers(talkRes.talkers ?? []);
      setExternal(extRes.destinations ?? []);
      setCerts(tlsRes.certs ?? []);
      setCountries(geoRes.countries ?? []);
    } catch (e) {
      setError(String(e));
    } finally {
      setLoading(false);
    }
  }, [window]);

  useEffect(() => { load(); }, [load]);

  const tabCounts: Record<Tab, number> = {
    talkers:     talkers.length,
    external:    external.length,
    tls:         certs.filter((c) => c.issue !== "ok").length,
    connections: 0,
    geo:         countries.length,
  };

  const tlsIssues = certs.filter((c) => c.issue !== "ok").length;

  return (
    <div className="p-6 space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-3">
          <div className="p-2 rounded-lg bg-emerald-500/10">
            <ClipboardList size={18} className="text-emerald-400" />
          </div>
          <div>
            <h1 className="text-xl font-semibold text-white">Compliance</h1>
            <p className="text-xs text-slate-500 mt-0.5">
              Network audit — external reach, TLS hygiene, top talkers
            </p>
          </div>
        </div>
        <div className="flex items-center gap-2">
          {/* Window picker */}
          <div className="relative">
            <select
              value={window}
              onChange={(e) => setWindow(e.target.value as Window)}
              className="appearance-none pl-3 pr-8 py-1.5 rounded-lg bg-[#12121f]
                         border border-white/[0.08] text-sm text-slate-300
                         focus:outline-none focus:border-indigo-500/50 cursor-pointer"
            >
              {WINDOWS.map((w) => (
                <option key={w} value={w}>{w}</option>
              ))}
            </select>
            <ChevronDown size={12} className="absolute right-2.5 top-1/2 -translate-y-1/2 text-slate-500 pointer-events-none" />
          </div>
          <button
            onClick={load}
            className="flex items-center gap-2 px-3 py-1.5 rounded-lg bg-[#12121f]
                       border border-white/10 text-sm text-slate-300 hover:text-white
                       hover:border-white/20 transition-colors"
          >
            <RefreshCw size={14} className={loading ? "animate-spin" : ""} />
            Refresh
          </button>
        </div>
      </div>

      {error && (
        <div className="rounded-lg bg-red-900/20 border border-red-500/30 px-4 py-3 text-sm text-red-400">
          {error}
        </div>
      )}

      {/* Summary strip */}
      <div className="grid grid-cols-2 gap-4 lg:grid-cols-4">
        <StatCard
          label="Total connections"
          value={summary ? fmtNum(summary.total_connections) : "—"}
          sub={`last ${window}`}
          icon={<Activity size={16} className="text-indigo-400" />}
          accent="bg-indigo-500/10"
        />
        <StatCard
          label="External connections"
          value={summary ? fmtNum(summary.external_connections) : "—"}
          sub={
            summary
              ? `${summary.total_connections > 0
                  ? ((summary.external_connections / summary.total_connections) * 100).toFixed(1)
                  : "0"}% of total`
              : undefined
          }
          icon={<Globe size={16} className="text-orange-400" />}
          accent="bg-orange-500/10"
        />
        <StatCard
          label="TLS issues"
          value={loading ? "—" : tlsIssues}
          sub={tlsIssues === 0 ? "All certs healthy" : "expired or expiring soon"}
          icon={<ShieldAlert size={16} className={tlsIssues > 0 ? "text-red-400" : "text-emerald-400"} />}
          accent={tlsIssues > 0 ? "bg-red-500/10" : "bg-emerald-500/10"}
        />
        <StatCard
          label="External destinations"
          value={loading ? "—" : external.length}
          sub="unique non-RFC1918 IPs"
          icon={<ArrowUpRight size={16} className="text-purple-400" />}
          accent="bg-purple-500/10"
        />
      </div>

      {/* Tabbed detail panels */}
      <div className="rounded-xl bg-[#0d0d1a] border border-white/[0.06] overflow-hidden">
        <TabBar active={tab} onChange={setTab} counts={tabCounts} />
        <div>
          {tab === "talkers"     && <TopTalkersPanel talkers={talkers} />}
          {tab === "external"    && <ExternalPanel destinations={external} />}
          {tab === "tls"         && <TLSPanel certs={certs} />}
          {tab === "connections" && <ConnectionLogPanel window={window} />}
          {tab === "geo"         && <GeoPanel countries={countries} window={window} />}
        </div>
      </div>
    </div>
  );
}
