"use client";

import { useState, useEffect, useCallback } from "react";
import {
  fetchPolicies, createPolicy, updatePolicy, deletePolicy,
  fetchPolicyViolations,
  type ProcessPolicy, type PolicyViolation, type CreatePolicyRequest,
} from "@/lib/api";
import { ShieldAlert, Plus, X, Trash2, RefreshCw, AlertTriangle } from "lucide-react";
import { clsx } from "clsx";

const WINDOWS = ["1h", "6h", "24h", "7d"] as const;

// ── Create Policy Modal ────────────────────────────────────────────────────────

function CreatePolicyModal({ onClose, onCreated }: { onClose: () => void; onCreated: () => void }) {
  const [name, setName] = useState("");
  const [processName, setProcessName] = useState("");
  const [action, setAction] = useState<"alert" | "deny">("alert");
  const [dstCIDR, setDstCIDR] = useState("");
  const [dstPort, setDstPort] = useState("");
  const [description, setDescription] = useState("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");

  const submit = async () => {
    if (!name.trim() || !processName.trim()) {
      setError("Name and process name are required");
      return;
    }
    setLoading(true);
    setError("");
    try {
      const req: CreatePolicyRequest = {
        name: name.trim(),
        process_name: processName.trim(),
        action,
        dst_ip_cidr: dstCIDR.trim() || undefined,
        dst_port: dstPort ? parseInt(dstPort) : undefined,
        description: description.trim() || undefined,
      };
      await createPolicy(req);
      onCreated();
      onClose();
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to create policy");
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/60 backdrop-blur-sm">
      <div className="w-full max-w-lg bg-[#0d0d1a] border border-white/10 rounded-xl shadow-2xl">
        <div className="flex items-center justify-between px-5 py-4 border-b border-white/[0.06]">
          <div className="flex items-center gap-2.5">
            <div className="p-1.5 rounded-md bg-indigo-500/10">
              <ShieldAlert size={15} className="text-indigo-400" />
            </div>
            <p className="text-sm font-semibold text-white">New Process Policy</p>
          </div>
          <button onClick={onClose} className="p-1 rounded hover:bg-white/10 text-slate-500 hover:text-slate-300">
            <X size={16} />
          </button>
        </div>
        <div className="p-5 space-y-4">
          <p className="text-xs text-slate-400">
            Define which processes are allowed or should trigger alerts when they make network connections.
            Use <span className="font-mono text-slate-300">*</span> as a wildcard for process name.
          </p>
          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className="block text-xs text-slate-400 mb-1">Rule name</label>
              <input value={name} onChange={e => setName(e.target.value)}
                placeholder="block-curl-external"
                className="w-full bg-white/[0.04] border border-white/10 rounded-md px-3 py-2 text-sm text-white placeholder:text-slate-600 focus:outline-none focus:border-indigo-500/50" />
            </div>
            <div>
              <label className="block text-xs text-slate-400 mb-1">Process name</label>
              <input value={processName} onChange={e => setProcessName(e.target.value)}
                placeholder="curl"
                className="w-full bg-white/[0.04] border border-white/10 rounded-md px-3 py-2 text-sm text-white placeholder:text-slate-600 focus:outline-none focus:border-indigo-500/50" />
            </div>
          </div>
          <div className="grid grid-cols-3 gap-3">
            <div>
              <label className="block text-xs text-slate-400 mb-1">Action</label>
              <select value={action} onChange={e => setAction(e.target.value as "alert" | "deny")}
                className="w-full bg-white/[0.04] border border-white/10 rounded-md px-3 py-2 text-sm text-slate-300 focus:outline-none">
                <option value="alert">Alert</option>
                <option value="deny">Deny</option>
              </select>
            </div>
            <div>
              <label className="block text-xs text-slate-400 mb-1">Dest CIDR (optional)</label>
              <input value={dstCIDR} onChange={e => setDstCIDR(e.target.value)}
                placeholder="0.0.0.0/0"
                className="w-full bg-white/[0.04] border border-white/10 rounded-md px-3 py-2 text-sm text-white placeholder:text-slate-600 focus:outline-none focus:border-indigo-500/50" />
            </div>
            <div>
              <label className="block text-xs text-slate-400 mb-1">Dest port (optional)</label>
              <input value={dstPort} onChange={e => setDstPort(e.target.value)}
                placeholder="443"
                type="number"
                className="w-full bg-white/[0.04] border border-white/10 rounded-md px-3 py-2 text-sm text-white placeholder:text-slate-600 focus:outline-none focus:border-indigo-500/50" />
            </div>
          </div>
          <div>
            <label className="block text-xs text-slate-400 mb-1">Description (optional)</label>
            <input value={description} onChange={e => setDescription(e.target.value)}
              placeholder="Matches curl making any outbound connection"
              className="w-full bg-white/[0.04] border border-white/10 rounded-md px-3 py-2 text-sm text-white placeholder:text-slate-600 focus:outline-none focus:border-indigo-500/50" />
          </div>
          {error && <p className="text-xs text-red-400">{error}</p>}
          <button onClick={submit} disabled={loading || !name.trim() || !processName.trim()}
            className="w-full py-2 rounded-md text-sm font-medium bg-indigo-600 hover:bg-indigo-500 text-white disabled:opacity-40 transition-colors">
            {loading ? "Creating…" : "Create policy"}
          </button>
        </div>
      </div>
    </div>
  );
}

// ── Page ───────────────────────────────────────────────────────────────────────

export default function PoliciesPage() {
  const [policies, setPolicies] = useState<ProcessPolicy[]>([]);
  const [violations, setViolations] = useState<PolicyViolation[]>([]);
  const [tab, setTab] = useState<"rules" | "violations">("rules");
  const [window, setWindow] = useState("24h");
  const [showModal, setShowModal] = useState(false);
  const [loading, setLoading] = useState(false);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const [p, v] = await Promise.all([
        fetchPolicies(),
        fetchPolicyViolations({ window }),
      ]);
      setPolicies(p.policies ?? []);
      setViolations(v.violations ?? []);
    } catch (e) { console.error(e); }
    finally { setLoading(false); }
  }, [window]);

  useEffect(() => { load(); }, [load]);

  const toggle = async (p: ProcessPolicy) => {
    await updatePolicy(p.id, { enabled: !p.enabled });
    load();
  };
  const remove = async (id: string) => {
    if (!confirm("Delete this policy?")) return;
    await deletePolicy(id);
    load();
  };

  return (
    <div className="p-6 space-y-5">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-lg font-semibold text-white">Process Network Policies</h1>
          <p className="text-xs text-slate-500 mt-0.5">
            Define which processes may make network connections — powered by eBPF attribution
          </p>
        </div>
        <div className="flex items-center gap-2">
          <button onClick={load}
            className="p-2 rounded-md border border-white/10 text-slate-500 hover:text-slate-300 hover:bg-white/[0.04] transition-colors">
            <RefreshCw size={13} className={loading ? "animate-spin" : ""} />
          </button>
          <button onClick={() => setShowModal(true)}
            className="flex items-center gap-1.5 px-3 py-2 rounded-md text-xs bg-indigo-600 hover:bg-indigo-500 text-white transition-colors">
            <Plus size={13} /> New Policy
          </button>
        </div>
      </div>

      {/* What is this callout */}
      <div className="rounded-lg border border-indigo-500/20 bg-indigo-500/[0.05] px-4 py-3 text-xs text-slate-400 space-y-1">
        <p className="text-white font-medium flex items-center gap-1.5">
          <ShieldAlert size={13} className="text-indigo-400" />
          What are process policies?
        </p>
        <p>
          When running in <span className="text-emerald-400 font-medium">eBPF mode</span>, NetScope knows
          exactly which process made each connection. Policies let you define rules like
          &quot;<span className="text-slate-300 font-mono">curl</span> must never connect outside your network&quot;
          or &quot;<span className="text-slate-300 font-mono">python3</span> should not reach port 22&quot;.
          Violations are logged and can trigger alerts. Requires the{" "}
          <span className="text-indigo-400">eBPF-enabled agent</span> on Linux ≥ 5.8.
        </p>
      </div>

      {/* Tabs + window selector */}
      <div className="flex items-center justify-between">
        <div className="flex gap-1 bg-white/[0.04] rounded-lg p-1">
          {(["rules", "violations"] as const).map(t => (
            <button key={t} onClick={() => setTab(t)}
              className={clsx("px-3 py-1.5 rounded-md text-xs font-medium transition-colors capitalize",
                tab === t ? "bg-indigo-600 text-white" : "text-slate-400 hover:text-slate-200")}>
              {t}
              {t === "violations" && violations.length > 0 && (
                <span className="ml-1.5 px-1.5 py-0.5 rounded-full bg-red-500/20 text-red-400 text-[10px]">
                  {violations.length}
                </span>
              )}
            </button>
          ))}
        </div>
        {tab === "violations" && (
          <div className="flex gap-1">
            {WINDOWS.map(w => (
              <button key={w} onClick={() => setWindow(w)}
                className={clsx("px-2.5 py-1 rounded text-xs border transition-colors",
                  window === w
                    ? "border-indigo-500/50 bg-indigo-500/10 text-indigo-300"
                    : "border-white/[0.08] text-slate-500 hover:text-slate-300")}>
                {w}
              </button>
            ))}
          </div>
        )}
      </div>

      {/* Rules tab */}
      {tab === "rules" && (
        policies.length === 0 ? (
          <div className="rounded-xl bg-[#0d0d1a] border border-white/[0.06] flex flex-col items-center py-16 gap-3">
            <ShieldAlert size={32} className="text-slate-700" />
            <p className="text-sm text-slate-500">No policies defined yet</p>
            <button onClick={() => setShowModal(true)}
              className="flex items-center gap-1.5 px-4 py-2 rounded-md text-sm bg-indigo-600 hover:bg-indigo-500 text-white transition-colors">
              <Plus size={14} /> Create your first policy
            </button>
          </div>
        ) : (
          <div className="rounded-xl bg-[#0d0d1a] border border-white/[0.06] overflow-hidden">
            <div className="flex items-center px-4 py-2 border-b border-white/[0.06] text-[11px] font-semibold text-slate-500 uppercase tracking-wide">
              <span className="flex-1">Rule name</span>
              <span className="w-32 shrink-0">Process</span>
              <span className="w-20 shrink-0">Action</span>
              <span className="w-36 shrink-0">Destination</span>
              <span className="w-24 shrink-0 text-center">Status</span>
              <span className="w-16 shrink-0" />
            </div>
            <div className="divide-y divide-white/[0.03] text-sm">
              {policies.map(p => (
                <div key={p.id} className="flex items-center px-4 py-3 hover:bg-white/[0.02]">
                  <div className="flex-1 min-w-0">
                    <p className="text-white text-xs font-medium">{p.name}</p>
                    {p.description && <p className="text-slate-600 text-[10px] mt-0.5 truncate">{p.description}</p>}
                  </div>
                  <span className="w-32 shrink-0 font-mono text-xs text-emerald-400/80">{p.process_name}</span>
                  <span className={clsx("w-20 shrink-0 text-xs font-medium",
                    p.action === "deny" ? "text-red-400" : "text-amber-400")}>
                    {p.action}
                  </span>
                  <span className="w-36 shrink-0 font-mono text-xs text-slate-500">
                    {[p.dst_ip_cidr, p.dst_port ? `:${p.dst_port}` : ""].filter(Boolean).join("") || "any"}
                  </span>
                  <span className="w-24 shrink-0 flex justify-center">
                    <button onClick={() => toggle(p)}
                      className={clsx("relative inline-flex h-5 w-9 rounded-full transition-colors",
                        p.enabled ? "bg-indigo-600" : "bg-white/10")}>
                      <span className={clsx("inline-block h-4 w-4 rounded-full bg-white shadow transform transition-transform mt-0.5",
                        p.enabled ? "translate-x-4" : "translate-x-0.5")} />
                    </button>
                  </span>
                  <div className="w-16 shrink-0 flex justify-end">
                    <button onClick={() => remove(p.id)}
                      className="p-1.5 rounded text-slate-600 hover:text-red-400 hover:bg-red-500/10 transition-colors">
                      <Trash2 size={13} />
                    </button>
                  </div>
                </div>
              ))}
            </div>
          </div>
        )
      )}

      {/* Violations tab */}
      {tab === "violations" && (
        violations.length === 0 ? (
          <div className="rounded-xl bg-[#0d0d1a] border border-white/[0.06] flex flex-col items-center py-16 gap-3">
            <AlertTriangle size={32} className="text-slate-700" />
            <p className="text-sm text-slate-500">No violations in this window</p>
          </div>
        ) : (
          <div className="rounded-xl bg-[#0d0d1a] border border-white/[0.06] overflow-hidden">
            <div className="flex items-center px-4 py-2 border-b border-white/[0.06] text-[11px] font-semibold text-slate-500 uppercase tracking-wide">
              <span className="w-36 shrink-0">Time</span>
              <span className="w-28 shrink-0">Process</span>
              <span className="w-32 shrink-0">Policy</span>
              <span className="w-40 shrink-0">Destination</span>
              <span className="flex-1">Host</span>
            </div>
            <div className="divide-y divide-white/[0.03] font-mono text-xs">
              {violations.map(v => (
                <div key={v.id} className="flex items-center px-4 py-2 hover:bg-white/[0.02]">
                  <span className="w-36 shrink-0 text-slate-600">
                    {new Date(v.violated_at).toLocaleTimeString("en-US", { hour12: false, hour: "2-digit", minute: "2-digit", second: "2-digit" })}
                  </span>
                  <span className="w-28 shrink-0 text-emerald-400/80">{v.process_name}</span>
                  <span className="w-32 shrink-0 text-amber-400/80">{v.policy_name}</span>
                  <span className="w-40 shrink-0 text-slate-300">{v.dst_ip}:{v.dst_port}</span>
                  <span className="flex-1 text-slate-500">{v.hostname}</span>
                </div>
              ))}
            </div>
          </div>
        )
      )}

      {showModal && <CreatePolicyModal onClose={() => setShowModal(false)} onCreated={load} />}
    </div>
  );
}
