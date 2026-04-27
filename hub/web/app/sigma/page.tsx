"use client";

import { useState, useEffect, useCallback } from "react";
import {
  fetchSigmaRules, fetchSigmaMatches, createSigmaRule, updateSigmaRule, deleteSigmaRule,
  type SigmaRule, type SigmaMatch, type SigmaSeverity,
} from "@/lib/api";
import {
  Shield, Plus, Trash2, Edit2, X, Check, RefreshCw,
  AlertTriangle, Zap, ChevronDown, ChevronUp, Lock, Tag,
} from "lucide-react";
import { clsx } from "clsx";

// ── Severity colours ──────────────────────────────────────────────────────────

const SEV_COLORS: Record<string, string> = {
  critical: "text-red-400 bg-red-500/10 border-red-500/20",
  high:     "text-orange-400 bg-orange-500/10 border-orange-500/20",
  medium:   "text-amber-400 bg-amber-500/10 border-amber-500/20",
  low:      "text-sky-400 bg-sky-500/10 border-sky-500/20",
};

const SEV_DOT: Record<string, string> = {
  critical: "bg-red-400",
  high:     "bg-orange-400",
  medium:   "bg-amber-400",
  low:      "bg-sky-400",
};

function SevBadge({ sev }: { sev: string }) {
  return (
    <span className={clsx(
      "text-[10px] font-semibold uppercase tracking-wide px-1.5 py-0.5 rounded border",
      SEV_COLORS[sev] ?? SEV_COLORS.medium,
    )}>
      {sev}
    </span>
  );
}

function relTime(ts: string) {
  const diff = Date.now() - new Date(ts).getTime();
  if (diff < 60_000) return "just now";
  if (diff < 3_600_000) return `${Math.floor(diff / 60_000)}m ago`;
  if (diff < 86_400_000) return `${Math.floor(diff / 3_600_000)}h ago`;
  return `${Math.floor(diff / 86_400_000)}d ago`;
}

// ── Rule card ─────────────────────────────────────────────────────────────────

function RuleCard({
  rule,
  plan,
  onToggle,
  onDelete,
}: {
  rule: SigmaRule;
  plan: string;
  onToggle: (id: string, enabled: boolean) => void;
  onDelete: (id: string) => void;
}) {
  const [expanded, setExpanded] = useState(false);
  const canEdit = plan === "enterprise" && !rule.builtin;

  return (
    <div className={clsx(
      "rounded-lg border transition-colors",
      rule.enabled
        ? "bg-[#0d0d1a] border-white/[0.06]"
        : "bg-[#0a0a15] border-white/[0.03] opacity-60",
    )}>
      {/* Header */}
      <div className="flex items-center gap-3 px-4 py-3">
        {/* Severity dot */}
        <span className={clsx("w-2 h-2 rounded-full shrink-0 mt-0.5", SEV_DOT[rule.severity] ?? SEV_DOT.medium)} />

        {/* Title + badges */}
        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-2 flex-wrap">
            <span className="text-sm font-medium text-white truncate">{rule.title}</span>
            <SevBadge sev={rule.severity} />
            {rule.builtin && (
              <span className="text-[9px] font-semibold uppercase tracking-wide px-1.5 py-0.5
                               rounded border text-emerald-400 bg-emerald-500/10 border-emerald-500/20">
                Built-in
              </span>
            )}
            {!rule.enabled && (
              <span className="text-[9px] font-semibold uppercase tracking-wide px-1.5 py-0.5
                               rounded border text-slate-500 bg-white/[0.04] border-white/[0.06]">
                Disabled
              </span>
            )}
          </div>
          {rule.description && (
            <p className="text-xs text-slate-500 mt-0.5 line-clamp-1">{rule.description}</p>
          )}
        </div>

        {/* Actions */}
        <div className="flex items-center gap-2 shrink-0">
          {rule.tags.length > 0 && (
            <button
              onClick={() => setExpanded((o) => !o)}
              className="flex items-center gap-1 text-[10px] text-slate-600 hover:text-slate-400 transition-colors"
            >
              <Tag size={11} />
              {rule.tags.length}
            </button>
          )}

          {/* Enable/disable toggle */}
          {(canEdit || rule.builtin) && (
            <button
              onClick={() => onToggle(rule.id, !rule.enabled)}
              className={clsx(
                "w-8 h-4 rounded-full transition-colors relative",
                rule.enabled ? "bg-indigo-600" : "bg-white/[0.08]",
              )}
            >
              <span className={clsx(
                "absolute top-0.5 w-3 h-3 rounded-full bg-white transition-all",
                rule.enabled ? "left-[18px]" : "left-0.5",
              )} />
            </button>
          )}

          {canEdit && (
            <button
              onClick={() => onDelete(rule.id)}
              className="p-1 rounded text-slate-600 hover:text-red-400 hover:bg-red-500/10 transition-colors"
            >
              <Trash2 size={14} />
            </button>
          )}

          {rule.builtin && plan !== "enterprise" && (
            <span title="Upgrade to Enterprise to manage built-in rules">
              <Lock size={12} className="text-slate-700" />
            </span>
          )}

          <button
            onClick={() => setExpanded((o) => !o)}
            className="p-1 rounded text-slate-600 hover:text-slate-400 transition-colors"
          >
            {expanded ? <ChevronUp size={14} /> : <ChevronDown size={14} />}
          </button>
        </div>
      </div>

      {/* Expanded: query + tags */}
      {expanded && (
        <div className="px-4 pb-4 border-t border-white/[0.04] pt-3 space-y-3">
          {rule.tags.length > 0 && (
            <div className="flex flex-wrap gap-1.5">
              {rule.tags.map((t) => (
                <span key={t} className="text-[10px] px-1.5 py-0.5 rounded bg-indigo-500/10 text-indigo-400 border border-indigo-500/20">
                  {t}
                </span>
              ))}
            </div>
          )}
          <div>
            <p className="text-[10px] text-slate-600 uppercase tracking-wider mb-1.5 font-semibold">Detection query</p>
            <pre className="text-xs text-emerald-300/80 bg-[#0a0a15] border border-white/[0.04] rounded p-3
                            overflow-x-auto font-mono leading-relaxed whitespace-pre-wrap">
              {rule.query}
            </pre>
          </div>
        </div>
      )}
    </div>
  );
}

// ── New rule form ─────────────────────────────────────────────────────────────

function NewRuleForm({
  onSave,
  onCancel,
}: {
  onSave: (r: Omit<SigmaRule, "id" | "builtin" | "created_at" | "updated_at">) => Promise<void>;
  onCancel: () => void;
}) {
  const [title, setTitle]     = useState("");
  const [desc, setDesc]       = useState("");
  const [severity, setSev]    = useState<SigmaSeverity>("medium");
  const [tagsRaw, setTagsRaw] = useState("");
  const [query, setQuery]     = useState(`SELECT src_ip, dst_ip, dst_port, protocol, hostname, ts
FROM flows
WHERE ts > now() - INTERVAL 5 MINUTE
  AND protocol = 'TCP'
  -- add your detection logic here
LIMIT 100`);
  const [enabled, setEnabled] = useState(true);
  const [saving, setSaving]   = useState(false);
  const [err, setErr]         = useState("");

  async function submit() {
    if (!title.trim()) { setErr("Title is required"); return; }
    if (!query.trim()) { setErr("Query is required"); return; }
    setSaving(true);
    setErr("");
    try {
      await onSave({
        title: title.trim(),
        description: desc.trim(),
        severity,
        tags: tagsRaw.split(",").map((t) => t.trim()).filter(Boolean),
        query: query.trim(),
        enabled,
      });
    } catch (e) {
      setErr(String(e));
    } finally {
      setSaving(false);
    }
  }

  const inputCls = "bg-[#0d0d1a] border border-white/10 rounded px-3 py-2 text-sm text-slate-300 " +
    "placeholder-slate-600 focus:outline-none focus:border-indigo-500/50 w-full";

  return (
    <div className="rounded-lg border border-indigo-500/20 bg-indigo-500/[0.03] p-5 space-y-4">
      <div className="flex items-center justify-between">
        <h3 className="text-sm font-semibold text-white flex items-center gap-2">
          <Plus size={14} className="text-indigo-400" /> New Detection Rule
        </h3>
        <button onClick={onCancel} className="p-1 rounded text-slate-500 hover:text-white transition-colors">
          <X size={14} />
        </button>
      </div>

      <div className="grid grid-cols-2 gap-3">
        <div className="col-span-2">
          <label className="block text-xs text-slate-500 mb-1">Title *</label>
          <input className={inputCls} placeholder="e.g. Lateral Movement via SMB" value={title} onChange={(e) => setTitle(e.target.value)} />
        </div>
        <div className="col-span-2">
          <label className="block text-xs text-slate-500 mb-1">Description</label>
          <input className={inputCls} placeholder="What does this rule detect?" value={desc} onChange={(e) => setDesc(e.target.value)} />
        </div>
        <div>
          <label className="block text-xs text-slate-500 mb-1">Severity</label>
          <select value={severity} onChange={(e) => setSev(e.target.value as SigmaSeverity)}
            className={inputCls + " appearance-none"}>
            <option value="low">Low</option>
            <option value="medium">Medium</option>
            <option value="high">High</option>
            <option value="critical">Critical</option>
          </select>
        </div>
        <div>
          <label className="block text-xs text-slate-500 mb-1">Tags (comma-separated)</label>
          <input className={inputCls + " font-mono"} placeholder="recon, attack.discovery" value={tagsRaw} onChange={(e) => setTagsRaw(e.target.value)} />
        </div>
        <div className="col-span-2">
          <label className="block text-xs text-slate-500 mb-1">
            Detection Query * <span className="text-indigo-500">(ClickHouse SQL — rows returned = rule fires)</span>
          </label>
          <textarea
            rows={7}
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            className="bg-[#0a0a15] border border-white/10 rounded px-3 py-2.5 text-xs text-emerald-300/90
                       placeholder-slate-600 focus:outline-none focus:border-indigo-500/50 w-full font-mono
                       leading-relaxed resize-none"
          />
        </div>
        <div className="flex items-center gap-2">
          <button
            onClick={() => setEnabled((e) => !e)}
            className={clsx("w-8 h-4 rounded-full transition-colors relative",
              enabled ? "bg-indigo-600" : "bg-white/[0.08]")}
          >
            <span className={clsx("absolute top-0.5 w-3 h-3 rounded-full bg-white transition-all",
              enabled ? "left-[18px]" : "left-0.5")} />
          </button>
          <span className="text-xs text-slate-400">Enable immediately</span>
        </div>
      </div>

      {err && <p className="text-xs text-red-400">{err}</p>}

      <div className="flex gap-2 pt-1">
        <button
          onClick={submit}
          disabled={saving}
          className="flex items-center gap-2 px-4 py-2 rounded bg-indigo-600 text-white text-sm font-medium
                     hover:bg-indigo-500 disabled:opacity-50 transition-colors"
        >
          {saving ? <RefreshCw size={13} className="animate-spin" /> : <Check size={13} />}
          Save Rule
        </button>
        <button onClick={onCancel}
          className="px-4 py-2 rounded bg-[#12121f] border border-white/10 text-sm text-slate-300
                     hover:text-white hover:border-white/20 transition-colors">
          Cancel
        </button>
      </div>
    </div>
  );
}

// ── Match row ─────────────────────────────────────────────────────────────────

function MatchRow({ m }: { m: SigmaMatch }) {
  const [open, setOpen] = useState(false);
  let parsed: Record<string, unknown> = {};
  try { parsed = JSON.parse(m.match_data); } catch { /* ignore */ }

  return (
    <div className="border border-white/[0.04] rounded-lg overflow-hidden">
      <button
        onClick={() => setOpen((o) => !o)}
        className="w-full flex items-center gap-3 px-4 py-2.5 text-left hover:bg-white/[0.03] transition-colors"
      >
        <span className={clsx("w-1.5 h-1.5 rounded-full shrink-0", SEV_DOT[m.severity] ?? SEV_DOT.medium)} />
        <span className="text-xs text-slate-300 flex-1 truncate">{m.rule_title}</span>
        <SevBadge sev={m.severity} />
        <span className="text-[10px] text-slate-600 ml-2 shrink-0">{relTime(m.fired_at)}</span>
        {open ? <ChevronUp size={12} className="text-slate-600" /> : <ChevronDown size={12} className="text-slate-600" />}
      </button>
      {open && (
        <div className="px-4 pb-3 border-t border-white/[0.04]">
          <div className="mt-2 grid grid-cols-2 gap-x-6 gap-y-1">
            {Object.entries(parsed).map(([k, v]) => (
              <div key={k} className="flex items-baseline gap-1.5">
                <span className="text-[10px] text-slate-600 font-mono shrink-0">{k}</span>
                <span className="text-[10px] text-slate-300 font-mono truncate">{String(v)}</span>
              </div>
            ))}
          </div>
          <p className="text-[10px] text-slate-700 mt-2 font-mono">{new Date(m.fired_at).toLocaleString()}</p>
        </div>
      )}
    </div>
  );
}

// ── Enterprise gate ───────────────────────────────────────────────────────────

function EnterpriseGate() {
  return (
    <div className="rounded-xl border border-amber-500/20 bg-amber-500/[0.05] p-6 text-center space-y-3">
      <div className="w-10 h-10 rounded-full bg-amber-500/10 flex items-center justify-center mx-auto">
        <Zap size={20} className="text-amber-400" />
      </div>
      <h3 className="text-sm font-semibold text-white">Custom Detection Rules require Enterprise</h3>
      <p className="text-xs text-slate-500 max-w-sm mx-auto">
        You have full access to the 5 built-in detection rules. Upgrade to Enterprise to create
        unlimited custom Sigma-style rules with your own ClickHouse queries.
      </p>
      <a href="/settings/license"
        className="inline-flex items-center gap-1.5 px-4 py-2 rounded bg-amber-500/10 border border-amber-500/20
                   text-xs text-amber-400 font-medium hover:bg-amber-500/20 transition-colors">
        <Zap size={12} /> View License & Plans
      </a>
    </div>
  );
}

// ── Page ──────────────────────────────────────────────────────────────────────

export default function SigmaPage() {
  const [rules, setRules]       = useState<SigmaRule[]>([]);
  const [matches, setMatches]   = useState<SigmaMatch[]>([]);
  const [plan, setPlan]         = useState("community");
  const [loading, setLoading]   = useState(true);
  const [showNew, setShowNew]   = useState(false);
  const [activeTab, setTab]     = useState<"rules" | "matches">("rules");

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const [rulesRes, matchRes] = await Promise.all([
        fetchSigmaRules(),
        fetchSigmaMatches({ limit: 100 }),
      ]);
      setRules(rulesRes.rules ?? []);
      setPlan(rulesRes.plan ?? "community");
      setMatches(matchRes.matches ?? []);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { load(); }, [load]);

  async function handleToggle(id: string, enabled: boolean) {
    await updateSigmaRule(id, { enabled }).catch(() => {});
    setRules((prev) => prev.map((r) => r.id === id ? { ...r, enabled } : r));
  }

  async function handleDelete(id: string) {
    await deleteSigmaRule(id).catch(() => {});
    setRules((prev) => prev.filter((r) => r.id !== id));
  }

  async function handleCreate(body: Omit<SigmaRule, "id" | "builtin" | "created_at" | "updated_at">) {
    const r = await createSigmaRule(body);
    setRules((prev) => [...prev, r]);
    setShowNew(false);
  }

  const builtinRules = rules.filter((r) => r.builtin);
  const customRules  = rules.filter((r) => !r.builtin);

  // Match severity counts
  const sevCounts = matches.reduce((acc, m) => {
    acc[m.severity] = (acc[m.severity] ?? 0) + 1;
    return acc;
  }, {} as Record<string, number>);

  return (
    <div className="p-6 space-y-6 max-w-4xl">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-xl font-semibold text-white flex items-center gap-2">
            <Shield size={20} className="text-indigo-400" />
            Detection Rules
          </h1>
          <p className="text-xs text-slate-500 mt-1">
            Sigma-style rules evaluated every 5 minutes against your flow data.
            {plan !== "enterprise" && (
              <span className="ml-1 text-amber-400">
                Community: 5 built-in rules · <a href="/settings/license" className="underline">Upgrade</a> for custom rules
              </span>
            )}
          </p>
        </div>
        <div className="flex items-center gap-2">
          <button onClick={load}
            className="flex items-center gap-2 px-3 py-1.5 rounded bg-[#12121f] border border-white/10
                       text-sm text-slate-300 hover:text-white hover:border-white/20 transition-colors">
            <RefreshCw size={14} className={loading ? "animate-spin" : ""} />
            Refresh
          </button>
          {plan === "enterprise" && !showNew && (
            <button onClick={() => setShowNew(true)}
              className="flex items-center gap-2 px-3 py-1.5 rounded bg-indigo-600 text-white text-sm font-medium
                         hover:bg-indigo-500 transition-colors">
              <Plus size={14} /> New Rule
            </button>
          )}
        </div>
      </div>

      {/* Stats row */}
      <div className="grid grid-cols-4 gap-3">
        {[
          { label: "Rules", value: rules.length, sub: `${rules.filter((r) => r.enabled).length} active` },
          { label: "Critical Matches", value: sevCounts.critical ?? 0, color: "text-red-400" },
          { label: "High Matches", value: sevCounts.high ?? 0, color: "text-orange-400" },
          { label: "Total Fires (last 30d)", value: matches.length, color: "text-slate-300" },
        ].map(({ label, value, sub, color }) => (
          <div key={label} className="rounded-lg border border-white/[0.06] bg-[#0d0d1a] p-4">
            <p className="text-[11px] text-slate-500">{label}</p>
            <p className={clsx("text-2xl font-bold mt-1", color ?? "text-white")}>{value}</p>
            {sub && <p className="text-[10px] text-slate-600 mt-0.5">{sub}</p>}
          </div>
        ))}
      </div>

      {/* Tabs */}
      <div className="flex gap-1 border-b border-white/[0.06]">
        {(["rules", "matches"] as const).map((tab) => (
          <button key={tab} onClick={() => setTab(tab)}
            className={clsx(
              "px-4 py-2 text-sm font-medium capitalize transition-colors border-b-2 -mb-px",
              activeTab === tab
                ? "text-indigo-400 border-indigo-500"
                : "text-slate-400 border-transparent hover:text-slate-200",
            )}>
            {tab === "matches" ? `Matches (${matches.length})` : `Rules (${rules.length})`}
          </button>
        ))}
      </div>

      {/* Rules tab */}
      {activeTab === "rules" && (
        <div className="space-y-4">
          {showNew && (
            <NewRuleForm onSave={handleCreate} onCancel={() => setShowNew(false)} />
          )}

          {/* Built-in rules */}
          {builtinRules.length > 0 && (
            <div className="space-y-2">
              <div className="flex items-center gap-2">
                <p className="text-[11px] font-semibold text-slate-500 uppercase tracking-wider">
                  Built-in Rules
                </p>
                <span className="text-[9px] px-1.5 py-0.5 rounded bg-emerald-500/10 text-emerald-400 border border-emerald-500/20 font-semibold uppercase tracking-wide">
                  Community
                </span>
              </div>
              {builtinRules.map((r) => (
                <RuleCard key={r.id} rule={r} plan={plan} onToggle={handleToggle} onDelete={handleDelete} />
              ))}
            </div>
          )}

          {/* Custom rules */}
          <div className="space-y-2">
            <div className="flex items-center gap-2">
              <p className="text-[11px] font-semibold text-slate-500 uppercase tracking-wider">
                Custom Rules
              </p>
              <span className="text-[9px] px-1.5 py-0.5 rounded bg-amber-500/10 text-amber-400 border border-amber-500/20 font-semibold uppercase tracking-wide">
                Enterprise
              </span>
            </div>
            {plan !== "enterprise" ? (
              <EnterpriseGate />
            ) : customRules.length === 0 ? (
              <div className="rounded-lg border border-dashed border-white/10 py-10 text-center">
                <AlertTriangle size={24} className="text-slate-700 mx-auto mb-2" />
                <p className="text-sm text-slate-600">No custom rules yet.</p>
                <button onClick={() => setShowNew(true)}
                  className="mt-3 flex items-center gap-1.5 px-3 py-1.5 rounded bg-indigo-600 text-white
                             text-xs font-medium hover:bg-indigo-500 transition-colors mx-auto">
                  <Plus size={12} /> Create your first rule
                </button>
              </div>
            ) : (
              customRules.map((r) => (
                <RuleCard key={r.id} rule={r} plan={plan} onToggle={handleToggle} onDelete={handleDelete} />
              ))
            )}
          </div>
        </div>
      )}

      {/* Matches tab */}
      {activeTab === "matches" && (
        <div className="space-y-2">
          {matches.length === 0 ? (
            <div className="rounded-lg border border-dashed border-white/10 py-12 text-center">
              <Shield size={28} className="text-slate-700 mx-auto mb-2" />
              <p className="text-sm text-slate-500">No rule matches in the last 30 days.</p>
              <p className="text-xs text-slate-700 mt-1">Rules are evaluated every 5 minutes against live flows.</p>
            </div>
          ) : (
            matches.map((m) => <MatchRow key={m.id} m={m} />)
          )}
        </div>
      )}
    </div>
  );
}
