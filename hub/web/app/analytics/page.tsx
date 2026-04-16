"use client";

import { useCallback, useEffect, useState } from "react";
import {
  BarChart,
  Bar,
  XAxis,
  YAxis,
  Tooltip as RechartsTooltip,
  ResponsiveContainer,
  Cell,
} from "recharts";
import { BarChart3, RefreshCw, AlertTriangle, CheckCircle, Clock } from "lucide-react";
import { clsx } from "clsx";
import { fetchEndpointStats } from "@/lib/api";
import type { EndpointStat, EndpointStatsResponse } from "@/lib/api";

const WINDOWS = [
  { value: "15m", label: "15 min" },
  { value: "1h",  label: "1 hour" },
  { value: "6h",  label: "6 hours" },
  { value: "24h", label: "24 hours" },
];

// ── Helpers ────────────────────────────────────────────────────────────────────

function latencyColor(ms: number): string {
  if (ms < 100) return "text-emerald-400";
  if (ms < 500) return "text-yellow-400";
  return "text-red-400";
}

function errorRateColor(rate: number): string {
  if (rate < 1)  return "text-emerald-400";
  if (rate < 5)  return "text-yellow-400";
  return "text-red-400";
}

function fmtMs(ms: number): string {
  return ms >= 1000 ? `${(ms / 1000).toFixed(2)}s` : `${ms.toFixed(0)}ms`;
}

function methodBadgeClass(method: string): string {
  switch (method.toUpperCase()) {
    case "GET":    return "bg-emerald-500/10 text-emerald-400";
    case "POST":   return "bg-blue-500/10 text-blue-400";
    case "PUT":    return "bg-yellow-500/10 text-yellow-400";
    case "PATCH":  return "bg-purple-500/10 text-purple-400";
    case "DELETE": return "bg-red-500/10 text-red-400";
    default:       return "bg-slate-700/50 text-slate-400";
  }
}

// ── Summary cards ──────────────────────────────────────────────────────────────

function computeSummary(endpoints: EndpointStat[]) {
  if (endpoints.length === 0) {
    return { totalRequests: 0, avgErrorRate: 0, avgLatency: 0, p99: 0 };
  }
  const totalRequests = endpoints.reduce((s, e) => s + e.count, 0);
  const totalErrors   = endpoints.reduce((s, e) => s + e.error_count, 0);
  const avgErrorRate  = totalRequests > 0 ? (totalErrors / totalRequests) * 100 : 0;
  const weightedLat   = endpoints.reduce((s, e) => s + e.avg_latency_ms * e.count, 0);
  const avgLatency    = weightedLat / totalRequests;
  const p99           = Math.max(...endpoints.map((e) => e.p99_ms));
  return { totalRequests, avgErrorRate, avgLatency, p99 };
}

// ── Page ───────────────────────────────────────────────────────────────────────

export default function AnalyticsPage() {
  const [timeWindow, setTimeWindow] = useState("1h");
  const [data, setData] = useState<EndpointStatsResponse | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(async (w: string) => {
    setLoading(true);
    setError(null);
    try {
      setData(await fetchEndpointStats(w));
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to load analytics");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { load(timeWindow); }, [timeWindow, load]);

  const endpoints = data?.endpoints ?? [];
  const { totalRequests, avgErrorRate, avgLatency, p99 } = computeSummary(endpoints);

  // Top 10 for bar chart (by count)
  const chartData = endpoints
    .slice(0, 10)
    .map((e) => ({
      name: `${e.method} ${e.path.length > 28 ? e.path.slice(0, 26) + "…" : e.path}`,
      count: e.count,
      errorRate: e.error_rate,
    }));

  return (
    <div className="p-6 space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-3">
          <div className="p-2 rounded-lg bg-indigo-500/10">
            <BarChart3 size={20} className="text-indigo-400" />
          </div>
          <div>
            <h1 className="text-lg font-semibold text-white">HTTP Endpoint Analytics</h1>
            <p className="text-xs text-slate-400 mt-0.5">
              Latency histograms and error rates per endpoint
            </p>
          </div>
        </div>

        <div className="flex items-center gap-3">
          <div className="flex rounded-md overflow-hidden border border-white/10">
            {WINDOWS.map((w) => (
              <button
                key={w.value}
                onClick={() => setTimeWindow(w.value)}
                className={clsx(
                  "px-3 py-1.5 text-xs transition-colors",
                  timeWindow === w.value
                    ? "bg-indigo-500/20 text-indigo-300"
                    : "text-slate-400 hover:text-slate-200 hover:bg-white/[0.04]",
                )}
              >
                {w.label}
              </button>
            ))}
          </div>

          <button
            onClick={() => load(timeWindow)}
            disabled={loading}
            className="flex items-center gap-1.5 px-3 py-1.5 rounded-md text-xs
                       bg-white/[0.04] border border-white/10 text-slate-300
                       hover:bg-white/[0.08] disabled:opacity-40 transition-colors"
          >
            <RefreshCw size={12} className={loading ? "animate-spin" : ""} />
            Refresh
          </button>
        </div>
      </div>

      {error && (
        <div className="flex items-center gap-2 text-sm text-red-400 bg-red-500/10
                        border border-red-500/20 rounded-lg px-4 py-3">
          <AlertTriangle size={14} />
          {error}
        </div>
      )}

      {/* Summary cards */}
      <div className="grid grid-cols-2 lg:grid-cols-4 gap-4">
        <SummaryCard
          icon={<BarChart3 size={16} className="text-indigo-400" />}
          label="Total requests"
          value={totalRequests.toLocaleString()}
        />
        <SummaryCard
          icon={<AlertTriangle size={16} className={errorRateColor(avgErrorRate)} />}
          label="Error rate"
          value={`${avgErrorRate.toFixed(2)}%`}
          valueClass={errorRateColor(avgErrorRate)}
        />
        <SummaryCard
          icon={<Clock size={16} className={latencyColor(avgLatency)} />}
          label="Avg latency"
          value={fmtMs(avgLatency)}
          valueClass={latencyColor(avgLatency)}
        />
        <SummaryCard
          icon={<CheckCircle size={16} className={latencyColor(p99)} />}
          label="p99 latency"
          value={fmtMs(p99)}
          valueClass={latencyColor(p99)}
        />
      </div>

      {/* Bar chart */}
      {endpoints.length > 0 && (
        <div className="bg-[#0d0d1a] border border-white/[0.06] rounded-xl p-4">
          <p className="text-sm font-medium text-slate-300 mb-4">Top 10 endpoints by request count</p>
          <ResponsiveContainer width="100%" height={240}>
            <BarChart data={chartData} layout="vertical" margin={{ left: 8, right: 24 }}>
              <XAxis
                type="number"
                tick={{ fill: "#64748b", fontSize: 11 }}
                axisLine={false}
                tickLine={false}
              />
              <YAxis
                type="category"
                dataKey="name"
                width={200}
                tick={{ fill: "#94a3b8", fontSize: 10 }}
                axisLine={false}
                tickLine={false}
              />
              <RechartsTooltip
                contentStyle={{
                  background: "#0f172a",
                  border: "1px solid #1e293b",
                  borderRadius: 6,
                  fontSize: 12,
                  color: "#e2e8f0",
                }}
                cursor={{ fill: "rgba(255,255,255,0.03)" }}
              />
              <Bar dataKey="count" radius={[0, 4, 4, 0]} maxBarSize={20}>
                {chartData.map((entry, i) => (
                  <Cell
                    key={i}
                    fill={entry.errorRate > 5 ? "#ef4444" : entry.errorRate > 1 ? "#f59e0b" : "#6366f1"}
                    fillOpacity={0.8}
                  />
                ))}
              </Bar>
            </BarChart>
          </ResponsiveContainer>
        </div>
      )}

      {/* Detailed table */}
      <div className="bg-[#0d0d1a] border border-white/[0.06] rounded-xl overflow-hidden">
        <div className="flex items-center justify-between px-4 py-3 border-b border-white/[0.06]">
          <p className="text-sm font-medium text-slate-300">Endpoint details</p>
          {data && (
            <p className="text-xs text-slate-500">{data.total} endpoints in the last {data.window}</p>
          )}
        </div>

        {loading && !data ? (
          <div className="flex items-center justify-center h-40">
            <RefreshCw size={20} className="animate-spin text-indigo-400" />
          </div>
        ) : endpoints.length === 0 ? (
          <div className="flex flex-col items-center justify-center h-40 text-center gap-2">
            <p className="text-sm text-slate-500">No HTTP flows in the selected window</p>
            <p className="text-xs text-slate-600">Capture HTTP traffic to populate this view</p>
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-white/[0.04]">
                  {["Method", "Path", "Requests", "Error Rate", "Avg", "p50", "p95", "p99"].map((h) => (
                    <th
                      key={h}
                      className="text-left px-4 py-2.5 text-xs font-medium text-slate-500 whitespace-nowrap"
                    >
                      {h}
                    </th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {endpoints.map((e, i) => (
                  <tr
                    key={i}
                    className="border-b border-white/[0.03] hover:bg-white/[0.02] transition-colors"
                  >
                    <td className="px-4 py-2.5">
                      <span className={clsx(
                        "px-2 py-0.5 rounded text-[10px] font-semibold",
                        methodBadgeClass(e.method),
                      )}>
                        {e.method}
                      </span>
                    </td>
                    <td className="px-4 py-2.5 font-mono text-xs text-slate-300 max-w-[280px] truncate">
                      {e.path}
                    </td>
                    <td className="px-4 py-2.5 text-xs text-slate-300">
                      {e.count.toLocaleString()}
                    </td>
                    <td className="px-4 py-2.5">
                      <ErrorRateBar rate={e.error_rate} />
                    </td>
                    <td className={clsx("px-4 py-2.5 text-xs font-mono", latencyColor(e.avg_latency_ms))}>
                      {fmtMs(e.avg_latency_ms)}
                    </td>
                    <td className={clsx("px-4 py-2.5 text-xs font-mono", latencyColor(e.p50_ms))}>
                      {fmtMs(e.p50_ms)}
                    </td>
                    <td className={clsx("px-4 py-2.5 text-xs font-mono", latencyColor(e.p95_ms))}>
                      {fmtMs(e.p95_ms)}
                    </td>
                    <td className={clsx("px-4 py-2.5 text-xs font-mono", latencyColor(e.p99_ms))}>
                      {fmtMs(e.p99_ms)}
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

// ── Sub-components ─────────────────────────────────────────────────────────────

function SummaryCard({
  icon,
  label,
  value,
  valueClass,
}: {
  icon: React.ReactNode;
  label: string;
  value: string;
  valueClass?: string;
}) {
  return (
    <div className="bg-[#0d0d1a] border border-white/[0.06] rounded-lg px-4 py-3 flex flex-col gap-2">
      <div className="flex items-center gap-2 text-xs text-slate-500">
        {icon}
        {label}
      </div>
      <p className={clsx("text-2xl font-semibold", valueClass ?? "text-white")}>{value}</p>
    </div>
  );
}

function ErrorRateBar({ rate }: { rate: number }) {
  const color = rate < 1 ? "bg-emerald-500" : rate < 5 ? "bg-yellow-500" : "bg-red-500";
  return (
    <div className="flex items-center gap-2">
      <div className="w-20 h-1.5 rounded-full bg-white/10">
        <div
          className={clsx("h-full rounded-full", color)}
          style={{ width: `${Math.min(rate, 100)}%` }}
        />
      </div>
      <span className={clsx("text-xs font-mono", errorRateColor(rate))}>
        {rate.toFixed(1)}%
      </span>
    </div>
  );
}
