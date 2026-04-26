"use client";

import { useCallback, useEffect, useState } from "react";
import { Settings, Plus, Trash2, Copy, Check, RefreshCw, Key, Shield, Eye, Download } from "lucide-react";
import { clsx } from "clsx";
import {
  fetchAPITokens, createAPIToken, revokeAPIToken,
  fetchEnrollmentTokens, createEnrollmentToken, revokeEnrollmentToken,
  fetchAuditEvents, auditExportURL,
} from "@/lib/api";
import type { APIToken, EnrollmentToken, AuditEvent } from "@/lib/api";

// ── Copy button ────────────────────────────────────────────────────────────────

function CopyButton({ text }: { text: string }) {
  const [copied, setCopied] = useState(false);
  const copy = () => {
    navigator.clipboard.writeText(text).then(() => {
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    });
  };
  return (
    <button
      onClick={copy}
      className="ml-1 p-1 rounded hover:bg-white/10 text-slate-500 hover:text-slate-300 transition-colors"
      title="Copy"
    >
      {copied ? <Check size={11} className="text-emerald-400" /> : <Copy size={11} />}
    </button>
  );
}

// ── Section wrapper ────────────────────────────────────────────────────────────

function Section({ title, subtitle, children }: {
  title: string; subtitle: string; children: React.ReactNode;
}) {
  return (
    <div className="bg-[#0d0d1a] border border-white/[0.06] rounded-xl overflow-hidden">
      <div className="px-5 py-4 border-b border-white/[0.06]">
        <p className="text-sm font-semibold text-white">{title}</p>
        <p className="text-xs text-slate-500 mt-0.5">{subtitle}</p>
      </div>
      <div className="p-5">{children}</div>
    </div>
  );
}

// ── Audit Log Section ──────────────────────────────────────────────────────────

// ── Export helpers ─────────────────────────────────────────────────────────────

const QUICK_RANGES = [
  { label: "Last 24h",  hours: 24 },
  { label: "Last 7d",   hours: 168 },
  { label: "Last 30d",  hours: 720 },
] as const;

type ExportFormat = "json" | "cef" | "leef";

function AuditLogSection() {
  const [events,   setEvents]   = useState<AuditEvent[]>([]);
  const [loading,  setLoading]  = useState(false);
  // Export state
  const [format,   setFormat]   = useState<ExportFormat>("json");
  const [rangeIdx, setRangeIdx] = useState(0);

  useEffect(() => {
    setLoading(true);
    fetchAuditEvents({ limit: 100 })
      .then((r) => setEvents(r.events ?? []))
      .catch(() => {})
      .finally(() => setLoading(false));
  }, []);

  const handleExport = () => {
    const hours = QUICK_RANGES[rangeIdx].hours;
    const now   = new Date();
    const from  = new Date(now.getTime() - hours * 3600 * 1000);
    const url   = auditExportURL({
      format,
      from:  from.toISOString(),
      to:    now.toISOString(),
      limit: 50000,
    });
    // Open in same tab — browser treats it as a download due to Content-Disposition.
    window.location.href = url;
  };

  const statusColor = (s: number) =>
    s >= 500 ? "text-red-400" :
    s >= 400 ? "text-amber-400" :
    s >= 200 ? "text-emerald-400" : "text-slate-400";

  return (
    <div className="space-y-3">
      {/* Export toolbar */}
      <div className="flex items-center gap-2 flex-wrap">
        <span className="text-[10px] text-slate-500 font-medium uppercase tracking-wider">Export:</span>
        <div className="flex rounded-lg border border-white/10 overflow-hidden">
          {(["json", "cef", "leef"] as ExportFormat[]).map(f => (
            <button key={f} onClick={() => setFormat(f)}
              className={clsx(
                "px-2.5 py-1 text-[10px] font-mono font-medium uppercase transition-colors",
                format === f
                  ? "bg-indigo-600 text-white"
                  : "text-slate-400 hover:text-slate-200 hover:bg-white/[0.04]",
              )}>
              {f}
            </button>
          ))}
        </div>
        <div className="flex rounded-lg border border-white/10 overflow-hidden">
          {QUICK_RANGES.map((r, i) => (
            <button key={r.label} onClick={() => setRangeIdx(i)}
              className={clsx(
                "px-2.5 py-1 text-[10px] font-medium transition-colors",
                rangeIdx === i
                  ? "bg-indigo-600 text-white"
                  : "text-slate-400 hover:text-slate-200 hover:bg-white/[0.04]",
              )}>
              {r.label}
            </button>
          ))}
        </div>
        <button onClick={handleExport}
          className="flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-[11px] font-medium
                     bg-white/[0.05] border border-white/10 text-slate-300
                     hover:bg-white/[0.09] transition-colors">
          <Download size={11} /> Download
        </button>
      </div>

      {/* Live table */}
      {loading ? (
        <div className="flex items-center justify-center h-16">
          <RefreshCw size={16} className="animate-spin text-slate-600" />
        </div>
      ) : events.length === 0 ? (
        <p className="text-xs text-slate-600 text-center py-6">No audit events yet</p>
      ) : (
        <div className="overflow-x-auto max-h-72 overflow-y-auto">
          <table className="w-full text-left">
            <thead className="sticky top-0 bg-[#0d0d1a]">
              <tr>
                {["Time", "Method", "Path", "Status", "Role", "IP", "Latency"].map((h) => (
                  <th key={h} className="px-3 py-2 text-[10px] font-medium text-slate-500 uppercase tracking-wider whitespace-nowrap">
                    {h}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {events.map((e) => (
                <tr key={e.id} className="border-t border-white/[0.04] hover:bg-white/[0.02]">
                  <td className="px-3 py-2 text-[10px] text-slate-500 whitespace-nowrap font-mono">
                    {new Date(e.ts).toLocaleTimeString()}
                  </td>
                  <td className="px-3 py-2 text-[10px] font-mono text-indigo-400">{e.method}</td>
                  <td className="px-3 py-2 text-[10px] text-slate-300 font-mono max-w-[200px] truncate">{e.path}</td>
                  <td className={`px-3 py-2 text-[10px] font-mono font-bold ${statusColor(e.status)}`}>{e.status}</td>
                  <td className="px-3 py-2 text-[10px] text-slate-500">{e.role}</td>
                  <td className="px-3 py-2 text-[10px] text-slate-500 font-mono">{e.client_ip}</td>
                  <td className="px-3 py-2 text-[10px] text-slate-500 text-right">{e.latency_ms}ms</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}

// ── Page ───────────────────────────────────────────────────────────────────────

export default function SettingsPage() {
  // API tokens state
  const [tokens, setTokens] = useState<APIToken[]>([]);
  const [tokensLoading, setTokensLoading] = useState(false);
  const [newTokenName, setNewTokenName] = useState("");
  const [newTokenRole, setNewTokenRole] = useState<"admin" | "viewer">("viewer");
  const [createdToken, setCreatedToken] = useState<APIToken | null>(null);

  // Enrollment tokens state
  const [enrollTokens, setEnrollTokens] = useState<EnrollmentToken[]>([]);
  const [enrollLoading, setEnrollLoading] = useState(false);
  const [newEnrollName, setNewEnrollName] = useState("");
  const [newEnrollExpiry, setNewEnrollExpiry] = useState("7d");
  const [createdEnroll, setCreatedEnroll] = useState<EnrollmentToken | null>(null);

  const loadTokens = useCallback(async () => {
    setTokensLoading(true);
    try { setTokens((await fetchAPITokens()).tokens ?? []); } catch { /* ignore */ }
    finally { setTokensLoading(false); }
  }, []);

  const loadEnroll = useCallback(async () => {
    setEnrollLoading(true);
    try { setEnrollTokens((await fetchEnrollmentTokens()).tokens ?? []); } catch { /* ignore */ }
    finally { setEnrollLoading(false); }
  }, []);

  useEffect(() => { loadTokens(); loadEnroll(); }, [loadTokens, loadEnroll]);

  const handleCreateToken = async () => {
    if (!newTokenName.trim()) return;
    try {
      const t = await createAPIToken(newTokenName.trim(), newTokenRole);
      setCreatedToken(t);
      setNewTokenName("");
      loadTokens();
    } catch (e) {
      alert(e instanceof Error ? e.message : "Failed to create token");
    }
  };

  const handleRevokeToken = async (id: string) => {
    if (!confirm("Revoke this token? This cannot be undone.")) return;
    await revokeAPIToken(id);
    loadTokens();
  };

  const handleCreateEnroll = async () => {
    if (!newEnrollName.trim()) return;
    try {
      const t = await createEnrollmentToken(newEnrollName.trim(), newEnrollExpiry);
      setCreatedEnroll(t);
      setNewEnrollName("");
      loadEnroll();
    } catch (e) {
      alert(e instanceof Error ? e.message : "Failed to create enrollment token");
    }
  };

  const handleRevokeEnroll = async (id: string) => {
    if (!confirm("Revoke this enrollment token?")) return;
    await revokeEnrollmentToken(id);
    loadEnroll();
  };

  const hubURL = typeof window !== "undefined"
    ? window.location.origin.replace(":3000", ":8080")
    : "http://localhost:8080";

  return (
    <div className="p-6 space-y-6">
      {/* Header */}
      <div className="flex items-center gap-3">
        <div className="p-2 rounded-lg bg-indigo-500/10">
          <Settings size={20} className="text-indigo-400" />
        </div>
        <div>
          <h1 className="text-lg font-semibold text-white">Settings</h1>
          <p className="text-xs text-slate-400 mt-0.5">API tokens, agent enrollment, and access control</p>
        </div>
      </div>

      {/* API Tokens */}
      <Section
        title="API Tokens"
        subtitle="Scoped tokens for dashboard integrations. Viewer tokens are read-only; admin tokens can create rules and delete resources."
      >
        {/* Created token one-time reveal */}
        {createdToken && (
          <div className="mb-4 p-3 rounded-lg bg-emerald-500/10 border border-emerald-500/20">
            <p className="text-xs font-semibold text-emerald-400 mb-1">
              Token created — copy it now, it won&apos;t be shown again
            </p>
            <div className="flex items-center gap-2 font-mono text-xs text-white break-all">
              {createdToken.token}
              <CopyButton text={createdToken.token} />
            </div>
            <button
              onClick={() => setCreatedToken(null)}
              className="mt-2 text-[10px] text-slate-500 hover:text-slate-300 underline"
            >
              Dismiss
            </button>
          </div>
        )}

        {/* Create form */}
        <div className="flex gap-2 mb-4">
          <input
            value={newTokenName}
            onChange={(e) => setNewTokenName(e.target.value)}
            placeholder="Token name (e.g. grafana-readonly)"
            className="flex-1 bg-white/[0.04] border border-white/10 rounded-md px-3 py-2
                       text-xs text-white placeholder:text-slate-600 focus:outline-none focus:border-indigo-500/50"
          />
          <select
            value={newTokenRole}
            onChange={(e) => setNewTokenRole(e.target.value as "admin" | "viewer")}
            className="bg-white/[0.04] border border-white/10 rounded-md px-2 py-2
                       text-xs text-slate-300 focus:outline-none"
          >
            <option value="viewer">Viewer</option>
            <option value="admin">Admin</option>
          </select>
          <button
            onClick={handleCreateToken}
            disabled={!newTokenName.trim()}
            className="flex items-center gap-1.5 px-3 py-2 rounded-md text-xs
                       bg-indigo-600 hover:bg-indigo-500 text-white disabled:opacity-40 transition-colors"
          >
            <Plus size={12} /> Create
          </button>
        </div>

        {/* Token list */}
        {tokensLoading && tokens.length === 0 ? (
          <div className="flex items-center justify-center h-16">
            <RefreshCw size={16} className="animate-spin text-slate-600" />
          </div>
        ) : tokens.length === 0 ? (
          <p className="text-xs text-slate-600 text-center py-6">No additional tokens — the bootstrap API key is always active</p>
        ) : (
          <div className="space-y-2">
            {tokens.filter((t) => !t.revoked).map((t) => (
              <div key={t.id} className="flex items-center justify-between px-3 py-2.5
                                         bg-white/[0.02] border border-white/[0.05] rounded-md">
                <div className="flex items-center gap-3">
                  {t.role === "admin"
                    ? <Shield size={13} className="text-indigo-400" />
                    : <Eye size={13} className="text-slate-500" />
                  }
                  <div>
                    <p className="text-xs font-medium text-slate-200">{t.name}</p>
                    <p className="text-[10px] text-slate-600 font-mono">{t.token}</p>
                  </div>
                </div>
                <div className="flex items-center gap-3">
                  <span className={clsx(
                    "text-[10px] px-1.5 py-0.5 rounded font-medium",
                    t.role === "admin"
                      ? "bg-indigo-500/10 text-indigo-400"
                      : "bg-slate-700/50 text-slate-400",
                  )}>
                    {t.role}
                  </span>
                  <button
                    onClick={() => handleRevokeToken(t.id)}
                    className="p-1 rounded hover:bg-red-500/10 text-slate-600 hover:text-red-400 transition-colors"
                  >
                    <Trash2 size={12} />
                  </button>
                </div>
              </div>
            ))}
          </div>
        )}
      </Section>

      {/* Enrollment tokens */}
      <Section
        title="Agent Enrollment Tokens"
        subtitle="Generate short-lived tokens to enroll new agents. Each token produces a one-line install command."
      >
        {/* Created enroll token */}
        {createdEnroll && (
          <div className="mb-4 p-3 rounded-lg bg-indigo-500/10 border border-indigo-500/20 space-y-2">
            <p className="text-xs font-semibold text-indigo-400">
              Enrollment token ready — share the install command below
            </p>
            <div className="font-mono text-[11px] text-slate-300 bg-black/30 rounded px-3 py-2 break-all">
              curl -sSL &quot;{hubURL}/install?token={createdEnroll.token}&quot; | sh
              <CopyButton text={`curl -sSL "${hubURL}/install?token=${createdEnroll.token}" | sh`} />
            </div>
            <p className="text-[10px] text-slate-500">
              Or run manually: set <code className="text-slate-400">ENROLLMENT_TOKEN={createdEnroll.token}</code> then start the agent with <code className="text-slate-400">--output hub</code>
            </p>
            <button
              onClick={() => setCreatedEnroll(null)}
              className="text-[10px] text-slate-500 hover:text-slate-300 underline"
            >
              Dismiss
            </button>
          </div>
        )}

        {/* Create form */}
        <div className="flex gap-2 mb-4">
          <input
            value={newEnrollName}
            onChange={(e) => setNewEnrollName(e.target.value)}
            placeholder="Label (e.g. prod-web-01)"
            className="flex-1 bg-white/[0.04] border border-white/10 rounded-md px-3 py-2
                       text-xs text-white placeholder:text-slate-600 focus:outline-none focus:border-indigo-500/50"
          />
          <select
            value={newEnrollExpiry}
            onChange={(e) => setNewEnrollExpiry(e.target.value)}
            className="bg-white/[0.04] border border-white/10 rounded-md px-2 py-2
                       text-xs text-slate-300 focus:outline-none"
          >
            <option value="24h">24 hours</option>
            <option value="7d">7 days</option>
            <option value="30d">30 days</option>
            <option value="never">Never</option>
          </select>
          <button
            onClick={handleCreateEnroll}
            disabled={!newEnrollName.trim()}
            className="flex items-center gap-1.5 px-3 py-2 rounded-md text-xs
                       bg-indigo-600 hover:bg-indigo-500 text-white disabled:opacity-40 transition-colors"
          >
            <Plus size={12} /> Generate
          </button>
        </div>

        {/* Token list */}
        {enrollLoading && enrollTokens.length === 0 ? (
          <div className="flex items-center justify-center h-16">
            <RefreshCw size={16} className="animate-spin text-slate-600" />
          </div>
        ) : enrollTokens.filter((t) => !t.revoked).length === 0 ? (
          <p className="text-xs text-slate-600 text-center py-6">No active enrollment tokens</p>
        ) : (
          <div className="space-y-2">
            {enrollTokens.filter((t) => !t.revoked).map((t) => (
              <div key={t.id} className="flex items-center justify-between px-3 py-2.5
                                         bg-white/[0.02] border border-white/[0.05] rounded-md">
                <div className="flex items-center gap-3">
                  <Key size={13} className="text-slate-500" />
                  <div>
                    <p className="text-xs font-medium text-slate-200">{t.name}</p>
                    <p className="text-[10px] text-slate-600">
                      Used {t.used_count}× · Expires {new Date(t.expires_at).toLocaleDateString()}
                    </p>
                  </div>
                </div>
                <button
                  onClick={() => handleRevokeEnroll(t.id)}
                  className="p-1 rounded hover:bg-red-500/10 text-slate-600 hover:text-red-400 transition-colors"
                >
                  <Trash2 size={12} />
                </button>
              </div>
            ))}
          </div>
        )}
      </Section>

      {/* Audit log */}
      <Section
        title="Audit Log"
        subtitle="Every authenticated API request — last 100 events."
      >
        <AuditLogSection />
      </Section>
    </div>
  );
}
