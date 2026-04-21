import { useEffect, useState } from "react";
import { invoke } from "@tauri-apps/api/core";
import {
  Radio, Wifi, Link2, Check, ChevronRight, Monitor, X, ShieldCheck,
} from "lucide-react";
import type { InterfaceDto } from "@/types/flow";
import { useCaptureStore } from "@/store/captureStore";

export const ONBOARDING_KEY = "netscope.onboarded";

interface Props {
  onDone: () => void;
}

const STEPS = [
  { label: "Welcome",        icon: Radio       },
  { label: "Interface",      icon: Wifi        },
  { label: "Hub (optional)", icon: Link2       },
] as const;

export function OnboardingWizard({ onDone }: Props) {
  const [step, setStep]               = useState(0);
  const [privOk, setPrivOk]           = useState<boolean | null>(null);
  const [checking, setChecking]       = useState(false);
  const [interfaces, setInterfaces]   = useState<InterfaceDto[]>([]);
  const [selected, setSelected]       = useState("");
  const [hubUrl, setHubUrl]           = useState("");
  const [hubToken, setHubToken]       = useState("");
  const [hubBusy, setHubBusy]         = useState(false);
  const [hubErr, setHubErr]           = useState<string | null>(null);

  const { setInterface } = useCaptureStore();

  const checkPriv = async () => {
    setChecking(true);
    try {
      const ok = await invoke<boolean>("check_privileges");
      setPrivOk(ok);
      if (ok) {
        const ifaces = await invoke<InterfaceDto[]>("list_interfaces");
        setInterfaces(ifaces);
        if (ifaces.length > 0) setSelected(ifaces[0].name);
      }
    } finally {
      setChecking(false);
    }
  };

  useEffect(() => { checkPriv(); }, []);

  const finish = () => {
    localStorage.setItem(ONBOARDING_KEY, "1");
    onDone();
  };

  const handleIfaceNext = () => {
    if (selected) setInterface(selected);
    setStep(2);
  };

  const handleHubNext = async () => {
    if (!hubUrl.trim()) { finish(); return; }
    setHubBusy(true);
    setHubErr(null);
    try {
      await invoke("set_hub_config", { url: hubUrl.trim(), token: hubToken.trim() });
      await invoke("test_hub_connection");
      finish();
    } catch (e) {
      setHubErr(e instanceof Error ? e.message : String(e));
    } finally {
      setHubBusy(false);
    }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/80 backdrop-blur-sm">
      <div className="w-full max-w-md bg-[#0d0d1a] border border-white/10 rounded-2xl shadow-2xl overflow-hidden">

        {/* Header */}
        <div className="px-8 pt-8 pb-6 border-b border-white/[0.06]">
          <div className="flex items-center gap-3 mb-5">
            <div className="p-2 rounded-xl bg-blue-500/10">
              <Radio size={20} className="text-blue-400" />
            </div>
            <div>
              <p className="text-sm font-bold text-white">Welcome to NetScope</p>
              <p className="text-[11px] text-slate-500">Quick setup — takes about 30 seconds</p>
            </div>
          </div>

          {/* Step pills */}
          <div className="flex items-center gap-1.5">
            {STEPS.map(({ label, icon: Icon }, i) => (
              <div key={i} className="flex items-center gap-1.5">
                <div className={`flex items-center gap-1.5 px-2.5 py-1 rounded-full text-[11px] font-medium border transition-colors ${
                  i === step
                    ? "bg-blue-500/15 text-blue-300 border-blue-500/30"
                    : i < step
                    ? "bg-emerald-500/10 text-emerald-400 border-emerald-500/20"
                    : "bg-transparent text-slate-600 border-white/[0.06]"
                }`}>
                  {i < step ? <Check size={9} /> : <Icon size={9} />}
                  {label}
                </div>
                {i < STEPS.length - 1 && (
                  <div className={`h-px w-3 shrink-0 ${i < step ? "bg-emerald-500/40" : "bg-white/10"}`} />
                )}
              </div>
            ))}
          </div>
        </div>

        {/* Body */}
        <div className="p-8">

          {/* ── Step 0: Privileges ────────────────────────────────── */}
          {step === 0 && (
            <div className="space-y-5">
              <p className="text-xs text-slate-400 leading-relaxed">
                NetScope captures raw network packets. It needs permission to read from your
                network interfaces.
              </p>

              <div className={`flex items-start gap-3 p-4 rounded-xl border transition-colors ${
                privOk === true  ? "border-emerald-500/30 bg-emerald-500/5"
                : privOk === false ? "border-red-500/25 bg-red-500/5"
                : "border-white/[0.06] bg-white/[0.02]"
              }`}>
                <div className={`mt-0.5 w-7 h-7 rounded-full flex items-center justify-center shrink-0 ${
                  privOk === true  ? "bg-emerald-500/20"
                  : privOk === false ? "bg-red-500/15"
                  : "bg-white/5"
                }`}>
                  {privOk === true  ? <Check size={13} className="text-emerald-400" />
                   : privOk === false ? <X size={13} className="text-red-400" />
                   : <Monitor size={13} className="text-slate-500" />}
                </div>
                <div>
                  <p className="text-sm font-medium text-white">Capture permission</p>
                  <p className="text-[11px] text-slate-500 mt-0.5">
                    {checking        ? "Checking…"
                     : privOk === true  ? "Ready — you can capture network traffic"
                     : privOk === false ? "Permission denied — run with elevated privileges"
                     : "Not yet checked"}
                  </p>
                </div>
              </div>

              {privOk === false && (
                <div className="p-3 rounded-lg bg-amber-500/5 border border-amber-500/20">
                  <p className="text-[11px] font-semibold text-amber-400 mb-1.5">Grant access (macOS)</p>
                  <code className="block text-[10px] text-slate-400 bg-black/30 rounded px-2.5 py-2 font-mono">
                    sudo chmod +r /dev/bpf*
                  </code>
                </div>
              )}

              <div className="flex gap-2 pt-1">
                <button
                  onClick={checkPriv}
                  disabled={checking}
                  className="flex-1 py-2.5 rounded-lg border border-white/10 text-sm text-slate-400 hover:text-white hover:bg-white/[0.04] transition-colors disabled:opacity-40"
                >
                  {checking ? "Checking…" : "Re-check"}
                </button>
                <button
                  onClick={() => setStep(1)}
                  className="flex-1 py-2.5 rounded-lg bg-blue-600 hover:bg-blue-500 text-sm font-semibold text-white transition-colors flex items-center justify-center gap-1.5"
                >
                  Continue <ChevronRight size={13} />
                </button>
              </div>
            </div>
          )}

          {/* ── Step 1: Interface ─────────────────────────────────── */}
          {step === 1 && (
            <div className="space-y-4">
              <p className="text-xs text-slate-400">Select the network interface to monitor.</p>

              <div className="space-y-1.5 max-h-52 overflow-y-auto pr-0.5">
                {interfaces.length === 0 ? (
                  <div className="flex flex-col items-center justify-center py-8 text-center gap-2">
                    <ShieldCheck size={20} className="text-slate-600" />
                    <p className="text-xs text-slate-600">No interfaces found — check capture permission</p>
                  </div>
                ) : (
                  interfaces.map((iface) => (
                    <button
                      key={iface.name}
                      onClick={() => setSelected(iface.name)}
                      className={`w-full flex items-center gap-3 px-3 py-2.5 rounded-lg text-left border transition-colors ${
                        selected === iface.name
                          ? "border-blue-500/40 bg-blue-500/10"
                          : "border-white/[0.06] bg-white/[0.02] hover:bg-white/[0.04]"
                      }`}
                    >
                      <Wifi size={13} className={selected === iface.name ? "text-blue-400" : "text-slate-600"} />
                      <div className="flex-1 min-w-0">
                        <p className="text-sm font-medium text-white truncate">{iface.name}</p>
                        {iface.description && (
                          <p className="text-[10px] text-slate-600 truncate">{iface.description}</p>
                        )}
                      </div>
                      {selected === iface.name && <Check size={11} className="text-blue-400 shrink-0" />}
                    </button>
                  ))
                )}
              </div>

              <div className="flex gap-2 pt-1">
                <button
                  onClick={() => setStep(0)}
                  className="flex-1 py-2.5 rounded-lg border border-white/10 text-sm text-slate-400 hover:text-white hover:bg-white/[0.04] transition-colors"
                >
                  Back
                </button>
                <button
                  onClick={handleIfaceNext}
                  disabled={!selected}
                  className="flex-1 py-2.5 rounded-lg bg-blue-600 hover:bg-blue-500 disabled:opacity-40 text-sm font-semibold text-white transition-colors flex items-center justify-center gap-1.5"
                >
                  Continue <ChevronRight size={13} />
                </button>
              </div>
            </div>
          )}

          {/* ── Step 2: Hub ───────────────────────────────────────── */}
          {step === 2 && (
            <div className="space-y-4">
              <p className="text-xs text-slate-400 leading-relaxed">
                Optionally connect to a NetScope Hub to sync flows, run compliance checks,
                and receive alerts. You can set this up later in the toolbar.
              </p>

              <div className="space-y-2">
                <input
                  value={hubUrl}
                  onChange={(e) => setHubUrl(e.target.value)}
                  placeholder="Hub URL — e.g. https://hub.example.com"
                  className="w-full bg-white/[0.04] border border-white/10 rounded-lg px-3 py-2.5
                             text-sm text-white placeholder:text-slate-600 focus:outline-none focus:border-blue-500/50"
                />
                <input
                  value={hubToken}
                  onChange={(e) => setHubToken(e.target.value)}
                  type="password"
                  placeholder="API key"
                  className="w-full bg-white/[0.04] border border-white/10 rounded-lg px-3 py-2.5
                             text-sm text-white placeholder:text-slate-600 focus:outline-none focus:border-blue-500/50"
                />
                {hubErr && (
                  <p className="text-[11px] text-red-400 bg-red-500/5 border border-red-500/20 rounded px-2 py-1.5">{hubErr}</p>
                )}
              </div>

              <div className="flex gap-2 pt-1">
                <button
                  onClick={finish}
                  className="flex-1 py-2.5 rounded-lg border border-white/10 text-sm text-slate-400 hover:text-white hover:bg-white/[0.04] transition-colors"
                >
                  Skip for now
                </button>
                <button
                  onClick={handleHubNext}
                  disabled={hubBusy}
                  className="flex-1 py-2.5 rounded-lg bg-blue-600 hover:bg-blue-500 disabled:opacity-40 text-sm font-semibold text-white transition-colors flex items-center justify-center gap-1.5"
                >
                  {hubBusy ? "Connecting…" : hubUrl.trim() ? "Connect & finish" : "Finish setup"}
                  {!hubBusy && <Check size={13} />}
                </button>
              </div>
            </div>
          )}

        </div>
      </div>
    </div>
  );
}
