import { useMemo, useState } from "react";
import { BarChart2, TrendingUp, TrendingDown, Minus } from "lucide-react";
import { useCaptureStore } from "@/store/captureStore";
import { cn } from "@/lib/utils";
import type { EndpointStats } from "@/store/captureStore";

// ── Method badge ──────────────────────────────────────────────────────────────

function MethodBadge({ method }: { method: string }) {
  const cfg: Record<string, string> = {
    GET:    "bg-emerald-900/40 text-emerald-300",
    POST:   "bg-blue-900/40    text-blue-300",
    PUT:    "bg-amber-900/40   text-amber-300",
    PATCH:  "bg-purple-900/40  text-purple-300",
    DELETE: "bg-red-900/40     text-red-300",
  };
  return (
    <span
      className={cn(
        "rounded px-1.5 py-px text-[9px] font-bold",
        cfg[method.toUpperCase()] ?? "bg-gray-800 text-gray-400",
      )}
    >
      {method.toUpperCase()}
    </span>
  );
}

// ── Latency bar ───────────────────────────────────────────────────────────────

function LatencyBar({
  value,
  max,
  colorClass,
}: {
  value: number;
  max: number;
  colorClass: string;
}) {
  const pct = max > 0 ? Math.min(100, (value / max) * 100) : 0;
  return (
    <div className="flex items-center gap-1.5">
      <div className="h-1.5 w-16 rounded-full bg-white/10 overflow-hidden">
        <div
          className={cn("h-full rounded-full transition-all", colorClass)}
          style={{ width: `${pct}%` }}
        />
      </div>
      <span className="tabular-nums text-gray-300 w-12">{value}ms</span>
    </div>
  );
}

// ── Sort control ──────────────────────────────────────────────────────────────

type SortKey = "count" | "p95" | "errorRate";

// ── Analytics pane ────────────────────────────────────────────────────────────

export function AnalyticsPane() {
  const { endpointStats } = useCaptureStore();
  const [sortBy, setSortBy] = useState<SortKey>("count");

  const sorted = useMemo(() => {
    return [...endpointStats].sort((a, b) => {
      if (sortBy === "count") return b.count - a.count;
      if (sortBy === "p95") return b.p95 - a.p95;
      if (sortBy === "errorRate") return b.errorRate - a.errorRate;
      return 0;
    });
  }, [endpointStats, sortBy]);

  const maxP99 = useMemo(
    () => Math.max(1, ...endpointStats.map((s) => s.p99)),
    [endpointStats],
  );

  const totalRequests = endpointStats.reduce((sum, s) => sum + s.count, 0);
  const avgError =
    endpointStats.length > 0
      ? endpointStats.reduce((sum, s) => sum + s.errorRate, 0) / endpointStats.length
      : 0;
  const avgP95 =
    endpointStats.length > 0
      ? endpointStats.reduce((sum, s) => sum + s.p95, 0) / endpointStats.length
      : 0;

  if (endpointStats.length === 0) {
    return (
      <div className="flex h-full items-center justify-center gap-2 text-xs text-gray-500">
        <BarChart2 className="h-4 w-4 text-gray-700" />
        No HTTP flows yet — analytics will appear as requests are captured
      </div>
    );
  }

  return (
    <div className="flex h-full flex-col overflow-hidden bg-[#0a0a14]">
      {/* Summary cards */}
      <div className="flex shrink-0 gap-4 border-b border-white/10 px-4 py-2">
        <StatCard label="Total requests" value={totalRequests.toLocaleString()} />
        <StatCard label="Avg error rate" value={`${avgError.toFixed(1)}%`} warn={avgError > 5} />
        <StatCard label="Avg p95 latency" value={`${avgP95.toFixed(0)}ms`} warn={avgP95 > 500} />
        <StatCard label="Endpoints" value={endpointStats.length.toString()} />
      </div>

      {/* Table header */}
      <div className="flex shrink-0 items-center gap-3 border-b border-white/10 bg-[#0d0d1a] px-4 py-1.5 text-[10px] font-semibold uppercase tracking-wide text-gray-500">
        <span className="w-12">Method</span>
        <span className="flex-1">Endpoint</span>
        <SortHeader label="Reqs" value="count" current={sortBy} onSort={setSortBy} />
        <span className="w-28">p50 / p95 / p99</span>
        <SortHeader label="Error %" value="errorRate" current={sortBy} onSort={setSortBy} />
      </div>

      {/* Rows */}
      <div className="flex-1 overflow-y-auto">
        {sorted.map((stat, i) => (
          <EndpointRow key={i} stat={stat} maxP99={maxP99} />
        ))}
      </div>
    </div>
  );
}

function StatCard({
  label,
  value,
  warn = false,
}: {
  label: string;
  value: string;
  warn?: boolean;
}) {
  return (
    <div className="flex flex-col">
      <span className="text-[10px] text-gray-500">{label}</span>
      <span className={cn("text-sm font-semibold tabular-nums", warn ? "text-amber-400" : "text-white")}>
        {value}
      </span>
    </div>
  );
}

function SortHeader({
  label,
  value,
  current,
  onSort,
}: {
  label: string;
  value: SortKey;
  current: SortKey;
  onSort: (k: SortKey) => void;
}) {
  return (
    <button
      className={cn(
        "w-20 text-left transition-colors hover:text-white",
        current === value ? "text-blue-400" : "",
      )}
      onClick={() => onSort(value)}
    >
      {label} {current === value && "▴"}
    </button>
  );
}

function EndpointRow({
  stat,
  maxP99,
}: {
  stat: EndpointStats;
  maxP99: number;
}) {
  const errorHigh = stat.errorRate > 10;
  const ErrorIcon = errorHigh ? TrendingDown : stat.errorRate > 0 ? Minus : TrendingUp;
  const errorCls = errorHigh
    ? "text-red-400"
    : stat.errorRate > 0
    ? "text-amber-400"
    : "text-emerald-400";

  return (
    <div className="flex items-center gap-3 border-b border-white/5 px-4 py-1.5 text-xs font-mono hover:bg-white/[0.03]">
      <span className="w-12">
        <MethodBadge method={stat.method} />
      </span>
      <span className="flex-1 min-w-0">
        <span className="text-gray-400 truncate block" title={`${stat.host}${stat.path}`}>
          <span className="text-gray-500">{stat.host}</span>
          {stat.path}
        </span>
      </span>
      <span className="w-20 tabular-nums text-gray-300">{stat.count.toLocaleString()}</span>
      <div className="w-28 space-y-0.5">
        <LatencyBar value={stat.p50} max={maxP99} colorClass="bg-emerald-500/70" />
        <LatencyBar value={stat.p95} max={maxP99} colorClass="bg-amber-500/70" />
        <LatencyBar value={stat.p99} max={maxP99} colorClass="bg-red-500/70" />
      </div>
      <span className={cn("w-20 flex items-center gap-1 tabular-nums", errorCls)}>
        <ErrorIcon className="h-3 w-3 shrink-0" />
        {stat.errorRate.toFixed(1)}%
      </span>
    </div>
  );
}
