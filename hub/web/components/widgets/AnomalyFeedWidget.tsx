"use client";

import { useEffect, useState } from "react";
import { Activity } from "lucide-react";
import type { AnomalyFeedConfig } from "@/lib/dashboard";
import { fetchAnomalies, type AnomalyEvent, type AnomalySeverity } from "@/lib/api";

interface Props {
  title: string;
  config: AnomalyFeedConfig;
}

const SEV_COLOR: Record<AnomalySeverity, string> = {
  high:   "text-red-400 bg-red-400/10",
  medium: "text-amber-400 bg-amber-400/10",
  low:    "text-blue-400 bg-blue-400/10",
};

function relTime(ts: string) {
  const diff = (Date.now() - new Date(ts).getTime()) / 1000;
  if (diff < 60)    return `${Math.floor(diff)}s ago`;
  if (diff < 3600)  return `${Math.floor(diff / 60)}m ago`;
  if (diff < 86400) return `${Math.floor(diff / 3600)}h ago`;
  return `${Math.floor(diff / 86400)}d ago`;
}

export function AnomalyFeedWidget({ title, config }: Props) {
  const [events, setEvents] = useState<AnomalyEvent[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    let active = true;
    setLoading(true);
    fetchAnomalies({
      limit: config.limit,
      severity:
        config.severity && config.severity !== "all"
          ? (config.severity as AnomalySeverity)
          : undefined,
    })
      .then(({ events }) => {
        if (active) { setEvents(events); setLoading(false); }
      })
      .catch(() => { if (active) setLoading(false); });
    return () => { active = false; };
  }, [config.limit, config.severity]);

  return (
    <div className="h-full flex flex-col p-4 rounded-xl bg-[#0d0d1a] border border-white/[0.06]">
      <div className="flex items-center gap-2 mb-3 shrink-0">
        <Activity size={12} className="text-indigo-400" />
        <p className="text-xs text-slate-400">{title}</p>
      </div>
      {loading ? (
        <div className="flex-1 space-y-2">
          {Array.from({ length: 4 }).map((_, i) => (
            <div key={i} className="h-8 rounded bg-white/[0.04] animate-pulse" />
          ))}
        </div>
      ) : events.length === 0 ? (
        <div className="flex-1 flex items-center justify-center text-xs text-slate-600">
          No anomalies
        </div>
      ) : (
        <div className="flex-1 overflow-auto space-y-1 min-h-0">
          {events.map((e) => (
            <div
              key={e.id}
              className="flex items-start justify-between rounded-lg px-3 py-2
                         bg-white/[0.02] hover:bg-white/[0.04] transition-colors gap-2"
            >
              <div className="min-w-0 flex-1">
                <p className="text-[11px] text-slate-200 truncate">{e.description}</p>
                <p className="text-[10px] text-slate-500">
                  {e.hostname || e.agent_id} · {e.protocol}
                </p>
              </div>
              <div className="flex flex-col items-end gap-1 shrink-0">
                <span
                  className={`text-[9px] font-semibold uppercase px-1.5 py-0.5 rounded ${SEV_COLOR[e.severity]}`}
                >
                  {e.severity}
                </span>
                <span className="text-[10px] text-slate-600">{relTime(e.detected_at)}</span>
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
