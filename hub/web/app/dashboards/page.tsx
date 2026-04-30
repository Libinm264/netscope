"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { Plus, LayoutDashboard, Trash2, Clock, ArrowRight } from "lucide-react";
import {
  listDashboards,
  createDashboard,
  deleteDashboard,
  type Dashboard,
} from "@/lib/dashboard";

function relTime(ts: string) {
  const diff = (Date.now() - new Date(ts).getTime()) / 1000;
  if (diff < 60)    return "just now";
  if (diff < 3600)  return `${Math.floor(diff / 60)}m ago`;
  if (diff < 86400) return `${Math.floor(diff / 3600)}h ago`;
  return `${Math.floor(diff / 86400)}d ago`;
}

export default function DashboardsPage() {
  const [dashboards, setDashboards] = useState<Dashboard[]>([]);
  const [loading, setLoading]       = useState(true);
  const [creating, setCreating]     = useState(false);
  const [newName, setNewName]       = useState("");
  const [newDesc, setNewDesc]       = useState("");
  const [saving, setSaving]         = useState(false);
  const [deleting, setDeleting]     = useState<string | null>(null);

  const load = () => {
    setLoading(true);
    listDashboards()
      .then(setDashboards)
      .catch(console.error)
      .finally(() => setLoading(false));
  };

  useEffect(load, []);

  const handleCreate = async () => {
    if (!newName.trim()) return;
    setSaving(true);
    try {
      const d = await createDashboard(newName.trim(), newDesc.trim(), []);
      setDashboards((prev) => [d, ...prev]);
      setCreating(false);
      setNewName("");
      setNewDesc("");
    } catch (e) {
      console.error(e);
    } finally {
      setSaving(false);
    }
  };

  const handleDelete = async (id: string, e: React.MouseEvent) => {
    e.preventDefault();
    if (!confirm("Delete this dashboard?")) return;
    setDeleting(id);
    try {
      await deleteDashboard(id);
      setDashboards((prev) => prev.filter((d) => d.id !== id));
    } catch (e) {
      console.error(e);
    } finally {
      setDeleting(null);
    }
  };

  return (
    <div className="p-6 space-y-6 max-w-5xl">
      {/* header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-xl font-semibold text-white">Custom Dashboards</h1>
          <p className="text-sm text-slate-500 mt-0.5">
            Build your own views from flow, alert, and anomaly data.
          </p>
        </div>
        <button
          onClick={() => setCreating(true)}
          className="flex items-center gap-2 px-4 py-2 rounded-lg bg-indigo-600
                     hover:bg-indigo-500 text-white text-sm font-medium transition-colors"
        >
          <Plus size={14} /> New dashboard
        </button>
      </div>

      {/* create modal */}
      {creating && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm">
          <div className="w-full max-w-md rounded-2xl bg-[#0d0d1a] border border-white/[0.08] p-6 space-y-4">
            <h2 className="text-base font-semibold text-white">New dashboard</h2>
            <label className="block">
              <span className="text-xs text-slate-400">Name *</span>
              <input
                autoFocus
                className="mt-1 w-full rounded-lg bg-white/[0.06] border border-white/[0.08]
                           px-3 py-2 text-sm text-slate-200 outline-none focus:border-indigo-500
                           placeholder:text-slate-600"
                placeholder="e.g. Security overview"
                value={newName}
                onChange={(e) => setNewName(e.target.value)}
                onKeyDown={(e) => e.key === "Enter" && handleCreate()}
              />
            </label>
            <label className="block">
              <span className="text-xs text-slate-400">Description (optional)</span>
              <input
                className="mt-1 w-full rounded-lg bg-white/[0.06] border border-white/[0.08]
                           px-3 py-2 text-sm text-slate-200 outline-none focus:border-indigo-500
                           placeholder:text-slate-600"
                placeholder="What is this dashboard for?"
                value={newDesc}
                onChange={(e) => setNewDesc(e.target.value)}
              />
            </label>
            <div className="flex justify-end gap-2 pt-1">
              <button
                onClick={() => { setCreating(false); setNewName(""); setNewDesc(""); }}
                className="px-4 py-2 text-sm text-slate-400 hover:text-white transition-colors"
              >
                Cancel
              </button>
              <button
                disabled={!newName.trim() || saving}
                onClick={handleCreate}
                className="px-4 py-2 rounded-lg bg-indigo-600 hover:bg-indigo-500 text-white
                           text-sm font-medium transition-colors disabled:opacity-40"
              >
                {saving ? "Creating…" : "Create"}
              </button>
            </div>
          </div>
        </div>
      )}

      {/* list */}
      {loading ? (
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
          {Array.from({ length: 3 }).map((_, i) => (
            <div key={i} className="h-32 rounded-xl bg-white/[0.03] animate-pulse" />
          ))}
        </div>
      ) : dashboards.length === 0 ? (
        <div className="flex flex-col items-center justify-center py-24 rounded-xl
                        border border-dashed border-white/[0.06] text-center">
          <LayoutDashboard size={32} className="text-slate-700 mb-3" />
          <p className="text-slate-500 text-sm">No dashboards yet</p>
          <p className="text-slate-600 text-xs mt-1">
            Click &ldquo;New dashboard&rdquo; to create your first one.
          </p>
        </div>
      ) : (
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
          {dashboards.map((d) => (
            <Link
              key={d.id}
              href={`/dashboards/${d.id}`}
              className="group relative flex flex-col justify-between p-5 rounded-xl
                         bg-[#0d0d1a] border border-white/[0.06]
                         hover:border-indigo-500/40 hover:bg-white/[0.03] transition-all"
            >
              <div>
                <div className="flex items-start justify-between gap-2">
                  <div className="p-2 rounded-lg bg-indigo-500/10">
                    <LayoutDashboard size={14} className="text-indigo-400" />
                  </div>
                  <button
                    onClick={(e) => handleDelete(d.id, e)}
                    disabled={deleting === d.id}
                    className="p-1.5 rounded-lg text-slate-600 hover:text-red-400
                               hover:bg-red-400/10 transition-colors opacity-0
                               group-hover:opacity-100 disabled:opacity-30"
                    title="Delete"
                  >
                    <Trash2 size={13} />
                  </button>
                </div>
                <h3 className="mt-3 text-sm font-semibold text-white">{d.name}</h3>
                {d.description && (
                  <p className="text-xs text-slate-500 mt-1 line-clamp-2">{d.description}</p>
                )}
                <p className="text-xs text-slate-600 mt-1">
                  {d.widgets.length} widget{d.widgets.length !== 1 ? "s" : ""}
                </p>
              </div>
              <div className="flex items-center justify-between mt-4 pt-3 border-t border-white/[0.04]">
                <div className="flex items-center gap-1 text-[10px] text-slate-600">
                  <Clock size={10} />
                  {relTime(d.updated_at)}
                </div>
                <ArrowRight size={12} className="text-slate-600 group-hover:text-indigo-400 transition-colors" />
              </div>
            </Link>
          ))}
        </div>
      )}
    </div>
  );
}
