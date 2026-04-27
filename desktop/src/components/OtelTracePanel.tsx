/// OtelTracePanel — OpenTelemetry trace side panel (F8).
///
/// Opens as a right-side drawer when a flow with distributed-tracing headers
/// is selected.  It lets the user:
///   • See the trace ID extracted from W3C traceparent / B3 / custom headers
///   • Configure the OTel backend URL (Jaeger, Zipkin, Grafana Tempo)
///   • Jump directly to the trace in the configured backend
///
/// The backend URL is persisted in Rust AppState via
/// `get_otel_backend_url` / `set_otel_backend_url` Tauri commands.

import { useEffect, useRef, useState } from "react";
import { invoke } from "@tauri-apps/api/core";
import { X, ExternalLink, Settings2, Check } from "lucide-react";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import type { FlowDto } from "@/types/flow";

// ── trace-header extraction ───────────────────────────────────────────────────

interface TraceContext {
  traceId: string;
  spanId?: string;
  sampled?: boolean;
  source: string; // which header we found it in
}

/**
 * Scan all HTTP (req + resp) headers for distributed tracing context.
 * Priority: W3C traceparent → B3 single → B3 multi-header → custom x-trace-id.
 */
function extractTraceContext(flow: FlowDto): TraceContext | null {
  const allHeaders: [string, string][] = [
    ...(flow.http?.reqHeaders ?? []),
    ...(flow.http?.respHeaders ?? []),
    ...(flow.http2?.request?.headers ?? []),
    ...(flow.http2?.response?.headers ?? []),
  ];

  const find = (key: string) =>
    allHeaders.find(([k]) => k.toLowerCase() === key.toLowerCase())?.[1];

  // W3C traceparent: "00-<traceId>-<spanId>-<flags>"
  const traceparent = find("traceparent");
  if (traceparent) {
    const parts = traceparent.split("-");
    if (parts.length >= 4) {
      return {
        traceId: parts[1],
        spanId: parts[2],
        sampled: parts[3] === "01",
        source: "traceparent",
      };
    }
  }

  // B3 single header: "<traceId>-<spanId>[-<sampled>]"
  const b3 = find("b3");
  if (b3) {
    const parts = b3.split("-");
    if (parts.length >= 2) {
      return {
        traceId: parts[0],
        spanId: parts[1],
        sampled: parts[2] === "1",
        source: "b3",
      };
    }
  }

  // B3 multi-header
  const b3TraceId = find("x-b3-traceid");
  if (b3TraceId) {
    return {
      traceId: b3TraceId,
      spanId: find("x-b3-spanid"),
      sampled: find("x-b3-sampled") === "1",
      source: "x-b3-traceid",
    };
  }

  // Custom single-header variants
  for (const key of ["x-trace-id", "x-request-id", "x-amzn-trace-id"]) {
    const val = find(key);
    if (val) {
      // For x-amzn-trace-id extract the Root= segment
      if (key === "x-amzn-trace-id") {
        const root = val.split(";").find((s) => s.startsWith("Root="));
        return { traceId: root ? root.slice(5) : val, source: key };
      }
      return { traceId: val, source: key };
    }
  }

  return null;
}

// ── backend URL helpers ────────────────────────────────────────────────────────

interface BackendPreset {
  name: string;
  hint: string;
  buildUrl: (base: string, traceId: string) => string;
}

const PRESETS: BackendPreset[] = [
  {
    name: "Jaeger",
    hint: "http://localhost:16686",
    buildUrl: (base, id) => `${base}/trace/${id}`,
  },
  {
    name: "Zipkin",
    hint: "http://localhost:9411",
    buildUrl: (base, id) => `${base}/zipkin/traces/${id}`,
  },
  {
    name: "Tempo (Grafana)",
    hint: "http://localhost:3000",
    buildUrl: (base, id) =>
      `${base}/explore?orgId=1&left={"datasource":"tempo","queries":[{"refId":"A","queryType":"traceql","query":"${id}"}]}`,
  },
  {
    name: "Honeycomb",
    hint: "https://ui.honeycomb.io",
    buildUrl: (base, id) => `${base}/trace?trace_id=${id}`,
  },
];

function buildTraceUrl(backendUrl: string, traceId: string): string {
  const lower = backendUrl.toLowerCase();
  for (const preset of PRESETS) {
    if (lower.includes(preset.hint.split("://")[1]?.split(":")[0] ?? "")) {
      return preset.buildUrl(backendUrl.replace(/\/$/, ""), traceId);
    }
  }
  // Generic fallback
  return `${backendUrl.replace(/\/$/, "")}/trace/${traceId}`;
}

// ── component ─────────────────────────────────────────────────────────────────

interface OtelTracePanelProps {
  flow: FlowDto;
  onClose: () => void;
}

export function OtelTracePanel({ flow, onClose }: OtelTracePanelProps) {
  const [backendUrl, setBackendUrl] = useState("");
  const [editingUrl, setEditingUrl] = useState(false);
  const [urlDraft, setUrlDraft] = useState("");
  const [saved, setSaved] = useState(false);
  const inputRef = useRef<HTMLInputElement>(null);

  const ctx = extractTraceContext(flow);

  // Load persisted backend URL on mount
  useEffect(() => {
    invoke<string | null>("get_otel_backend_url").then((u) => {
      if (u) setBackendUrl(u);
    });
  }, []);

  const startEdit = () => {
    setUrlDraft(backendUrl);
    setEditingUrl(true);
    setTimeout(() => inputRef.current?.focus(), 50);
  };

  const saveUrl = async () => {
    const trimmed = urlDraft.trim();
    await invoke("set_otel_backend_url", { url: trimmed });
    setBackendUrl(trimmed);
    setEditingUrl(false);
    setSaved(true);
    setTimeout(() => setSaved(false), 1500);
  };

  const openTrace = (traceId: string) => {
    if (!backendUrl) { startEdit(); return; }
    const url = buildTraceUrl(backendUrl, traceId);
    // Open in the system browser via Tauri shell
    invoke("open_url", { url }).catch(() => {
      // Fallback: copy to clipboard
      navigator.clipboard.writeText(url);
    });
  };

  return (
    <div className="flex h-full w-72 shrink-0 flex-col border-l border-white/10 bg-[#0a0a14] text-white overflow-hidden">
      {/* Header */}
      <div className="flex shrink-0 items-center justify-between border-b border-white/10 px-3 py-2">
        <div className="flex items-center gap-1.5">
          <span className="text-[11px] font-semibold text-white">OTel Trace</span>
          {ctx && (
            <span className="rounded bg-purple-500/20 px-1.5 py-px text-[9px] text-purple-400 border border-purple-500/30">
              found
            </span>
          )}
        </div>
        <button
          onClick={onClose}
          className="text-gray-500 hover:text-gray-300 transition-colors"
        >
          <X className="h-3.5 w-3.5" />
        </button>
      </div>

      <div className="flex-1 overflow-y-auto p-3 space-y-4">
        {/* No trace headers */}
        {!ctx && (
          <div className="rounded-lg border border-white/10 bg-white/5 p-3 text-center">
            <div className="text-[11px] text-gray-400 mb-1">No trace headers found</div>
            <div className="text-[10px] text-gray-600">
              This flow has no W3C traceparent, B3, or x-trace-id headers.
            </div>
          </div>
        )}

        {/* Trace context */}
        {ctx && (
          <section>
            <div className="mb-1.5 text-[10px] font-semibold text-gray-500 uppercase tracking-wider">
              Trace Context
            </div>
            <div className="rounded-lg border border-white/10 bg-white/5 divide-y divide-white/5">
              <ContextRow label="Trace ID" value={ctx.traceId} mono />
              {ctx.spanId && <ContextRow label="Span ID" value={ctx.spanId} mono />}
              <ContextRow label="Header" value={ctx.source} />
              {ctx.sampled !== undefined && (
                <ContextRow
                  label="Sampled"
                  value={ctx.sampled ? "yes" : "no"}
                  accent={ctx.sampled ? "emerald" : "gray"}
                />
              )}
            </div>
          </section>
        )}

        {/* Open in backend */}
        {ctx && (
          <section>
            <div className="mb-1.5 text-[10px] font-semibold text-gray-500 uppercase tracking-wider">
              Open Trace
            </div>

            {/* Backend URL setting */}
            <div className="rounded-lg border border-white/10 bg-white/5 p-2.5 mb-2">
              <div className="flex items-center justify-between mb-1.5">
                <span className="text-[10px] text-gray-400">Backend URL</span>
                <button
                  onClick={startEdit}
                  className="text-gray-500 hover:text-gray-300 transition-colors"
                  title="Edit backend URL"
                >
                  <Settings2 className="h-3 w-3" />
                </button>
              </div>

              {editingUrl ? (
                <div className="flex gap-1.5">
                  <Input
                    ref={inputRef}
                    value={urlDraft}
                    onChange={(e) => setUrlDraft(e.target.value)}
                    onKeyDown={(e) => { if (e.key === "Enter") saveUrl(); if (e.key === "Escape") setEditingUrl(false); }}
                    placeholder="http://localhost:16686"
                    className="h-6 text-[10px] font-mono flex-1"
                  />
                  <Button size="sm" variant="ghost" onClick={saveUrl} className="h-6 w-6 p-0">
                    <Check className="h-3 w-3 text-emerald-400" />
                  </Button>
                </div>
              ) : (
                <div
                  onClick={startEdit}
                  className="cursor-pointer rounded bg-white/5 px-2 py-1 text-[10px] font-mono text-gray-400 hover:bg-white/10 transition-colors truncate"
                  title={backendUrl || "Click to set backend URL"}
                >
                  {backendUrl || (
                    <span className="text-gray-600 italic">not configured — click to set</span>
                  )}
                </div>
              )}

              {saved && (
                <div className="mt-1 text-[10px] text-emerald-400">Saved.</div>
              )}
            </div>

            {/* Quick-open buttons */}
            <button
              onClick={() => openTrace(ctx.traceId)}
              className={cn(
                "w-full flex items-center justify-center gap-1.5 rounded-lg border px-3 py-2 text-[11px] font-semibold transition-all",
                backendUrl
                  ? "border-blue-500/40 bg-blue-500/10 text-blue-400 hover:bg-blue-500/20"
                  : "border-white/10 bg-white/5 text-gray-500 hover:bg-white/10",
              )}
            >
              <ExternalLink className="h-3.5 w-3.5" />
              {backendUrl ? "Open in Backend" : "Set URL to open trace"}
            </button>

            {/* Preset quick-set buttons */}
            {!backendUrl && (
              <div className="mt-2">
                <div className="mb-1 text-[10px] text-gray-600">Quick-set backend:</div>
                <div className="flex flex-wrap gap-1">
                  {PRESETS.map((p) => (
                    <button
                      key={p.name}
                      onClick={async () => {
                        await invoke("set_otel_backend_url", { url: p.hint });
                        setBackendUrl(p.hint);
                      }}
                      className="rounded bg-white/5 border border-white/10 px-2 py-1 text-[9px] text-gray-400 hover:bg-white/10 hover:text-gray-200 transition-colors"
                    >
                      {p.name}
                    </button>
                  ))}
                </div>
              </div>
            )}
          </section>
        )}

        {/* Flow context */}
        <section>
          <div className="mb-1.5 text-[10px] font-semibold text-gray-500 uppercase tracking-wider">
            Flow
          </div>
          <div className="rounded-lg border border-white/10 bg-white/5 divide-y divide-white/5">
            <ContextRow label="Protocol" value={flow.protocol} />
            <ContextRow label="Src" value={`${flow.srcIp}:${flow.srcPort}`} mono />
            <ContextRow label="Dst" value={`${flow.dstIp}:${flow.dstPort}`} mono />
            {flow.http?.method && (
              <ContextRow label="Method" value={flow.http.method} />
            )}
            {flow.http?.path && (
              <ContextRow label="Path" value={flow.http.path} mono />
            )}
            {flow.http?.statusCode != null && (
              <ContextRow
                label="Status"
                value={String(flow.http.statusCode)}
                accent={flow.http.statusCode >= 400 ? "red" : "emerald"}
              />
            )}
          </div>
        </section>
      </div>
    </div>
  );
}

// ── helpers ────────────────────────────────────────────────────────────────────

function ContextRow({
  label,
  value,
  mono,
  accent,
}: {
  label: string;
  value: string;
  mono?: boolean;
  accent?: "emerald" | "red" | "gray";
}) {
  const valueClass =
    accent === "emerald"
      ? "text-emerald-400"
      : accent === "red"
        ? "text-red-400"
        : accent === "gray"
          ? "text-gray-500"
          : "text-white";

  return (
    <div className="flex items-start justify-between gap-2 px-2.5 py-1.5">
      <span className="text-[10px] text-gray-500 shrink-0">{label}</span>
      <span
        className={cn(
          "text-[10px] text-right break-all",
          mono && "font-mono",
          valueClass,
        )}
      >
        {value}
      </span>
    </div>
  );
}
