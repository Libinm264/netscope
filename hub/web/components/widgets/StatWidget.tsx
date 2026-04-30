"use client";

import { useEffect, useState } from "react";
import { Activity, Server, Zap, AlertTriangle, Bell } from "lucide-react";
import type { StatConfig } from "@/lib/dashboard";
import { fetchStats, fetchAnomalyStats, fetchAlertEvents, fetchTimeseries } from "@/lib/api";

interface Props {
  title: string;
  config: StatConfig;
}

const META: Record<
  StatConfig["metric"],
  { icon: React.ElementType; color: string; unit: string }
> = {
  total_flows:   { icon: Zap,           color: "text-indigo-400", unit: "flows" },
  active_agents: { icon: Server,        color: "text-emerald-400", unit: "agents" },
  total_bytes:   { icon: Activity,      color: "text-sky-400",    unit: "" },
  anomaly_count: { icon: AlertTriangle, color: "text-amber-400",  unit: "anomalies (24h)" },
  alert_count:   { icon: Bell,          color: "text-red-400",    unit: "alerts (24h)" },
};

function formatBytes(b: number): string {
  if (b >= 1e12) return (b / 1e12).toFixed(1) + " TB";
  if (b >= 1e9)  return (b / 1e9).toFixed(1) + " GB";
  if (b >= 1e6)  return (b / 1e6).toFixed(1) + " MB";
  if (b >= 1e3)  return (b / 1e3).toFixed(1) + " KB";
  return b + " B";
}

export function StatWidget({ title, config }: Props) {
  const [value, setValue] = useState<number | string | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    let active = true;
    setLoading(true);

    (async () => {
      try {
        switch (config.metric) {
          case "total_flows": {
            const s = await fetchStats();
            if (active) setValue(s.total_flows);
            break;
          }
          case "active_agents": {
            const s = await fetchStats();
            if (active) setValue(s.active_agents ?? 0);
            break;
          }
          case "total_bytes": {
            const { points } = await fetchTimeseries(24);
            const total = points.reduce(
              (sum, p) => sum + (p.bytes_in ?? 0) + (p.bytes_out ?? 0),
              0
            );
            if (active) setValue(formatBytes(total));
            break;
          }
          case "anomaly_count": {
            const a = await fetchAnomalyStats();
            if (active) setValue(a.total_24h);
            break;
          }
          case "alert_count": {
            const { events } = await fetchAlertEvents();
            const last24h = events.filter(
              (e) => new Date(e.fired_at).getTime() > Date.now() - 86_400_000
            );
            if (active) setValue(last24h.length);
            break;
          }
        }
      } catch {
        if (active) setValue("—");
      } finally {
        if (active) setLoading(false);
      }
    })();

    return () => { active = false; };
  }, [config.metric]);

  const meta = META[config.metric];
  const Icon = meta.icon;

  return (
    <div className="h-full flex flex-col justify-between p-4 rounded-xl
                    bg-[#0d0d1a] border border-white/[0.06]">
      <div className="flex items-center gap-2 mb-2">
        <Icon size={14} className={meta.color} />
        <span className="text-xs text-slate-400 truncate">{title}</span>
      </div>
      <div>
        {loading ? (
          <div className="h-9 w-24 rounded bg-white/[0.04] animate-pulse" />
        ) : (
          <span className={`text-3xl font-bold tabular-nums ${meta.color}`}>
            {value}
          </span>
        )}
        {meta.unit && (
          <p className="text-xs text-slate-600 mt-1">{meta.unit}</p>
        )}
      </div>
    </div>
  );
}
