"use client";

import { useState, useEffect, useCallback } from "react";
import {
  fetchAlertRules,
  fetchAlertEvents,
  createAlertRule,
  updateAlertRule,
  deleteAlertRule,
  testAlertDelivery,
  type AlertRule,
  type AlertEvent,
  type CreateAlertRuleRequest,
  type AlertMetric,
  type AlertCondition,
  type AlertIntegrationType,
} from "@/lib/api";
import {
  Bell,
  Plus,
  Trash2,
  ToggleLeft,
  ToggleRight,
  RefreshCw,
  X,
  CheckCircle2,
  XCircle,
  Copy,
  Send,
} from "lucide-react";
import { clsx } from "clsx";

// ── Helpers ────────────────────────────────────────────────────────────────────

const METRIC_LABELS: Record<AlertMetric, string> = {
  flows_per_minute:    "Flows / minute",
  http_error_rate:     "HTTP error rate (%)",
  dns_nxdomain_rate:   "DNS NXDOMAIN rate (%)",
  anomaly_flow_rate:   "Anomaly — flow rate (σ)",
  anomaly_http_latency:"Anomaly — HTTP latency (σ)",
};

const INTEGRATION_LABELS: Record<AlertIntegrationType, string> = {
  webhook:   "Generic webhook",
  slack:     "Slack",
  pagerduty: "PagerDuty",
  opsgenie:  "OpsGenie",
  teams:     "Microsoft Teams",
  email:     "Email (SMTP)",
};

const INTEGRATION_PLACEHOLDER: Record<AlertIntegrationType, string> = {
  webhook:   "https://example.com/hooks/…",
  slack:     "https://hooks.slack.com/services/T…/B…/…",
  pagerduty: "Routing key (e.g. abc123…)",
  opsgenie:  "API key (e.g. xxxxxxxx-…)",
  teams:     "https://outlook.office.com/webhook/…",
  email:     "recipient@example.com",
};

const CONDITION_LABELS: Record<AlertCondition, string> = {
  gt: "greater than (>)",
  lt: "less than (<)",
};

function fmtDate(iso: string) {
  try {
    return new Date(iso).toLocaleString();
  } catch {
    return iso;
  }
}

// ── Create-Rule Modal ──────────────────────────────────────────────────────────

interface ModalProps {
  onClose: () => void;
  onCreated: (rule: AlertRule) => void;
}

function CreateRuleModal({ onClose, onCreated }: ModalProps) {
  const [form, setForm] = useState<CreateAlertRuleRequest>({
    name:             "",
    metric:           "flows_per_minute",
    condition:        "gt",
    threshold:        100,
    window_minutes:   5,
    cooldown_minutes: 15,
    integration_type: "webhook",
    webhook_url:      "",
    email_to:         "",
  });
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const set = <K extends keyof CreateAlertRuleRequest>(
    key: K,
    val: CreateAlertRuleRequest[K],
  ) => setForm((f) => ({ ...f, [key]: val }));

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    if (!form.name.trim()) { setError("Rule name is required"); return; }
    setSubmitting(true);
    setError(null);
    try {
      const rule = await createAlertRule(form);
      onCreated(rule);
    } catch (err) {
      setError(String(err));
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center">
      {/* Backdrop */}
      <div
        className="absolute inset-0 bg-black/60 backdrop-blur-sm"
        onClick={onClose}
      />
      {/* Dialog */}
      <div className="relative w-full max-w-md mx-4 rounded-2xl bg-[#0d0d1a]
                      border border-white/[0.08] shadow-2xl p-6 space-y-5">
        <div className="flex items-center justify-between">
          <h2 className="text-base font-semibold text-white">New alert rule</h2>
          <button
            onClick={onClose}
            className="p-1.5 rounded-md text-slate-500 hover:text-white hover:bg-white/[0.06]"
          >
            <X size={16} />
          </button>
        </div>

        <form onSubmit={submit} className="space-y-4">
          {/* Name */}
          <Field label="Rule name">
            <input
              type="text"
              placeholder="e.g. High traffic alert"
              value={form.name}
              onChange={(e) => set("name", e.target.value)}
              className={inputCls}
            />
          </Field>

          {/* Metric */}
          <Field label="Metric">
            <select
              value={form.metric}
              onChange={(e) => set("metric", e.target.value as AlertMetric)}
              className={inputCls}
            >
              {(Object.keys(METRIC_LABELS) as AlertMetric[]).map((m) => (
                <option key={m} value={m}>{METRIC_LABELS[m]}</option>
              ))}
            </select>
          </Field>

          {/* Condition + threshold */}
          <div className="grid grid-cols-2 gap-3">
            <Field label="Condition">
              <select
                value={form.condition}
                onChange={(e) => set("condition", e.target.value as AlertCondition)}
                className={inputCls}
              >
                {(Object.keys(CONDITION_LABELS) as AlertCondition[]).map((c) => (
                  <option key={c} value={c}>{CONDITION_LABELS[c]}</option>
                ))}
              </select>
            </Field>
            <Field label="Threshold">
              <input
                type="number"
                min={0}
                step={0.1}
                value={form.threshold}
                onChange={(e) => set("threshold", parseFloat(e.target.value) || 0)}
                className={inputCls}
              />
            </Field>
          </div>

          {/* Window + cooldown */}
          <div className="grid grid-cols-2 gap-3">
            <Field label="Window (minutes)">
              <input
                type="number"
                min={1}
                max={60}
                value={form.window_minutes}
                onChange={(e) => set("window_minutes", parseInt(e.target.value) || 5)}
                className={inputCls}
              />
            </Field>
            <Field label="Cooldown (minutes)">
              <input
                type="number"
                min={1}
                max={1440}
                value={form.cooldown_minutes}
                onChange={(e) => set("cooldown_minutes", parseInt(e.target.value) || 15)}
                className={inputCls}
              />
            </Field>
          </div>

          {/* Integration type */}
          <Field label="Integration">
            <select
              value={form.integration_type ?? "webhook"}
              onChange={(e) => set("integration_type", e.target.value as AlertIntegrationType)}
              className={inputCls}
            >
              {(Object.keys(INTEGRATION_LABELS) as AlertIntegrationType[]).map((t) => (
                <option key={t} value={t}>{INTEGRATION_LABELS[t]}</option>
              ))}
            </select>
          </Field>

          {/* Destination — label/placeholder adapts to integration type */}
          {form.integration_type === "email" ? (
            <Field label="Recipient email">
              <input
                type="email"
                placeholder={INTEGRATION_PLACEHOLDER["email"]}
                value={form.email_to ?? ""}
                onChange={(e) => set("email_to", e.target.value)}
                className={inputCls}
              />
            </Field>
          ) : (
            <Field label={
              form.integration_type === "pagerduty" ? "Routing key" :
              form.integration_type === "opsgenie"  ? "API key" :
              "Webhook URL"
            }>
              <input
                type={form.integration_type === "pagerduty" || form.integration_type === "opsgenie" ? "text" : "url"}
                placeholder={INTEGRATION_PLACEHOLDER[form.integration_type ?? "webhook"]}
                value={form.webhook_url}
                onChange={(e) => set("webhook_url", e.target.value)}
                className={inputCls}
              />
            </Field>
          )}

          {error && (
            <p className="text-sm text-red-400 bg-red-900/20 border border-red-500/30 rounded px-3 py-2">
              {error}
            </p>
          )}

          <div className="flex justify-end gap-3 pt-1">
            <button
              type="button"
              onClick={onClose}
              className="px-4 py-2 rounded-lg text-sm text-slate-400 hover:text-white
                         hover:bg-white/[0.04] transition-colors"
            >
              Cancel
            </button>
            <button
              type="submit"
              disabled={submitting}
              className="px-4 py-2 rounded-lg text-sm font-medium bg-indigo-600
                         hover:bg-indigo-500 active:bg-indigo-700 text-white transition-colors
                         disabled:opacity-50 disabled:cursor-not-allowed"
            >
              {submitting ? "Creating…" : "Create rule"}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}

const inputCls =
  "w-full bg-[#12121f] border border-white/[0.08] rounded-lg px-3 py-2 text-sm " +
  "text-slate-200 placeholder-slate-600 focus:outline-none focus:border-indigo-500/50 " +
  "focus:ring-1 focus:ring-indigo-500/30";

function Field({
  label,
  children,
}: {
  label: string;
  children: React.ReactNode;
}) {
  return (
    <div className="space-y-1.5">
      <label className="block text-xs font-medium text-slate-400">{label}</label>
      {children}
    </div>
  );
}

// ── Rule Row ───────────────────────────────────────────────────────────────────

function RuleRow({
  rule,
  onToggle,
  onDelete,
}: {
  rule: AlertRule;
  onToggle: (id: string, enabled: boolean) => void;
  onDelete: (id: string) => void;
}) {
  const [toggling, setToggling]   = useState(false);
  const [deleting, setDeleting]   = useState(false);
  const [testing,  setTesting]    = useState(false);
  const [testMsg,  setTestMsg]    = useState<{ ok: boolean; text: string } | null>(null);

  async function toggle() {
    setToggling(true);
    try {
      await updateAlertRule(rule.id, { enabled: rule.enabled ? 0 : 1 });
      onToggle(rule.id, !rule.enabled);
    } finally {
      setToggling(false);
    }
  }

  async function del() {
    if (!confirm(`Delete rule "${rule.name}"?`)) return;
    setDeleting(true);
    try {
      await deleteAlertRule(rule.id);
      onDelete(rule.id);
    } finally {
      setDeleting(false);
    }
  }

  async function sendTest() {
    setTesting(true);
    setTestMsg(null);
    try {
      const res = await testAlertDelivery(rule.id);
      setTestMsg({ ok: res.ok, text: res.message ?? (res.ok ? "Test delivered ✓" : "Delivery failed") });
    } catch (e) {
      setTestMsg({ ok: false, text: String(e) });
    } finally {
      setTesting(false);
      setTimeout(() => setTestMsg(null), 4000);
    }
  }

  const enabled = Boolean(rule.enabled);

  return (
    <tr className="border-b border-white/[0.04] hover:bg-white/[0.02] transition-colors">
      <td className="px-4 py-3">
        <p className="text-sm text-white font-medium">{rule.name}</p>
        <div className="flex items-center gap-1.5 mt-0.5">
          {rule.integration_type && rule.integration_type !== "webhook" && (
            <span className="text-[10px] px-1.5 py-0.5 rounded bg-indigo-500/10 text-indigo-400 border border-indigo-500/20">
              {INTEGRATION_LABELS[rule.integration_type as AlertIntegrationType] ?? rule.integration_type}
            </span>
          )}
          {rule.webhook_url && (
            <p className="text-xs text-slate-600 truncate max-w-[160px]">
              {rule.webhook_url}
            </p>
          )}
          {testMsg && (
            <span className={clsx(
              "text-[10px] px-1.5 py-0.5 rounded",
              testMsg.ok ? "text-emerald-400 bg-emerald-500/10" : "text-red-400 bg-red-500/10",
            )}>
              {testMsg.text}
            </span>
          )}
        </div>
      </td>
      <td className="px-4 py-3 text-sm text-slate-400">
        {METRIC_LABELS[rule.metric as AlertMetric] ?? rule.metric}
      </td>
      <td className="px-4 py-3 text-sm text-slate-300">
        <span className="font-mono">
          {rule.condition === "gt" ? ">" : "<"} {rule.threshold}
        </span>
        <span className="text-slate-600 ml-2 text-xs">
          over {rule.window_minutes}m
        </span>
      </td>
      <td className="px-4 py-3 text-xs text-slate-500">
        {rule.cooldown_minutes}m
      </td>
      <td className="px-4 py-3">
        <span
          className={clsx(
            "inline-flex items-center gap-1.5 text-xs px-2 py-0.5 rounded-full border",
            enabled
              ? "bg-emerald-500/10 text-emerald-400 border-emerald-500/20"
              : "bg-slate-700/20 text-slate-500 border-slate-700/40",
          )}
        >
          <span
            className={clsx(
              "w-1.5 h-1.5 rounded-full",
              enabled ? "bg-emerald-400" : "bg-slate-600",
            )}
          />
          {enabled ? "Active" : "Paused"}
        </span>
      </td>
      <td className="px-4 py-3">
        <div className="flex items-center gap-1.5">
          {/* Test delivery */}
          <button
            onClick={sendTest}
            disabled={testing}
            title="Send a test notification to verify delivery is configured"
            className="p-1.5 rounded-md text-slate-500 hover:text-indigo-400
                       hover:bg-indigo-500/10 transition-colors disabled:opacity-40"
          >
            <Send size={13} className={testing ? "animate-pulse" : ""} />
          </button>
          <button
            onClick={toggle}
            disabled={toggling}
            title={enabled ? "Pause rule" : "Enable rule"}
            className="p-1.5 rounded-md text-slate-500 hover:text-indigo-400
                       hover:bg-indigo-500/10 transition-colors disabled:opacity-40"
          >
            {enabled ? <ToggleRight size={16} /> : <ToggleLeft size={16} />}
          </button>
          <button
            onClick={del}
            disabled={deleting}
            title="Delete rule"
            className="p-1.5 rounded-md text-slate-500 hover:text-red-400
                       hover:bg-red-500/10 transition-colors disabled:opacity-40"
          >
            <Trash2 size={14} />
          </button>
        </div>
      </td>
    </tr>
  );
}

// ── Event Row ──────────────────────────────────────────────────────────────────

function EventRow({ event }: { event: AlertEvent }) {
  return (
    <tr className="border-b border-white/[0.04]">
      <td className="px-4 py-2.5 text-xs text-slate-500 whitespace-nowrap">
        {fmtDate(event.fired_at)}
      </td>
      <td className="px-4 py-2.5 text-sm text-white">{event.rule_name}</td>
      <td className="px-4 py-2.5 text-sm text-slate-400">
        {METRIC_LABELS[event.metric as AlertMetric] ?? event.metric}
      </td>
      <td className="px-4 py-2.5 text-sm text-slate-300 font-mono">
        {event.value.toFixed(2)}
        <span className="text-slate-600 ml-1 text-xs">
          (threshold: {event.threshold})
        </span>
      </td>
      <td className="px-4 py-2.5">
        {event.webhook_delivered ? (
          <CheckCircle2 size={14} className="text-emerald-400" />
        ) : (
          <XCircle size={14} className="text-slate-600" />
        )}
      </td>
    </tr>
  );
}

// ── Page ───────────────────────────────────────────────────────────────────────

export default function AlertsPage() {
  const [rules, setRules] = useState<AlertRule[]>([]);
  const [events, setEvents] = useState<AlertEvent[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [showCreate, setShowCreate] = useState(false);
  const [newRuleSecret, setNewRuleSecret] = useState<{ name: string; secret: string } | null>(null);

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const [rulesRes, eventsRes] = await Promise.all([
        fetchAlertRules(),
        fetchAlertEvents(),
      ]);
      setRules(rulesRes.rules ?? []);
      setEvents(eventsRes.events ?? []);
    } catch (e) {
      setError(String(e));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { load(); }, [load]);

  function handleToggle(id: string, enabled: boolean) {
    setRules((prev) =>
      prev.map((r) => (r.id === id ? { ...r, enabled: enabled ? 1 : 0 } : r)),
    );
  }

  function handleDelete(id: string) {
    setRules((prev) => prev.filter((r) => r.id !== id));
  }

  function handleCreated(rule: AlertRule & { webhook_secret?: string }) {
    setRules((prev) => [rule, ...prev]);
    setShowCreate(false);
    if (rule.webhook_secret && rule.integration_type !== "email") {
      setNewRuleSecret({ name: rule.name, secret: rule.webhook_secret });
    }
  }

  return (
    <div className="p-6 space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-3">
          <div className="p-2 rounded-lg bg-amber-500/10">
            <Bell size={18} className="text-amber-400" />
          </div>
          <div>
            <h1 className="text-xl font-semibold text-white">Alerts</h1>
            <p className="text-xs text-slate-500 mt-0.5">
              Rules are evaluated every 60 seconds
            </p>
          </div>
        </div>
        <div className="flex items-center gap-2">
          <button
            onClick={load}
            className="flex items-center gap-2 px-3 py-1.5 rounded-lg bg-[#12121f]
                       border border-white/10 text-sm text-slate-300 hover:text-white
                       hover:border-white/20 transition-colors"
          >
            <RefreshCw size={14} className={loading ? "animate-spin" : ""} />
            Refresh
          </button>
          <button
            onClick={() => setShowCreate(true)}
            className="flex items-center gap-2 px-3 py-1.5 rounded-lg bg-indigo-600
                       hover:bg-indigo-500 text-white text-sm font-medium transition-colors"
          >
            <Plus size={14} />
            New rule
          </button>
        </div>
      </div>

      {newRuleSecret && (
        <div className="rounded-lg bg-emerald-500/10 border border-emerald-500/20 px-4 py-3 space-y-1">
          <p className="text-xs font-semibold text-emerald-400">
            Webhook secret for &quot;{newRuleSecret.name}&quot; — copy it now, it won&apos;t be shown again
          </p>
          <div className="flex items-center gap-2 font-mono text-xs text-white break-all">
            {newRuleSecret.secret}
            <button onClick={() => navigator.clipboard.writeText(newRuleSecret.secret)}
                    className="ml-1 p-1 rounded hover:bg-white/10 text-slate-500 hover:text-slate-300">
              <Copy size={11} />
            </button>
          </div>
          <p className="text-[10px] text-slate-500">
            Use this as X-NetScope-Signature verification secret in your webhook receiver.
          </p>
          <button onClick={() => setNewRuleSecret(null)}
                  className="text-[10px] text-slate-500 hover:text-slate-300 underline">
            Dismiss
          </button>
        </div>
      )}

      {error && (
        <div className="rounded-lg bg-red-900/20 border border-red-500/30 px-4 py-3 text-sm text-red-400">
          {error}
        </div>
      )}

      {/* Rules table */}
      <section className="rounded-xl bg-[#0d0d1a] border border-white/[0.06] overflow-hidden">
        <div className="px-4 py-3 border-b border-white/[0.06] flex items-center justify-between">
          <h2 className="text-sm font-medium text-slate-300">
            Alert rules
            {rules.length > 0 && (
              <span className="ml-2 text-xs text-slate-600">
                ({rules.length})
              </span>
            )}
          </h2>
        </div>
        {loading && rules.length === 0 ? (
          <div className="px-4 py-12 text-center text-sm text-slate-600">
            Loading…
          </div>
        ) : rules.length === 0 ? (
          <div className="px-4 py-12 text-center space-y-2">
            <Bell size={24} className="mx-auto text-slate-700" />
            <p className="text-sm text-slate-600">No alert rules yet</p>
            <button
              onClick={() => setShowCreate(true)}
              className="text-xs text-indigo-400 hover:text-indigo-300 underline underline-offset-2"
            >
              Create your first rule
            </button>
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-left">
              <thead>
                <tr className="border-b border-white/[0.06]">
                  {["Name", "Metric", "Condition", "Cooldown", "Status", "Actions"].map(
                    (h) => (
                      <th
                        key={h}
                        className="px-4 py-2.5 text-xs font-medium text-slate-500 uppercase tracking-wider"
                      >
                        {h}
                      </th>
                    ),
                  )}
                </tr>
              </thead>
              <tbody>
                {rules.map((rule) => (
                  <RuleRow
                    key={rule.id}
                    rule={rule}
                    onToggle={handleToggle}
                    onDelete={handleDelete}
                  />
                ))}
              </tbody>
            </table>
          </div>
        )}
      </section>

      {/* Events table */}
      <section className="rounded-xl bg-[#0d0d1a] border border-white/[0.06] overflow-hidden">
        <div className="px-4 py-3 border-b border-white/[0.06]">
          <h2 className="text-sm font-medium text-slate-300">
            Recent firings
            {events.length > 0 && (
              <span className="ml-2 text-xs text-slate-600">
                (last {events.length})
              </span>
            )}
          </h2>
        </div>
        {events.length === 0 ? (
          <div className="px-4 py-8 text-center text-sm text-slate-600">
            No alert events yet
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-left">
              <thead>
                <tr className="border-b border-white/[0.06]">
                  {["Time", "Rule", "Metric", "Value", "Webhook"].map((h) => (
                    <th
                      key={h}
                      className="px-4 py-2.5 text-xs font-medium text-slate-500 uppercase tracking-wider"
                    >
                      {h}
                    </th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {events.map((ev) => (
                  <EventRow key={ev.id} event={ev} />
                ))}
              </tbody>
            </table>
          </div>
        )}
      </section>

      {showCreate && (
        <CreateRuleModal
          onClose={() => setShowCreate(false)}
          onCreated={handleCreated}
        />
      )}
    </div>
  );
}
