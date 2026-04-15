"use client";

import { type Flow } from "@/lib/api";
import { clsx } from "clsx";

const PROTOCOL_COLORS: Record<string, string> = {
  HTTP:  "bg-blue-500/15 text-blue-400 border-blue-500/30",
  HTTPS: "bg-blue-500/15 text-blue-400 border-blue-500/30",
  DNS:   "bg-purple-500/15 text-purple-400 border-purple-500/30",
  TCP:   "bg-slate-500/15 text-slate-400 border-slate-500/30",
  UDP:   "bg-emerald-500/15 text-emerald-400 border-emerald-500/30",
};

function ProtoBadge({ protocol }: { protocol: string }) {
  const cls =
    PROTOCOL_COLORS[protocol] ?? "bg-gray-500/15 text-gray-400 border-gray-500/30";
  return (
    <span
      className={clsx(
        "inline-block px-1.5 py-0.5 rounded text-[10px] font-semibold border",
        cls
      )}
    >
      {protocol}
    </span>
  );
}

function formatTime(iso: string) {
  try {
    return new Date(iso).toLocaleTimeString("en-US", {
      hour12: false,
      hour: "2-digit",
      minute: "2-digit",
      second: "2-digit",
    });
  } catch {
    return "—";
  }
}

interface Props {
  flows: Flow[];
  loading?: boolean;
}

const COLS = [
  { label: "Time",        cls: "w-24 shrink-0" },
  { label: "Protocol",    cls: "w-20 shrink-0" },
  { label: "Source",      cls: "w-44 shrink-0" },
  { label: "",            cls: "w-4  shrink-0 text-center" },
  { label: "Destination", cls: "w-44 shrink-0" },
  { label: "Length",      cls: "w-16 shrink-0 text-right" },
  { label: "Info",        cls: "flex-1 min-w-0" },
];

export function FlowTable({ flows, loading }: Props) {
  return (
    <div className="rounded-xl bg-[#0d0d1a] border border-white/[0.06] overflow-hidden">
      {/* Header */}
      <div className="flex items-center px-4 py-2 border-b border-white/[0.06]
                      text-[11px] font-semibold text-slate-500 uppercase tracking-wide">
        {COLS.map((col, i) => (
          <span key={i} className={col.cls}>
            {col.label}
          </span>
        ))}
      </div>

      {/* Body */}
      {loading ? (
        <div className="space-y-0">
          {Array.from({ length: 8 }).map((_, i) => (
            <div
              key={i}
              className="flex items-center gap-4 px-4 py-2 border-b border-white/[0.03]"
            >
              <div className="h-3 w-16 rounded bg-white/5 animate-pulse" />
              <div className="h-3 w-12 rounded bg-white/5 animate-pulse" />
              <div className="h-3 w-32 rounded bg-white/5 animate-pulse" />
              <div className="h-3 flex-1 rounded bg-white/5 animate-pulse" />
            </div>
          ))}
        </div>
      ) : flows.length === 0 ? (
        <div className="flex items-center justify-center py-16 text-slate-600 text-sm">
          No flows found
        </div>
      ) : (
        <div className="font-mono text-xs divide-y divide-white/[0.03]">
          {flows.map((f) => (
            <div
              key={f.id}
              className="flex items-center px-4 py-1.5 hover:bg-white/[0.02] transition-colors"
            >
              <span className="w-24 shrink-0 text-slate-600">
                {formatTime(f.timestamp)}
              </span>
              <span className="w-20 shrink-0">
                <ProtoBadge protocol={f.protocol} />
              </span>
              <span
                className="w-44 shrink-0 truncate text-slate-300"
                title={`${f.src_ip}:${f.src_port}`}
              >
                {f.src_ip}:{f.src_port}
              </span>
              <span className="w-4 shrink-0 text-center text-slate-600 text-[10px]">
                →
              </span>
              <span
                className="w-44 shrink-0 truncate text-slate-300"
                title={`${f.dst_ip}:${f.dst_port}`}
              >
                {f.dst_ip}:{f.dst_port}
              </span>
              <span className="w-16 shrink-0 text-right tabular-nums text-slate-500">
                {f.bytes_in + f.bytes_out}
              </span>
              <span className="flex-1 min-w-0 ml-4 truncate text-slate-400">
                {f.info}
              </span>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
