import { useCaptureStore } from "@/store/captureStore";
import { formatBytes } from "@/lib/utils";
import { Activity, WifiOff, Database } from "lucide-react";

export function StatusBar() {
  const { status, filteredFlows, flows, filter, interface_, sessionPath } =
    useCaptureStore();

  const totalBytes = flows.reduce((sum, f) => sum + f.length, 0);

  return (
    <div className="flex shrink-0 items-center gap-4 border-t border-white/10 bg-[#0a0a14] px-3 py-1 text-[11px] text-gray-400 font-mono">
      {/* Status indicator */}
      <div className="flex items-center gap-1.5">
        {status === "running" ? (
          <>
            <span className="relative flex h-2 w-2">
              <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-emerald-400 opacity-75" />
              <span className="relative inline-flex h-2 w-2 rounded-full bg-emerald-500" />
            </span>
            <span className="text-emerald-400">Capturing on {interface_}</span>
          </>
        ) : (
          <>
            <WifiOff className="h-3 w-3 text-gray-500" />
            <span>Idle</span>
          </>
        )}
      </div>

      <span className="text-white/20">|</span>

      {/* Flow count */}
      <div className="flex items-center gap-1">
        <Activity className="h-3 w-3" />
        <span>
          {filter
            ? `${filteredFlows.length} / ${flows.length} flows`
            : `${flows.length} flows`}
        </span>
      </div>

      <span className="text-white/20">|</span>

      {/* Total bytes */}
      <span>{formatBytes(totalBytes)}</span>

      {/* Session path */}
      {sessionPath && (
        <>
          <span className="text-white/20">|</span>
          <div className="flex items-center gap-1 text-gray-500">
            <Database className="h-3 w-3" />
            <span className="max-w-xs truncate">{sessionPath}</span>
          </div>
        </>
      )}
    </div>
  );
}
