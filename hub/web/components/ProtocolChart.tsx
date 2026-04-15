"use client";

import { useEffect, useState } from "react";
import { fetchStats } from "@/lib/api";
import {
  BarChart,
  Bar,
  XAxis,
  YAxis,
  Tooltip,
  ResponsiveContainer,
  Cell,
} from "recharts";

const BAR_COLORS = ["#6366f1", "#818cf8", "#a5b4fc", "#c7d2fe", "#e0e7ff"];

export function ProtocolChart() {
  const [data, setData] = useState<{ protocol: string; count: number }[]>([]);

  useEffect(() => {
    const load = async () => {
      try {
        const stats = await fetchStats();
        setData(stats.top_protocols ?? []);
      } catch {
        /* ignore */
      }
    };
    load();
    const id = setInterval(load, 15_000);
    return () => clearInterval(id);
  }, []);

  return (
    <div className="rounded-xl bg-[#0d0d1a] border border-white/[0.06] h-[480px] flex flex-col">
      <div className="px-4 py-3 border-b border-white/[0.06]">
        <span className="text-sm font-medium text-white">Protocols</span>
      </div>
      <div className="flex-1 p-4">
        {data.length === 0 ? (
          <div className="flex items-center justify-center h-full text-slate-600 text-sm">
            No data yet
          </div>
        ) : (
          <ResponsiveContainer width="100%" height="100%">
            <BarChart
              data={data}
              layout="vertical"
              margin={{ top: 0, right: 16, left: 8, bottom: 0 }}
            >
              <XAxis
                type="number"
                tick={{ fill: "#64748b", fontSize: 11 }}
                axisLine={false}
                tickLine={false}
              />
              <YAxis
                type="category"
                dataKey="protocol"
                tick={{ fill: "#94a3b8", fontSize: 12 }}
                axisLine={false}
                tickLine={false}
                width={52}
              />
              <Tooltip
                contentStyle={{
                  background: "#12121f",
                  border: "1px solid rgba(255,255,255,0.08)",
                  borderRadius: "8px",
                  color: "#e2e8f0",
                  fontSize: "12px",
                }}
                cursor={{ fill: "rgba(255,255,255,0.03)" }}
              />
              <Bar dataKey="count" radius={[0, 4, 4, 0]}>
                {data.map((_, i) => (
                  <Cell key={i} fill={BAR_COLORS[i % BAR_COLORS.length]} />
                ))}
              </Bar>
            </BarChart>
          </ResponsiveContainer>
        )}
      </div>
    </div>
  );
}
