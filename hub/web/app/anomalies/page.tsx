"use client";

import { useEffect, useState, useCallback } from "react";
import {
  fetchAnomalies,
  fetchAnomalyStats,
  type AnomalyEvent,
  type AnomalyStats,
  type AnomalySeverity,
} from "@/lib/api";
import { Activity, TrendingUp, TrendingDown, RefreshCw, AlertTriangle } from "lucide-react";
import { clsx } from "clsx";

// ── Severity helpers ──────────────────────────────────────────────────────────

const SEV_STYLES: Record<AnomalySeverity, string> = {
  high:   "bg-red-500/10    text-red-400    border-red-500/20",
  medium: "bg-amber-500/10  text-amber-400  border-amber-500/20",
  low:    "bg-blue-500/10   text-blue-400   border-blue-500/20",
};

const SEV_DOT: Record<AnomalySeverity, string> = {
  high:   "bg-red-500",
  medium: "bg-amber-500",
  low:    "bg-blue-500",
};

function SeverityBadge({ severity }: { severity: AnomalySeverity }) {
  return (
    <span className={clsx(
      "inline-flex items-center gap-1 px-1.5 py-0.5 rounded text-[10px] font-medium border",
      SEV_STYLES[severity],
    )}>
      <span className={clsx("h-1.5 w-1.5 rounded-full", SEV_DOT[severity])} />
      {severity.toUpperCase()}
    </span>
  );
}

function TypeIcon({ type }: { type: "spike" | "drop" }) {
  return type === "spike"
    ? <TrendingUp  className="h-3.5 w-3.5 text-red-400"  />
    : <TrendingDown className="h-3.5 w-3.5 text-blue-400" />;
}

// ── Stat card ─────────────────────────────────────────────────────────────────

function StatCard({
  label, value, color,
}: { label: string; value: number; color: string }) {
  return (
    <div className="rounded-lg border border-white/[0.06] bg-white/[0.02] px-4 py-3">
      <p className="text-[11px] text-slate-500 uppercase tracking-wide">{label}</p>
      <p className={clsx("mt-1 text-2xl font-semibold tabular-nums", color)}>
        {value.toLocaleString()}
      </p>
    </div>
  );
}

// ── Page ─────────────────────────────────────────────────────────────────────

const HOURS_OPTIONS = [6, 24, 48, 168] as const;
type HoursOption = (typeof HOURS_OPTIONS)[number];

export default function AnomaliesPage() {
  const [stats,     setStats]     = useState<AnomalyStats | null>(null);
  const [events,    setEvents]    = useState<AnomalyEvent[]>([]);
  const [loading,   setLoading]   = useState(true);
  const [error,     setError]     = useState<string | null>(null);
  const [hours,     setHours]     = useState<HoursOption>(24);
  const [severity,  setSeverity]  = useState<AnomalySeverity | "">("");
  const [refreshing, setRefreshing] = useState(false);

  const load = useCallback(async (showSpinner = false) => {
    if (showSpinner) setRefreshing(true);
    setError(null);
    try {
      const [s, e] = await Promise.all([
        fetchAnomalyStats(),
        fetchAnomalies({
          hours,
          severity: severity || undefined,
          limit: 200,
        }),
      ]);
      setStats(s);
      setEvents(e.events);
    } catch (err) {
      setError(String(err));
    } finally {
      setLoading(false);
      setRefreshing(false);
    }
  }, [hours, severity]);

  // Initial load + auto-refresh every 30 s
  useEffect(() => {
    load();
    const id = setInterval(() => load(), 30_000);
    return () => clearInterval(id);
  }, [load]);

  return (
    <div className="flex flex-col h-full min-h-0 bg-[#070711]">
      {/* Header */}
      <div className="flex items-center justify-between px-6 py-4 border-b border-white/[0.06]">
        <div className="flex items-center gap-3">
          <Activity className="h-5 w-5 text-indigo-400" />
          <div>
            <h1 className="text-base font-semibold text-white">Anomaly Detection</h1>
            <p className="text-xs text-slate-500">
              Z-score deviations from your 7-day behavioral baseline
            </p>
          </div>
        </div>
        <button
          onClick={() => load(true)}
          disabled={refreshing}
          className="flex items-center gap-1.5 text-xs text-slate-400 hover:text-white
                     px-2.5 py-1.5 rounded border border-white/10 hover:border-white/20
                     transition-colors disabled:opacity-50"
        >
          <RefreshCw className={clsx("h-3 w-3", refreshing && "animate-spin")} />
          Refresh
        </button>
      </div>

      <div className="flex-1 overflow-y-auto px-6 py-5 space-y-5">
        {/* Stats row */}
        {stats && (
          <div className="grid grid-cols-4 gap-3">
            <StatCard label="Total (24 h)"  value={stats.total_24h} color="text-white" />
            <StatCard label="High"          value={stats.high}      color="text-red-400" />
            <StatCard label="Medium"        value={stats.medium}    color="text-amber-400" />
            <StatCard label="Low"           value={stats.low}       color="text-blue-400" />
          </div>
        )}

        {/* Filters */}
        <div className="flex items-center gap-3">
          <div className="flex items-center gap-1 rounded-md border border-white/[0.08] bg-white/[0.02] p-0.5">
            {HOURS_OPTIONS.map((h) => (
              <button
                key={h}
                onClick={() => setHours(h)}
                className={clsx(
                  "px-3 py-1 rounded text-xs font-medium transition-colors",
                  hours === h
                    ? "bg-indigo-600 text-white"
                    : "text-slate-400 hover:text-white",
                )}
              >
                {h < 48 ? `${h}h` : `${h / 24}d`}
              </button>
            ))}
          </div>

          <select
            value={severity}
            onChange={(e) => setSeverity(e.target.value as AnomalySeverity | "")}
            className="text-xs bg-white/[0.03] border border-white/[0.08] rounded px-2.5 py-1.5
                       text-slate-300 focus:outline-none focus:ring-1 focus:ring-indigo-500"
          >
            <option value="">All severities</option>
            <option value="high">High</option>
            <option value="medium">Medium</option>
            <option value="low">Low</option>
          </select>
        </div>

        {/* Content */}
        {loading ? (
          <div className="flex justify-center py-20">
            <RefreshCw className="h-5 w-5 animate-spin text-slate-500" />
          </div>
        ) : error ? (
          <div className="flex items-center gap-2 rounded-lg border border-red-500/20
                          bg-red-500/5 px-4 py-3 text-sm text-red-400">
            <AlertTriangle className="h-4 w-4 shrink-0" />
            {error}
          </div>
        ) : events.length === 0 ? (
          <div className="flex flex-col items-center justify-center py-20 text-center">
            <Activity className="h-8 w-8 text-slate-600 mb-3" />
            <p className="text-sm text-slate-400">No anomalies detected</p>
            <p className="text-xs text-slate-600 mt-1">
              {stats?.total_24h === 0
                ? "The baseline engine needs at least 3 hours of traffic to start detecting anomalies."
                : `No anomalies in the last ${hours < 48 ? `${hours}h` : `${hours / 24}d`}.`}
            </p>
          </div>
        ) : (
          <div className="rounded-lg border border-white/[0.06] overflow-hidden">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-white/[0.06] bg-white/[0.02]">
                  <th className="px-4 py-2.5 text-left text-xs font-medium text-slate-500">Time</th>
                  <th className="px-4 py-2.5 text-left text-xs font-medium text-slate-500">Severity</th>
                  <th className="px-4 py-2.5 text-left text-xs font-medium text-slate-500">Type</th>
                  <th className="px-4 py-2.5 text-left text-xs font-medium text-slate-500">Agent</th>
                  <th className="px-4 py-2.5 text-left text-xs font-medium text-slate-500">Protocol</th>
                  <th className="px-4 py-2.5 text-left text-xs font-medium text-slate-500">Z-Score</th>
                  <th className="px-4 py-2.5 text-left text-xs font-medium text-slate-500 w-1/3">Description</th>
                </tr>
              </thead>
              <tbody>
                {events.map((ev) => (
                  <tr
                    key={ev.id}
                    className="border-b border-white/[0.04] hover:bg-white/[0.02] transition-colors"
                  >
                    <td className="px-4 py-2.5 text-xs text-slate-400 whitespace-nowrap">
                      {new Date(ev.detected_at).toLocaleString()}
                    </td>
                    <td className="px-4 py-2.5">
                      <SeverityBadge severity={ev.severity as AnomalySeverity} />
                    </td>
                    <td className="px-4 py-2.5">
                      <div className="flex items-center gap-1.5">
                        <TypeIcon type={ev.anomaly_type} />
                        <span className="text-xs text-slate-300 capitalize">{ev.anomaly_type}</span>
                      </div>
                    </td>
                    <td className="px-4 py-2.5 text-xs text-slate-300 font-mono">
                      {ev.hostname || ev.agent_id.slice(0, 8)}
                    </td>
                    <td className="px-4 py-2.5">
                      <span className="px-1.5 py-0.5 rounded text-[10px] font-medium
                                       bg-slate-800 text-slate-300 border border-white/[0.08]">
                        {ev.protocol}
                      </span>
                    </td>
                    <td className="px-4 py-2.5 text-xs tabular-nums font-mono">
                      <span className={clsx(
                        ev.z_score > 0 ? "text-red-400" : "text-blue-400",
                      )}>
                        {ev.z_score > 0 ? "+" : ""}{ev.z_score.toFixed(1)}σ
                      </span>
                    </td>
                    <td className="px-4 py-2.5 text-xs text-slate-400 leading-relaxed">
                      {ev.description}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </div>
  );
}
