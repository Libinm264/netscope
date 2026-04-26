"use client";

import { useCallback, useEffect, useState } from "react";
import { Building2, Save, RefreshCw } from "lucide-react";
import { clsx } from "clsx";
import { fetchOrg, updateOrg } from "@/lib/api";
import type { OrgInfo } from "@/lib/api";

export default function OrgSettingsPage() {
  const [org, setOrg]       = useState<OrgInfo | null>(null);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving]  = useState(false);
  const [saved, setSaved]    = useState(false);
  const [name, setName]      = useState("");
  const [quota, setQuota]    = useState(10);
  const [retention, setRetention] = useState(90);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const data = await fetchOrg();
      setOrg(data);
      setName(data.name);
      setQuota(data.agent_quota);
      setRetention(data.retention_days);
    } catch { /* hub offline */ }
    finally { setLoading(false); }
  }, []);

  useEffect(() => { load(); }, [load]);

  const handleSave = async () => {
    setSaving(true);
    try {
      await updateOrg({ name, agent_quota: quota, retention_days: retention });
      setSaved(true);
      setTimeout(() => setSaved(false), 2500);
    } catch (e) {
      alert(e instanceof Error ? e.message : "Save failed");
    } finally {
      setSaving(false);
    }
  };

  const planColor =
    org?.plan === "enterprise" ? "text-amber-300 bg-amber-500/10 border-amber-500/25" :
    org?.plan === "team"       ? "text-indigo-300 bg-indigo-500/10 border-indigo-500/25" :
                                 "text-slate-400 bg-slate-700/40 border-white/10";

  return (
    <div className="p-6 space-y-6">
      {/* Header */}
      <div className="flex items-center gap-3">
        <div className="p-2 rounded-lg bg-indigo-500/10">
          <Building2 size={20} className="text-indigo-400" />
        </div>
        <div>
          <h1 className="text-lg font-semibold text-white">Organisation</h1>
          <p className="text-xs text-slate-400 mt-0.5">
            Name, quotas, and data retention for this hub deployment
          </p>
        </div>
      </div>

      {loading ? (
        <div className="flex items-center justify-center h-32">
          <RefreshCw size={20} className="animate-spin text-slate-600" />
        </div>
      ) : (
        <div className="bg-[#0d0d1a] border border-white/[0.06] rounded-xl p-5 space-y-5">

          {/* Plan badge */}
          <div className="flex items-center gap-2">
            <p className="text-xs text-slate-500">Current plan</p>
            <span className={clsx("px-2 py-0.5 rounded-full text-[11px] font-semibold border", planColor)}>
              {(org?.plan ?? "community").charAt(0).toUpperCase() + (org?.plan ?? "community").slice(1)}
            </span>
          </div>

          {/* Name */}
          <div className="space-y-1.5">
            <label className="text-xs font-medium text-slate-300">Organisation name</label>
            <input
              value={name}
              onChange={(e) => setName(e.target.value)}
              className="w-full bg-white/[0.04] border border-white/10 rounded-lg px-3 py-2
                         text-sm text-white placeholder:text-slate-600
                         focus:outline-none focus:border-indigo-500/50"
              placeholder="Acme Corp"
            />
          </div>

          {/* Org ID (read-only) */}
          <div className="space-y-1.5">
            <label className="text-xs font-medium text-slate-300">Organisation ID</label>
            <input
              value={org?.org_id ?? "default"}
              readOnly
              className="w-full bg-white/[0.02] border border-white/[0.06] rounded-lg px-3 py-2
                         text-sm text-slate-500 font-mono cursor-default"
            />
            <p className="text-[10px] text-slate-600">
              Used to scope agent enrollment tokens and API access.
            </p>
          </div>

          {/* Agent quota */}
          <div className="space-y-1.5">
            <label className="text-xs font-medium text-slate-300">Agent quota</label>
            <div className="flex items-center gap-3">
              <input
                type="number"
                min={1}
                value={quota}
                onChange={(e) => setQuota(Number(e.target.value))}
                className="w-28 bg-white/[0.04] border border-white/10 rounded-lg px-3 py-2
                           text-sm text-white focus:outline-none focus:border-indigo-500/50"
              />
              <p className="text-xs text-slate-500">
                Maximum agents that can register with this hub.
                Community plan allows up to 10.
              </p>
            </div>
          </div>

          {/* Retention */}
          <div className="space-y-1.5">
            <label className="text-xs font-medium text-slate-300">Flow retention (days)</label>
            <div className="flex items-center gap-3">
              <input
                type="number"
                min={7}
                value={retention}
                onChange={(e) => setRetention(Number(e.target.value))}
                className="w-28 bg-white/[0.04] border border-white/10 rounded-lg px-3 py-2
                           text-sm text-white focus:outline-none focus:border-indigo-500/50"
              />
              <p className="text-xs text-slate-500">
                Flows older than this are automatically purged by ClickHouse TTL.
                Custom retention beyond 90 days requires Team or Enterprise plan.
              </p>
            </div>
          </div>

          <button
            onClick={handleSave}
            disabled={saving || !name.trim()}
            className={clsx(
              "flex items-center gap-1.5 px-4 py-2 rounded-lg text-sm font-medium transition-colors",
              saved
                ? "bg-emerald-600/20 text-emerald-400 border border-emerald-500/30"
                : "bg-indigo-600 hover:bg-indigo-500 text-white disabled:opacity-40",
            )}
          >
            {saving
              ? <RefreshCw size={13} className="animate-spin" />
              : <Save size={13} />
            }
            {saved ? "Saved!" : "Save changes"}
          </button>
        </div>
      )}
    </div>
  );
}
