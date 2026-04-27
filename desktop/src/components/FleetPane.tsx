/// FleetPane — desktop fleet overview panel (F7).
///
/// Requires a hub connection.  Shows:
///   • Stats row  — total clusters, total agents, online, flows/hr
///   • Cluster grid cards with health badge, version chips, flow rate
///   • Agent table below, filtered by the selected cluster card
///
/// Data is fetched from the hub via the `get_fleet_clusters` and
/// `get_fleet_agents` Tauri commands wired to HubClient's fleet API.

import { useCallback, useEffect, useRef, useState } from "react";
import { invoke } from "@tauri-apps/api/core";
import { cn } from "@/lib/utils";
import { useCaptureStore } from "@/store/captureStore";
import type { ClusterSummary, AgentInfo } from "@/types/fleet";

// ── helpers ──────────────────────────────────────────────────────────────────

function healthPct(s: ClusterSummary): number {
  if (s.totalAgents === 0) return 0;
  return Math.round((s.onlineAgents / s.totalAgents) * 100);
}

function healthColor(pct: number): string {
  if (pct >= 90) return "bg-emerald-500/20 text-emerald-400 border-emerald-500/30";
  if (pct >= 60) return "bg-yellow-500/20 text-yellow-400 border-yellow-500/30";
  return "bg-red-500/20 text-red-400 border-red-500/30";
}

function fmtFlows(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M/hr`;
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}K/hr`;
  return `${n}/hr`;
}

function relativeTime(iso: string): string {
  try {
    const diff = Math.floor((Date.now() - new Date(iso).getTime()) / 1000);
    if (diff < 60)  return `${diff}s ago`;
    if (diff < 3600) return `${Math.floor(diff / 60)}m ago`;
    if (diff < 86400) return `${Math.floor(diff / 3600)}h ago`;
    return `${Math.floor(diff / 86400)}d ago`;
  } catch {
    return "—";
  }
}

// ── component ─────────────────────────────────────────────────────────────────

export function FleetPane() {
  const { hubConfig } = useCaptureStore();

  const [clusters, setClusters] = useState<ClusterSummary[]>([]);
  const [agents, setAgents] = useState<AgentInfo[]>([]);
  const [selectedCluster, setSelectedCluster] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const pollRef = useRef<ReturnType<typeof setInterval> | null>(null);

  const fetchData = useCallback(async (cluster: string | null) => {
    if (!hubConfig) return;
    setLoading(true);
    setError(null);
    try {
      const [cls, ags] = await Promise.all([
        invoke<ClusterSummary[]>("get_fleet_clusters"),
        invoke<AgentInfo[]>("get_fleet_agents", { cluster: cluster ?? undefined }),
      ]);
      setClusters(cls);
      setAgents(ags);
    } catch (e) {
      setError(String(e));
    } finally {
      setLoading(false);
    }
  }, [hubConfig]);

  // Initial fetch + polling every 30 s
  useEffect(() => {
    fetchData(selectedCluster);
    pollRef.current = setInterval(() => fetchData(selectedCluster), 30_000);
    return () => {
      if (pollRef.current) clearInterval(pollRef.current);
    };
  }, [fetchData, selectedCluster]);

  // ── derived stats ──────────────────────────────────────────────────────────
  const totalAgents  = clusters.reduce((a, c) => a + c.totalAgents, 0);
  const onlineAgents = clusters.reduce((a, c) => a + c.onlineAgents, 0);
  const totalFlows   = clusters.reduce((a, c) => a + c.flowsPerHour, 0);

  // ── no hub ────────────────────────────────────────────────────────────────
  if (!hubConfig) {
    return (
      <div className="flex h-full items-center justify-center text-gray-500 text-sm">
        Connect to a hub to view fleet data.
      </div>
    );
  }

  return (
    <div className="flex h-full flex-col overflow-hidden text-white">
      {/* ── Stats row ──────────────────────────────────────────────────── */}
      <div className="flex shrink-0 gap-3 border-b border-white/10 px-3 py-2">
        <StatPill label="Clusters"     value={String(clusters.length)} />
        <StatPill label="Agents"       value={String(totalAgents)} />
        <StatPill label="Online"       value={String(onlineAgents)} accent="emerald" />
        <StatPill label="Flows/hr"     value={fmtFlows(totalFlows)} accent="blue" />
        {loading && (
          <span className="ml-auto text-[10px] text-gray-500 self-center animate-pulse">
            Refreshing…
          </span>
        )}
        <button
          onClick={() => fetchData(selectedCluster)}
          className="ml-auto text-[10px] text-gray-500 hover:text-gray-300 transition-colors"
          title="Refresh fleet data"
        >
          ↺ Refresh
        </button>
      </div>

      {/* ── Error ──────────────────────────────────────────────────────── */}
      {error && (
        <div className="shrink-0 bg-red-500/10 border-b border-red-500/20 px-3 py-1.5 text-[11px] text-red-400">
          {error}
        </div>
      )}

      <div className="flex-1 overflow-y-auto">
        {/* ── Cluster cards ──────────────────────────────────────────── */}
        {clusters.length > 0 && (
          <div className="border-b border-white/10 p-3">
            <div className="mb-2 text-[10px] font-semibold text-gray-500 uppercase tracking-wider">
              Clusters
            </div>
            <div className="flex flex-wrap gap-2">
              {clusters.map((c) => {
                const pct = healthPct(c);
                const isSelected = selectedCluster === c.cluster;
                return (
                  <button
                    key={c.cluster}
                    onClick={() =>
                      setSelectedCluster(isSelected ? null : c.cluster)
                    }
                    className={cn(
                      "flex flex-col gap-1 rounded-lg border p-2.5 text-left transition-all min-w-[140px]",
                      isSelected
                        ? "border-blue-500/60 bg-blue-500/10"
                        : "border-white/10 bg-white/5 hover:border-white/20 hover:bg-white/10",
                    )}
                  >
                    {/* Cluster name + health badge */}
                    <div className="flex items-center justify-between gap-2">
                      <span className="text-[11px] font-semibold text-white truncate max-w-[90px]">
                        {c.cluster}
                      </span>
                      <span
                        className={cn(
                          "rounded px-1.5 py-0.5 text-[9px] font-bold border",
                          healthColor(pct),
                        )}
                      >
                        {pct}%
                      </span>
                    </div>

                    {/* Agent counts */}
                    <div className="text-[10px] text-gray-400">
                      {c.onlineAgents}/{c.totalAgents} agents
                    </div>

                    {/* Flow rate */}
                    <div className="text-[10px] text-blue-400">
                      {fmtFlows(c.flowsPerHour)}
                    </div>

                    {/* Version chips */}
                    <div className="flex flex-wrap gap-1 mt-0.5">
                      {c.versions.slice(0, 3).map((v) => (
                        <span
                          key={v}
                          className="rounded bg-white/10 px-1 py-px text-[8px] text-gray-300"
                        >
                          {v}
                        </span>
                      ))}
                      {c.versions.length > 3 && (
                        <span className="text-[8px] text-gray-500">
                          +{c.versions.length - 3}
                        </span>
                      )}
                    </div>
                  </button>
                );
              })}
            </div>
          </div>
        )}

        {/* ── Agent table ──────────────────────────────────────────────── */}
        <div className="p-3">
          <div className="mb-2 flex items-center gap-2">
            <div className="text-[10px] font-semibold text-gray-500 uppercase tracking-wider">
              Agents
            </div>
            {selectedCluster && (
              <span className="rounded bg-blue-500/20 px-1.5 py-px text-[9px] text-blue-400">
                {selectedCluster}
              </span>
            )}
          </div>

          {agents.length === 0 && !loading && (
            <div className="text-[11px] text-gray-600 py-4 text-center">
              {selectedCluster
                ? `No agents in cluster "${selectedCluster}".`
                : "No agents found."}
            </div>
          )}

          {agents.length > 0 && (
            <div className="overflow-x-auto">
              <table className="w-full text-[11px]">
                <thead>
                  <tr className="text-gray-500 text-[10px] uppercase tracking-wider">
                    <th className="text-left pb-1.5 pr-3 font-medium">Hostname</th>
                    <th className="text-left pb-1.5 pr-3 font-medium">Cluster</th>
                    <th className="text-left pb-1.5 pr-3 font-medium">Mode</th>
                    <th className="text-left pb-1.5 pr-3 font-medium">Version</th>
                    <th className="text-right pb-1.5 pr-3 font-medium">Flows/hr</th>
                    <th className="text-right pb-1.5 font-medium">Last seen</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-white/5">
                  {agents.map((a) => (
                    <tr
                      key={a.agentId}
                      className="hover:bg-white/5 transition-colors"
                    >
                      <td className="py-1.5 pr-3 text-white font-mono">
                        {a.hostname}
                      </td>
                      <td className="py-1.5 pr-3 text-gray-400">
                        {a.cluster}
                      </td>
                      <td className="py-1.5 pr-3">
                        <ModeBadge mode={a.mode} ebpf={a.ebpfEnabled} />
                      </td>
                      <td className="py-1.5 pr-3 text-gray-400 font-mono">
                        {a.version}
                      </td>
                      <td className="py-1.5 pr-3 text-right text-blue-400 tabular-nums">
                        {fmtFlows(a.flowCount1h)}
                      </td>
                      <td className="py-1.5 text-right text-gray-500">
                        {relativeTime(a.lastSeen)}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}

// ── sub-components ─────────────────────────────────────────────────────────────

function StatPill({
  label,
  value,
  accent,
}: {
  label: string;
  value: string;
  accent?: "emerald" | "blue";
}) {
  const valueClass =
    accent === "emerald"
      ? "text-emerald-400"
      : accent === "blue"
        ? "text-blue-400"
        : "text-white";
  return (
    <div className="flex items-baseline gap-1.5">
      <span className={cn("text-sm font-bold tabular-nums", valueClass)}>
        {value}
      </span>
      <span className="text-[10px] text-gray-500">{label}</span>
    </div>
  );
}

function ModeBadge({ mode, ebpf }: { mode: string; ebpf: boolean }) {
  const isEbpf = ebpf || mode === "ebpf";
  return (
    <span
      className={cn(
        "rounded px-1.5 py-0.5 text-[9px] font-semibold",
        isEbpf
          ? "bg-purple-500/20 text-purple-400"
          : "bg-slate-500/20 text-slate-400",
      )}
    >
      {isEbpf ? "eBPF" : "pcap"}
    </span>
  );
}
