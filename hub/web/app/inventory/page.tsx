"use client";

import { useCallback, useEffect, useState } from "react";
import { fetchInventory, type InventoryEndpoint } from "@/lib/api";
import { Database, RefreshCw, Search, AlertTriangle, X } from "lucide-react";
import { clsx } from "clsx";

// ── Protocol badge ────────────────────────────────────────────────────────────

const PROTO_STYLE: Record<string, string> = {
  HTTP:  "bg-blue-500/10 text-blue-400 border-blue-500/20",
  HTTPS: "bg-blue-500/10 text-blue-400 border-blue-500/20",
  HTTP2: "bg-indigo-500/10 text-indigo-400 border-indigo-500/20",
  GRPC:  "bg-indigo-500/10 text-indigo-400 border-indigo-500/20",
};

function ProtoBadge({ proto }: { proto: string }) {
  const cls = PROTO_STYLE[proto.toUpperCase()] ?? "bg-slate-700/40 text-slate-400 border-white/[0.06]";
  return (
    <span className={clsx("inline-flex px-1.5 py-0.5 rounded text-[10px] font-medium border", cls)}>
      {proto}
    </span>
  );
}

// ── Method badge ──────────────────────────────────────────────────────────────

const METHOD_STYLE: Record<string, string> = {
  GET:     "text-emerald-400",
  POST:    "text-indigo-400",
  PUT:     "text-amber-400",
  PATCH:   "text-orange-400",
  DELETE:  "text-red-400",
};

function MethodBadge({ method }: { method: string }) {
  const cls = METHOD_STYLE[method.toUpperCase()] ?? "text-slate-400";
  return <span className={clsx("text-xs font-mono font-semibold w-14 shrink-0", cls)}>{method}</span>;
}

// ── Error rate bar ────────────────────────────────────────────────────────────

function ErrorBar({ rate }: { rate: number }) {
  const pct = Math.min(rate, 100);
  const color = rate > 20 ? "bg-red-500" : rate > 5 ? "bg-amber-500" : "bg-emerald-500";
  return (
    <div className="flex items-center gap-2">
      <div className="w-16 h-1.5 rounded-full bg-white/[0.06] overflow-hidden">
        <div className={clsx("h-full rounded-full transition-all", color)} style={{ width: `${pct}%` }} />
      </div>
      <span className={clsx("text-xs tabular-nums", rate > 20 ? "text-red-400" : rate > 5 ? "text-amber-400" : "text-emerald-400")}>
        {rate.toFixed(1)}%
      </span>
    </div>
  );
}

// ── Relative time ─────────────────────────────────────────────────────────────

function timeAgo(iso: string) {
  const secs = Math.floor((Date.now() - new Date(iso).getTime()) / 1000);
  if (secs < 60)  return `${secs}s ago`;
  const mins = Math.floor(secs / 60);
  if (mins < 60)  return `${mins}m ago`;
  const hrs = Math.floor(mins / 60);
  if (hrs < 24)   return `${hrs}h ago`;
  return `${Math.floor(hrs / 24)}d ago`;
}

// ── Page ──────────────────────────────────────────────────────────────────────

const WINDOWS = ["1h", "6h", "24h", "7d"] as const;
type WindowOpt = (typeof WINDOWS)[number];

export default function InventoryPage() {
  const [endpoints, setEndpoints] = useState<InventoryEndpoint[]>([]);
  const [loading,   setLoading]   = useState(true);
  const [error,     setError]     = useState<string | null>(null);
  const [window,    setWindow]    = useState<WindowOpt>("24h");
  const [search,    setSearch]    = useState("");
  const [hostname,  setHostname]  = useState("");

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const res = await fetchInventory({
        window,
        hostname: hostname || undefined,
        search:   search   || undefined,
      });
      setEndpoints(res.endpoints ?? []);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setLoading(false);
    }
  }, [window, hostname, search]);

  useEffect(() => { load(); }, [load]);

  const newCount = endpoints.filter((e) => e.is_new).length;
  const hostnames = [...new Set(endpoints.map((e) => e.hostname))].sort();

  return (
    <div className="flex flex-col h-full min-h-0 bg-[#070711]">
      {/* Header */}
      <div className="flex items-center justify-between px-6 py-4 border-b border-white/[0.06] shrink-0">
        <div className="flex items-center gap-3">
          <div className="p-1.5 rounded-md bg-indigo-500/10">
            <Database size={15} className="text-indigo-400" />
          </div>
          <div>
            <h1 className="text-sm font-semibold text-white">
              API Inventory
              {newCount > 0 && (
                <span className="ml-2 px-1.5 py-0.5 rounded text-[9px] font-bold uppercase tracking-wide
                                 bg-emerald-500/15 border border-emerald-500/25 text-emerald-400">
                  {newCount} new today
                </span>
              )}
            </h1>
            <p className="text-xs text-slate-500 mt-0.5">
              Auto-discovered from observed HTTP, HTTP/2 &amp; gRPC traffic — zero instrumentation
            </p>
          </div>
        </div>
        <button
          onClick={load}
          disabled={loading}
          className="flex items-center gap-1.5 text-xs text-slate-400 hover:text-white
                     px-2.5 py-1.5 rounded border border-white/10 hover:border-white/20
                     transition-colors disabled:opacity-50"
        >
          <RefreshCw className={clsx("h-3 w-3", loading && "animate-spin")} />
          Refresh
        </button>
      </div>

      {/* Filters */}
      <div className="flex items-center gap-3 px-6 py-3 border-b border-white/[0.04] shrink-0 flex-wrap">
        {/* Window selector */}
        <div className="flex items-center gap-1 rounded-md border border-white/[0.08] bg-white/[0.02] p-0.5">
          {WINDOWS.map((w) => (
            <button
              key={w}
              onClick={() => setWindow(w)}
              className={clsx(
                "px-3 py-1 rounded text-xs font-medium transition-colors",
                window === w ? "bg-indigo-600 text-white" : "text-slate-400 hover:text-white",
              )}
            >
              {w}
            </button>
          ))}
        </div>

        {/* Path search */}
        <div className="relative">
          <Search size={12} className="absolute left-2.5 top-1/2 -translate-y-1/2 text-slate-500 pointer-events-none" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            onKeyDown={(e) => e.key === "Enter" && load()}
            placeholder="Filter by path…"
            className="bg-white/[0.03] border border-white/[0.08] rounded pl-7 pr-3 py-1.5
                       text-xs text-slate-300 placeholder:text-slate-600
                       focus:outline-none focus:ring-1 focus:ring-indigo-500 w-48"
          />
          {search && (
            <button onClick={() => setSearch("")} className="absolute right-2 top-1/2 -translate-y-1/2 text-slate-600 hover:text-slate-400">
              <X size={10} />
            </button>
          )}
        </div>

        {/* Hostname filter */}
        <select
          value={hostname}
          onChange={(e) => setHostname(e.target.value)}
          className="bg-white/[0.03] border border-white/[0.08] rounded px-2.5 py-1.5
                     text-xs text-slate-300 focus:outline-none focus:ring-1 focus:ring-indigo-500"
        >
          <option value="">All agents</option>
          {hostnames.map((h) => <option key={h} value={h}>{h}</option>)}
        </select>

        <span className="text-xs text-slate-600 ml-auto">
          {endpoints.length.toLocaleString()} endpoints
        </span>
      </div>

      {/* Error */}
      {error && (
        <div className="mx-6 mt-4 px-4 py-3 rounded-lg border border-red-500/20 bg-red-500/5 text-xs text-red-400 flex items-center gap-2">
          <AlertTriangle size={13} />
          {error}
        </div>
      )}

      {/* Table */}
      <div className="flex-1 overflow-auto">
        {loading && endpoints.length === 0 ? (
          <div className="flex justify-center py-20">
            <RefreshCw className="h-5 w-5 animate-spin text-slate-600" />
          </div>
        ) : endpoints.length === 0 ? (
          <div className="flex flex-col items-center justify-center py-20 text-center gap-2">
            <Database size={28} className="text-slate-700" />
            <p className="text-sm text-slate-400">No endpoints discovered yet</p>
            <p className="text-xs text-slate-600">
              NetScope will auto-populate this as HTTP traffic is observed.
            </p>
          </div>
        ) : (
          <table className="w-full text-sm">
            <thead className="sticky top-0 z-10">
              <tr className="border-b border-white/[0.06] bg-[#070711]">
                <th className="px-4 py-2.5 text-left text-xs font-medium text-slate-500 w-24">Proto</th>
                <th className="px-4 py-2.5 text-left text-xs font-medium text-slate-500 w-16">Method</th>
                <th className="px-4 py-2.5 text-left text-xs font-medium text-slate-500">Path</th>
                <th className="px-4 py-2.5 text-left text-xs font-medium text-slate-500">Agent</th>
                <th className="px-4 py-2.5 text-right text-xs font-medium text-slate-500 w-20">Calls</th>
                <th className="px-4 py-2.5 text-left text-xs font-medium text-slate-500 w-28">Error rate</th>
                <th className="px-4 py-2.5 text-right text-xs font-medium text-slate-500 w-20">p95</th>
                <th className="px-4 py-2.5 text-right text-xs font-medium text-slate-500 w-20">Last seen</th>
              </tr>
            </thead>
            <tbody>
              {endpoints.map((ep, i) => (
                <tr
                  key={`${ep.hostname}-${ep.protocol}-${ep.method}-${ep.path}-${i}`}
                  className="border-b border-white/[0.03] hover:bg-white/[0.02] transition-colors"
                >
                  <td className="px-4 py-2">
                    <div className="flex items-center gap-1.5">
                      <ProtoBadge proto={ep.protocol} />
                      {ep.is_new && (
                        <span className="px-1 py-0.5 rounded text-[8px] font-bold uppercase tracking-wide
                                         bg-emerald-500/10 border border-emerald-500/20 text-emerald-400">
                          new
                        </span>
                      )}
                    </div>
                  </td>
                  <td className="px-4 py-2">
                    <MethodBadge method={ep.method} />
                  </td>
                  <td className="px-4 py-2 font-mono text-xs text-slate-300 max-w-xs truncate" title={ep.path}>
                    {ep.path}
                  </td>
                  <td className="px-4 py-2 text-xs text-slate-400 font-mono">
                    {ep.hostname}
                  </td>
                  <td className="px-4 py-2 text-xs text-slate-300 text-right tabular-nums">
                    {ep.call_count.toLocaleString()}
                  </td>
                  <td className="px-4 py-2">
                    <ErrorBar rate={ep.error_rate} />
                  </td>
                  <td className="px-4 py-2 text-xs text-slate-400 text-right tabular-nums">
                    {ep.p95_ms.toFixed(0)} ms
                  </td>
                  <td className="px-4 py-2 text-xs text-slate-500 text-right whitespace-nowrap">
                    {timeAgo(ep.last_seen)}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
    </div>
  );
}
