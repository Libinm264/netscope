import { useRef, useEffect, useCallback } from "react";
import { useVirtualizer } from "@tanstack/react-virtual";
import { useCaptureStore } from "@/store/captureStore";
import { rowBgColor, cn } from "@/lib/utils";
import type { FlowDto } from "@/types/flow";

const COL_WIDTHS = {
  time:     "w-28 min-w-[7rem] shrink-0",
  protocol: "w-24 min-w-[6rem] shrink-0",
  src:      "w-44 min-w-0 shrink-0 overflow-hidden",
  dst:      "w-44 min-w-0 shrink-0 overflow-hidden",
  length:   "w-16 min-w-[4rem] shrink-0 text-right",
  info:     "flex-1 min-w-0 overflow-hidden",
};

// ── Protocol badge colours ────────────────────────────────────────────────────

function protocolBadge(protocol: string): { bg: string; text: string } {
  const p = protocol.toUpperCase();
  if (p.startsWith("HTTP 4") || p.startsWith("HTTP 5"))
    return { bg: "bg-red-900/40",    text: "text-red-400" };
  if (p === "HTTP")   return { bg: "bg-sky-900/30",     text: "text-sky-300" };
  if (p === "DNS")    return { bg: "bg-purple-900/30",  text: "text-purple-300" };
  if (p === "TLS")    return { bg: "bg-indigo-900/30",  text: "text-indigo-300" };
  if (p === "ICMP")   return { bg: "bg-cyan-900/30",    text: "text-cyan-300" };
  if (p === "ARP")    return { bg: "bg-amber-900/30",   text: "text-amber-300" };
  if (p === "TCP")    return { bg: "bg-emerald-900/25", text: "text-emerald-400" };
  if (p === "UDP")    return { bg: "bg-teal-900/25",    text: "text-teal-400" };
  return               { bg: "bg-gray-800/50",          text: "text-gray-400" };
}

// Badges shown after the protocol name in the Info column
function FlowBadges({ flow }: { flow: FlowDto }) {
  const badges: { label: string; cls: string }[] = [];

  // TLS alert severity
  if (flow.tls?.alertLevel === "fatal") {
    badges.push({ label: "fatal", cls: "bg-red-900/50 text-red-400 border-red-700/50" });
  }
  // Expired certificate
  if (flow.tls?.certExpired) {
    badges.push({ label: "EXPIRED", cls: "bg-red-900/50 text-red-400 border-red-700/50" });
  }
  // Weak cipher
  if (flow.tls?.hasWeakCipher) {
    badges.push({ label: "weak", cls: "bg-amber-900/50 text-amber-400 border-amber-700/50" });
  }
  // TCP retransmissions
  if (flow.tcpStats && flow.tcpStats.retransmissions > 0) {
    badges.push({
      label: `↺${flow.tcpStats.retransmissions}`,
      cls: "bg-amber-900/40 text-amber-400 border-amber-700/40",
    });
  }
  // ARP spoofing hint (is-at coming from unexpected sender)
  if (flow.arp?.operation === "is-at") {
    badges.push({ label: "is-at", cls: "bg-amber-900/30 text-amber-400 border-amber-700/30" });
  }

  if (badges.length === 0) return null;

  return (
    <>
      {badges.map((b, i) => (
        <span
          key={i}
          className={cn(
            "ml-1.5 shrink-0 rounded px-1 py-px text-[9px] font-semibold border",
            b.cls,
          )}
        >
          {b.label}
        </span>
      ))}
    </>
  );
}

// ── Row component ─────────────────────────────────────────────────────────────

function FlowRow({
  flow,
  selected,
  onClick,
}: {
  flow: FlowDto;
  selected: boolean;
  onClick: () => void;
}) {
  const { bg, text } = protocolBadge(flow.protocol);

  return (
    <div
      className={cn(
        "flex items-center border-b border-white/5 cursor-pointer select-none px-2 py-0.5 text-xs font-mono",
        rowBgColor(flow, selected),
      )}
      onClick={onClick}
    >
      {/* Time */}
      <span className={cn("shrink-0 text-gray-400", COL_WIDTHS.time)}>
        {flow.timeStr}
      </span>

      {/* Protocol badge */}
      <span className={cn("shrink-0", COL_WIDTHS.protocol)}>
        <span
          className={cn(
            "inline-block rounded px-1.5 py-px text-[10px] font-semibold",
            bg,
            text,
          )}
        >
          {flow.protocol}
        </span>
      </span>

      {/* Source */}
      <span
        className={cn("truncate text-gray-300", COL_WIDTHS.src)}
        title={`${flow.srcIp}:${flow.srcPort}`}
      >
        {flow.srcIp}
        {flow.srcPort > 0 && `:${flow.srcPort}`}
      </span>

      <span className="w-5 shrink-0 text-center text-[10px] text-gray-500">→</span>

      {/* Destination */}
      <span
        className={cn("truncate text-gray-300", COL_WIDTHS.dst)}
        title={`${flow.dstIp}:${flow.dstPort}`}
      >
        {flow.dstIp}
        {flow.dstPort > 0 && `:${flow.dstPort}`}
      </span>

      {/* Length */}
      <span className={cn("shrink-0 tabular-nums text-gray-400", COL_WIDTHS.length)}>
        {flow.length}
      </span>

      {/* Info + badges */}
      <span className={cn("ml-2 flex items-center min-w-0", COL_WIDTHS.info)}>
        <span className="truncate text-gray-300">{flow.info}</span>
        <FlowBadges flow={flow} />
      </span>
    </div>
  );
}

// ── List pane ─────────────────────────────────────────────────────────────────

export function PacketListPane() {
  const { filteredFlows, selectedFlow, setSelectedFlow } = useCaptureStore();
  const parentRef = useRef<HTMLDivElement>(null);
  const autoScrollRef = useRef(true);

  const rowVirtualizer = useVirtualizer({
    count: filteredFlows.length,
    getScrollElement: () => parentRef.current,
    estimateSize: () => 22,
    overscan: 20,
  });

  // Auto-scroll to bottom when new flows arrive (unless user scrolled up)
  useEffect(() => {
    const el = parentRef.current;
    if (!el) return;
    const handleScroll = () => {
      autoScrollRef.current =
        el.scrollHeight - el.scrollTop - el.clientHeight < 80;
    };
    el.addEventListener("scroll", handleScroll, { passive: true });
    return () => el.removeEventListener("scroll", handleScroll);
  }, []);

  useEffect(() => {
    if (autoScrollRef.current && filteredFlows.length > 0) {
      rowVirtualizer.scrollToIndex(filteredFlows.length - 1, { align: "end" });
    }
  }, [filteredFlows.length, rowVirtualizer]);

  const handleClick = useCallback(
    (flow: FlowDto) => {
      setSelectedFlow(flow.id === selectedFlow?.id ? null : flow);
    },
    [selectedFlow, setSelectedFlow],
  );

  return (
    <div className="flex flex-col h-full overflow-hidden">
      {/* Header */}
      <div className="flex shrink-0 items-center border-b border-white/10 bg-[#0d0d1a] px-2 py-1 text-[11px] font-semibold text-gray-400 uppercase tracking-wide">
        <span className={COL_WIDTHS.time}>Time</span>
        <span className={COL_WIDTHS.protocol}>Protocol</span>
        <span className={COL_WIDTHS.src}>Source</span>
        <span className="w-5" />
        <span className={COL_WIDTHS.dst}>Destination</span>
        <span className={cn(COL_WIDTHS.length)}>Len</span>
        <span className="ml-2 flex-1">Info</span>
      </div>

      {/* Virtualised rows */}
      <div ref={parentRef} className="flex-1 overflow-auto">
        <div
          style={{ height: rowVirtualizer.getTotalSize() }}
          className="relative w-full"
        >
          {rowVirtualizer.getVirtualItems().map((virtualRow) => {
            const flow = filteredFlows[virtualRow.index];
            return (
              <div
                key={virtualRow.key}
                data-index={virtualRow.index}
                ref={rowVirtualizer.measureElement}
                style={{
                  position: "absolute",
                  top: 0,
                  transform: `translateY(${virtualRow.start}px)`,
                  width: "100%",
                }}
              >
                <FlowRow
                  flow={flow}
                  selected={selectedFlow?.id === flow.id}
                  onClick={() => handleClick(flow)}
                />
              </div>
            );
          })}
        </div>
      </div>

      {/* Empty state */}
      {filteredFlows.length === 0 && (
        <div className="absolute inset-0 flex items-center justify-center text-gray-500 text-sm pointer-events-none">
          No packets captured yet — select an interface and press Start
        </div>
      )}
    </div>
  );
}
