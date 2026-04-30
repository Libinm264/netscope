"use client";

import { useEffect, useState } from "react";
import type { TopTalkersConfig, Talker } from "@/lib/dashboard";
import { fetchTopTalkers } from "@/lib/dashboard";

interface Props {
  title: string;
  config: TopTalkersConfig;
}

function formatBytes(b: number) {
  if (b >= 1e9) return (b / 1e9).toFixed(1) + " GB";
  if (b >= 1e6) return (b / 1e6).toFixed(1) + " MB";
  if (b >= 1e3) return (b / 1e3).toFixed(1) + " KB";
  return b + " B";
}

export function TopTalkersWidget({ title, config }: Props) {
  const [talkers, setTalkers] = useState<Talker[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    let active = true;
    setLoading(true);
    fetchTopTalkers(config.window, config.by, config.limit)
      .then((t) => { if (active) { setTalkers(t); setLoading(false); } })
      .catch(() => { if (active) setLoading(false); });
    return () => { active = false; };
  }, [config.window, config.by, config.limit]);

  const max = talkers[0]
    ? config.by === "bytes" ? talkers[0].total_bytes : talkers[0].flow_count
    : 1;

  return (
    <div className="h-full flex flex-col p-4 rounded-xl bg-[#0d0d1a] border border-white/[0.06]">
      <div className="flex items-center justify-between mb-3 shrink-0">
        <p className="text-xs text-slate-400">{title}</p>
        <span className="text-[10px] text-slate-600 uppercase tracking-wide">
          {config.window} · by {config.by}
        </span>
      </div>
      {loading ? (
        <div className="flex-1 space-y-2">
          {Array.from({ length: 5 }).map((_, i) => (
            <div key={i} className="h-6 rounded bg-white/[0.04] animate-pulse" />
          ))}
        </div>
      ) : talkers.length === 0 ? (
        <div className="flex-1 flex items-center justify-center text-xs text-slate-600">
          No data
        </div>
      ) : (
        <div className="flex-1 overflow-auto space-y-1.5 min-h-0">
          {talkers.map((t, i) => {
            const val = config.by === "bytes" ? t.total_bytes : t.flow_count;
            const pct = max > 0 ? (val / max) * 100 : 0;
            return (
              <div key={i} className="group">
                <div className="flex justify-between text-[11px] mb-0.5">
                  <span className="text-slate-300 font-mono">{t.src_ip}</span>
                  <span className="text-slate-500">
                    {config.by === "bytes"
                      ? formatBytes(t.total_bytes)
                      : t.flow_count.toLocaleString() + " flows"}
                  </span>
                </div>
                <div className="h-1 rounded-full bg-white/[0.04]">
                  <div
                    className="h-1 rounded-full bg-indigo-500 transition-all"
                    style={{ width: `${pct}%` }}
                  />
                </div>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}
