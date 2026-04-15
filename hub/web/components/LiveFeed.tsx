"use client";

import { useEffect, useRef, useState, useCallback } from "react";
import { createFlowStream, type Flow } from "@/lib/api";
import { clsx } from "clsx";

const MAX_ROWS = 100;

const PROTOCOL_COLORS: Record<string, string> = {
  HTTP:  "bg-blue-500/15 text-blue-400 border-blue-500/30",
  HTTPS: "bg-blue-500/15 text-blue-400 border-blue-500/30",
  DNS:   "bg-purple-500/15 text-purple-400 border-purple-500/30",
  TCP:   "bg-slate-500/15 text-slate-400 border-slate-500/30",
  UDP:   "bg-emerald-500/15 text-emerald-400 border-emerald-500/30",
};

function ProtocolBadge({ protocol }: { protocol: string }) {
  const cls = PROTOCOL_COLORS[protocol] ?? "bg-gray-500/15 text-gray-400 border-gray-500/30";
  return (
    <span className={clsx("px-1.5 py-0.5 rounded text-[10px] font-semibold border", cls)}>
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

export function LiveFeed() {
  const [flows, setFlows] = useState<Flow[]>([]);
  const [connected, setConnected] = useState(false);
  const bottomRef = useRef<HTMLDivElement>(null);
  const autoScrollRef = useRef(true);

  const addFlow = useCallback((flow: Flow) => {
    setFlows((prev) => {
      const next = [flow, ...prev];
      return next.length > MAX_ROWS ? next.slice(0, MAX_ROWS) : next;
    });
  }, []);

  useEffect(() => {
    let cleanup: (() => void) | undefined;
    let retryTimeout: ReturnType<typeof setTimeout> | undefined;

    const connect = () => {
      cleanup = createFlowStream(
        addFlow,
        () => {
          setConnected(false);
          retryTimeout = setTimeout(connect, 5_000);
        },
        () => setConnected(true),
      );
    };

    connect();
    return () => {
      cleanup?.();
      if (retryTimeout) clearTimeout(retryTimeout);
    };
  }, [addFlow]);

  // Auto-scroll to bottom when new rows arrive
  useEffect(() => {
    if (autoScrollRef.current) {
      bottomRef.current?.scrollIntoView({ behavior: "instant" });
    }
  }, [flows.length]);

  return (
    <div className="rounded-xl bg-[#0d0d1a] border border-white/[0.06] flex flex-col h-[480px]">
      {/* Header */}
      <div className="flex items-center justify-between px-4 py-3 border-b border-white/[0.06]">
        <span className="text-sm font-medium text-white">Live Feed</span>
        <div className="flex items-center gap-1.5 text-xs">
          <span
            className={clsx(
              "inline-block w-1.5 h-1.5 rounded-full",
              connected ? "bg-emerald-400 animate-pulse" : "bg-red-400"
            )}
          />
          <span className={connected ? "text-emerald-400" : "text-red-400"}>
            {connected ? "Live" : "Reconnecting…"}
          </span>
        </div>
      </div>

      {/* Rows */}
      <div
        className="flex-1 overflow-y-auto font-mono text-xs"
        onScroll={(e) => {
          const el = e.currentTarget;
          autoScrollRef.current =
            el.scrollHeight - el.scrollTop - el.clientHeight < 50;
        }}
      >
        {flows.length === 0 ? (
          <div className="flex items-center justify-center h-full text-slate-600 text-sm">
            Waiting for flows…
          </div>
        ) : (
          flows.map((f) => (
            <div
              key={f.id}
              className="flex items-center gap-3 px-4 py-1.5 border-b border-white/[0.03]
                         hover:bg-white/[0.02] transition-colors"
            >
              <span className="text-slate-600 shrink-0 w-20">
                {formatTime(f.timestamp)}
              </span>
              <ProtocolBadge protocol={f.protocol} />
              <span className="text-slate-400 shrink-0">
                {f.src_ip}:{f.src_port}
              </span>
              <span className="text-slate-600">→</span>
              <span className="text-slate-400 shrink-0">
                {f.dst_ip}:{f.dst_port}
              </span>
              <span className="text-slate-500 truncate flex-1">{f.info}</span>
            </div>
          ))
        )}
        <div ref={bottomRef} />
      </div>
    </div>
  );
}
