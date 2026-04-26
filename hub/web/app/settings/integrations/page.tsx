"use client";

import { useCallback, useEffect, useState } from "react";
import {
  Plug, RefreshCw, CheckCircle, XCircle, ChevronDown, ChevronUp,
  Zap, BarChart2, Database, Activity,
} from "lucide-react";
import { clsx } from "clsx";
import {
  fetchIntegrations, upsertIntegration, deleteIntegration, testIntegration,
  fetchLicense,
} from "@/lib/api";
import type { SinkType, IntegrationConfig } from "@/lib/api";
import { EnterpriseGate } from "@/components/EnterpriseGate";

// ── Sink metadata ─────────────────────────────────────────────────────────────

interface SinkMeta {
  type:        SinkType;
  label:       string;
  description: string;
  icon:        React.ReactNode;
  docsUrl:     string;
  fields:      { key: string; label: string; placeholder: string; secret?: boolean; hint?: string }[];
}

const SINKS: SinkMeta[] = [
  {
    type:        "splunk",
    label:       "Splunk HEC",
    description: "Ship audit events to Splunk via HTTP Event Collector.",
    icon:        <Zap size={16} className="text-orange-400" />,
    docsUrl:     "https://docs.splunk.com/Documentation/Splunk/latest/Data/UsetheHTTPEventCollector",
    fields: [
      { key: "url",   label: "HEC URL",   placeholder: "https://splunk.company.com:8088",
        hint: "Base URL of your Splunk instance — the path /services/collector/event is appended automatically." },
      { key: "token", label: "HEC Token", placeholder: "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx", secret: true },
      { key: "index", label: "Index",     placeholder: "main (optional)" },
    ],
  },
  {
    type:        "elastic",
    label:       "Elasticsearch",
    description: "Forward events to Elasticsearch using the ECS schema and Bulk API.",
    icon:        <BarChart2 size={16} className="text-yellow-400" />,
    docsUrl:     "https://www.elastic.co/guide/en/elasticsearch/reference/current/index.html",
    fields: [
      { key: "url",     label: "Elasticsearch URL", placeholder: "https://elastic.company.com:9200" },
      { key: "api_key", label: "API Key (Base64)",  placeholder: "id:api_key_value", secret: true,
        hint: "Encoded Elasticsearch API key (id:api_key, Base64 encoded)." },
      { key: "index",   label: "Index",             placeholder: "netscope-audit (optional)" },
    ],
  },
  {
    type:        "datadog",
    label:       "Datadog Logs",
    description: "Push audit events to Datadog Logs ingestion API.",
    icon:        <Activity size={16} className="text-violet-400" />,
    docsUrl:     "https://docs.datadoghq.com/logs/",
    fields: [
      { key: "api_key", label: "API Key", placeholder: "Your Datadog API key", secret: true },
      { key: "site",    label: "Site",    placeholder: "datadoghq.com",
        hint: "datadoghq.com (US), datadoghq.eu (EU), us3.datadoghq.com, etc." },
    ],
  },
  {
    type:        "loki",
    label:       "Grafana Loki",
    description: "Stream audit logs to a Grafana Loki instance via push API.",
    icon:        <Database size={16} className="text-orange-300" />,
    docsUrl:     "https://grafana.com/docs/loki/latest/",
    fields: [
      { key: "url",       label: "Loki URL",   placeholder: "http://loki.company.com:3100" },
      { key: "tenant_id", label: "Tenant ID",  placeholder: "optional" },
      { key: "username",  label: "Username",   placeholder: "optional (basic auth)" },
      { key: "password",  label: "Password",   placeholder: "optional (basic auth)", secret: true },
    ],
  },
];

// ── Toggle switch ─────────────────────────────────────────────────────────────

function Toggle({ checked, onChange }: { checked: boolean; onChange: (v: boolean) => void }) {
  return (
    <button
      role="switch"
      aria-checked={checked}
      onClick={() => onChange(!checked)}
      className={clsx(
        "relative inline-flex h-5 w-9 shrink-0 cursor-pointer rounded-full border-2 border-transparent",
        "transition-colors duration-200 focus:outline-none",
        checked ? "bg-indigo-600" : "bg-slate-700",
      )}
    >
      <span className={clsx(
        "pointer-events-none inline-block h-4 w-4 rounded-full bg-white shadow",
        "transform transition duration-200",
        checked ? "translate-x-4" : "translate-x-0",
      )} />
    </button>
  );
}

// ── Sink card ─────────────────────────────────────────────────────────────────

interface SinkCardProps {
  meta:       SinkMeta;
  saved?:     IntegrationConfig;
  onSaved:    () => void;
}

function SinkCard({ meta, saved, onSaved }: SinkCardProps) {
  // Initialise form fields from saved config (secrets show as "***")
  const initFields = useCallback(() => {
    const init: Record<string, string> = {};
    for (const f of meta.fields) init[f.key] = saved?.config[f.key] ?? "";
    return init;
  }, [meta.fields, saved?.config]);

  const [expanded, setExpanded] = useState(false);
  const [enabled,  setEnabled]  = useState(saved?.enabled ?? false);
  const [fields,   setFields]   = useState<Record<string, string>>(initFields);
  const [saving,   setSaving]   = useState(false);
  const [testing,  setTesting]  = useState(false);
  const [saveErr,  setSaveErr]  = useState("");
  const [testRes,  setTestRes]  = useState<{ ok: boolean; msg: string } | null>(null);

  // Sync when saved prop changes
  useEffect(() => {
    setEnabled(saved?.enabled ?? false);
    setFields(initFields());
  }, [saved, initFields]);

  const handleSave = async () => {
    setSaving(true);
    setSaveErr("");
    setTestRes(null);
    try {
      await upsertIntegration(meta.type, { enabled, config: fields });
      onSaved();
    } catch (e) {
      setSaveErr(e instanceof Error ? e.message : "Save failed");
    } finally {
      setSaving(false);
    }
  };

  const handleDelete = async () => {
    if (!confirm(`Remove ${meta.label} integration?`)) return;
    try {
      await deleteIntegration(meta.type);
      onSaved();
    } catch (e) {
      setSaveErr(e instanceof Error ? e.message : "Delete failed");
    }
  };

  const handleTest = async () => {
    setTesting(true);
    setTestRes(null);
    try {
      const res = await testIntegration(meta.type, fields);
      setTestRes(res.ok
        ? { ok: true,  msg: `Connected — ${res.latency_ms}ms` }
        : { ok: false, msg: res.error ?? "Connection failed" });
    } catch (e) {
      setTestRes({ ok: false, msg: e instanceof Error ? e.message : "Test failed" });
    } finally {
      setTesting(false);
    }
  };

  const isConfigured = !!saved;

  return (
    <div className={clsx(
      "bg-[#0d0d1a] border rounded-xl transition-colors",
      expanded ? "border-indigo-500/20" : "border-white/[0.06]",
    )}>
      {/* Header row */}
      <div className="flex items-center gap-3 px-5 py-4">
        <div className="p-1.5 rounded-lg bg-white/[0.05]">{meta.icon}</div>
        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-2">
            <p className="text-sm font-semibold text-white">{meta.label}</p>
            {isConfigured && (
              <span className={clsx(
                "text-[9px] px-1.5 py-0.5 rounded font-semibold border leading-none",
                enabled
                  ? "text-emerald-400 bg-emerald-500/10 border-emerald-500/20"
                  : "text-slate-500 bg-slate-700/30 border-white/10",
              )}>
                {enabled ? "active" : "disabled"}
              </span>
            )}
          </div>
          <p className="text-xs text-slate-500 mt-0.5 truncate">{meta.description}</p>
          {saved?.last_shipped && (
            <p className="text-[10px] text-slate-600 mt-0.5">
              Last shipped: {new Date(saved.last_shipped).toLocaleString()}
            </p>
          )}
        </div>

        <Toggle checked={enabled} onChange={(v) => { setEnabled(v); setExpanded(true); }} />

        <button
          onClick={() => setExpanded(e => !e)}
          className="p-1 text-slate-500 hover:text-slate-300 transition-colors"
          title={expanded ? "Collapse" : "Configure"}
        >
          {expanded ? <ChevronUp size={14} /> : <ChevronDown size={14} />}
        </button>
      </div>

      {/* Config panel */}
      {expanded && (
        <div className="border-t border-white/[0.06] px-5 py-4 space-y-4">
          <div className="space-y-3">
            {meta.fields.map(f => (
              <div key={f.key}>
                <label className="text-xs text-slate-400 mb-1 block">{f.label}</label>
                <input
                  type={f.secret ? "password" : "text"}
                  value={fields[f.key] ?? ""}
                  onChange={e => setFields(prev => ({ ...prev, [f.key]: e.target.value }))}
                  placeholder={fields[f.key] === "***" ? "unchanged (secret hidden)" : f.placeholder}
                  className="w-full bg-white/[0.04] border border-white/10 rounded-lg px-3 py-2
                             text-xs text-white placeholder:text-slate-600
                             focus:outline-none focus:border-indigo-500/50"
                />
                {f.hint && (
                  <p className="text-[10px] text-slate-600 mt-1">{f.hint}</p>
                )}
              </div>
            ))}
          </div>

          {/* Test result */}
          {testRes && (
            <div className={clsx(
              "flex items-center gap-2 text-xs px-3 py-2 rounded-lg border",
              testRes.ok
                ? "text-emerald-400 bg-emerald-500/10 border-emerald-500/20"
                : "text-red-400 bg-red-500/10 border-red-500/20",
            )}>
              {testRes.ok
                ? <CheckCircle size={12} />
                : <XCircle size={12} />}
              {testRes.msg}
            </div>
          )}

          {saveErr && (
            <p className="text-xs text-red-400">{saveErr}</p>
          )}

          {/* Actions */}
          <div className="flex items-center gap-2 pt-1">
            <button
              onClick={handleTest}
              disabled={testing}
              className="px-3 py-1.5 rounded-lg text-xs border border-white/10 text-slate-300
                         hover:bg-white/[0.04] disabled:opacity-40 transition-colors"
            >
              {testing ? <RefreshCw size={11} className="animate-spin inline mr-1" /> : null}
              {testing ? "Testing…" : "Test connection"}
            </button>
            <button
              onClick={handleSave}
              disabled={saving}
              className="px-3 py-1.5 rounded-lg text-xs bg-indigo-600 hover:bg-indigo-500
                         text-white disabled:opacity-40 transition-colors"
            >
              {saving ? "Saving…" : "Save"}
            </button>
            {isConfigured && (
              <button
                onClick={handleDelete}
                className="ml-auto px-3 py-1.5 rounded-lg text-xs text-slate-500
                           hover:text-red-400 hover:bg-red-500/10 transition-colors"
              >
                Remove
              </button>
            )}
          </div>

          <p className="text-[10px] text-slate-600">
            Docs:{" "}
            <a href={meta.docsUrl} target="_blank" rel="noopener noreferrer"
               className="text-indigo-500 hover:text-indigo-400 underline">
              {meta.label} integration guide
            </a>
          </p>
        </div>
      )}
    </div>
  );
}

// ── Page ──────────────────────────────────────────────────────────────────────

export default function IntegrationsPage() {
  const [configs,  setConfigs]  = useState<IntegrationConfig[]>([]);
  const [loading,  setLoading]  = useState(true);
  const [locked,   setLocked]   = useState(false);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const [data, lic] = await Promise.all([
        fetchIntegrations().catch(() => ({ integrations: [] as IntegrationConfig[] })),
        fetchLicense(),
      ]);
      setConfigs(data.integrations ?? []);
      setLocked(lic.plan === "community");
    } catch { /* hub offline */ }
    finally { setLoading(false); }
  }, []);

  useEffect(() => { load(); }, [load]);

  const getSaved = (type: SinkType) =>
    configs.find(c => c.type === type);

  return (
    <div className="p-6 space-y-6">
      {/* Header */}
      <div className="flex items-center gap-3">
        <div className="p-2 rounded-lg bg-indigo-500/10">
          <Plug size={20} className="text-indigo-400" />
        </div>
        <div>
          <h1 className="text-lg font-semibold text-white">SIEM Integrations</h1>
          <p className="text-xs text-slate-400 mt-0.5">
            Forward audit events to your existing security stack
          </p>
        </div>
      </div>

      {locked ? (
        <EnterpriseGate
          feature="audit_export"
          title="Team plan required"
          description="SIEM integrations are available on the Team and Enterprise plans. Events are forwarded to Splunk, Elastic, Datadog, or Grafana Loki in real time."
        />
      ) : loading ? (
        <div className="flex items-center justify-center h-32">
          <RefreshCw size={20} className="animate-spin text-slate-600" />
        </div>
      ) : (
        <div className="space-y-3">
          <p className="text-xs text-slate-500">
            Enable one or more sinks. New audit events are shipped in batches every 30 seconds.
            Credentials are encrypted at rest and redacted from the UI after saving.
          </p>

          {SINKS.map(meta => (
            <SinkCard
              key={meta.type}
              meta={meta}
              saved={getSaved(meta.type)}
              onSaved={load}
            />
          ))}
        </div>
      )}
    </div>
  );
}
