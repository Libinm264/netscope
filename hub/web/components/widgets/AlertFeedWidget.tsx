"use client";

import { useEffect, useState } from "react";
import { Bell } from "lucide-react";
import type { AlertFeedConfig } from "@/lib/dashboard";
import { fetchAlertEvents, type AlertEvent } from "@/lib/api";

interface Props {
  title: string;
  config: AlertFeedConfig;
}

function relTime(ts: string) {
  const diff = (Date.now() - new Date(ts).getTime()) / 1000;
  if (diff < 60)   return `${Math.floor(diff)}s ago`;
  if (diff < 3600) return `${Math.floor(diff / 60)}m ago`;
  if (diff < 86400) return `${Math.floor(diff / 3600)}h ago`;
  return `${Math.floor(diff / 86400)}d ago`;
}

export function AlertFeedWidget({ title, config }: Props) {
  const [events, setEvents] = useState<AlertEvent[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    let active = true;
    setLoading(true);
    fetchAlertEvents()
      .then(({ events }) => {
        if (active) {
          setEvents(events.slice(0, config.limit));
          setLoading(false);
        }
      })
      .catch(() => { if (active) setLoading(false); });
    return () => { active = false; };
  }, [config.limit]);

  return (
    <div className="h-full flex flex-col p-4 rounded-xl bg-[#0d0d1a] border border-white/[0.06]">
      <div className="flex items-center gap-2 mb-3 shrink-0">
        <Bell size={12} className="text-red-400" />
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
          No alerts
        </div>
      ) : (
        <div className="flex-1 overflow-auto space-y-1 min-h-0">
          {events.map((e) => (
            <div
              key={e.id}
              className="flex items-center justify-between rounded-lg px-3 py-2
                         bg-white/[0.02] hover:bg-white/[0.04] transition-colors"
            >
              <div className="min-w-0">
                <p className="text-[11px] font-medium text-slate-200 truncate">
                  {e.rule_name}
                </p>
                <p className="text-[10px] text-slate-500">
                  {e.metric}: {e.value.toFixed(2)} &gt; {e.threshold}
                </p>
              </div>
              <span className="text-[10px] text-slate-600 shrink-0 ml-2">
                {relTime(e.fired_at)}
              </span>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
