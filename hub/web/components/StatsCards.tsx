"use client";

import { useEffect, useState } from "react";
import { fetchStats, type StatsResponse } from "@/lib/api";
import { Activity, Zap, Server, Wifi } from "lucide-react";

const CARDS = [
  {
    key: "total_flows" as const,
    label: "Total Flows",
    icon: Activity,
    color: "text-indigo-400",
    bg: "bg-indigo-500/10",
    format: (v: number) => v.toLocaleString(),
  },
  {
    key: "flows_per_minute" as const,
    label: "Flows / min",
    icon: Zap,
    color: "text-amber-400",
    bg: "bg-amber-500/10",
    format: (v: number) => v.toFixed(0),
  },
  {
    key: "active_agents" as const,
    label: "Active Agents",
    icon: Server,
    color: "text-emerald-400",
    bg: "bg-emerald-500/10",
    format: (v: number) => String(v),
  },
  {
    key: "top_protocol" as const,
    label: "Top Protocol",
    icon: Wifi,
    color: "text-sky-400",
    bg: "bg-sky-500/10",
    format: (v: string) => v,
  },
];

export function StatsCards() {
  const [stats, setStats] = useState<StatsResponse | null>(null);
  const [error, setError] = useState(false);

  useEffect(() => {
    const load = async () => {
      try {
        setStats(await fetchStats());
        setError(false);
      } catch {
        setError(true);
      }
    };
    load();
    const id = setInterval(load, 10_000);
    return () => clearInterval(id);
  }, []);

  const topProtocol =
    stats?.top_protocols?.[0]?.protocol ?? "—";

  const values: Record<string, number | string> = {
    total_flows:     stats?.total_flows     ?? 0,
    flows_per_minute: stats?.flows_per_minute ?? 0,
    active_agents:   stats?.active_agents   ?? 0,
    top_protocol:    topProtocol,
  };

  return (
    <div className="grid grid-cols-2 xl:grid-cols-4 gap-4">
      {CARDS.map(({ key, label, icon: Icon, color, bg, format }) => (
        <div
          key={key}
          className="rounded-xl bg-[#0d0d1a] border border-white/[0.06] px-5 py-4 space-y-3"
        >
          <div className="flex items-center justify-between">
            <span className="text-xs text-slate-500 uppercase tracking-wide">{label}</span>
            <div className={`p-1.5 rounded-md ${bg}`}>
              <Icon size={14} className={color} />
            </div>
          </div>
          {error ? (
            <p className="text-2xl font-bold text-slate-600">—</p>
          ) : stats === null ? (
            <div className="h-8 w-24 rounded bg-white/5 animate-pulse" />
          ) : (
            <p className={`text-2xl font-bold ${color}`}>
              {format(values[key] as never)}
            </p>
          )}
        </div>
      ))}
    </div>
  );
}
