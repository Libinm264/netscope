"use client";

import { useEffect, useState, useCallback } from "react";
import { useParams, useRouter } from "next/navigation";
import { Pencil, Check, X, ArrowLeft, Loader2 } from "lucide-react";
import {
  getDashboard,
  updateDashboard,
  type Dashboard,
  type Widget,
} from "@/lib/dashboard";
import { DashboardGrid } from "@/components/DashboardGrid";

export default function DashboardViewPage() {
  const { id } = useParams<{ id: string }>();
  const router  = useRouter();

  const [dash,    setDash]    = useState<Dashboard | null>(null);
  const [loading, setLoading] = useState(true);
  const [editing, setEditing] = useState(false);
  const [widgets, setWidgets] = useState<Widget[]>([]);
  const [name,    setName]    = useState("");
  const [desc,    setDesc]    = useState("");
  const [saving,  setSaving]  = useState(false);
  const [saveErr, setSaveErr] = useState("");

  const load = useCallback(() => {
    setLoading(true);
    getDashboard(id)
      .then((d) => {
        setDash(d);
        setWidgets(d.widgets);
        setName(d.name);
        setDesc(d.description);
      })
      .catch(() => router.push("/dashboards"))
      .finally(() => setLoading(false));
  }, [id, router]);

  useEffect(load, [load]);

  const startEdit = () => {
    setWidgets(dash?.widgets ?? []);
    setName(dash?.name ?? "");
    setDesc(dash?.description ?? "");
    setEditing(true);
    setSaveErr("");
  };

  const cancelEdit = () => {
    setEditing(false);
    setSaveErr("");
    // Restore from last saved state.
    if (dash) {
      setWidgets(dash.widgets);
      setName(dash.name);
      setDesc(dash.description);
    }
  };

  const save = async () => {
    if (!name.trim()) { setSaveErr("Name is required"); return; }
    setSaving(true);
    setSaveErr("");
    try {
      const updated = await updateDashboard(id, name.trim(), desc.trim(), widgets);
      setDash(updated);
      setWidgets(updated.widgets);
      setEditing(false);
    } catch (e) {
      setSaveErr("Failed to save — please try again");
      console.error(e);
    } finally {
      setSaving(false);
    }
  };

  if (loading) {
    return (
      <div className="flex items-center justify-center h-64">
        <Loader2 size={20} className="text-indigo-400 animate-spin" />
      </div>
    );
  }

  return (
    <div className="p-6 space-y-5">
      {/* ── Header ── */}
      <div className="flex items-start gap-4">
        <button
          onClick={() => router.push("/dashboards")}
          className="mt-0.5 p-1.5 rounded-lg text-slate-500 hover:text-white
                     hover:bg-white/[0.04] transition-colors"
        >
          <ArrowLeft size={15} />
        </button>

        <div className="flex-1 min-w-0">
          {editing ? (
            <div className="space-y-2">
              <input
                className="w-full max-w-sm rounded-lg bg-white/[0.06] border border-white/[0.08]
                           px-3 py-1.5 text-base font-semibold text-white outline-none
                           focus:border-indigo-500 placeholder:text-slate-600"
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder="Dashboard name"
              />
              <input
                className="w-full max-w-sm rounded-lg bg-white/[0.06] border border-white/[0.08]
                           px-3 py-1.5 text-xs text-slate-300 outline-none
                           focus:border-indigo-500 placeholder:text-slate-600"
                value={desc}
                onChange={(e) => setDesc(e.target.value)}
                placeholder="Description (optional)"
              />
              {saveErr && <p className="text-xs text-red-400">{saveErr}</p>}
            </div>
          ) : (
            <>
              <h1 className="text-xl font-semibold text-white">{dash?.name}</h1>
              {dash?.description && (
                <p className="text-sm text-slate-500 mt-0.5">{dash.description}</p>
              )}
            </>
          )}
        </div>

        {/* action buttons */}
        <div className="flex items-center gap-2 shrink-0">
          {editing ? (
            <>
              <button
                onClick={cancelEdit}
                className="flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs
                           border border-white/[0.08] text-slate-400 hover:text-white
                           hover:bg-white/[0.04] transition-colors"
              >
                <X size={12} /> Cancel
              </button>
              <button
                onClick={save}
                disabled={saving}
                className="flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs
                           bg-indigo-600 hover:bg-indigo-500 text-white font-medium
                           transition-colors disabled:opacity-40"
              >
                {saving ? (
                  <Loader2 size={12} className="animate-spin" />
                ) : (
                  <Check size={12} />
                )}
                {saving ? "Saving…" : "Save"}
              </button>
            </>
          ) : (
            <button
              onClick={startEdit}
              className="flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs
                         border border-white/[0.08] text-slate-300 hover:text-white
                         hover:bg-white/[0.04] transition-colors"
            >
              <Pencil size={12} /> Edit
            </button>
          )}
        </div>
      </div>

      {/* ── Grid ── */}
      <DashboardGrid
        widgets={widgets}
        editing={editing}
        onChange={setWidgets}
      />
    </div>
  );
}
