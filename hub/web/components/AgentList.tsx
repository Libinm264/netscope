"use client";

import { useEffect, useState } from "react";
import { fetchAgents, type Agent } from "@/lib/api";
import { Server, RefreshCw } from "lucide-react";
import { clsx } from "clsx";

function onlineStatus(lastSeen: string): "online" | "idle" | "offline" {
  try {
    const diffMs = Date.now() - new Date(lastSeen).getTime();
    const minutes = diffMs / 60_000;
    if (minutes < 5) return "online";
    if (minutes < 30) return "idle";
    return "offline";
  } catch {
    return "offline";
  }
}

const STATUS_STYLES = {
  online:  { dot: "bg-emerald-400", label: "Online",  text: "text-emerald-400" },
  idle:    { dot: "bg-amber-400",   label: "Idle",    text: "text-amber-400"   },
  offline: { dot: "bg-slate-600",   label: "Offline", text: "text-slate-500"   },
};

function timeAgo(iso: string) {
  try {
    const diffMs = Date.now() - new Date(iso).getTime();
    const secs = Math.floor(diffMs / 1000);
    if (secs < 60) return `${secs}s ago`;
    const mins = Math.floor(secs / 60);
    if (mins < 60) return `${mins}m ago`;
    const hrs = Math.floor(mins / 60);
    if (hrs < 24) return `${hrs}h ago`;
    return `${Math.floor(hrs / 24)}d ago`;
  } catch {
    return "—";
  }
}

export function AgentList() {
  const [agents, setAgents] = useState<Agent[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const load = async () => {
    setLoading(true);
    setError(null);
    try {
      const res = await fetchAgents();
      setAgents(res.agents ?? []);
    } catch (e) {
      setError(String(e));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    load();
    const id = setInterval(load, 30_000);
    return () => clearInterval(id);
  }, []);

  if (error) {
    return (
      <div className="rounded bg-red-900/20 border border-red-500/30 px-4 py-3 text-sm text-red-400">
        {error}
      </div>
    );
  }

  if (loading && agents.length === 0) {
    return (
      <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-4">
        {Array.from({ length: 3 }).map((_, i) => (
          <div
            key={i}
            className="rounded-xl bg-[#0d0d1a] border border-white/[0.06] p-4 space-y-3"
          >
            <div className="h-4 w-32 rounded bg-white/5 animate-pulse" />
            <div className="h-3 w-48 rounded bg-white/5 animate-pulse" />
          </div>
        ))}
      </div>
    );
  }

  if (agents.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center py-24 text-slate-600 space-y-2">
        <Server size={32} />
        <p className="text-sm">No agents registered yet</p>
        <p className="text-xs text-slate-700">
          Run <code className="font-mono text-slate-500">netscope-agent capture --hub-url …</code> to register one
        </p>
      </div>
    );
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <span className="text-sm text-slate-500">{agents.length} agent{agents.length !== 1 ? "s" : ""}</span>
        <button
          onClick={load}
          className="flex items-center gap-1.5 text-xs text-slate-500 hover:text-slate-300 transition-colors"
        >
          <RefreshCw size={12} className={loading ? "animate-spin" : ""} />
          Refresh
        </button>
      </div>

      <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-4">
        {agents.map((agent) => {
          const status = onlineStatus(agent.last_seen);
          const style = STATUS_STYLES[status];
          return (
            <div
              key={agent.agent_id}
              className="rounded-xl bg-[#0d0d1a] border border-white/[0.06]
                         p-4 space-y-3 hover:border-white/10 transition-colors"
            >
              <div className="flex items-start justify-between">
                <div className="flex items-center gap-2.5">
                  <div className="p-1.5 rounded-md bg-indigo-500/10">
                    <Server size={14} className="text-indigo-400" />
                  </div>
                  <p className="font-semibold text-white text-sm leading-tight">
                    {agent.hostname}
                  </p>
                </div>
                <div className="flex items-center gap-1.5 shrink-0">
                  <span
                    className={clsx(
                      "inline-block w-1.5 h-1.5 rounded-full",
                      style.dot
                    )}
                  />
                  <span className={clsx("text-xs", style.text)}>{style.label}</span>
                </div>
              </div>

              <div className="space-y-1 font-mono text-xs">
                <div className="flex gap-2 text-slate-600">
                  <span className="w-16 shrink-0">ID</span>
                  <span className="text-slate-500 truncate" title={agent.agent_id}>
                    {agent.agent_id.slice(0, 8)}…
                  </span>
                </div>
                {agent.interface && (
                  <div className="flex gap-2 text-slate-600">
                    <span className="w-16 shrink-0">Interface</span>
                    <span className="text-slate-400">{agent.interface}</span>
                  </div>
                )}
                {agent.version && (
                  <div className="flex gap-2 text-slate-600">
                    <span className="w-16 shrink-0">Version</span>
                    <span className="text-slate-400">{agent.version}</span>
                  </div>
                )}
                <div className="flex gap-2 text-slate-600">
                  <span className="w-16 shrink-0">Last seen</span>
                  <span className="text-slate-500">{timeAgo(agent.last_seen)}</span>
                </div>
              </div>
            </div>
          );
        })}
      </div>
    </div>
  );
}
