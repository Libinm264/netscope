"use client";

import { useCallback, useEffect, useState } from "react";
import { Zap, CheckCircle, XCircle, Clock, RefreshCw, ChevronRight } from "lucide-react";
import { clsx } from "clsx";
import { fetchLicense } from "@/lib/api";
import type { LicenseInfo } from "@/lib/api";

const FEATURE_LABELS: Record<string, string> = {
  sso:               "SSO (SAML / OIDC)",
  multi_tenant:      "Multi-tenant organisations",
  scim:              "SCIM 2.0 provisioning",
  audit_export:      "Audit log export (CEF / JSON)",
  custom_retention:  "Custom data retention",
  custom_roles:      "Custom RBAC roles",
  pii_redaction:     "PII redaction on eBPF captures",
  otel_correlation:  "OpenTelemetry trace correlation",
};

const PLAN_FEATURES: Record<string, string[]> = {
  community:  [],
  team: ["sso", "multi_tenant", "scim", "audit_export", "custom_retention"],
  enterprise: Object.keys(FEATURE_LABELS),
};

function PlanBadge({ plan }: { plan: string }) {
  return (
    <span className={clsx(
      "px-2 py-0.5 rounded-full text-xs font-semibold",
      plan === "enterprise" ? "bg-amber-500/15 text-amber-300 border border-amber-500/30" :
      plan === "team"       ? "bg-indigo-500/15 text-indigo-300 border border-indigo-500/30" :
                              "bg-slate-700/50 text-slate-400 border border-white/10",
    )}>
      {plan.charAt(0).toUpperCase() + plan.slice(1)}
    </span>
  );
}

export default function LicensePage() {
  const [info, setInfo]       = useState<LicenseInfo | null>(null);
  const [loading, setLoading] = useState(true);
  const [keyInput, setKeyInput] = useState("");
  const [saving, setSaving]   = useState(false);
  const [saveMsg, setSaveMsg] = useState<string | null>(null);

  const load = useCallback(async () => {
    setLoading(true);
    try { setInfo(await fetchLicense()); }
    catch { /* hub may be offline */ }
    finally { setLoading(false); }
  }, []);

  useEffect(() => { load(); }, [load]);

  const handleApplyKey = async () => {
    if (!keyInput.trim()) return;
    setSaving(true);
    setSaveMsg(null);
    try {
      // Applying a new license requires restarting the hub with the
      // ENTERPRISE_LICENSE_KEY env var set. Show instructions.
      setSaveMsg(
        "Set ENTERPRISE_LICENSE_KEY=" + keyInput.trim() +
        " in your hub .env file and restart the hub to activate.",
      );
    } finally {
      setSaving(false);
    }
  };

  const plan     = info?.plan ?? "community";
  const features = info?.features ?? [];
  const featureSet = new Set(features);

  return (
    <div className="p-6 space-y-6">
      {/* Header */}
      <div className="flex items-center gap-3">
        <div className="p-2 rounded-lg bg-amber-500/10">
          <Zap size={20} className="text-amber-400" />
        </div>
        <div>
          <h1 className="text-lg font-semibold text-white">License & Plan</h1>
          <p className="text-xs text-slate-400 mt-0.5">
            Current plan, active features, and license key management
          </p>
        </div>
      </div>

      {loading ? (
        <div className="flex items-center justify-center h-32">
          <RefreshCw size={20} className="animate-spin text-slate-600" />
        </div>
      ) : (
        <>
          {/* Current plan card */}
          <div className="bg-[#0d0d1a] border border-white/[0.06] rounded-xl p-5 space-y-4">
            <div className="flex items-start justify-between">
              <div>
                <p className="text-sm font-semibold text-white">Current plan</p>
                <p className="text-xs text-slate-500 mt-0.5">
                  {info?.org_name ?? "Default Organisation"}
                </p>
              </div>
              <PlanBadge plan={plan} />
            </div>

            <div className="grid grid-cols-2 gap-3 text-xs">
              <div className="bg-white/[0.03] rounded-lg px-3 py-2.5">
                <p className="text-slate-500">Agent quota</p>
                <p className="text-white font-semibold mt-0.5">
                  {info?.agent_quota === -1 ? "Unlimited" : `${info?.agent_quota ?? 10} agents`}
                </p>
              </div>
              <div className="bg-white/[0.03] rounded-lg px-3 py-2.5">
                <p className="text-slate-500">License status</p>
                <p className={clsx(
                  "font-semibold mt-0.5",
                  info?.valid ? "text-emerald-400" : "text-red-400",
                )}>
                  {info?.expired ? "Expired" : info?.valid ? "Active" : "Invalid"}
                </p>
              </div>
            </div>

            {info?.expires_at && (
              <div className="flex items-center gap-1.5 text-xs text-slate-500">
                <Clock size={11} />
                Expires {new Date(info.expires_at).toLocaleDateString()}
              </div>
            )}
          </div>

          {/* Feature matrix */}
          <div className="bg-[#0d0d1a] border border-white/[0.06] rounded-xl overflow-hidden">
            <div className="px-5 py-3 border-b border-white/[0.06]">
              <p className="text-sm font-semibold text-white">Feature access</p>
              <p className="text-xs text-slate-500 mt-0.5">
                What's included in each plan
              </p>
            </div>
            <div className="divide-y divide-white/[0.04]">
              {Object.entries(FEATURE_LABELS).map(([key, label]) => {
                const inCurrentPlan = featureSet.has(key) || plan === "enterprise";
                const inTeam        = PLAN_FEATURES.team.includes(key);
                const inEnterprise  = PLAN_FEATURES.enterprise.includes(key);
                return (
                  <div key={key} className="flex items-center px-5 py-2.5 gap-4">
                    <div className="flex-1 text-xs text-slate-300">{label}</div>
                    {/* Community */}
                    <XCircle size={14} className="text-slate-700 w-16 text-center" />
                    {/* Team */}
                    {inTeam
                      ? <CheckCircle size={14} className="text-emerald-500 w-16 text-center" />
                      : <XCircle    size={14} className="text-slate-700    w-16 text-center" />
                    }
                    {/* Enterprise */}
                    {inEnterprise
                      ? <CheckCircle size={14} className="text-amber-400 w-16 text-center" />
                      : <XCircle    size={14} className="text-slate-700  w-16 text-center" />
                    }
                    {/* Current status */}
                    {inCurrentPlan
                      ? <span className="text-[10px] text-emerald-400 font-medium w-16">Active</span>
                      : <span className="text-[10px] text-slate-600 w-16">Locked</span>
                    }
                  </div>
                );
              })}
            </div>
            {/* Column headers */}
            <div className="flex items-center px-5 py-2 border-t border-white/[0.06] bg-white/[0.01]">
              <div className="flex-1" />
              <p className="text-[10px] text-slate-500 w-16 text-center">Community</p>
              <p className="text-[10px] text-slate-500 w-16 text-center">Team</p>
              <p className="text-[10px] text-slate-500 w-16 text-center">Enterprise</p>
              <p className="text-[10px] text-slate-500 w-16">Status</p>
            </div>
          </div>

          {/* Apply license key */}
          <div className="bg-[#0d0d1a] border border-white/[0.06] rounded-xl p-5 space-y-3">
            <p className="text-sm font-semibold text-white">Apply license key</p>
            <p className="text-xs text-slate-500">
              Enter your NetScope license key below. You'll need to restart the hub
              with the key set as <code className="text-slate-300 bg-white/[0.06] px-1 rounded">ENTERPRISE_LICENSE_KEY</code>.
            </p>
            <div className="flex gap-2">
              <input
                type="text"
                value={keyInput}
                onChange={(e) => setKeyInput(e.target.value)}
                placeholder="eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."
                className="flex-1 bg-white/[0.04] border border-white/10 rounded-lg px-3 py-2
                           text-xs text-white placeholder:text-slate-600 font-mono
                           focus:outline-none focus:border-indigo-500/50"
              />
              <button
                onClick={handleApplyKey}
                disabled={!keyInput.trim() || saving}
                className="px-4 py-2 rounded-lg bg-indigo-600 hover:bg-indigo-500 text-white
                           text-xs font-medium disabled:opacity-40 transition-colors"
              >
                Apply
              </button>
            </div>
            {saveMsg && (
              <p className="text-xs text-amber-400 bg-amber-500/10 border border-amber-500/20 rounded-lg px-3 py-2">
                {saveMsg}
              </p>
            )}
          </div>

          {/* Upgrade CTA */}
          {plan === "community" && (
            <div className="flex items-center justify-between p-4 rounded-xl
                            bg-indigo-500/[0.06] border border-indigo-500/20">
              <div>
                <p className="text-sm font-semibold text-white">Unlock enterprise features</p>
                <p className="text-xs text-slate-400 mt-0.5">
                  SSO, multi-tenancy, custom RBAC, SCIM provisioning and more.
                </p>
              </div>
              <a
                href="https://netscope.io/pricing"
                target="_blank"
                rel="noopener noreferrer"
                className="flex items-center gap-1 px-4 py-2 rounded-lg bg-indigo-600
                           hover:bg-indigo-500 text-white text-xs font-medium transition-colors shrink-0"
              >
                Get a license <ChevronRight size={12} />
              </a>
            </div>
          )}
        </>
      )}
    </div>
  );
}
