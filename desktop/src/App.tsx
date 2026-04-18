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
import { GeoIpBanner } from "@/components/GeoIpBanner";
import { HubConnectModal } from "@/components/HubConnectModal";
import { CertSidebar } from "@/components/CertSidebar";
import { AnalyticsPane } from "@/components/AnalyticsPane";
import { ServiceMapPane } from "@/components/ServiceMapPane";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import type { FlowDto } from "@/types/flow";

import {
  Play,
  Square,
  Trash2,
  Save,
  FolderOpen,
  Radio,
  Link2,
  Shield,
  Binary,
  BarChart2,
  Network,
} from "lucide-react";

// Resizable pane heights (top %, middle %, bottom %)
const DEFAULT_SPLIT = [50, 28, 22];

// Bottom pane tab options
type BottomTab = "hex" | "analytics" | "servicemap";

export default function App() {
  const {
    status,
    interface_,
    filter,
    privilegeGranted,
    sessionName,
    hubConfig,
    hubConnected,
    certInventory,
    addFlow,
    setFlows,
    setStatus,
    clearFlows,
    setSessionPath,
    setPrivilegeGranted,
    setGeoipAvailable,
  } = useCaptureStore();

  const [showPrivModal, setShowPrivModal] = useState(false);
  const [showHubModal, setShowHubModal] = useState(false);
  const [showCertSidebar, setShowCertSidebar] = useState(false);
  const [bottomTab, setBottomTab] = useState<BottomTab>("hex");
  const [split, setSplit] = useState(DEFAULT_SPLIT);
  const draggingRef = useRef<{ divider: number; startY: number; startSplit: number[] } | null>(null);

  // Check privileges + GeoIP status on mount
  useEffect(() => {
    invoke<boolean>("check_privileges").then((ok) => setPrivilegeGranted(ok));
    invoke<boolean>("get_geoip_status").then((ok) => setGeoipAvailable(ok));
  }, [setPrivilegeGranted, setGeoipAvailable]);

  // Subscribe to flow events from Rust backend
  useEffect(() => {
    const unlisten = listen<FlowDto>("flow", (event) => addFlow(event.payload));
    const unlistenStop = listen("capture-stopped", () => setStatus("idle"));
    return () => {
      unlisten.then((f) => f());
      unlistenStop.then((f) => f());
    };
  }, [addFlow, setStatus]);

  const handleStart = useCallback(async () => {
    if (!privilegeGranted) { setShowPrivModal(true); return; }
    clearFlows();
    try {
      await invoke("start_capture", { interface: interface_, filter: filter || null });
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
    try { await invoke("stop_capture"); } catch (_) {}
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
        const containerH = window.innerHeight - 60;
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
    [split],
  );

  // Cert count for badge
  const criticalCerts = certInventory.filter(
    (c) => c.expired || (c.expiry && (new Date(c.expiry).getTime() - Date.now()) / 86_400_000 < 7),
  ).length;

  return (
    <div className="flex h-screen flex-col bg-[#0d0d1a] text-white overflow-hidden">

      {/* ── Toolbar ──────────────────────────────────────────────────────────── */}
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

        {/* Hub connect button */}
        <Button
          size="sm"
          variant="ghost"
          onClick={() => setShowHubModal(true)}
          title="Connect to Hub"
          className={cn("gap-1.5 text-xs", hubConnected && "text-blue-400")}
        >
          <Link2 className="h-3.5 w-3.5" />
          {hubConfig && <span className="hidden sm:inline">{hubConnected ? "Hub ●" : "Hub"}</span>}
        </Button>

        {/* Cert sidebar toggle */}
        <Button
          size="sm"
          variant="ghost"
          onClick={() => setShowCertSidebar((v) => !v)}
          title="TLS Certificates"
          className={cn(
            "relative gap-1.5 text-xs",
            showCertSidebar && "text-indigo-400",
          )}
        >
          <Shield className="h-3.5 w-3.5" />
          {criticalCerts > 0 && (
            <span className="absolute -top-0.5 -right-0.5 h-3.5 w-3.5 rounded-full bg-red-500 text-[8px] flex items-center justify-center font-bold">
              {criticalCerts}
            </span>
          )}
        </Button>

        <div className="h-4 w-px bg-white/20 mx-1" />

        <Button size="sm" variant="ghost" onClick={handleSave} title="Save session">
          <Save className="h-3.5 w-3.5" />
        </Button>
        <Button size="sm" variant="ghost" onClick={handleOpen} title="Open session">
          <FolderOpen className="h-3.5 w-3.5" />
        </Button>
      </div>

      {/* ── GeoIP banner ─────────────────────────────────────────────────────── */}
      <GeoIpBanner />

      {/* ── Main layout ──────────────────────────────────────────────────────── */}
      <div className="flex flex-1 overflow-hidden">
        {/* Three-pane column */}
        <div className="flex flex-1 flex-col overflow-hidden">

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

          {/* Bottom pane with tab switcher */}
          <div style={{ height: `${split[2]}%` }} className="flex flex-col overflow-hidden">
            {/* Tab bar */}
            <div className="flex shrink-0 items-center gap-0.5 border-b border-white/10 bg-[#0a0a14] px-2">
              {(
                [
                  { id: "hex",        label: "Hex Dump",    Icon: Binary     },
                  { id: "analytics",  label: "Analytics",   Icon: BarChart2  },
                  { id: "servicemap", label: "Service Map", Icon: Network    },
                ] as { id: BottomTab; label: string; Icon: typeof Binary }[]
              ).map(({ id, label, Icon }) => (
                <button
                  key={id}
                  onClick={() => setBottomTab(id)}
                  className={cn(
                    "flex items-center gap-1.5 px-3 py-1.5 text-[10px] font-semibold transition-colors",
                    bottomTab === id
                      ? "border-b-2 border-blue-400 text-blue-400"
                      : "text-gray-500 hover:text-gray-300",
                  )}
                >
                  <Icon className="h-3 w-3" />
                  {label}
                </button>
              ))}
            </div>

            {/* Tab content */}
            <div className="flex-1 overflow-hidden">
              {bottomTab === "hex"        && <HexDumpPane />}
              {bottomTab === "analytics"  && <AnalyticsPane />}
              {bottomTab === "servicemap" && <ServiceMapPane />}
            </div>
          </div>
        </div>

        {/* Cert sidebar (right panel) */}
        {showCertSidebar && (
          <CertSidebar onClose={() => setShowCertSidebar(false)} />
        )}
      </div>

      {/* ── Status bar ───────────────────────────────────────────────────────── */}
      <StatusBar />

      {/* ── Modals ───────────────────────────────────────────────────────────── */}
      {showPrivModal && (
        <PrivilegeModal
          interface_={interface_}
          onDismiss={() => setShowPrivModal(false)}
        />
      )}
      {showHubModal && (
        <HubConnectModal onClose={() => setShowHubModal(false)} />
      )}
    </div>
  );
}
