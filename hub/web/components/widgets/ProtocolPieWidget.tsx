"use client";

import { useEffect, useState } from "react";
import { PieChart, Pie, Cell, Tooltip, ResponsiveContainer, Legend } from "recharts";
import type { ProtocolPieConfig } from "@/lib/dashboard";
import { fetchProtocolBreakdown, type ProtocolCount } from "@/lib/api";

interface Props {
  title: string;
  config: ProtocolPieConfig;
}

const COLORS = [
  "#6366f1", "#22d3ee", "#f59e0b", "#34d399",
  "#f87171", "#a78bfa", "#fb923c", "#60a5fa",
];

export function ProtocolPieWidget({ title, config: _config }: Props) {
  const [data, setData] = useState<ProtocolCount[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    let active = true;
    setLoading(true);
    fetchProtocolBreakdown(1)
      .then(({ protocols }) => { if (active) { setData(protocols); setLoading(false); } })
      .catch(() => { if (active) setLoading(false); });
    return () => { active = false; };
  }, []);

  const total = data.reduce((s, d) => s + d.count, 0);

  return (
    <div className="h-full flex flex-col p-4 rounded-xl bg-[#0d0d1a] border border-white/[0.06]">
      <p className="text-xs text-slate-400 mb-3 shrink-0">{title}</p>
      {loading ? (
        <div className="flex-1 rounded bg-white/[0.04] animate-pulse" />
      ) : data.length === 0 ? (
        <div className="flex-1 flex items-center justify-center text-xs text-slate-600">
          No data
        </div>
      ) : (
        <div className="flex-1 min-h-0">
          <ResponsiveContainer width="100%" height="100%">
            <PieChart>
              <Pie
                data={data}
                dataKey="count"
                nameKey="protocol"
                cx="50%"
                cy="45%"
                innerRadius="55%"
                outerRadius="70%"
                paddingAngle={2}
              >
                {data.map((_, i) => (
                  <Cell key={i} fill={COLORS[i % COLORS.length]} />
                ))}
              </Pie>
              <Tooltip
                contentStyle={{
                  background: "#0d0d1a",
                  border: "1px solid rgba(255,255,255,0.08)",
                  borderRadius: 8,
                  fontSize: 12,
                }}
                formatter={(v: number, name: string) => [
                  `${v.toLocaleString()} (${total ? ((v / total) * 100).toFixed(1) : 0}%)`,
                  name,
                ]}
              />
              <Legend
                iconType="circle"
                iconSize={8}
                wrapperStyle={{ fontSize: 11, color: "#94a3b8" }}
              />
            </PieChart>
          </ResponsiveContainer>
        </div>
      )}
    </div>
  );
}
