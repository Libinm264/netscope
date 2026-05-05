"use client";

import { useCallback, useEffect, useState } from "react";
import Link from "next/link";
import { Server, Plus, X, Copy, Check, RefreshCw, Cpu, Globe2, Box, Activity, Play } from "lucide-react";
import { clsx } from "clsx";
import { fetchAgents, fetchAgentPerf, fetchAgentSampling, pushAgentSampling, createEnrollmentToken } from "@/lib/api";
import type { Agent, PerfSample, SamplingMode, EnrollmentToken } from "@/lib/api";

// ── Status helpers ─────────────────────────────────────────────────────────────

function onlineStatus(lastSeen: string): "online" | "idle" | "offline" {
  const mins = (Date.now() - new Date(lastSeen).getTime()) / 60_000;
  if (mins < 5)  return "online";
  if (mins < 30) return "idle";
  return "offline";
}

const STATUS = {
  online:  { dot: "bg-emerald-400", label: "Online",  text: "text-emerald-400" },
  idle:    { dot: "bg-amber-400",   label: "Idle",    text: "text-amber-400"   },
  offline: { dot: "bg-slate-600",   label: "Offline", text: "text-slate-500"   },
};

function timeAgo(iso: string) {
  const secs = Math.floor((Date.now() - new Date(iso).getTime()) / 1000);
  if (secs < 60)  return `${secs}s ago`;
  const mins = Math.floor(secs / 60);
  if (mins < 60)  return `${mins}m ago`;
  const hrs = Math.floor(mins / 60);
  if (hrs < 24)   return `${hrs}h ago`;
  return `${Math.floor(hrs / 24)}d ago`;
}

// ── OS icon / label ────────────────────────────────────────────────────────────

function OsBadge({ os }: { os?: string }) {
  if (!os) return null;
  const label =
    os === "linux"   ? "Linux" :
    os === "macos"   ? "macOS" :
    os === "windows" ? "Windows" : os;
  return (
    <span className="inline-flex items-center gap-1 px-1.5 py-0.5 rounded text-[10px]
                     bg-slate-700/40 border border-white/[0.06] text-slate-400">
      <Globe2 size={9} />
      {label}
    </span>
  );
}

// ── Capture mode badge ─────────────────────────────────────────────────────────

function CaptureBadge({ mode, ebpf }: { mode?: string; ebpf?: boolean }) {
  const isEbpf = ebpf || mode === "ebpf";
  return (
    <span className={clsx(
      "inline-flex items-center gap-1 px-1.5 py-0.5 rounded text-[10px] border font-medium",
      isEbpf
        ? "bg-emerald-500/10 border-emerald-500/25 text-emerald-400"
        : "bg-slate-700/30 border-white/[0.06] text-slate-500",
    )}>
      <Cpu size={9} />
      {isEbpf ? "eBPF" : "pcap"}
    </span>
  );
}

// ── Copy button ────────────────────────────────────────────────────────────────

function CopyButton({ text, className }: { text: string; className?: string }) {
  const [copied, setCopied] = useState(false);
  return (
    <button
      onClick={() => {
        navigator.clipboard.writeText(text);
        setCopied(true);
        setTimeout(() => setCopied(false), 2000);
      }}
      className={clsx("p-1 rounded transition-colors",
        className ?? "hover:bg-white/10 text-slate-500 hover:text-slate-300")}
    >
      {copied ? <Check size={12} className="text-emerald-400" /> : <Copy size={12} />}
    </button>
  );
}

// ── CPU sparkline ──────────────────────────────────────────────────────────────

function Sparkline({ cpus, color }: { cpus: number[]; color: string }) {
  if (cpus.length < 2) return null;
  const W = 56, H = 14;
  const max = Math.max(...cpus, 1);
  const pts = cpus
    .map((v, i) => {
      const x = ((i / (cpus.length - 1)) * W).toFixed(1);
      const y = (H - (v / max) * H).toFixed(1);
      return `${x},${y}`;
    })
    .join(" ");

  return (
    <svg width={W} height={H} className="overflow-visible shrink-0">
      <polyline
        points={pts}
        fill="none"
        stroke={color}
        strokeWidth="1.5"
        strokeLinejoin="round"
        strokeLinecap="round"
        opacity={0.85}
      />
    </svg>
  );
}

// ── Performance bar (sparkline + latest CPU + mem) ────────────────────────────

function PerfBar({ agentId }: { agentId: string }) {
  const [samples, setSamples] = useState<PerfSample[]>([]);

  useEffect(() => {
    let cancelled = false;
    const load = async () => {
      try {
        const data = await fetchAgentPerf(agentId, 10);
        if (!cancelled) setSamples(data.samples ?? []);
      } catch {
        // no perf data yet — older agent or first heartbeat
      }
    };
    load();
    const id = setInterval(load, 30_000);
    return () => {
      cancelled = true;
      clearInterval(id);
    };
  }, [agentId]);

  if (samples.length === 0) return null;

  // API returns samples DESC (newest first); reverse for sparkline (oldest→newest left→right)
  const latest = samples[0];
  const cpuPct = latest.cpu_pct ?? 0;
  const memMB  = latest.mem_mb  ?? 0;
  const dropped = latest.packets_dropped ?? 0;

  const cpuColor =
    cpuPct > 80 ? "#f87171" :
    cpuPct > 50 ? "#fbbf24" : "#818cf8";

  const cpuClass =
    cpuPct > 80 ? "text-red-400" :
    cpuPct > 50 ? "text-amber-400" : "text-indigo-400";

  // Build sparkline series oldest→newest
  const sparkCpus = [...samples].reverse().map(s => s.cpu_pct ?? 0);

  return (
    <div className="flex items-center gap-2.5 pt-2 mt-1 border-t border-white/[0.04]">
      <Activity size={10} className="text-slate-600 shrink-0" />
      <Sparkline cpus={sparkCpus} color={cpuColor} />

      {/* CPU % */}
      <div className="text-[10px] tabular-nums leading-none">
        <span className={clsx("font-semibold", cpuClass)}>
          {cpuPct.toFixed(1)}%
        </span>
        <span className="text-slate-600 ml-0.5">cpu</span>
      </div>

      {/* Divider */}
      <span className="text-slate-700 text-[10px]">·</span>

      {/* Memory */}
      <div className="text-[10px] tabular-nums leading-none text-slate-500">
        <span className="text-slate-400">
          {memMB < 1024
            ? `${memMB}`
            : `${(memMB / 1024).toFixed(1)}G`}
        </span>
        <span className="ml-0.5">{memMB < 1024 ? "MB" : "GB"}</span>
      </div>

      {/* Drop counter — only when non-zero */}
      {dropped > 0 && (
        <>
          <span className="text-slate-700 text-[10px]">·</span>
          <span className="text-[10px] text-amber-500 tabular-nums">
            ⚠ {dropped.toLocaleString()} drops
          </span>
        </>
      )}
    </div>
  );
}

// ── Sampling mode toggle ───────────────────────────────────────────────────────
//
// Shows the current sampling mode and lets an admin switch between
// "metadata" (default — no body content) and "full" (body previews included).
// The change is pushed to the Hub and the agent picks it up within 30 s.

function SamplingToggle({ agentId }: { agentId: string }) {
  const [mode, setMode]       = useState<SamplingMode | null>(null);
  const [pushing, setPushing] = useState(false);

  useEffect(() => {
    let cancelled = false;
    fetchAgentSampling(agentId)
      .then(d => { if (!cancelled) setMode(d.mode); })
      .catch(() => { if (!cancelled) setMode("metadata"); });
    return () => { cancelled = true; };
  }, [agentId]);

  const toggle = async () => {
    if (pushing || mode === null) return;
    const next: SamplingMode = mode === "metadata" ? "full" : "metadata";
    setPushing(true);
    try {
      await pushAgentSampling(agentId, next);
      setMode(next);
    } catch {
      // silent — mode stays unchanged
    } finally {
      setPushing(false);
    }
  };

  if (mode === null) return null;

  const isFull = mode === "full";

  return (
    <div className="flex items-center justify-between pt-2 mt-1 border-t border-white/[0.04]">
      <div className="flex items-center gap-1.5 text-[10px]">
        <span className={clsx(
          "font-medium",
          isFull ? "text-amber-400" : "text-slate-500",
        )}>
          {isFull ? "Full capture" : "Metadata only"}
        </span>
        {isFull && (
          <span className="px-1 py-0.5 rounded text-[9px] bg-amber-500/10 border border-amber-500/20 text-amber-500">
            bodies included
          </span>
        )}
      </div>
      <button
        onClick={toggle}
        disabled={pushing}
        title={isFull ? "Switch to metadata-only mode" : "Enable full body capture"}
        className={clsx(
          "relative inline-flex h-4 w-7 shrink-0 cursor-pointer items-center rounded-full",
          "transition-colors duration-200 focus:outline-none disabled:opacity-50",
          isFull ? "bg-amber-500/70" : "bg-slate-700",
        )}
      >
        <span className={clsx(
          "inline-block h-3 w-3 rounded-full bg-white shadow transition-transform duration-200",
          isFull ? "translate-x-3.5" : "translate-x-0.5",
        )} />
      </button>
    </div>
  );
}

// ── Add Agent Modal ────────────────────────────────────────────────────────────

function AddAgentModal({ onClose }: { onClose: () => void }) {
  const [step, setStep]     = useState<"form" | "token">("form");
  const [name, setName]     = useState("");
  const [expiry, setExpiry] = useState("7d");
  const [iface, setIface]   = useState("en0");
  const [loading, setLoading] = useState(false);
  const [token, setToken]   = useState<EnrollmentToken | null>(null);

  const hubURL = typeof window !== "undefined"
    ? window.location.origin.replace(":3000", ":8080")
    : "http://localhost:8080";

  const generate = async () => {
    if (!name.trim()) return;
    setLoading(true);
    try {
      const t = await createEnrollmentToken(name.trim(), expiry);
      setToken(t);
      setStep("token");
    } catch (e) {
      alert(e instanceof Error ? e.message : "Failed to generate token");
    } finally {
      setLoading(false);
    }
  };

  const installCmd  = token ? `curl -sSL "${hubURL}/install?token=${token.token}" | INTERFACE=${iface} sh` : "";
  const manualCmd   = token
    ? `netscope-agent capture \\\n  --interface ${iface} \\\n  --output hub \\\n  --hub-url ${hubURL} \\\n  --api-key <your-api-key>`
    : "";
  const ebpfCmd     = token
    ? `# Linux eBPF mode — process attribution + TLS plaintext\nsudo netscope-agent-ebpf \\\n  --hub-url ${hubURL} \\\n  --api-key <your-api-key>`
    : "";

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/60 backdrop-blur-sm">
      <div className="w-full max-w-lg bg-[#0d0d1a] border border-white/10 rounded-xl shadow-2xl">
        <div className="flex items-center justify-between px-5 py-4 border-b border-white/[0.06]">
          <div className="flex items-center gap-2.5">
            <div className="p-1.5 rounded-md bg-indigo-500/10">
              <Server size={15} className="text-indigo-400" />
            </div>
            <p className="text-sm font-semibold text-white">Add Agent</p>
          </div>
          <button onClick={onClose} className="p-1 rounded hover:bg-white/10 text-slate-500 hover:text-slate-300">
            <X size={16} />
          </button>
        </div>

        <div className="p-5 space-y-4">
          {step === "form" ? (
            <>
              <p className="text-xs text-slate-400">
                Generate a short-lived enrollment token. Use it with the one-line install
                command to register a new agent without exposing your admin API key.
              </p>
              <div className="space-y-3">
                <div>
                  <label className="block text-xs text-slate-400 mb-1">Agent label</label>
                  <input autoFocus value={name} onChange={(e) => setName(e.target.value)}
                    onKeyDown={(e) => e.key === "Enter" && generate()}
                    placeholder="prod-web-01"
                    className="w-full bg-white/[0.04] border border-white/10 rounded-md px-3 py-2
                               text-sm text-white placeholder:text-slate-600
                               focus:outline-none focus:border-indigo-500/50" />
                </div>
                <div className="grid grid-cols-2 gap-3">
                  <div>
                    <label className="block text-xs text-slate-400 mb-1">Network interface</label>
                    <input value={iface} onChange={(e) => setIface(e.target.value)} placeholder="en0"
                      className="w-full bg-white/[0.04] border border-white/10 rounded-md px-3 py-2
                                 text-sm text-white placeholder:text-slate-600 focus:outline-none focus:border-indigo-500/50" />
                  </div>
                  <div>
                    <label className="block text-xs text-slate-400 mb-1">Token expires in</label>
                    <select value={expiry} onChange={(e) => setExpiry(e.target.value)}
                      className="w-full bg-white/[0.04] border border-white/10 rounded-md px-3 py-2
                                 text-sm text-slate-300 focus:outline-none">
                      <option value="24h">24 hours</option>
                      <option value="7d">7 days</option>
                      <option value="30d">30 days</option>
                    </select>
                  </div>
                </div>
              </div>
              <button onClick={generate} disabled={!name.trim() || loading}
                className="w-full py-2 rounded-md text-sm font-medium bg-indigo-600 hover:bg-indigo-500
                           text-white disabled:opacity-40 transition-colors">
                {loading ? "Generating…" : "Generate install command"}
              </button>
            </>
          ) : (
            <>
              <p className="text-xs text-slate-400">
                Run one of the commands below on the target machine. The agent will enroll
                and start capturing immediately.
              </p>

              {/* pcap one-liner */}
              <div>
                <p className="text-xs font-medium text-slate-300 mb-1.5">
                  One-line install (pcap mode — all platforms)
                </p>
                <div className="relative bg-black/40 border border-white/10 rounded-md px-3 py-2.5 pr-10">
                  <code className="text-[11px] text-emerald-300 break-all">{installCmd}</code>
                  <div className="absolute top-2 right-2"><CopyButton text={installCmd} /></div>
                </div>
              </div>

              {/* eBPF one-liner */}
              <div>
                <p className="text-xs font-medium text-slate-300 mb-1.5 flex items-center gap-1.5">
                  <Cpu size={11} className="text-emerald-400" />
                  eBPF mode (Linux only — adds process attribution + TLS plaintext)
                </p>
                <div className="relative bg-black/40 border border-emerald-500/20 rounded-md px-3 py-2.5 pr-10">
                  <pre className="text-[11px] text-emerald-300 whitespace-pre-wrap">{ebpfCmd}</pre>
                  <div className="absolute top-2 right-2"><CopyButton text={ebpfCmd} /></div>
                </div>
              </div>

              {/* Manual */}
              <div>
                <p className="text-xs font-medium text-slate-300 mb-1.5">Or start manually</p>
                <div className="relative bg-black/40 border border-white/10 rounded-md px-3 py-2.5 pr-10">
                  <pre className="text-[11px] text-slate-400 whitespace-pre-wrap">{manualCmd}</pre>
                  <div className="absolute top-2 right-2"><CopyButton text={manualCmd} /></div>
                </div>
              </div>

              <div className="flex items-center gap-2 text-xs text-slate-500">
                <span className="inline-block w-1.5 h-1.5 rounded-full bg-amber-400" />
                Token expires {new Date(token!.expires_at).toLocaleDateString()} · valid for one agent
              </div>

              <div className="flex gap-2 pt-1">
                <button onClick={() => setStep("form")}
                  className="flex-1 py-2 rounded-md text-xs border border-white/10 text-slate-400
                             hover:text-slate-200 hover:bg-white/[0.04]">
                  Generate another
                </button>
                <button onClick={onClose}
                  className="flex-1 py-2 rounded-md text-xs bg-indigo-600 hover:bg-indigo-500 text-white">
                  Done
                </button>
              </div>
            </>
          )}
        </div>
      </div>
    </div>
  );
}

// ── Page ───────────────────────────────────────────────────────────────────────

export default function AgentsPage() {
  const [agents, setAgents]   = useState<Agent[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError]     = useState<string | null>(null);
  const [showModal, setShowModal] = useState(false);

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try { setAgents((await fetchAgents()).agents ?? []); }
    catch (e) { setError(e instanceof Error ? e.message : String(e)); }
    finally { setLoading(false); }
  }, []);

  useEffect(() => {
    load();
    const id = setInterval(load, 30_000);
    return () => clearInterval(id);
  }, [load]);

  const onlineCount  = agents.filter(a => onlineStatus(a.last_seen) === "online").length;
  const ebpfCount    = agents.filter(a => a.ebpf_enabled || a.capture_mode === "ebpf").length;

  return (
    <div className="p-6 space-y-5">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-lg font-semibold text-white">Agent Fleet</h1>
          <p className="text-xs text-slate-500 mt-0.5">
            {onlineCount} online · {agents.length} total · {ebpfCount} in eBPF mode
          </p>
        </div>
        <div className="flex items-center gap-2">
          <button onClick={load}
            className="p-2 rounded-md border border-white/10 text-slate-500 hover:text-slate-300
                       hover:bg-white/[0.04] transition-colors">
            <RefreshCw size={13} className={loading ? "animate-spin" : ""} />
          </button>
          <button onClick={() => setShowModal(true)}
            className="flex items-center gap-1.5 px-3 py-2 rounded-md text-xs
                       bg-indigo-600 hover:bg-indigo-500 text-white transition-colors">
            <Plus size={13} /> Add Agent
          </button>
        </div>
      </div>

      {/* eBPF mode callout — shown when no agents are in eBPF mode */}
      {agents.length > 0 && ebpfCount === 0 && (
        <div className="flex items-start gap-3 px-4 py-3 rounded-lg border border-indigo-500/20 bg-indigo-500/[0.05] text-xs text-slate-400">
          <Cpu size={14} className="text-indigo-400 shrink-0 mt-0.5" />
          <div>
            <span className="text-white font-medium">All agents in pcap mode.</span>{" "}
            Switch to the <span className="text-indigo-400 font-medium">eBPF-enabled binary</span> on
            Linux to unlock process attribution (which process owns each connection), TLS plaintext
            capture, and Kubernetes pod enrichment. Click{" "}
            <button onClick={() => setShowModal(true)} className="text-indigo-400 underline underline-offset-2">
              Add Agent
            </button>{" "}
            to see the eBPF install command.
          </div>
        </div>
      )}

      {/* API error banner */}
      {error && (
        <div className="flex items-start gap-3 px-4 py-3 rounded-lg border border-red-500/30
                        bg-red-500/[0.07] text-xs text-red-400">
          <span className="font-medium shrink-0">Failed to load agents:</span>
          <span className="font-mono break-all">{error}</span>
        </div>
      )}

      {/* Grid */}
      {loading && agents.length === 0 ? (
        <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-4">
          {Array.from({ length: 3 }).map((_, i) => (
            <div key={i} className="rounded-xl bg-[#0d0d1a] border border-white/[0.06] p-4 space-y-3">
              <div className="h-4 w-32 rounded bg-white/5 animate-pulse" />
              <div className="h-3 w-48 rounded bg-white/5 animate-pulse" />
            </div>
          ))}
        </div>
      ) : agents.length === 0 ? (
        <div className="flex flex-col items-center justify-center py-20 text-center gap-3">
          <div className="p-4 rounded-full bg-slate-800/50">
            <Server size={28} className="text-slate-600" />
          </div>
          <p className="text-sm text-slate-400">No agents enrolled yet</p>
          <p className="text-xs text-slate-600">
            Click <strong className="text-slate-400">Add Agent</strong> to get a one-line install command
          </p>
          <button onClick={() => setShowModal(true)}
            className="mt-1 flex items-center gap-1.5 px-4 py-2 rounded-md text-sm
                       bg-indigo-600 hover:bg-indigo-500 text-white transition-colors">
            <Plus size={14} /> Add your first agent
          </button>
        </div>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-4">
          {agents.map((agent) => {
            const status = onlineStatus(agent.last_seen);
            const s = STATUS[status];
            const isEbpf = agent.ebpf_enabled || agent.capture_mode === "ebpf";

            return (
              <div key={agent.agent_id}
                className={clsx(
                  "rounded-xl bg-[#0d0d1a] border p-4 space-y-3 transition-colors",
                  isEbpf
                    ? "border-emerald-500/20 hover:border-emerald-500/30"
                    : "border-white/[0.06] hover:border-white/10",
                )}>
                {/* Card header */}
                <div className="flex items-start justify-between gap-2">
                  <div className="flex items-center gap-2.5 min-w-0">
                    <div className={clsx(
                      "p-1.5 rounded-md shrink-0",
                      isEbpf ? "bg-emerald-500/10" : "bg-indigo-500/10",
                    )}>
                      <Server size={14} className={isEbpf ? "text-emerald-400" : "text-indigo-400"} />
                    </div>
                    <p className="font-semibold text-white text-sm truncate">{agent.hostname}</p>
                  </div>
                  <div className="flex items-center gap-1.5 shrink-0">
                    <span className={clsx("w-1.5 h-1.5 rounded-full", s.dot)} />
                    <span className={clsx("text-xs", s.text)}>{s.label}</span>
                  </div>
                </div>

                {/* Badges */}
                <div className="flex flex-wrap gap-1.5">
                  <CaptureBadge mode={agent.capture_mode} ebpf={agent.ebpf_enabled} />
                  <OsBadge os={agent.os} />
                  {agent.flow_count_1h !== undefined && agent.flow_count_1h > 0 && (
                    <span className="inline-flex items-center gap-1 px-1.5 py-0.5 rounded text-[10px]
                                     bg-slate-700/40 border border-white/[0.06] text-slate-400">
                      <Box size={9} />
                      {agent.flow_count_1h.toLocaleString()} flows/1h
                    </span>
                  )}
                </div>

                {/* Detail rows */}
                <div className="space-y-1 font-mono text-xs">
                  {[
                    ["ID",        agent.agent_id.slice(0, 8) + "…"],
                    ["Interface", agent.interface],
                    ["Version",   agent.version],
                    ["Last seen", timeAgo(agent.last_seen)],
                  ].filter(([, v]) => v).map(([k, v]) => (
                    <div key={k} className="flex gap-2 text-slate-600">
                      <span className="w-16 shrink-0">{k}</span>
                      <span className="text-slate-400">{v}</span>
                    </div>
                  ))}
                </div>

                {/* Performance sparkline — only renders if perf data available */}
                <PerfBar agentId={agent.agent_id} />

                {/* Sampling mode toggle */}
                <SamplingToggle agentId={agent.agent_id} />

                {/* Replay timeline link */}
                <div className="pt-2 mt-1 border-t border-white/[0.04]">
                  <Link
                    href={`/replay?agent_id=${encodeURIComponent(agent.agent_id)}&around=${encodeURIComponent(agent.last_seen)}&hostname=${encodeURIComponent(agent.hostname)}&window_mins=5`}
                    className="inline-flex items-center gap-1.5 text-[10px] font-medium
                               text-indigo-400 hover:text-indigo-300 transition-colors"
                  >
                    <Play size={9} />
                    View recent timeline
                  </Link>
                </div>
              </div>
            );
          })}
        </div>
      )}

      {showModal && <AddAgentModal onClose={() => { setShowModal(false); load(); }} />}
    </div>
  );
}
