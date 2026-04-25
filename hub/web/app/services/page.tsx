"use client";

import { useCallback, useEffect, useState } from "react";
import { RefreshCw, GitFork, Info, AlertTriangle, Cpu } from "lucide-react";
import { clsx } from "clsx";
import { fetchServiceGraph, fetchThreats } from "@/lib/api";
import type { ServiceGraph, ThreatIP } from "@/lib/api";
import { ServiceGraphViz } from "@/components/ServiceGraph";

const WINDOWS = [
  { value: "15m", label: "15 min" },
  { value: "1h",  label: "1 hour" },
  { value: "6h",  label: "6 hours" },
  { value: "24h", label: "24 hours" },
];

export default function ServicesPage() {
  const [window, setWindow]     = useState("1h");
  const [graph, setGraph]       = useState<ServiceGraph | null>(null);
  const [threats, setThreats]   = useState<Map<string, ThreatIP>>(new Map());
  const [loading, setLoading]   = useState(false);
  const [error, setError]       = useState<string | null>(null);
  const [lastRefresh, setLastRefresh] = useState<Date | null>(null);

  const load = useCallback(async (w: string) => {
    setLoading(true);
    setError(null);
    try {
      const [data, threatRes] = await Promise.all([
        fetchServiceGraph(w),
        fetchThreats({ window: w, limit: 200 }).catch(() => ({ threats: [] as ThreatIP[], summary: { total: 0, high: 0, medium: 0, low: 0 }, window: w })),
      ]);
      setGraph(data);
      // Index threats by IP for O(1) lookup in the node table
      const tmap = new Map<string, ThreatIP>();
      for (const t of threatRes.threats) tmap.set(t.dst_ip, t);
      setThreats(tmap);
      setLastRefresh(new Date());
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to load service graph");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    load(window);
  }, [window, load]);

  const nodeCount = graph?.nodes.length ?? 0;
  const edgeCount = graph?.edges.length ?? 0;
  const knownCount = graph?.nodes.filter((n) => n.is_known).length ?? 0;

  return (
    <div className="p-6 space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-3">
          <div className="p-2 rounded-lg bg-indigo-500/10">
            <GitFork size={20} className="text-indigo-400" />
          </div>
          <div>
            <h1 className="text-lg font-semibold text-white">Service Dependency Map</h1>
            <p className="text-xs text-slate-400 mt-0.5">
              Auto-generated topology from observed network flows
            </p>
          </div>
        </div>

        <div className="flex items-center gap-3">
          {/* Window picker */}
          <div className="flex rounded-md overflow-hidden border border-white/10">
            {WINDOWS.map((w) => (
              <button
                key={w.value}
                onClick={() => setWindow(w.value)}
                className={clsx(
                  "px-3 py-1.5 text-xs transition-colors",
                  window === w.value
                    ? "bg-indigo-500/20 text-indigo-300"
                    : "text-slate-400 hover:text-slate-200 hover:bg-white/[0.04]",
                )}
              >
                {w.label}
              </button>
            ))}
          </div>

          <button
            onClick={() => load(window)}
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

      {/* Stats strip */}
      <div className="grid grid-cols-3 gap-4">
        {[
          { label: "Nodes observed",  value: nodeCount },
          { label: "Active agents",   value: knownCount },
          { label: "Unique flows",    value: edgeCount },
        ].map(({ label, value }) => (
          <div key={label} className="bg-[#0d0d1a] border border-white/[0.06] rounded-lg px-4 py-3">
            <p className="text-2xl font-semibold text-white">{value}</p>
            <p className="text-xs text-slate-400 mt-0.5">{label}</p>
          </div>
        ))}
      </div>

      {/* Graph panel */}
      <div className="bg-[#0d0d1a] border border-white/[0.06] rounded-xl overflow-hidden">
        <div className="flex items-center justify-between px-4 py-3 border-b border-white/[0.06]">
          <p className="text-sm font-medium text-slate-300">Topology</p>
          {lastRefresh && (
            <p className="text-xs text-slate-500">
              Last updated {lastRefresh.toLocaleTimeString()}
            </p>
          )}
        </div>

        <div className="p-4">
          {error ? (
            <div className="flex flex-col items-center justify-center h-60 text-center">
              <p className="text-sm text-red-400">{error}</p>
              <button
                onClick={() => load(window)}
                className="mt-3 text-xs text-slate-400 hover:text-slate-200 underline"
              >
                Retry
              </button>
            </div>
          ) : loading && !graph ? (
            <div className="flex items-center justify-center h-60">
              <div className="flex flex-col items-center gap-3">
                <RefreshCw size={24} className="animate-spin text-indigo-400" />
                <p className="text-xs text-slate-400">Building topology…</p>
              </div>
            </div>
          ) : graph && graph.nodes.length === 0 ? (
            <EmptyState window={window} />
          ) : graph ? (
            <ServiceGraphViz data={graph} />
          ) : null}
        </div>
      </div>

      {/* Node table */}
      {graph && graph.nodes.length > 0 && (
        <div className="bg-[#0d0d1a] border border-white/[0.06] rounded-xl overflow-hidden">
          <div className="px-4 py-3 border-b border-white/[0.06]">
            <p className="text-sm font-medium text-slate-300">Hosts</p>
          </div>
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-white/[0.04]">
                  {["IP / Host", "Hostname", "Total Flows", "Type", "Threat", "Processes (eBPF)"].map((h) => (
                    <th key={h} className="text-left px-4 py-2.5 text-xs font-medium text-slate-500">{h}</th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {[...graph.nodes]
                  .sort((a, b) => b.flow_count - a.flow_count)
                  .map((node) => {
                    const threat = threats.get(node.ip);
                    return (
                      <tr key={node.id}
                        className={clsx(
                          "border-b border-white/[0.03] hover:bg-white/[0.02] transition-colors",
                          threat?.threat_level === "high" && "bg-red-500/[0.03]",
                        )}>
                        <td className="px-4 py-2.5 font-mono text-xs text-slate-200">{node.ip}</td>
                        <td className="px-4 py-2.5 text-xs text-slate-400">{node.hostname || "—"}</td>
                        <td className="px-4 py-2.5 text-xs text-slate-300 tabular-nums">{node.flow_count.toLocaleString()}</td>
                        <td className="px-4 py-2.5">
                          {node.is_known ? (
                            <span className="px-2 py-0.5 rounded text-[10px] font-medium bg-indigo-500/10 text-indigo-400">Agent</span>
                          ) : (
                            <span className="px-2 py-0.5 rounded text-[10px] font-medium bg-slate-700/50 text-slate-400">External</span>
                          )}
                        </td>
                        <td className="px-4 py-2.5">
                          {threat ? (
                            <span className={clsx(
                              "inline-flex items-center gap-1 px-1.5 py-0.5 rounded text-[10px] font-semibold border",
                              threat.threat_level === "high"   ? "bg-red-500/10 text-red-400 border-red-500/25" :
                              threat.threat_level === "medium" ? "bg-amber-500/10 text-amber-400 border-amber-500/25" :
                                                                  "bg-yellow-500/10 text-yellow-400 border-yellow-500/25",
                            )} title={`Score: ${threat.threat_score} · ${threat.as_org}`}>
                              <AlertTriangle size={8} />
                              {threat.threat_level.toUpperCase()} · {threat.threat_score}
                            </span>
                          ) : (
                            <span className="text-slate-700 text-xs">—</span>
                          )}
                        </td>
                        <td className="px-4 py-2.5 text-xs text-emerald-400/70">
                          {threat?.processes?.length
                            ? <span className="flex items-center gap-1"><Cpu size={10} />{threat.processes.slice(0,3).join(", ")}</span>
                            : <span className="text-slate-700">—</span>
                          }
                        </td>
                      </tr>
                    );
                  })}
              </tbody>
            </table>
          </div>
        </div>
      )}
    </div>
  );
}

function EmptyState({ window }: { window: string }) {
  return (
    <div className="flex flex-col items-center justify-center h-60 text-center gap-3">
      <div className="p-3 rounded-full bg-slate-800/50">
        <Info size={20} className="text-slate-500" />
      </div>
      <p className="text-sm text-slate-400">No flows observed in the last {window}</p>
      <p className="text-xs text-slate-600">
        Start the NetScope agent to capture network traffic
      </p>
    </div>
  );
}
