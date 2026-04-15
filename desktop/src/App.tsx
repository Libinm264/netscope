import { useEffect, useRef, useState, useCallback } from "react";
import { invoke } from "@tauri-apps/api/core";
import { listen } from "@tauri-apps/api/event";
import { save as saveDialog, open as openDialog } from "@tauri-apps/plugin-dialog";

import { useCaptureStore } from "@/store/captureStore";
import { PacketListPane } from "@/components/PacketListPane";
import { PacketDetailPane } from "@/components/PacketDetailPane";
import { HexDumpPane } from "@/components/HexDumpPane";
import { FilterBar } from "@/components/FilterBar";
import { InterfaceSelector } from "@/components/InterfaceSelector";
import { StatusBar } from "@/components/StatusBar";
import { PrivilegeModal } from "@/components/PrivilegeModal";
import { Button } from "@/components/ui/button";
import type { FlowDto } from "@/types/flow";

import {
  Play,
  Square,
  Trash2,
  Save,
  FolderOpen,
  Radio,
} from "lucide-react";

// Resizable pane heights (top %, middle %, bottom %)
const DEFAULT_SPLIT = [50, 28, 22];

export default function App() {
  const {
    status,
    interface_,
    filter,
    privilegeGranted,
    sessionName,
    addFlow,
    setFlows,
    setStatus,
    clearFlows,
    setSessionPath,
    setPrivilegeGranted,
  } = useCaptureStore();

  const [showPrivModal, setShowPrivModal] = useState(false);
  const [split, setSplit] = useState(DEFAULT_SPLIT);
  const draggingRef = useRef<{ divider: number; startY: number; startSplit: number[] } | null>(null);

  // Check privileges on mount
  useEffect(() => {
    invoke<boolean>("check_privileges").then((ok) => {
      setPrivilegeGranted(ok);
    });
  }, [setPrivilegeGranted]);

  // Subscribe to flow events from Rust backend
  useEffect(() => {
    const unlisten = listen<FlowDto>("flow", (event) => {
      addFlow(event.payload);
    });
    const unlistenStop = listen("capture-stopped", () => {
      setStatus("idle");
    });
    return () => {
      unlisten.then((f) => f());
      unlistenStop.then((f) => f());
    };
  }, [addFlow, setStatus]);

  const handleStart = useCallback(async () => {
    if (!privilegeGranted) {
      setShowPrivModal(true);
      return;
    }
    clearFlows();
    try {
      await invoke("start_capture", {
        interface: interface_,
        filter: filter || null,
      });
      setStatus("running");
    } catch (e) {
      console.error("start_capture failed:", e);
      if (String(e).toLowerCase().includes("privilege")) {
        setPrivilegeGranted(false);
        setShowPrivModal(true);
      }
    }
  }, [privilegeGranted, interface_, filter, clearFlows, setStatus, setPrivilegeGranted]);

  const handleStop = useCallback(async () => {
    try {
      await invoke("stop_capture");
    } catch (_) {}
    setStatus("idle");
  }, [setStatus]);

  const handleSave = useCallback(async () => {
    const path = await saveDialog({
      defaultPath: `${sessionName}.nscope`,
      filters: [{ name: "NetScope Session", extensions: ["nscope"] }],
    });
    if (!path) return;
    await invoke("save_session", { path, name: sessionName });
    setSessionPath(path);
  }, [sessionName, setSessionPath]);

  const handleOpen = useCallback(async () => {
    const path = await openDialog({
      filters: [{ name: "NetScope Session", extensions: ["nscope"] }],
    });
    if (!path || Array.isArray(path)) return;
    const flows = await invoke<FlowDto[]>("load_session", { path });
    setFlows(flows);
    setSessionPath(path);
  }, [setFlows, setSessionPath]);

  // ── Resizable dividers ────────────────────────────────────────────────────
  const onMouseDown = useCallback(
    (divider: number) => (e: React.MouseEvent) => {
      e.preventDefault();
      draggingRef.current = { divider, startY: e.clientY, startSplit: [...split] };

      const onMove = (ev: MouseEvent) => {
        if (!draggingRef.current) return;
        const { divider: d, startY, startSplit } = draggingRef.current;
        const containerH = window.innerHeight - 60; // approx chrome height
        const deltaPct = ((ev.clientY - startY) / containerH) * 100;
        const next = [...startSplit];
        if (d === 0) {
          next[0] = Math.max(20, Math.min(70, startSplit[0] + deltaPct));
          next[1] = Math.max(10, startSplit[1] - deltaPct);
        } else {
          next[1] = Math.max(10, Math.min(60, startSplit[1] + deltaPct));
          next[2] = Math.max(10, startSplit[2] - deltaPct);
        }
        setSplit(next);
      };
      const onUp = () => {
        draggingRef.current = null;
        window.removeEventListener("mousemove", onMove);
        window.removeEventListener("mouseup", onUp);
      };
      window.addEventListener("mousemove", onMove);
      window.addEventListener("mouseup", onUp);
    },
    [split]
  );

  return (
    <div className="flex h-screen flex-col bg-[#0d0d1a] text-white overflow-hidden">
      {/* ── Toolbar ────────────────────────────────────────────────────────── */}
      <div className="flex shrink-0 items-center gap-2 border-b border-white/10 bg-[#0a0a14] px-3 py-1.5">
        {/* Logo */}
        <div className="flex items-center gap-1.5 mr-2">
          <Radio className="h-4 w-4 text-blue-400" />
          <span className="text-sm font-bold text-white tracking-tight">NetScope</span>
        </div>

        <InterfaceSelector />

        {status === "idle" ? (
          <Button size="sm" variant="success" onClick={handleStart} className="gap-1.5">
            <Play className="h-3.5 w-3.5" /> Start
          </Button>
        ) : (
          <Button size="sm" variant="destructive" onClick={handleStop} className="gap-1.5">
            <Square className="h-3.5 w-3.5" /> Stop
          </Button>
        )}

        <Button size="sm" variant="ghost" onClick={clearFlows} title="Clear flows">
          <Trash2 className="h-3.5 w-3.5" />
        </Button>

        <div className="h-4 w-px bg-white/20 mx-1" />

        <FilterBar />

        <div className="h-4 w-px bg-white/20 mx-1" />

        <Button size="sm" variant="ghost" onClick={handleSave} title="Save session">
          <Save className="h-3.5 w-3.5" />
        </Button>
        <Button size="sm" variant="ghost" onClick={handleOpen} title="Open session">
          <FolderOpen className="h-3.5 w-3.5" />
        </Button>
      </div>

      {/* ── Three-pane layout ──────────────────────────────────────────────── */}
      <div className="flex-1 flex flex-col overflow-hidden">
        {/* Packet list (top) */}
        <div
          style={{ height: `${split[0]}%` }}
          className="relative overflow-hidden border-b border-white/10"
        >
          <PacketListPane />
        </div>

        {/* Divider 0 */}
        <div
          className="h-1 shrink-0 cursor-row-resize bg-white/5 hover:bg-blue-500/40 transition-colors"
          onMouseDown={onMouseDown(0)}
        />

        {/* Packet detail (middle) */}
        <div
          style={{ height: `${split[1]}%` }}
          className="overflow-hidden border-b border-white/10"
        >
          <PacketDetailPane />
        </div>

        {/* Divider 1 */}
        <div
          className="h-1 shrink-0 cursor-row-resize bg-white/5 hover:bg-blue-500/40 transition-colors"
          onMouseDown={onMouseDown(1)}
        />

        {/* Hex dump (bottom) */}
        <div style={{ height: `${split[2]}%` }} className="overflow-hidden">
          <HexDumpPane />
        </div>
      </div>

      {/* ── Status bar ─────────────────────────────────────────────────────── */}
      <StatusBar />

      {/* ── Privilege modal ────────────────────────────────────────────────── */}
      {showPrivModal && (
        <PrivilegeModal
          interface_={interface_}
          onDismiss={() => setShowPrivModal(false)}
        />
      )}
    </div>
  );
}
