"use client";

import { useEffect, useState } from "react";
import { Cloud, Plus, Trash2, Play, CheckCircle, XCircle, Clock, ChevronDown, ChevronUp } from "lucide-react";

interface CloudSource {
  id: string;
  provider: string;
  name: string;
  enabled: boolean;
  last_pulled?: string;
  error_msg?: string;
  created_at: string;
}

interface PullLog {
  id: string;
  rows_ingested: number;
  pulled_at: string;
  duration_ms: number;
  error?: string;
}

const PROVIDER_LABELS: Record<string, { label: string; color: string; tier: string }> = {
  aws:   { label: "AWS",   color: "text-orange-400", tier: "Community" },
  gcp:   { label: "GCP",   color: "text-blue-400",   tier: "Enterprise" },
  azure: { label: "Azure", color: "text-sky-400",     tier: "Enterprise" },
};

export default function CloudPage() {
  const [sources, setSources]         = useState<CloudSource[]>([]);
  const [loading, setLoading]         = useState(true);
  const [showForm, setShowForm]       = useState(false);
  const [expandedLog, setExpandedLog] = useState<string | null>(null);
  const [logData, setLogData]         = useState<Record<string, PullLog[]>>({});
  const [form, setForm] = useState({
    provider: "aws", name: "", region: "", accessKeyId: "",
    secretAccessKey: "", s3Bucket: "", s3Prefix: "", enabled: true,
  });

  const fetchSources = async () => {
    setLoading(true);
    try {
      const r = await fetch("/api/v1/cloud/sources");
      const d = await r.json();
      setSources(d.sources ?? []);
    } finally {
      setLoading(false);
    }
  };

  const fetchLog = async (id: string) => {
    if (expandedLog === id) { setExpandedLog(null); return; }
    const r = await fetch(`/api/v1/cloud/sources/${id}/log`);
    const d = await r.json();
    setLogData(prev => ({ ...prev, [id]: d.log ?? [] }));
    setExpandedLog(id);
  };

  const createSource = async () => {
    const config: Record<string, string> = {
      region: form.region,
      access_key_id: form.accessKeyId,
      secret_access_key: form.secretAccessKey,
    };
    if (form.s3Bucket) config.s3_bucket = form.s3Bucket;
    if (form.s3Prefix) config.s3_prefix = form.s3Prefix;

    await fetch("/api/v1/cloud/sources", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        provider: form.provider,
        name: form.name,
        config,
        enabled: form.enabled,
      }),
    });
    setShowForm(false);
    fetchSources();
  };

  const toggleEnabled = async (src: CloudSource) => {
    await fetch(`/api/v1/cloud/sources/${src.id}`, {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ enabled: !src.enabled }),
    });
    fetchSources();
  };

  const deleteSource = async (id: string) => {
    if (!confirm("Remove this cloud source?")) return;
    await fetch(`/api/v1/cloud/sources/${id}`, { method: "DELETE" });
    fetchSources();
  };

  useEffect(() => { fetchSources(); }, []);

  return (
    <div className="p-6 space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-3">
          <Cloud size={22} className="text-indigo-400" />
          <div>
            <h1 className="text-xl font-bold text-white">Cloud Flow Sources</h1>
            <p className="text-xs text-slate-400 mt-0.5">
              Ingest VPC Flow Logs from AWS, GCP, and Azure without installing agents
            </p>
          </div>
        </div>
        <button
          onClick={() => setShowForm(s => !s)}
          className="flex items-center gap-2 px-3 py-1.5 rounded-lg bg-indigo-500
                     hover:bg-indigo-600 text-white text-sm font-medium transition-colors"
        >
          <Plus size={15} />
          Add Source
        </button>
      </div>

      {/* Tier info */}
      <div className="flex gap-3 text-xs">
        <span className="px-2 py-1 rounded bg-emerald-500/10 text-emerald-400 border border-emerald-500/20">
          ✓ AWS — Community
        </span>
        <span className="px-2 py-1 rounded bg-amber-500/10 text-amber-400 border border-amber-500/20">
          ⬆ GCP + Azure — Enterprise
        </span>
      </div>

      {/* Add form */}
      {showForm && (
        <div className="bg-white/[0.03] border border-white/[0.08] rounded-xl p-5 space-y-4">
          <h2 className="text-sm font-semibold text-slate-300">New Cloud Source</h2>
          <div className="grid grid-cols-2 gap-4">
            <div>
              <label className="text-xs text-slate-400 mb-1 block">Provider</label>
              <select
                value={form.provider}
                onChange={e => setForm(f => ({ ...f, provider: e.target.value }))}
                className="w-full bg-white/[0.05] border border-white/[0.10] rounded-lg px-3 py-2
                           text-slate-200 text-sm focus:outline-none focus:border-indigo-500/50"
              >
                <option value="aws">AWS VPC Flow Logs</option>
                <option value="gcp">GCP VPC Flow Logs (Enterprise)</option>
                <option value="azure">Azure NSG Flow Logs (Enterprise)</option>
              </select>
            </div>
            <div>
              <label className="text-xs text-slate-400 mb-1 block">Name</label>
              <input
                value={form.name}
                onChange={e => setForm(f => ({ ...f, name: e.target.value }))}
                placeholder="e.g. prod-vpc-us-east-1"
                className="w-full bg-white/[0.05] border border-white/[0.10] rounded-lg px-3 py-2
                           text-slate-200 text-sm focus:outline-none focus:border-indigo-500/50"
              />
            </div>
            <div>
              <label className="text-xs text-slate-400 mb-1 block">Region</label>
              <input
                value={form.region}
                onChange={e => setForm(f => ({ ...f, region: e.target.value }))}
                placeholder="us-east-1"
                className="w-full bg-white/[0.05] border border-white/[0.10] rounded-lg px-3 py-2
                           text-slate-200 text-sm focus:outline-none focus:border-indigo-500/50"
              />
            </div>
            <div>
              <label className="text-xs text-slate-400 mb-1 block">S3 Bucket</label>
              <input
                value={form.s3Bucket}
                onChange={e => setForm(f => ({ ...f, s3Bucket: e.target.value }))}
                placeholder="my-vpc-flow-logs"
                className="w-full bg-white/[0.05] border border-white/[0.10] rounded-lg px-3 py-2
                           text-slate-200 text-sm focus:outline-none focus:border-indigo-500/50"
              />
            </div>
            <div>
              <label className="text-xs text-slate-400 mb-1 block">Access Key ID</label>
              <input
                value={form.accessKeyId}
                onChange={e => setForm(f => ({ ...f, accessKeyId: e.target.value }))}
                className="w-full bg-white/[0.05] border border-white/[0.10] rounded-lg px-3 py-2
                           text-slate-200 text-sm focus:outline-none focus:border-indigo-500/50"
              />
            </div>
            <div>
              <label className="text-xs text-slate-400 mb-1 block">Secret Access Key</label>
              <input
                type="password"
                value={form.secretAccessKey}
                onChange={e => setForm(f => ({ ...f, secretAccessKey: e.target.value }))}
                className="w-full bg-white/[0.05] border border-white/[0.10] rounded-lg px-3 py-2
                           text-slate-200 text-sm focus:outline-none focus:border-indigo-500/50"
              />
            </div>
          </div>
          <div className="flex gap-3 pt-1">
            <button
              onClick={createSource}
              className="px-4 py-2 rounded-lg bg-indigo-500 hover:bg-indigo-600 text-white text-sm font-medium"
            >
              Create Source
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

      {/* Source list */}
      {loading && sources.length === 0 ? (
        <p className="text-slate-500 text-sm">Loading sources…</p>
      ) : sources.length === 0 ? (
        <div className="bg-white/[0.03] border border-white/[0.06] rounded-xl p-10 text-center">
          <Cloud size={36} className="mx-auto text-slate-600 mb-3" />
          <p className="text-slate-400 text-sm">No cloud flow sources configured.</p>
          <p className="text-slate-500 text-xs mt-1">
            Add an AWS, GCP, or Azure source to ingest VPC flow logs without agents.
          </p>
        </div>
      ) : (
        <div className="space-y-3">
          {sources.map(src => {
            const p = PROVIDER_LABELS[src.provider] ?? { label: src.provider, color: "text-slate-400", tier: "" };
            const hasError = !!src.error_msg;
            const logs = logData[src.id] ?? [];
            const expanded = expandedLog === src.id;

            return (
              <div key={src.id} className="bg-white/[0.03] border border-white/[0.06] rounded-xl overflow-hidden">
                <div className="flex items-center gap-4 p-4">
                  {/* Provider badge */}
                  <span className={`text-sm font-bold ${p.color} min-w-[50px]`}>{p.label}</span>

                  {/* Name + status */}
                  <div className="flex-1 min-w-0">
                    <p className="text-sm font-medium text-white truncate">{src.name}</p>
                    {src.last_pulled && (
                      <p className="text-xs text-slate-500 flex items-center gap-1 mt-0.5">
                        <Clock size={11} />
                        Last pull: {new Date(src.last_pulled).toLocaleString()}
                      </p>
                    )}
                    {hasError && (
                      <p className="text-xs text-red-400 mt-0.5 truncate">⚠ {src.error_msg}</p>
                    )}
                  </div>

                  {/* Status indicator */}
                  {src.enabled
                    ? <CheckCircle size={16} className="text-emerald-400 flex-shrink-0" />
                    : <XCircle    size={16} className="text-slate-600   flex-shrink-0" />
                  }

                  {/* Actions */}
                  <div className="flex items-center gap-2">
                    <button
                      onClick={() => toggleEnabled(src)}
                      className={`text-xs px-2.5 py-1 rounded-md font-medium transition-colors ${
                        src.enabled
                          ? "bg-slate-700/50 text-slate-300 hover:bg-slate-700"
                          : "bg-emerald-500/10 text-emerald-400 hover:bg-emerald-500/20"
                      }`}
                    >
                      {src.enabled ? "Disable" : "Enable"}
                    </button>
                    <button
                      onClick={() => fetchLog(src.id)}
                      className="text-xs px-2 py-1 rounded-md bg-white/[0.04] text-slate-400
                                 hover:text-slate-200 flex items-center gap-1"
                    >
                      {expanded ? <ChevronUp size={12} /> : <ChevronDown size={12} />}
                      Log
                    </button>
                    <button
                      onClick={() => deleteSource(src.id)}
                      className="p-1.5 rounded-md text-slate-600 hover:text-red-400 hover:bg-red-500/10 transition-colors"
                    >
                      <Trash2 size={14} />
                    </button>
                  </div>
                </div>

                {/* Pull log */}
                {expanded && (
                  <div className="border-t border-white/[0.06] px-4 py-3">
                    <p className="text-xs text-slate-500 mb-2">Last 50 pull runs</p>
                    {logs.length === 0 ? (
                      <p className="text-xs text-slate-600">No pull history yet.</p>
                    ) : (
                      <table className="w-full text-xs text-slate-400">
                        <thead>
                          <tr className="text-slate-600">
                            <th className="text-left pb-1">Time</th>
                            <th className="text-left pb-1">Rows</th>
                            <th className="text-left pb-1">Duration</th>
                            <th className="text-left pb-1">Status</th>
                          </tr>
                        </thead>
                        <tbody className="divide-y divide-white/[0.03]">
                          {logs.map(l => (
                            <tr key={l.id}>
                              <td className="py-1">{new Date(l.pulled_at).toLocaleString()}</td>
                              <td className="py-1">{l.rows_ingested.toLocaleString()}</td>
                              <td className="py-1">{l.duration_ms}ms</td>
                              <td className="py-1">
                                {l.error
                                  ? <span className="text-red-400">✗ {l.error.slice(0, 60)}</span>
                                  : <span className="text-emerald-400">✓ OK</span>
                                }
                              </td>
                            </tr>
                          ))}
                        </tbody>
                      </table>
                    )}
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
