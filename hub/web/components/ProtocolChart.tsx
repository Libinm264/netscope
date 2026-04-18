"use client";

import { useCallback, useEffect, useState } from "react";
import { PieChart, Pie, Cell, Tooltip, ResponsiveContainer, Legend } from "recharts";
import { fetchProtocolBreakdown, type ProtocolCount } from "@/lib/api";
import { PieChart as PieIcon } from "lucide-react";

const COLORS = [
  "#6366f1", "#06b6d4", "#10b981", "#f59e0b",
  "#ef4444", "#8b5cf6", "#ec4899", "#64748b",
];

export function ProtocolChart() {
  const [data, setData] = useState<ProtocolCount[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(false);

  const load = useCallback(async () => {
    setLoading(true);
    setError(false);
    try {
      const res = await fetchProtocolBreakdown(1);
      setData(res.protocols ?? []);
    } catch {
      setError(true);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { load(); }, [load]);

  // Auto-refresh every 30s
  useEffect(() => {
    const id = setInterval(load, 30_000);
    return () => clearInterval(id);
  }, [load]);

  const total = data.reduce((s, d) => s + d.count, 0);

  return (
    <div className="bg-[#0d0d1a] border border-white/[0.06] rounded-xl p-5 h-full">
      <div className="flex items-center gap-2 mb-4">
        <PieIcon size={16} className="text-indigo-400" />
        <span className="text-sm font-medium text-white">Protocols</span>
        {!loading && total > 0 && (
          <span className="text-xs text-slate-500 ml-1">last hour</span>
        )}
      </div>

      {loading && data.length === 0 ? (
        <div className="h-48 flex items-center justify-center">
          <div className="w-5 h-5 border-2 border-indigo-500 border-t-transparent rounded-full animate-spin" />
        </div>
      ) : error || data.length === 0 ? (
        <div className="h-48 flex items-center justify-center text-xs text-slate-600">
          {error ? "Failed to load" : "No data"}
        </div>
      ) : (
        <ResponsiveContainer width="100%" height={200}>
          <PieChart>
            <Pie
              data={data}
              dataKey="count"
              nameKey="protocol"
              cx="50%"
              cy="50%"
              innerRadius={50}
              outerRadius={80}
              paddingAngle={2}
            >
              {data.map((_, i) => (
                <Cell key={i} fill={COLORS[i % COLORS.length]} />
              ))}
            </Pie>
            <Tooltip
              contentStyle={{
                background: "#12121f",
                border: "1px solid rgba(255,255,255,0.08)",
                borderRadius: 8,
                fontSize: 12,
              }}
              itemStyle={{ color: "#e2e8f0" }}
              formatter={(val: number, name: string) => [
                `${val.toLocaleString()} (${total > 0 ? ((val / total) * 100).toFixed(1) : 0}%)`,
                name,
              ]}
            />
            <Legend
              formatter={(value: string) => (
                <span style={{ color: "#94a3b8", fontSize: 11 }}>{value}</span>
              )}
            />
          </PieChart>
        </ResponsiveContainer>
      )}
    </div>
  );
}
