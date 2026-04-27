"use client";

import { useEffect, useState } from "react";
import { FileBarChart2, Plus, Play, Download, Trash2, Clock, CheckCircle, XCircle, AlertTriangle } from "lucide-react";

interface ReportSchedule {
  id: string;
  name: string;
  framework: string;
  format: string;
  schedule: string;
  recipients: string[];
  enabled: boolean;
  last_sent?: string;
}

interface ReportRun {
  id: string;
  framework: string;
  format: string;
  rows: number;
  sent_at: string;
  error?: string;
}

const FRAMEWORK_LABELS: Record<string, string> = {
  "soc2":    "SOC 2 Type II",
  "pci-dss": "PCI-DSS v4.0",
  "hipaa":   "HIPAA Security Rule",
};

const STATUS_ICON: Record<string, string> = {
  pass: "✓",
  warn: "⚠",
  fail: "✗",
  info: "ℹ",
};

export default function ComplianceReportsPage() {
  const [schedules, setSchedules] = useState<ReportSchedule[]>([]);
  const [history, setHistory]     = useState<Record<string, ReportRun[]>>({});
  const [loading, setLoading]     = useState(true);
  const [enterprise, setEnterprise] = useState(true);
  const [showForm, setShowForm]   = useState(false);
  const [runResult, setRunResult] = useState<any>(null);
  const [running, setRunning]     = useState<string | null>(null);
  const [form, setForm] = useState({
    name: "", framework: "soc2", format: "pdf", schedule: "weekly",
    recipients: "", enabled: true,
  });

  const fetchSchedules = async () => {
    setLoading(true);
    try {
      const r = await fetch("/api/v1/enterprise/compliance/reports");
      if (r.status === 402) { setEnterprise(false); return; }
      const d = await r.json();
      setSchedules(d.schedules ?? []);
    } finally {
      setLoading(false);
    }
  };

  const fetchHistory = async (id: string) => {
    const r = await fetch(`/api/v1/enterprise/compliance/reports/${id}/history`);
    const d = await r.json();
    setHistory(prev => ({ ...prev, [id]: d.runs ?? [] }));
  };

  const createSchedule = async () => {
    await fetch("/api/v1/enterprise/compliance/reports", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        ...form,
        recipients: form.recipients.split(",").map(s => s.trim()).filter(Boolean),
      }),
    });
    setShowForm(false);
    fetchSchedules();
  };

  const runNow = async (id: string) => {
    setRunning(id);
    setRunResult(null);
    try {
      const r = await fetch(`/api/v1/enterprise/compliance/reports/${id}/run`, { method: "POST" });
      const d = await r.json();
      setRunResult(d);
      fetchHistory(id);
    } finally {
      setRunning(null);
    }
  };

  const downloadPreview = (id: string) => {
    window.open(`/api/v1/enterprise/compliance/reports/${id}/preview`, "_blank");
  };

  const deleteSchedule = async (id: string) => {
    if (!confirm("Delete this report schedule?")) return;
    await fetch(`/api/v1/enterprise/compliance/reports/${id}`, { method: "DELETE" });
    fetchSchedules();
  };

  useEffect(() => {
    fetchSchedules();
  }, []);

  if (!enterprise) return (
    <div className="p-6">
      <div className="max-w-md mx-auto mt-20 text-center">
        <FileBarChart2 size={40} className="mx-auto text-slate-600 mb-4" />
        <h2 className="text-lg font-semibold text-white mb-2">Compliance Reports</h2>
        <p className="text-slate-400 text-sm mb-4">
          SOC 2, PCI-DSS, and HIPAA scheduled reports require Enterprise plan.
        </p>
        <a href="/settings/license"
           className="px-4 py-2 rounded-lg bg-indigo-500 hover:bg-indigo-600 text-white text-sm font-medium">
          Upgrade to Enterprise
        </a>
      </div>
    </div>
  );

  return (
    <div className="p-6 space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-3">
          <FileBarChart2 size={22} className="text-indigo-400" />
          <div>
            <h1 className="text-xl font-bold text-white">Compliance Reports</h1>
            <p className="text-xs text-slate-400 mt-0.5">
              Scheduled SOC 2 · PCI-DSS · HIPAA evidence reports with PDF/CSV export
            </p>
          </div>
        </div>
        <button
          onClick={() => setShowForm(s => !s)}
          className="flex items-center gap-2 px-3 py-1.5 rounded-lg bg-indigo-500
                     hover:bg-indigo-600 text-white text-sm font-medium"
        >
          <Plus size={14} />
          New Schedule
        </button>
      </div>

      {/* Add form */}
      {showForm && (
        <div className="bg-white/[0.03] border border-white/[0.08] rounded-xl p-5 space-y-4">
          <h2 className="text-sm font-semibold text-slate-300">New Report Schedule</h2>
          <div className="grid grid-cols-2 gap-4">
            <div>
              <label className="text-xs text-slate-400 mb-1 block">Schedule Name</label>
              <input
                value={form.name}
                onChange={e => setForm(f => ({ ...f, name: e.target.value }))}
                placeholder="Weekly SOC 2 Report"
                className="w-full bg-white/[0.05] border border-white/[0.10] rounded-lg px-3 py-2
                           text-slate-200 text-sm focus:outline-none focus:border-indigo-500/50"
              />
            </div>
            <div>
              <label className="text-xs text-slate-400 mb-1 block">Framework</label>
              <select
                value={form.framework}
                onChange={e => setForm(f => ({ ...f, framework: e.target.value }))}
                className="w-full bg-white/[0.05] border border-white/[0.10] rounded-lg px-3 py-2
                           text-slate-200 text-sm focus:outline-none focus:border-indigo-500/50"
              >
                <option value="soc2">SOC 2 Type II</option>
                <option value="pci-dss">PCI-DSS v4.0</option>
                <option value="hipaa">HIPAA Security Rule</option>
              </select>
            </div>
            <div>
              <label className="text-xs text-slate-400 mb-1 block">Format</label>
              <select
                value={form.format}
                onChange={e => setForm(f => ({ ...f, format: e.target.value }))}
                className="w-full bg-white/[0.05] border border-white/[0.10] rounded-lg px-3 py-2
                           text-slate-200 text-sm focus:outline-none focus:border-indigo-500/50"
              >
                <option value="pdf">PDF</option>
                <option value="csv">CSV</option>
              </select>
            </div>
            <div>
              <label className="text-xs text-slate-400 mb-1 block">Schedule</label>
              <select
                value={form.schedule}
                onChange={e => setForm(f => ({ ...f, schedule: e.target.value }))}
                className="w-full bg-white/[0.05] border border-white/[0.10] rounded-lg px-3 py-2
                           text-slate-200 text-sm focus:outline-none focus:border-indigo-500/50"
              >
                <option value="daily">Daily</option>
                <option value="weekly">Weekly</option>
                <option value="monthly">Monthly</option>
              </select>
            </div>
            <div className="col-span-2">
              <label className="text-xs text-slate-400 mb-1 block">Recipients (comma-separated emails)</label>
              <input
                value={form.recipients}
                onChange={e => setForm(f => ({ ...f, recipients: e.target.value }))}
                placeholder="ciso@company.com, auditor@company.com"
                className="w-full bg-white/[0.05] border border-white/[0.10] rounded-lg px-3 py-2
                           text-slate-200 text-sm focus:outline-none focus:border-indigo-500/50"
              />
            </div>
          </div>
          <div className="flex gap-3">
            <button
              onClick={createSchedule}
              className="px-4 py-2 rounded-lg bg-indigo-500 hover:bg-indigo-600 text-white text-sm font-medium"
            >
              Create Schedule
            </button>
            <button
              onClick={() => setShowForm(false)}
              className="px-4 py-2 rounded-lg bg-white/[0.05] text-slate-300 text-sm hover:bg-white/[0.08]"
            >
              Cancel
            </button>
          </div>
        </div>
      )}

      {/* Inline run result */}
      {runResult && (
        <div className="bg-white/[0.03] border border-white/[0.08] rounded-xl p-5">
          <div className="flex items-center justify-between mb-4">
            <h3 className="text-sm font-semibold text-white">
              {FRAMEWORK_LABELS[runResult.framework] ?? runResult.framework} — Run Result
            </h3>
            <button onClick={() => setRunResult(null)} className="text-slate-500 hover:text-slate-300 text-xs">✕ Close</button>
          </div>
          <div className="space-y-2 max-h-80 overflow-y-auto">
            {(runResult.checks ?? []).map((check: any) => (
              <div key={check.check} className="flex items-start gap-3 py-2 border-b border-white/[0.04]">
                <span className={`text-sm font-bold ${
                  check.status === "pass" ? "text-emerald-400" :
                  check.status === "fail" ? "text-red-400" :
                  check.status === "warn" ? "text-amber-400" :
                  "text-blue-400"
                }`}>{STATUS_ICON[check.status] ?? "?"}</span>
                <div className="min-w-0">
                  <p className="text-xs text-slate-200 font-medium">{check.check}</p>
                  <p className="text-[10px] text-slate-500">{check.row_count} findings</p>
                </div>
              </div>
            ))}
          </div>
        </div>
      )}

      {/* Schedule list */}
      {loading && schedules.length === 0 ? (
        <p className="text-slate-500 text-sm">Loading schedules…</p>
      ) : schedules.length === 0 ? (
        <div className="bg-white/[0.03] border border-white/[0.06] rounded-xl p-10 text-center">
          <FileBarChart2 size={36} className="mx-auto text-slate-600 mb-3" />
          <p className="text-slate-400 text-sm">No report schedules configured.</p>
          <p className="text-slate-500 text-xs mt-1">
            Create a schedule to automatically generate and email SOC 2, PCI-DSS, or HIPAA evidence reports.
          </p>
        </div>
      ) : (
        <div className="space-y-4">
          {schedules.map(sched => {
            const runs = history[sched.id] ?? [];
            return (
              <div key={sched.id} className="bg-white/[0.03] border border-white/[0.06] rounded-xl overflow-hidden">
                <div className="flex items-center gap-4 p-4">
                  {/* Framework badge */}
                  <div className="flex-shrink-0">
                    <span className="text-xs font-semibold text-indigo-400 bg-indigo-500/10 px-2 py-1 rounded">
                      {FRAMEWORK_LABELS[sched.framework] ?? sched.framework}
                    </span>
                  </div>

                  {/* Info */}
                  <div className="flex-1 min-w-0">
                    <p className="text-sm font-medium text-white">{sched.name}</p>
                    <div className="flex items-center gap-3 mt-1 text-xs text-slate-500">
                      <span>{sched.format.toUpperCase()} · {sched.schedule}</span>
                      {sched.last_sent && (
                        <span className="flex items-center gap-1">
                          <Clock size={11} />
                          Last: {new Date(sched.last_sent).toLocaleDateString()}
                        </span>
                      )}
                      {sched.recipients?.length > 0 && (
                        <span>{sched.recipients.length} recipient(s)</span>
                      )}
                    </div>
                  </div>

                  {/* Status */}
                  {sched.enabled
                    ? <CheckCircle size={15} className="text-emerald-400 flex-shrink-0" />
                    : <XCircle    size={15} className="text-slate-600   flex-shrink-0" />
                  }

                  {/* Actions */}
                  <div className="flex items-center gap-2">
                    <button
                      onClick={() => runNow(sched.id)}
                      disabled={running === sched.id}
                      className="flex items-center gap-1.5 text-xs px-2.5 py-1.5 rounded-md
                                 bg-indigo-500/10 text-indigo-400 hover:bg-indigo-500/20 disabled:opacity-50"
                    >
                      <Play size={11} />
                      {running === sched.id ? "Running…" : "Run Now"}
                    </button>
                    <button
                      onClick={() => downloadPreview(sched.id)}
                      className="flex items-center gap-1.5 text-xs px-2.5 py-1.5 rounded-md
                                 bg-white/[0.04] text-slate-400 hover:text-slate-200"
                    >
                      <Download size={11} />
                      Preview
                    </button>
                    <button
                      onClick={() => { fetchHistory(sched.id); }}
                      className="text-xs px-2.5 py-1.5 rounded-md bg-white/[0.04] text-slate-400 hover:text-slate-200"
                    >
                      History
                    </button>
                    <button
                      onClick={() => deleteSchedule(sched.id)}
                      className="p-1.5 rounded-md text-slate-600 hover:text-red-400 hover:bg-red-500/10"
                    >
                      <Trash2 size={13} />
                    </button>
                  </div>
                </div>

                {/* Run history */}
                {runs.length > 0 && (
                  <div className="border-t border-white/[0.06] px-4 py-3">
                    <p className="text-xs text-slate-500 mb-2">Run history</p>
                    <table className="w-full text-xs text-slate-400">
                      <thead>
                        <tr className="text-slate-600">
                          <th className="text-left pb-1">Time</th>
                          <th className="text-left pb-1">Findings</th>
                          <th className="text-left pb-1">Status</th>
                        </tr>
                      </thead>
                      <tbody className="divide-y divide-white/[0.03]">
                        {runs.map(run => (
                          <tr key={run.id}>
                            <td className="py-1">{new Date(run.sent_at).toLocaleString()}</td>
                            <td className="py-1">{run.rows.toLocaleString()}</td>
                            <td className="py-1">
                              {run.error
                                ? <span className="flex items-center gap-1 text-red-400"><AlertTriangle size={10} />{run.error.slice(0, 50)}</span>
                                : <span className="text-emerald-400">✓ OK</span>
                              }
                            </td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>
                )}
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}
