"use client";

import { useEffect, useState } from "react";
import {
  LineChart,
  Line,
  XAxis,
  YAxis,
  Tooltip,
  ResponsiveContainer,
  CartesianGrid,
} from "recharts";
import type { TimeseriesConfig } from "@/lib/dashboard";
import { fetchTimeseries, type TimeseriesPoint } from "@/lib/api";

interface Props {
  title: string;
  config: TimeseriesConfig;
}

const WINDOWS: Record<TimeseriesConfig["window"], number> = {
  "1h": 1,
  "6h": 6,
  "24h": 24,
};

function formatTime(ts: string) {
  const d = new Date(ts);
  return d.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });
}

export function TimeseriesWidget({ title, config }: Props) {
  const [points, setPoints] = useState<TimeseriesPoint[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    let active = true;
    setLoading(true);
    fetchTimeseries(WINDOWS[config.window])
      .then(({ points }) => { if (active) { setPoints(points); setLoading(false); } })
      .catch(() => { if (active) setLoading(false); });
    return () => { active = false; };
  }, [config.window]);

  return (
    <div className="h-full flex flex-col p-4 rounded-xl bg-[#0d0d1a] border border-white/[0.06]">
      <p className="text-xs text-slate-400 mb-3 shrink-0">{title}</p>
      {loading ? (
        <div className="flex-1 rounded bg-white/[0.04] animate-pulse" />
      ) : (
        <div className="flex-1 min-h-0">
          <ResponsiveContainer width="100%" height="100%">
            <LineChart data={points} margin={{ top: 4, right: 8, left: -20, bottom: 0 }}>
              <CartesianGrid strokeDasharray="3 3" stroke="rgba(255,255,255,0.04)" />
              <XAxis
                dataKey="ts"
                tickFormatter={formatTime}
                tick={{ fontSize: 10, fill: "#64748b" }}
                tickLine={false}
                interval="preserveStartEnd"
              />
              <YAxis tick={{ fontSize: 10, fill: "#64748b" }} tickLine={false} axisLine={false} />
              <Tooltip
                contentStyle={{
                  background: "#0d0d1a",
                  border: "1px solid rgba(255,255,255,0.08)",
                  borderRadius: 8,
                  fontSize: 12,
                }}
                labelFormatter={formatTime}
                formatter={(v: number) => [v.toLocaleString(), "flows"]}
              />
              <Line
                type="monotone"
                dataKey="count"
                stroke="#6366f1"
                strokeWidth={2}
                dot={false}
                activeDot={{ r: 4, fill: "#6366f1" }}
              />
            </LineChart>
          </ResponsiveContainer>
        </div>
      )}
    </div>
  );
}
