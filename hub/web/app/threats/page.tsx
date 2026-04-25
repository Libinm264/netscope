"use client";

import { useState, useEffect, useCallback } from "react";
import { fetchThreats, type ThreatIP, type ThreatSummary } from "@/lib/api";
import { Shield, RefreshCw, Globe, Cpu } from "lucide-react";
import { clsx } from "clsx";

const WINDOWS = ["1h", "6h", "24h", "7d"] as const;

function ThreatBadge({ level }: { level: string }) {
  const cls = {
    high:   "bg-red-500/15 text-red-400 border-red-500/30",
    medium: "bg-amber-500/15 text-amber-400 border-amber-500/30",
    low:    "bg-yellow-500/15 text-yellow-400 border-yellow-500/30",
  }[level] ?? "bg-slate-500/15 text-slate-400 border-slate-500/30";
  return (
    <span className={clsx("inline-flex items-center px-1.5 py-0.5 rounded text-[10px] font-semibold border uppercase", cls)}>
      {level || "unknown"}
    </span>
  );
}

function ScoreBar({ score }: { score: number }) {
  const pct = Math.min(100, score);
  const color = score >= 70 ? "bg-red-500" : score >= 40 ? "bg-amber-500" : "bg-yellow-500";
  return (
    <div className="flex items-center gap-2">
      <div className="w-16 h-1.5 rounded-full bg-white/[0.06] overflow-hidden">
        <div className={clsx("h-full rounded-full", color)} style={{ width: `${pct}%` }} />
      </div>
      <span className="text-xs tabular-nums text-slate-400">{score}</span>
    </div>
  );
}

export default function ThreatsPage() {
  const [threats, setThreats] = useState<ThreatIP[]>([]);
  const [summary, setSummary] = useState<ThreatSummary>({ total: 0, high: 0, medium: 0, low: 0 });
  const [window, setWindow] = useState("24h");
  const [loading, setLoading] = useState(false);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const res = await fetchThreats({ window });
      setThreats(res.threats ?? []);
      setSummary(res.summary ?? { total: 0, high: 0, medium: 0, low: 0 });
    } catch (e) {
      console.error(e);
    } finally {
      setLoading(false);
    }
  }, [window]);

  useEffect(() => { load(); }, [load]);

  return (
    <div className="p-6 space-y-5">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-lg font-semibold text-white">Threat Intelligence</h1>
          <p className="text-xs text-slate-500 mt-0.5">
            Destination IPs scored against AbuseIPDB and threat blocklists
          </p>
        </div>
        <div className="flex items-center gap-2">
          {WINDOWS.map(w => (
            <button key={w} onClick={() => setWindow(w)}
              className={clsx("px-2.5 py-1 rounded text-xs border transition-colors",
                window === w
                  ? "border-indigo-500/50 bg-indigo-500/10 text-indigo-300"
                  : "border-white/[0.08] text-slate-500 hover:text-slate-300 hover:border-white/20"
              )}>
              {w}
            </button>
          ))}
          <button onClick={load}
            className="p-2 rounded-md border border-white/10 text-slate-500 hover:text-slate-300 hover:bg-white/[0.04] transition-colors ml-1">
            <RefreshCw size={13} className={loading ? "animate-spin" : ""} />
          </button>
        </div>
      </div>

      {/* ── What is this callout ── */}
      <div className="rounded-lg border border-slate-700/50 bg-slate-800/20 px-4 py-3 text-xs text-slate-400 space-y-1">
        <p className="text-white font-medium flex items-center gap-1.5">
          <Shield size={13} className="text-indigo-400" />
          How threat scoring works
        </p>
        <p>
          Every destination IP seen in your flows is scored 0–100 against{" "}
          <span className="text-slate-300">AbuseIPDB</span> and a curated blocklist (C2 servers, Tor exits, scanners).
          Scores ≥ 70 = <span className="text-red-400 font-medium">High</span>,{" "}
          40–69 = <span className="text-amber-400 font-medium">Medium</span>,{" "}
          1–39 = <span className="text-yellow-400 font-medium">Low</span>.
          Set your <span className="text-indigo-400">ABUSEIPDB_KEY</span> environment variable on the hub to enable live scoring.
          In eBPF mode, the <span className="text-emerald-400">Process</span> column shows which process made the connection.
        </p>
      </div>

      {/* Summary cards */}
      <div className="grid grid-cols-4 gap-3">
        {[
          { label: "Total threat IPs", value: summary.total, color: "text-white" },
          { label: "High severity",    value: summary.high,   color: "text-red-400" },
          { label: "Medium severity",  value: summary.medium, color: "text-amber-400" },
          { label: "Low severity",     value: summary.low,    color: "text-yellow-400" },
        ].map(({ label, value, color }) => (
          <div key={label} className="rounded-xl bg-[#0d0d1a] border border-white/[0.06] p-4">
            <p className="text-xs text-slate-500">{label}</p>
            <p className={clsx("text-2xl font-bold mt-1", color)}>{value}</p>
          </div>
        ))}
      </div>

      {/* Threat table */}
      {threats.length === 0 && !loading ? (
        <div className="rounded-xl bg-[#0d0d1a] border border-white/[0.06] flex flex-col items-center justify-center py-16 gap-3">
          <Shield size={32} className="text-slate-700" />
          <p className="text-sm text-slate-500">No threat-scored IPs in this window</p>
          <p className="text-xs text-slate-600">
            Set ABUSEIPDB_KEY on the hub or add a custom blocklist to enable threat scoring.
          </p>
        </div>
      ) : (
        <div className="rounded-xl bg-[#0d0d1a] border border-white/[0.06] overflow-hidden">
          {/* Table header */}
          <div className="flex items-center px-4 py-2 border-b border-white/[0.06] text-[11px] font-semibold text-slate-500 uppercase tracking-wide">
            <span className="w-36 shrink-0">IP Address</span>
            <span className="w-24 shrink-0">Severity</span>
            <span className="w-28 shrink-0">Score</span>
            <span className="w-32 shrink-0 flex items-center gap-1"><Globe size={10} /> Country</span>
            <span className="w-40 shrink-0">ASN / Org</span>
            <span className="w-16 shrink-0 text-right">Flows</span>
            <span className="w-36 shrink-0 pl-4 flex items-center gap-1"><Cpu size={10} /> Processes</span>
            <span className="flex-1 min-w-0 text-right">Last seen</span>
          </div>
          <div className="divide-y divide-white/[0.03] font-mono text-xs">
            {threats.map(t => (
              <div key={t.dst_ip}
                className="flex items-center px-4 py-2 hover:bg-white/[0.02] transition-colors">
                <span className="w-36 shrink-0 text-slate-200">{t.dst_ip}</span>
                <span className="w-24 shrink-0"><ThreatBadge level={t.threat_level} /></span>
                <span className="w-28 shrink-0"><ScoreBar score={t.threat_score} /></span>
                <span className="w-32 shrink-0 text-slate-400">
                  {t.country_code ? `${t.country_code} ${t.country_name}` : "—"}
                </span>
                <span className="w-40 shrink-0 truncate text-slate-500" title={t.as_org}>{t.as_org || "—"}</span>
                <span className="w-16 shrink-0 text-right tabular-nums text-slate-400">{t.flow_count.toLocaleString()}</span>
                <span className="w-36 shrink-0 pl-4 truncate text-emerald-400/70">
                  {t.processes.length > 0 ? t.processes.slice(0, 2).join(", ") : "—"}
                </span>
                <span className="flex-1 min-w-0 text-right text-slate-600">
                  {new Date(t.last_seen).toLocaleTimeString("en-US", { hour12: false, hour: "2-digit", minute: "2-digit", second: "2-digit" })}
                </span>
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}
