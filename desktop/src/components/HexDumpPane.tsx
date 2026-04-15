import { useMemo } from "react";
import { useCaptureStore } from "@/store/captureStore";

const BYTES_PER_ROW = 16;

function hexDump(hex: string): Array<{ offset: number; hex: string[]; ascii: string }> {
  if (!hex) return [];

  // Decode hex string to bytes
  const bytes: number[] = [];
  for (let i = 0; i < hex.length; i += 2) {
    bytes.push(parseInt(hex.slice(i, i + 2), 16));
  }

  const rows = [];
  for (let offset = 0; offset < bytes.length; offset += BYTES_PER_ROW) {
    const chunk = bytes.slice(offset, offset + BYTES_PER_ROW);
    const hexCols = chunk.map((b) => b.toString(16).padStart(2, "0").toUpperCase());
    const ascii = chunk.map((b) => (b >= 32 && b < 127 ? String.fromCharCode(b) : ".")).join("");
    rows.push({ offset, hex: hexCols, ascii });
  }
  return rows;
}

export function HexDumpPane() {
  const selectedFlow = useCaptureStore((s) => s.selectedFlow);
  const rows = useMemo(() => hexDump(selectedFlow?.rawHex ?? ""), [selectedFlow]);

  if (!selectedFlow) {
    return (
      <div className="flex h-full items-center justify-center text-gray-500 text-xs">
        Select a packet to see hex dump
      </div>
    );
  }

  if (rows.length === 0) {
    return (
      <div className="flex h-full items-center justify-center text-gray-500 text-xs">
        Raw bytes not available for this flow
        <span className="ml-1 text-gray-600">(added in Phase 4)</span>
      </div>
    );
  }

  return (
    <div className="h-full overflow-auto bg-[#050510] p-2 font-mono text-xs">
      {rows.map((row) => (
        <div key={row.offset} className="flex gap-4 leading-5 hover:bg-white/5">
          {/* Offset */}
          <span className="w-16 shrink-0 text-gray-500">
            {row.offset.toString(16).padStart(8, "0")}
          </span>

          {/* Hex bytes — two groups of 8 */}
          <span className="shrink-0 text-gray-300">
            {row.hex.slice(0, 8).join(" ")}
            <span className="mx-2" />
            {row.hex.slice(8).join(" ")}
            {/* Pad incomplete last row */}
            {row.hex.length < BYTES_PER_ROW && (
              <span className="text-transparent">
                {" ".repeat((BYTES_PER_ROW - row.hex.length) * 3)}
              </span>
            )}
          </span>

          {/* ASCII */}
          <span className="text-emerald-400">{row.ascii}</span>
        </div>
      ))}
    </div>
  );
}
