"use client";

import { type Flow } from "@/lib/api";
import { clsx } from "clsx";
import { Info, AlertTriangle } from "lucide-react";

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

const COLS: Array<{ label: string; cls: string; tooltip?: string }> = [
  { label: "Time",        cls: "w-24 shrink-0" },
  { label: "Protocol",    cls: "w-20 shrink-0" },
  {
    label: "Process",
    cls:   "w-32 shrink-0",
    tooltip:
      "Process name and PID captured by eBPF kernel probes (Linux only). " +
      "Shows the exact process making each TLS/TCP connection — curl, node, python, etc. " +
      "Use the eBPF-enabled agent binary (netscope-agent-ebpf-…) to populate this column. " +
      "In pcap-only mode the column shows '—'.",
  },
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
          <span key={i} className={clsx(col.cls, "flex items-center gap-1")}>
            {col.label}
            {col.tooltip && (
              <span
                title={col.tooltip}
                className="text-slate-600 hover:text-indigo-400 transition-colors cursor-help normal-case"
                aria-label={`About the ${col.label} column`}
              >
                <Info size={11} />
              </span>
            )}
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
              <span className="w-32 shrink-0 truncate" title={
                f.process_name
                  ? `${f.process_name} (PID ${f.pid})${f.pod_name ? ` · pod: ${f.pod_name}` : ""}`
                  : "pcap mode — run eBPF agent to see process info"
              }>
                {f.process_name ? (
                  <span className="inline-flex flex-col gap-0">
                    <span className="inline-flex items-center gap-1">
                      <span className="text-emerald-400/80">{f.process_name}</span>
                      <span className="text-slate-600 text-[10px]">{f.pid}</span>
                    </span>
                    {f.pod_name && (
                      <span className="text-indigo-400/60 text-[10px] truncate">{f.pod_name}</span>
                    )}
                  </span>
                ) : (
                  <span className="text-slate-700">—</span>
                )}
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
              <span className="flex-1 min-w-0 ml-4 truncate text-slate-400 flex items-center gap-2">
                {f.threat_level && f.threat_level !== "" && (
                  <span className={clsx(
                    "inline-flex items-center gap-0.5 px-1 py-0.5 rounded text-[9px] font-semibold border shrink-0",
                    f.threat_level === "high"   ? "bg-red-500/10 text-red-400 border-red-500/25" :
                    f.threat_level === "medium" ? "bg-amber-500/10 text-amber-400 border-amber-500/25" :
                                                   "bg-yellow-500/10 text-yellow-400 border-yellow-500/25",
                  )} title={`Threat score: ${f.threat_score}`}>
                    <AlertTriangle size={8} />
                    {f.threat_level.toUpperCase()}
                  </span>
                )}
                <span className="truncate">{f.info}</span>
              </span>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
