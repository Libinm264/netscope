"use client";

import { useCallback, useEffect, useState } from "react";
import { ShieldCheck, ShieldAlert, ShieldX, RefreshCw, Clock } from "lucide-react";
import { clsx } from "clsx";
import { fetchCerts } from "@/lib/api";
import type { TlsCert, CertSummary } from "@/lib/api";

// ── Status helpers ─────────────────────────────────────────────────────────────

type CertStatus = "expired" | "critical" | "warning" | "ok";

function certStatus(cert: TlsCert): CertStatus {
  if (cert.expired || cert.days_left < 0) return "expired";
  if (cert.days_left <= 7)  return "critical";
  if (cert.days_left <= 30) return "warning";
  return "ok";
}

const STATUS_CONFIG = {
  expired:  { label: "Expired",    bg: "bg-red-500/10",    text: "text-red-400",     icon: ShieldX,     border: "border-red-500/30"    },
  critical: { label: "< 7 days",   bg: "bg-orange-500/10", text: "text-orange-400",  icon: ShieldAlert, border: "border-orange-500/30" },
  warning:  { label: "< 30 days",  bg: "bg-yellow-500/10", text: "text-yellow-400",  icon: ShieldAlert, border: "border-yellow-500/30" },
  ok:       { label: "Valid",       bg: "bg-emerald-500/10",text: "text-emerald-400", icon: ShieldCheck, border: "border-emerald-500/20" },
};

function daysLabel(days: number): string {
  if (days < 0) return `Expired ${Math.abs(days)}d ago`;
  if (days === 0) return "Expires today";
  return `${days}d left`;
}

// ── Page ───────────────────────────────────────────────────────────────────────

export default function CertsPage() {
  const [certs, setCerts] = useState<TlsCert[]>([]);
  const [summary, setSummary] = useState<CertSummary>({ expired: 0, critical: 0, warning: 0, ok: 0 });
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [filter, setFilter] = useState<CertStatus | "all">("all");

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const res = await fetchCerts();
      setCerts(res.certs ?? []);
      setSummary(res.summary);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to load certificates");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { load(); }, [load]);

  const visible = filter === "all"
    ? certs
    : certs.filter((c) => certStatus(c) === filter);

  return (
    <div className="p-6 space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-3">
          <div className="p-2 rounded-lg bg-indigo-500/10">
            <ShieldCheck size={20} className="text-indigo-400" />
          </div>
          <div>
            <h1 className="text-lg font-semibold text-white">TLS Certificate Fleet</h1>
            <p className="text-xs text-slate-400 mt-0.5">
              All certificates observed across your agent fleet
            </p>
          </div>
        </div>
        <button
          onClick={load}
          disabled={loading}
          className="flex items-center gap-1.5 px-3 py-1.5 rounded-md text-xs
                     bg-white/[0.04] border border-white/10 text-slate-300
                     hover:bg-white/[0.08] disabled:opacity-40 transition-colors"
        >
          <RefreshCw size={12} className={loading ? "animate-spin" : ""} />
          Refresh
        </button>
      </div>

      {/* Summary cards */}
      <div className="grid grid-cols-2 lg:grid-cols-4 gap-4">
        {(["expired", "critical", "warning", "ok"] as CertStatus[]).map((s) => {
          const cfg = STATUS_CONFIG[s];
          const Icon = cfg.icon;
          const count = summary[s];
          return (
            <button
              key={s}
              onClick={() => setFilter(filter === s ? "all" : s)}
              className={clsx(
                "text-left rounded-lg px-4 py-3 border transition-all",
                cfg.bg, cfg.border,
                filter === s ? "ring-1 ring-white/20" : "hover:brightness-110",
              )}
            >
              <div className="flex items-center gap-2 mb-1">
                <Icon size={14} className={cfg.text} />
                <span className={clsx("text-xs font-medium", cfg.text)}>{cfg.label}</span>
              </div>
              <p className="text-2xl font-semibold text-white">{count}</p>
            </button>
          );
        })}
      </div>

      {error && (
        <div className="text-sm text-red-400 bg-red-500/10 border border-red-500/20 rounded-lg px-4 py-3">
          {error}
        </div>
      )}

      {/* Cert table */}
      <div className="bg-[#0d0d1a] border border-white/[0.06] rounded-xl overflow-hidden">
        <div className="flex items-center justify-between px-4 py-3 border-b border-white/[0.06]">
          <p className="text-sm font-medium text-slate-300">
            {filter === "all" ? "All certificates" : `${STATUS_CONFIG[filter].label} certificates`}
            <span className="ml-2 text-xs text-slate-500">({visible.length})</span>
          </p>
          {filter !== "all" && (
            <button
              onClick={() => setFilter("all")}
              className="text-xs text-slate-500 hover:text-slate-300 underline"
            >
              Show all
            </button>
          )}
        </div>

        {loading && certs.length === 0 ? (
          <div className="flex items-center justify-center h-40">
            <RefreshCw size={20} className="animate-spin text-indigo-400" />
          </div>
        ) : visible.length === 0 ? (
          <div className="flex flex-col items-center justify-center h-40 gap-2 text-center">
            <ShieldCheck size={24} className="text-slate-600" />
            <p className="text-sm text-slate-500">No certificates observed yet</p>
            <p className="text-xs text-slate-600">
              TLS certificate details are extracted automatically from captured traffic
            </p>
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-white/[0.04]">
                  {["Status", "Common Name", "Issuer", "Expiry", "SANs", "Agent / Host", "Last seen"].map((h) => (
                    <th key={h} className="text-left px-4 py-2.5 text-xs font-medium text-slate-500 whitespace-nowrap">
                      {h}
                    </th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {visible.map((cert) => {
                  const s = certStatus(cert);
                  const cfg = STATUS_CONFIG[s];
                  const Icon = cfg.icon;
                  return (
                    <tr key={cert.fingerprint} className="border-b border-white/[0.03] hover:bg-white/[0.02]">
                      <td className="px-4 py-3">
                        <span className={clsx("flex items-center gap-1.5 text-xs font-medium w-max", cfg.text)}>
                          <Icon size={12} />
                          {cfg.label}
                        </span>
                      </td>
                      <td className="px-4 py-3 font-mono text-xs text-slate-200 max-w-[200px] truncate">
                        {cert.cn || "—"}
                      </td>
                      <td className="px-4 py-3 text-xs text-slate-400 max-w-[160px] truncate">
                        {cert.issuer || "—"}
                      </td>
                      <td className="px-4 py-3">
                        <div className="flex flex-col">
                          <span className="text-xs text-slate-300">{cert.expiry || "—"}</span>
                          {cert.expiry && (
                            <span className={clsx("text-[10px] flex items-center gap-0.5 mt-0.5", cfg.text)}>
                              <Clock size={9} />
                              {daysLabel(cert.days_left)}
                            </span>
                          )}
                        </div>
                      </td>
                      <td className="px-4 py-3 text-xs text-slate-500 max-w-[160px] truncate">
                        {cert.sans?.slice(0, 2).join(", ") || "—"}
                        {(cert.sans?.length ?? 0) > 2 && ` +${cert.sans.length - 2}`}
                      </td>
                      <td className="px-4 py-3 text-xs text-slate-400">
                        {cert.hostname || cert.dst_ip || "—"}
                      </td>
                      <td className="px-4 py-3 text-xs text-slate-500">
                        {cert.last_seen ? new Date(cert.last_seen).toLocaleString() : "—"}
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </div>
  );
}
