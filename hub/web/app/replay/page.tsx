"use client";

import { Suspense, useEffect, useState, useCallback } from "react";
import { useSearchParams, useRouter } from "next/navigation";
import { fetchReplay, type ReplayFlow, type ReplayResponse } from "@/lib/api";
import { RefreshCw, Clock, ArrowLeft, ChevronRight, X } from "lucide-react";
import { clsx } from "clsx";

// ── Protocol lane config ───────────────────────────────────────────────────────

interface LaneConfig {
  proto: string;
  label: string;
  color: string;
  bg: string;
  match: (f: ReplayFlow) => boolean;
}

const LANES: LaneConfig[] = [
  {
    proto: "HTTP",
    label: "HTTP",
    color: "#3b82f6",
    bg: "rgba(59,130,246,0.15)",
    match: (f) => f.protocol.toUpperCase() === "HTTP",
  },
  {
    proto: "HTTP2",
    label: "HTTP/2 · gRPC",
    color: "#818cf8",
    bg: "rgba(129,140,248,0.15)",
    match: (f) => {
      const p = f.protocol.toUpperCase();
      return p === "HTTP2" || p === "HTTP/2" || p === "GRPC";
    },
  },
  {
    proto: "DNS",
    label: "DNS",
    color: "#e879f9",
    bg: "rgba(232,121,249,0.15)",
    match: (f) => f.protocol.toUpperCase() === "DNS",
  },
  {
    proto: "TLS",
    label: "TLS",
    color: "#22d3ee",
    bg: "rgba(34,211,238,0.15)",
    match: (f) => f.protocol.toUpperCase() === "TLS",
  },
  {
    proto: "OTHER",
    label: "TCP / UDP",
    color: "#64748b",
    bg: "rgba(100,116,139,0.15)",
    match: () => true, // catch-all
  },
];

// ── Helpers ────────────────────────────────────────────────────────────────────

function fmtBytes(n: number): string {
  if (n < 1024) return `${n} B`;
  if (n < 1_048_576) return `${(n / 1024).toFixed(1)} KB`;
  return `${(n / 1_048_576).toFixed(2)} MB`;
}

function fmtMs(ms: number): string {
  if (ms < 1000) return `${ms} ms`;
  return `${(ms / 1000).toFixed(2)} s`;
}

function timeLabel(iso: string): string {
  return new Date(iso).toLocaleTimeString([], {
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
  });
}

// ── Flow block on the lane ─────────────────────────────────────────────────────

function FlowBlock({
  flow,
  fromMs,
  totalMs,
  color,
  bg,
  selected,
  onSelect,
}: {
  flow: ReplayFlow;
  fromMs: number;
  totalMs: number;
  color: string;
  bg: string;
  selected: boolean;
  onSelect: (f: ReplayFlow) => void;
}) {
  const startMs = new Date(flow.timestamp).getTime();
  const leftPct = Math.max(0, Math.min(99.5, ((startMs - fromMs) / totalMs) * 100));
  const durPct  = (flow.duration_ms / totalMs) * 100;
  const widthPct = Math.max(0.3, Math.min(durPct, 100 - leftPct));

  return (
    <div
      role="button"
      tabIndex={0}
      onClick={(e) => { e.stopPropagation(); onSelect(flow); }}
      onKeyDown={(e) => { if (e.key === "Enter") onSelect(flow); }}
      style={{
        position: "absolute",
        left: `${leftPct}%`,
        width: `${widthPct}%`,
        minWidth: 4,
        top: 4,
        bottom: 4,
        backgroundColor: selected ? color : bg,
        borderLeft: `2px solid ${color}`,
        borderRadius: 2,
        cursor: "pointer",
        zIndex: selected ? 10 : 1,
        outline: selected ? `1px solid ${color}` : "none",
        transition: "background-color 80ms",
      }}
      title={`${flow.protocol}  ${flow.src_ip}:${flow.src_port} → ${flow.dst_ip}:${flow.dst_port}\n${flow.info}`}
    />
  );
}

// ── Timeline lane row ──────────────────────────────────────────────────────────

function TimelineLane({
  label,
  flows,
  fromMs,
  totalMs,
  color,
  bg,
  centerPct,
  selected,
  onSelect,
}: {
  label: string;
  flows: ReplayFlow[];
  fromMs: number;
  totalMs: number;
  color: string;
  bg: string;
  centerPct: number;
  selected: ReplayFlow | null;
  onSelect: (f: ReplayFlow | null) => void;
}) {
  return (
    <div className="flex items-stretch border-b border-white/[0.04] group">
      {/* Label */}
      <div className="w-32 shrink-0 flex items-center px-3 border-r border-white/[0.04]">
        <div className="flex items-center gap-1.5">
          <span
            className="w-1.5 h-1.5 rounded-full shrink-0"
            style={{ backgroundColor: color }}
          />
          <span className="text-[10px] text-slate-400 font-medium">{label}</span>
          <span className="text-[9px] text-slate-600 tabular-nums ml-0.5">
            ({flows.length})
          </span>
        </div>
      </div>

      {/* Lane area */}
      <div
        className="relative flex-1 h-8 cursor-default"
        onClick={() => onSelect(null)}
      >
        {/* Trigger line within this lane row */}
        <div
          className="absolute top-0 bottom-0 w-px bg-red-500/50 z-20 pointer-events-none"
          style={{ left: `${centerPct}%` }}
        />

        {/* Flow blocks */}
        {flows.map((f) => (
          <FlowBlock
            key={f.id}
            flow={f}
            fromMs={fromMs}
            totalMs={totalMs}
            color={color}
            bg={bg}
            selected={selected?.id === f.id}
            onSelect={onSelect}
          />
        ))}
      </div>
    </div>
  );
}

// ── Time axis header ───────────────────────────────────────────────────────────

function TimeAxis({
  fromMs,
  toMs,
  centerPct,
}: {
  fromMs: number;
  toMs: number;
  centerPct: number;
}) {
  const totalMs = Math.max(toMs - fromMs, 1);

  const ticks: { label: string; pct: number }[] = [];
  const firstMin = Math.ceil(fromMs / 60_000) * 60_000;
  for (let ms = firstMin; ms <= toMs; ms += 60_000) {
    const pct = ((ms - fromMs) / totalMs) * 100;
    const d = new Date(ms);
    ticks.push({
      label: d.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" }),
      pct,
    });
  }

  return (
    <div className="flex items-stretch border-b border-white/[0.06] bg-white/[0.01] sticky top-0 z-30">
      {/* Label column spacer */}
      <div className="w-32 shrink-0 border-r border-white/[0.04] flex items-center px-3">
        <span className="text-[9px] text-slate-600 uppercase tracking-wide">Protocol</span>
      </div>

      {/* Tick marks */}
      <div className="relative flex-1 h-7">
        {/* Trigger line */}
        <div
          className="absolute top-0 bottom-0 w-px bg-red-500 z-20 pointer-events-none"
          style={{ left: `${centerPct}%` }}
        >
          {/* Red dot at top */}
          <div className="absolute -top-0.5 left-1/2 -translate-x-1/2 w-2 h-2 rounded-full bg-red-500 shadow-[0_0_4px_#ef4444]" />
        </div>

        {ticks.map((t, i) => (
          <div
            key={i}
            className="absolute flex flex-col items-center"
            style={{ left: `${t.pct}%` }}
          >
            <div className="w-px h-2.5 bg-white/10 mt-0.5" />
            <span className="text-[9px] text-slate-600 -translate-x-1/2 mt-0.5 whitespace-nowrap">
              {t.label}
            </span>
          </div>
        ))}
      </div>
    </div>
  );
}

// ── Flow detail bottom panel ───────────────────────────────────────────────────

function FlowDetail({
  flow,
  onClose,
}: {
  flow: ReplayFlow;
  onClose: () => void;
}) {
  return (
    <div className="border-t border-white/[0.06] bg-[#0a0a18] px-6 py-4 shrink-0">
      <div className="flex items-center justify-between mb-3">
        <div className="flex items-center gap-2 min-w-0">
          <span className="text-xs font-semibold text-white shrink-0">{flow.protocol}</span>
          <ChevronRight size={12} className="text-slate-600 shrink-0" />
          <span className="text-xs font-mono text-slate-400 truncate">
            {flow.src_ip}:{flow.src_port} → {flow.dst_ip}:{flow.dst_port}
          </span>
        </div>
        <button
          onClick={onClose}
          className="p-1 rounded hover:bg-white/[0.06] text-slate-500 hover:text-slate-300 transition-colors shrink-0"
        >
          <X size={14} />
        </button>
      </div>

      <div className="grid grid-cols-2 sm:grid-cols-4 gap-x-6 gap-y-2.5">
        {[
          ["Time",      timeLabel(flow.timestamp)],
          ["Duration",  fmtMs(flow.duration_ms)],
          ["Bytes in",  fmtBytes(flow.bytes_in)],
          ["Bytes out", fmtBytes(flow.bytes_out)],
          ["Agent",     flow.hostname || flow.agent_id.slice(0, 8)],
          ["Flow ID",   flow.id.slice(0, 16) + "…"],
          ["Info",      flow.info || "—"],
        ].map(([k, v]) => (
          <div key={k}>
            <p className="text-[10px] text-slate-600 uppercase tracking-wide mb-0.5">{k}</p>
            <p className="text-xs text-slate-300 font-mono truncate" title={String(v)}>
              {v}
            </p>
          </div>
        ))}
      </div>
    </div>
  );
}

// ── Main timeline component (needs Suspense) ───────────────────────────────────

function ReplayTimeline() {
  const params    = useSearchParams();
  const router    = useRouter();

  const agentId   = params.get("agent_id")    ?? "";
  const around    = params.get("around")       ?? new Date().toISOString();
  const windowMin = parseInt(params.get("window_mins") ?? "5", 10);
  const hostLabel = params.get("hostname")     ?? (agentId ? agentId.slice(0, 8) : "unknown");

  const [data,     setData]     = useState<ReplayResponse | null>(null);
  const [loading,  setLoading]  = useState(true);
  const [error,    setError]    = useState<string | null>(null);
  const [selected, setSelected] = useState<ReplayFlow | null>(null);

  const load = useCallback(async () => {
    if (!agentId) { setLoading(false); return; }
    setLoading(true);
    setError(null);
    try {
      const d = await fetchReplay(agentId, around, windowMin);
      setData(d);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setLoading(false);
    }
  }, [agentId, around, windowMin]);

  useEffect(() => { load(); }, [load]);

  // Time range
  const fromMs   = data ? new Date(data.from).getTime()  : 0;
  const toMs     = data ? new Date(data.to).getTime()    : 0;
  const aroundMs = new Date(around).getTime();
  const totalMs  = Math.max(toMs - fromMs, 1);
  const centerPct = Math.max(0, Math.min(100, ((aroundMs - fromMs) / totalMs) * 100));

  // Distribute flows into lanes (first matching lane wins, catch-all last)
  const claimed = new Set<string>();
  const distributed = LANES.map((lane, i) => {
    const isLast = i === LANES.length - 1;
    const flows = (data?.flows ?? []).filter((f) => {
      if (claimed.has(f.id)) return false;
      return isLast ? true : lane.match(f);
    });
    flows.forEach((f) => claimed.add(f.id));
    return { ...lane, flows };
  });

  return (
    <div className="flex flex-col h-full min-h-0 bg-[#070711]">
      {/* ── Header ── */}
      <div className="flex items-center justify-between px-6 py-4 border-b border-white/[0.06] shrink-0">
        <div className="flex items-center gap-3 min-w-0">
          <button
            onClick={() => router.back()}
            className="p-1.5 rounded hover:bg-white/[0.06] text-slate-500
                       hover:text-slate-300 transition-colors shrink-0"
          >
            <ArrowLeft size={15} />
          </button>

          <div className="p-1.5 rounded-md bg-indigo-500/10 shrink-0">
            <Clock size={14} className="text-indigo-400" />
          </div>

          <div className="min-w-0">
            <h1 className="text-sm font-semibold text-white truncate">
              Incident Replay{" "}
              <span className="text-indigo-300">{hostLabel}</span>
            </h1>
            <p className="text-xs text-slate-500 mt-0.5">
              ±{windowMin} min window · {data?.total ?? "—"} flows ·{" "}
              triggered at <span className="font-mono">{timeLabel(around)}</span>
            </p>
          </div>
        </div>

        <button
          onClick={load}
          disabled={loading}
          className="flex items-center gap-1.5 text-xs text-slate-400 hover:text-white
                     px-2.5 py-1.5 rounded border border-white/10 hover:border-white/20
                     transition-colors disabled:opacity-50 shrink-0"
        >
          <RefreshCw className={clsx("h-3 w-3", loading && "animate-spin")} />
          Refresh
        </button>
      </div>

      {/* ── Error ── */}
      {error && (
        <div className="mx-6 mt-4 shrink-0 px-4 py-3 rounded-lg border border-red-500/20
                        bg-red-500/5 text-xs text-red-400">
          {error}
        </div>
      )}

      {/* ── No agent selected ── */}
      {!agentId && (
        <div className="flex-1 flex flex-col items-center justify-center text-center gap-2">
          <Clock size={32} className="text-slate-700" />
          <p className="text-sm text-slate-400">No agent selected</p>
          <p className="text-xs text-slate-600">
            Navigate here via an anomaly event or agent card.
          </p>
        </div>
      )}

      {/* ── Loading skeleton ── */}
      {loading && !data && agentId && (
        <div className="flex-1 flex items-center justify-center">
          <RefreshCw className="h-6 w-6 animate-spin text-slate-600" />
        </div>
      )}

      {/* ── Timeline ── */}
      {data && (
        <div className="flex-1 min-h-0 flex flex-col overflow-hidden">
          {/* Scrollable lane area */}
          <div className="flex-1 overflow-y-auto overflow-x-hidden">
            {/* Axis (sticky) */}
            <TimeAxis fromMs={fromMs} toMs={toMs} centerPct={centerPct} />

            {/* Protocol lanes */}
            {distributed.map((lane) => (
              <TimelineLane
                key={lane.proto}
                label={lane.label}
                flows={lane.flows}
                fromMs={fromMs}
                totalMs={totalMs}
                color={lane.color}
                bg={lane.bg}
                centerPct={centerPct}
                selected={selected}
                onSelect={setSelected}
              />
            ))}

            {/* Legend */}
            <div className="px-4 pt-3 pb-4 flex items-center gap-4 flex-wrap border-t border-white/[0.04]">
              {LANES.map((l) => (
                <div key={l.proto} className="flex items-center gap-1.5">
                  <span
                    className="w-2 h-2 rounded-sm"
                    style={{ backgroundColor: l.color, opacity: 0.8 }}
                  />
                  <span className="text-[10px] text-slate-500">{l.label}</span>
                </div>
              ))}
              <div className="flex items-center gap-1.5 ml-2">
                <span className="w-2 h-2 rounded-sm bg-red-500 opacity-70" />
                <span className="text-[10px] text-slate-500">Incident trigger</span>
              </div>
            </div>

            {/* Empty state */}
            {data.total === 0 && (
              <div className="flex flex-col items-center justify-center py-12 text-center gap-2">
                <Clock size={28} className="text-slate-700" />
                <p className="text-sm text-slate-400">No flows captured in this window</p>
                <p className="text-xs text-slate-600">
                  Try expanding the window or verify the agent was active at this time.
                </p>
              </div>
            )}
          </div>

          {/* Flow detail panel */}
          {selected && (
            <FlowDetail flow={selected} onClose={() => setSelected(null)} />
          )}
        </div>
      )}
    </div>
  );
}

// ── Page export (Suspense boundary for useSearchParams) ───────────────────────

export default function ReplayPage() {
  return (
    <Suspense
      fallback={
        <div className="flex items-center justify-center h-full">
          <RefreshCw className="h-6 w-6 animate-spin text-slate-600" />
        </div>
      }
    >
      <ReplayTimeline />
    </Suspense>
  );
}
