import { useRef, useEffect, useCallback } from "react";
import { useVirtualizer } from "@tanstack/react-virtual";
import { useCaptureStore } from "@/store/captureStore";
import { rowBgColor, protocolColor, cn } from "@/lib/utils";
import type { FlowDto } from "@/types/flow";

const COL_WIDTHS = {
  time: "w-28 min-w-[7rem] shrink-0",
  protocol: "w-20 min-w-[5rem] shrink-0",
  src: "w-44 min-w-0 shrink-0 overflow-hidden",
  dst: "w-44 min-w-0 shrink-0 overflow-hidden",
  length: "w-16 min-w-[4rem] shrink-0 text-right",
  info: "flex-1 min-w-0 overflow-hidden",
};

function FlowRow({
  flow,
  selected,
  onClick,
}: {
  flow: FlowDto;
  selected: boolean;
  onClick: () => void;
}) {
  return (
    <div
      className={cn(
        "flex items-center border-b border-white/5 cursor-pointer select-none px-2 py-0.5 text-xs font-mono",
        rowBgColor(flow, selected)
      )}
      onClick={onClick}
    >
      <span className={cn("shrink-0 text-gray-400", COL_WIDTHS.time)}>
        {flow.timeStr}
      </span>
      <span className={cn("shrink-0 font-semibold", COL_WIDTHS.protocol, protocolColor(flow.protocol))}>
        {flow.protocol}
      </span>
      <span
        className={cn("truncate text-gray-300", COL_WIDTHS.src)}
        title={`${flow.srcIp}:${flow.srcPort}`}
      >
        {flow.srcIp}:{flow.srcPort}
      </span>
      <span className="w-5 shrink-0 text-center text-[10px] text-gray-400">→</span>
      <span
        className={cn("truncate text-gray-300", COL_WIDTHS.dst)}
        title={`${flow.dstIp}:${flow.dstPort}`}
      >
        {flow.dstIp}:{flow.dstPort}
      </span>
      <span className={cn("shrink-0 tabular-nums text-gray-400", COL_WIDTHS.length)}>
        {flow.length}
      </span>
      <span className={cn("ml-2 truncate text-gray-300", COL_WIDTHS.info)}>
        {flow.info}
      </span>
    </div>
  );
}

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

  // Auto-scroll to bottom when new flows arrive (unless user has scrolled up)
  useEffect(() => {
    const el = parentRef.current;
    if (!el) return;
    const handleScroll = () => {
      const atBottom = el.scrollHeight - el.scrollTop - el.clientHeight < 80;
      autoScrollRef.current = atBottom;
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
    [selectedFlow, setSelectedFlow]
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
        <span className={cn(COL_WIDTHS.length)}>Length</span>
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
