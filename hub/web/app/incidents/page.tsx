"use client";

import { useEffect, useState } from "react";
import { Siren, Plus, CheckCheck, ShieldCheck, MessageSquare, ExternalLink, Filter } from "lucide-react";

interface Incident {
  id: string;
  title: string;
  severity: string;
  status: string;
  source: string;
  source_id: string;
  notes: string;
  external_ref: string;
  created_at: string;
  updated_at: string;
}

const SEVERITY_COLORS: Record<string, string> = {
  critical: "bg-red-500/15 text-red-400 border-red-500/30",
  high:     "bg-orange-500/15 text-orange-400 border-orange-500/30",
  medium:   "bg-amber-500/15 text-amber-400 border-amber-500/30",
  low:      "bg-slate-500/15 text-slate-400 border-slate-500/30",
};

const STATUS_COLORS: Record<string, string> = {
  open:     "bg-red-500/10 text-red-400",
  ack:      "bg-amber-500/10 text-amber-400",
  resolved: "bg-emerald-500/10 text-emerald-400",
};

export default function IncidentsPage() {
  const [incidents, setIncidents] = useState<Incident[]>([]);
  const [loading, setLoading]     = useState(true);
  const [statusFilter, setStatusFilter] = useState("");
  const [selected, setSelected]   = useState<Incident | null>(null);
  const [note, setNote]           = useState("");
  const [showCreate, setShowCreate] = useState(false);
  const [newTitle, setNewTitle]   = useState("");
  const [newSeverity, setNewSeverity] = useState("medium");
  const [enterprise, setEnterprise] = useState(true);

  const fetchIncidents = async () => {
    setLoading(true);
    const params = statusFilter ? `?status=${statusFilter}` : "";
    try {
      const r = await fetch(`/api/v1/enterprise/incidents${params}`);
      if (r.status === 402) { setEnterprise(false); return; }
      const d = await r.json();
      setIncidents(d.incidents ?? []);
    } finally {
      setLoading(false);
    }
  };

  const action = async (id: string, endpoint: string, body?: object) => {
    await fetch(`/api/v1/enterprise/incidents/${id}/${endpoint}`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: body ? JSON.stringify(body) : undefined,
    });
    fetchIncidents();
    if (selected?.id === id) setSelected(null);
  };

  const createIncident = async () => {
    await fetch("/api/v1/enterprise/incidents", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ title: newTitle, severity: newSeverity }),
    });
    setShowCreate(false);
    setNewTitle("");
    fetchIncidents();
  };

  const addNote = async () => {
    if (!selected || !note.trim()) return;
    await action(selected.id, "notes", { note });
    setNote("");
  };

  useEffect(() => { fetchIncidents(); }, [statusFilter]);

  if (!enterprise) return (
    <div className="p-6">
      <div className="max-w-md mx-auto mt-20 text-center">
        <Siren size={40} className="mx-auto text-slate-600 mb-4" />
        <h2 className="text-lg font-semibold text-white mb-2">Incident Workflow</h2>
        <p className="text-slate-400 text-sm mb-4">
          Incident management with Jira, Linear, PagerDuty, and OpsGenie integration requires Enterprise plan.
        </p>
        <a href="/settings/license" className="px-4 py-2 rounded-lg bg-indigo-500 hover:bg-indigo-600 text-white text-sm font-medium">
          Upgrade to Enterprise
        </a>
      </div>
    </div>
  );

  const open     = incidents.filter(i => i.status === "open").length;
  const acked    = incidents.filter(i => i.status === "ack").length;
  const resolved = incidents.filter(i => i.status === "resolved").length;

  return (
    <div className="p-6 flex gap-6 h-[calc(100vh-1rem)]">
      {/* Left panel — list */}
      <div className="flex-1 flex flex-col min-w-0 space-y-4">
        {/* Header */}
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-3">
            <Siren size={22} className="text-red-400" />
            <div>
              <h1 className="text-xl font-bold text-white">Incidents</h1>
              <p className="text-xs text-slate-400 mt-0.5">
                {open} open · {acked} acknowledged · {resolved} resolved
              </p>
            </div>
          </div>
          <button
            onClick={() => setShowCreate(s => !s)}
            className="flex items-center gap-2 px-3 py-1.5 rounded-lg bg-indigo-500
                       hover:bg-indigo-600 text-white text-sm font-medium"
          >
            <Plus size={14} />
            New Incident
          </button>
        </div>

        {/* Create form */}
        {showCreate && (
          <div className="bg-white/[0.03] border border-white/[0.08] rounded-xl p-4 space-y-3">
            <input
              value={newTitle}
              onChange={e => setNewTitle(e.target.value)}
              placeholder="Incident title…"
              className="w-full bg-white/[0.05] border border-white/[0.10] rounded-lg px-3 py-2
                         text-slate-200 text-sm focus:outline-none focus:border-indigo-500/50"
            />
            <div className="flex gap-3">
              <select
                value={newSeverity}
                onChange={e => setNewSeverity(e.target.value)}
                className="bg-white/[0.05] border border-white/[0.10] rounded-lg px-3 py-2
                           text-slate-200 text-sm focus:outline-none focus:border-indigo-500/50"
              >
                {["critical","high","medium","low"].map(s => (
                  <option key={s} value={s}>{s}</option>
                ))}
              </select>
              <button onClick={createIncident}
                className="px-4 py-2 rounded-lg bg-indigo-500 hover:bg-indigo-600 text-white text-sm font-medium">
                Create
              </button>
              <button onClick={() => setShowCreate(false)}
                className="px-4 py-2 rounded-lg bg-white/[0.05] text-slate-300 text-sm hover:bg-white/[0.08]">
                Cancel
              </button>
            </div>
          </div>
        )}

        {/* Filter bar */}
        <div className="flex items-center gap-2">
          <Filter size={14} className="text-slate-500" />
          {["", "open", "ack", "resolved"].map(s => (
            <button
              key={s}
              onClick={() => setStatusFilter(s)}
              className={`text-xs px-2.5 py-1 rounded-md transition-colors ${
                statusFilter === s
                  ? "bg-indigo-500/20 text-indigo-400"
                  : "text-slate-500 hover:text-slate-300 hover:bg-white/[0.04]"
              }`}
            >
              {s === "" ? "All" : s.charAt(0).toUpperCase() + s.slice(1)}
            </button>
          ))}
        </div>

        {/* Incident list */}
        <div className="flex-1 overflow-y-auto space-y-2 pr-1">
          {loading ? (
            <p className="text-slate-500 text-sm">Loading incidents…</p>
          ) : incidents.length === 0 ? (
            <div className="text-center py-16">
              <ShieldCheck size={36} className="mx-auto text-slate-700 mb-3" />
              <p className="text-slate-500 text-sm">No incidents {statusFilter ? `with status "${statusFilter}"` : "found"}.</p>
            </div>
          ) : (
            incidents.map(inc => (
              <button
                key={inc.id}
                onClick={() => setSelected(inc === selected ? null : inc)}
                className={`w-full text-left p-4 rounded-xl border transition-colors ${
                  selected?.id === inc.id
                    ? "bg-white/[0.06] border-indigo-500/40"
                    : "bg-white/[0.03] border-white/[0.06] hover:bg-white/[0.05]"
                }`}
              >
                <div className="flex items-start justify-between gap-3">
                  <div className="flex-1 min-w-0">
                    <p className="text-sm text-white font-medium leading-snug truncate">{inc.title}</p>
                    <div className="flex items-center gap-2 mt-1.5">
                      <span className={`text-[10px] px-1.5 py-0.5 rounded border font-semibold uppercase ${SEVERITY_COLORS[inc.severity] ?? SEVERITY_COLORS.low}`}>
                        {inc.severity}
                      </span>
                      <span className={`text-[10px] px-1.5 py-0.5 rounded font-medium ${STATUS_COLORS[inc.status] ?? STATUS_COLORS.open}`}>
                        {inc.status}
                      </span>
                      {inc.source === "sigma" && (
                        <span className="text-[10px] text-indigo-400">⚡ Sigma</span>
                      )}
                      {inc.external_ref && (
                        <span className="text-[10px] text-slate-500 flex items-center gap-0.5">
                          <ExternalLink size={9} />{inc.external_ref}
                        </span>
                      )}
                    </div>
                  </div>
                  <span className="text-[10px] text-slate-600 whitespace-nowrap">
                    {new Date(inc.created_at).toLocaleDateString()}
                  </span>
                </div>
              </button>
            ))
          )}
        </div>
      </div>

      {/* Right panel — detail */}
      {selected && (
        <div className="w-[340px] flex-shrink-0 bg-white/[0.03] border border-white/[0.06]
                        rounded-xl flex flex-col overflow-hidden">
          <div className="p-4 border-b border-white/[0.06]">
            <h2 className="text-sm font-semibold text-white leading-snug">{selected.title}</h2>
            <div className="flex items-center gap-2 mt-2">
              <span className={`text-[10px] px-1.5 py-0.5 rounded border font-semibold uppercase ${SEVERITY_COLORS[selected.severity] ?? ""}`}>
                {selected.severity}
              </span>
              <span className={`text-[10px] px-1.5 py-0.5 rounded ${STATUS_COLORS[selected.status] ?? ""}`}>
                {selected.status}
              </span>
            </div>
            {selected.external_ref && (
              <p className="text-xs text-slate-500 mt-2">
                External: <span className="text-indigo-400">{selected.external_ref}</span>
              </p>
            )}
          </div>

          {/* Actions */}
          <div className="p-3 border-b border-white/[0.06] flex gap-2">
            {selected.status === "open" && (
              <button
                onClick={() => action(selected.id, "ack")}
                className="flex-1 flex items-center justify-center gap-1.5 py-1.5 rounded-md
                           bg-amber-500/10 text-amber-400 text-xs font-medium hover:bg-amber-500/20"
              >
                <CheckCheck size={12} />
                Acknowledge
              </button>
            )}
            {selected.status !== "resolved" && (
              <button
                onClick={() => action(selected.id, "resolve")}
                className="flex-1 flex items-center justify-center gap-1.5 py-1.5 rounded-md
                           bg-emerald-500/10 text-emerald-400 text-xs font-medium hover:bg-emerald-500/20"
              >
                <ShieldCheck size={12} />
                Resolve
              </button>
            )}
          </div>

          {/* Notes */}
          <div className="flex-1 overflow-y-auto p-4">
            {selected.notes ? (
              <pre className="text-xs text-slate-400 whitespace-pre-wrap font-sans leading-relaxed">
                {selected.notes}
              </pre>
            ) : (
              <p className="text-xs text-slate-600">No notes yet.</p>
            )}
          </div>

          {/* Add note */}
          <div className="p-3 border-t border-white/[0.06] flex gap-2">
            <input
              value={note}
              onChange={e => setNote(e.target.value)}
              placeholder="Add a note…"
              className="flex-1 bg-white/[0.05] border border-white/[0.10] rounded-lg px-3 py-1.5
                         text-slate-200 text-xs focus:outline-none focus:border-indigo-500/50"
            />
            <button
              onClick={addNote}
              className="px-3 py-1.5 rounded-lg bg-indigo-500 hover:bg-indigo-600 text-white text-xs"
            >
              <MessageSquare size={13} />
            </button>
          </div>
        </div>
      )}
    </div>
  );
}
