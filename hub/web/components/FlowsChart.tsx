"use client";

import { useEffect, useState, useCallback } from "react";
import {
  LineChart,
  Line,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  ResponsiveContainer,
} from "recharts";
import { fetchTimeseries, type TimeseriesPoint } from "@/lib/api";
import { TrendingUp } from "lucide-react";

const HOUR_OPTIONS = [
  { label: "1h",  hours: 1  },
  { label: "6h",  hours: 6  },
  { label: "24h", hours: 24 },
] as const;

function fmtTime(iso: string): string {
  try {
    const d = new Date(iso);
    return d.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });
  } catch {
    return iso;
  }
}

interface ChartDatum {
  time: string;
  flows: number;
  bytes_in: number;
  bytes_out: number;
}

export function FlowsChart() {
  const [data, setData] = useState<ChartDatum[]>([]);
  const [hours, setHours] = useState(1);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(false);

  const load = useCallback(async () => {
    setLoading(true);
    setError(false);
    try {
      const res = await fetchTimeseries(hours);
      const pts = (res.points ?? []).map((p: TimeseriesPoint) => ({
        time: fmtTime(p.ts),
        flows: p.count,
        bytes_in: p.bytes_in,
        bytes_out: p.bytes_out,
      }));
      setData(pts);
    } catch {
      setError(true);
    } finally {
      setLoading(false);
    }
  }, [hours]);

  useEffect(() => { load(); }, [load]);

  // Auto-refresh every 30 s
  useEffect(() => {
    const id = setInterval(load, 30_000);
    return () => clearInterval(id);
  }, [load]);

  const totalFlows = data.reduce((s, d) => s + d.flows, 0);

  return (
    <div className="bg-[#0d0d1a] border border-white/[0.06] rounded-xl p-5">
      <div className="flex items-center justify-between mb-4">
        <div className="flex items-center gap-2">
          <TrendingUp size={16} className="text-indigo-400" />
          <span className="text-sm font-medium text-white">Flow rate</span>
          {!loading && (
            <span className="text-xs text-slate-500 ml-1">
              {totalFlows.toLocaleString()} total
            </span>
          )}
        </div>
        <div className="flex gap-1">
          {HOUR_OPTIONS.map((opt) => (
            <button
              key={opt.hours}
              onClick={() => setHours(opt.hours)}
              className={`px-2.5 py-1 rounded text-xs transition-colors ${
                hours === opt.hours
                  ? "bg-indigo-600 text-white"
                  : "bg-white/[0.04] text-slate-400 hover:text-white hover:bg-white/[0.08]"
              }`}
            >
              {opt.label}
            </button>
          ))}
        </div>
      </div>

      {loading && data.length === 0 ? (
        <div className="h-40 flex items-center justify-center">
          <div className="w-5 h-5 border-2 border-indigo-500 border-t-transparent rounded-full animate-spin" />
        </div>
      ) : error ? (
        <div className="h-40 flex items-center justify-center text-xs text-slate-600">
          Failed to load — metrics endpoint may not be available yet
        </div>
      ) : data.length === 0 ? (
        <div className="h-40 flex items-center justify-center text-xs text-slate-600">
          No data for this time range
        </div>
      ) : (
        <ResponsiveContainer width="100%" height={160}>
          <LineChart data={data} margin={{ top: 4, right: 4, left: -20, bottom: 0 }}>
            <CartesianGrid strokeDasharray="3 3" stroke="rgba(255,255,255,0.04)" />
            <XAxis
              dataKey="time"
              tick={{ fill: "#64748b", fontSize: 10 }}
              tickLine={false}
              axisLine={false}
              interval="preserveStartEnd"
            />
            <YAxis
              tick={{ fill: "#64748b", fontSize: 10 }}
              tickLine={false}
              axisLine={false}
            />
            <Tooltip
              contentStyle={{
                background: "#12121f",
                border: "1px solid rgba(255,255,255,0.08)",
                borderRadius: 8,
                fontSize: 12,
              }}
              labelStyle={{ color: "#94a3b8" }}
              itemStyle={{ color: "#818cf8" }}
              formatter={(val: number) => [val.toLocaleString(), "Flows"]}
            />
            <Line
              type="monotone"
              dataKey="flows"
              stroke="#6366f1"
              strokeWidth={2}
              dot={false}
              activeDot={{ r: 3, fill: "#818cf8" }}
            />
          </LineChart>
        </ResponsiveContainer>
      )}
    </div>
  );
}
