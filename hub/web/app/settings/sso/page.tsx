"use client";

import { useCallback, useEffect, useState } from "react";
import { KeyRound, RefreshCw, Save, ExternalLink, CheckCircle } from "lucide-react";
import { clsx } from "clsx";
import { fetchSSOConfig, updateSSOConfig, fetchLicense } from "@/lib/api";
import type { SSOConfig } from "@/lib/api";
import { EnterpriseGate } from "@/components/EnterpriseGate";

type Provider = "saml" | "oidc";

function TabBtn({
  active, onClick, children,
}: {
  active: boolean; onClick: () => void; children: React.ReactNode;
}) {
  return (
    <button
      onClick={onClick}
      className={clsx(
        "px-4 py-2 text-xs font-medium border-b-2 transition-colors",
        active
          ? "border-indigo-400 text-indigo-300"
          : "border-transparent text-slate-500 hover:text-slate-300",
      )}
    >
      {children}
    </button>
  );
}

export default function SSOPage() {
  const [cfg, setCfg]         = useState<SSOConfig | null>(null);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving]   = useState(false);
  const [saved, setSaved]     = useState(false);
  const [locked, setLocked]   = useState(false);
  const [provider, setProvider] = useState<Provider>("oidc");

  // SAML fields
  const [entityId, setEntityId]       = useState("");
  const [ssoUrl, setSsoUrl]           = useState("");
  const [certificate, setCertificate] = useState("");

  // OIDC / Dex fields
  const [issuerUrl, setIssuerUrl]     = useState("");
  const [clientId, setClientId]       = useState("");
  const [clientSecret, setClientSecret] = useState("");

  const [enabled, setEnabled] = useState(false);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const [data, lic] = await Promise.all([fetchSSOConfig(), fetchLicense()]);
      setLocked(lic.plan === "community");
      if (data) {
        setCfg(data);
        setProvider((data.provider as Provider) || "oidc");
        setEnabled(data.enabled ?? false);
        setEntityId(data.entity_id ?? "");
        setSsoUrl(data.sso_url ?? "");
        setCertificate(data.certificate ?? "");
        setIssuerUrl(data.issuer_url ?? "");
        setClientId(data.client_id ?? "");
      }
    } catch { /* hub offline */ }
    finally { setLoading(false); }
  }, []);

  useEffect(() => { load(); }, [load]);

  const handleSave = async () => {
    setSaving(true);
    try {
      await updateSSOConfig({
        provider,
        enabled,
        entity_id:     entityId,
        sso_url:       ssoUrl,
        certificate,
        issuer_url:    issuerUrl,
        client_id:     clientId,
        client_secret: clientSecret,
      });
      setSaved(true);
      setClientSecret(""); // clear after save
      setTimeout(() => setSaved(false), 2500);
      load();
    } catch (e) {
      alert(e instanceof Error ? e.message : "Save failed");
    } finally {
      setSaving(false);
    }
  };

  void cfg;

  return (
    <div className="p-6 space-y-6">
      {/* Header */}
      <div className="flex items-center gap-3">
        <div className="p-2 rounded-lg bg-indigo-500/10">
          <KeyRound size={20} className="text-indigo-400" />
        </div>
        <div>
          <h1 className="text-lg font-semibold text-white">Single Sign-On</h1>
          <p className="text-xs text-slate-400 mt-0.5">
            Connect an identity provider so users sign in with their company account
          </p>
        </div>
      </div>

      {loading ? (
        <div className="flex items-center justify-center h-32">
          <RefreshCw size={20} className="animate-spin text-slate-600" />
        </div>
      ) : locked ? (
        <EnterpriseGate
          feature="sso"
          title="Team plan required for SSO"
          description="SAML 2.0 and OIDC single sign-on are available on the Team plan and above. Upgrade to connect Okta, Azure AD, Google Workspace, or any SAML/OIDC provider."
        />
      ) : (
        <>
          {/* Architecture note */}
          <div className="rounded-xl border border-indigo-500/20 bg-indigo-500/[0.04] p-4 space-y-2">
            <p className="text-xs font-semibold text-indigo-300">How SSO works in NetScope</p>
            <p className="text-xs text-slate-400 leading-relaxed">
              NetScope uses{" "}
              <a href="https://dexidp.io" target="_blank" rel="noopener noreferrer"
                className="text-indigo-400 hover:underline inline-flex items-center gap-0.5">
                Dex <ExternalLink size={10} />
              </a>{" "}
              as an open-source identity broker. Dex speaks SAML 2.0, OIDC, LDAP, and
              social logins. Configure your IdP below; Dex handles the protocol, and
              NetScope Hub receives a standard OIDC token.
            </p>
            <p className="text-xs text-slate-500">
              Deploy Dex alongside the hub — the Helm chart includes a Dex sidecar.
              Set <code className="text-slate-300 bg-white/[0.06] px-1 rounded">DEX_ISSUER_URL</code> and{" "}
              <code className="text-slate-300 bg-white/[0.06] px-1 rounded">DEX_CLIENT_ID</code> env vars on the hub.
            </p>
          </div>

          {/* Enabled toggle */}
          <div className="flex items-center justify-between bg-[#0d0d1a] border border-white/[0.06] rounded-xl px-5 py-4">
            <div>
              <p className="text-sm font-medium text-white">Enable SSO</p>
              <p className="text-xs text-slate-500 mt-0.5">
                When enabled, users must sign in through your identity provider.
              </p>
            </div>
            <button
              onClick={() => setEnabled(e => !e)}
              className={clsx(
                "relative inline-flex h-6 w-11 items-center rounded-full transition-colors",
                enabled ? "bg-indigo-600" : "bg-slate-700",
              )}
            >
              <span className={clsx(
                "inline-block h-4 w-4 transform rounded-full bg-white transition-transform",
                enabled ? "translate-x-6" : "translate-x-1",
              )} />
            </button>
          </div>

          {/* Provider tabs */}
          <div className="bg-[#0d0d1a] border border-white/[0.06] rounded-xl overflow-hidden">
            <div className="flex border-b border-white/[0.06] px-4">
              <TabBtn active={provider === "oidc"} onClick={() => setProvider("oidc")}>
                OIDC / Dex (recommended)
              </TabBtn>
              <TabBtn active={provider === "saml"} onClick={() => setProvider("saml")}>
                SAML 2.0
              </TabBtn>
            </div>

            <div className="p-5 space-y-4">
              {provider === "oidc" ? (
                <>
                  <p className="text-xs text-slate-500">
                    Configure Dex as your OIDC provider, or connect any OIDC-compliant IdP
                    (Okta, Azure AD, Google Workspace, Keycloak, Auth0).
                  </p>
                  <Field label="Issuer URL" help="e.g. https://dex.yourcompany.com or https://yourcompany.okta.com">
                    <input value={issuerUrl} onChange={e => setIssuerUrl(e.target.value)}
                      placeholder="https://dex.example.com"
                      className={inputCls} />
                  </Field>
                  <Field label="Client ID">
                    <input value={clientId} onChange={e => setClientId(e.target.value)}
                      placeholder="netscope"
                      className={inputCls} />
                  </Field>
                  <Field label="Client secret" help="Write-only. Stored as SSO_CLIENT_SECRET env var, never in the database.">
                    <input type="password" value={clientSecret}
                      onChange={e => setClientSecret(e.target.value)}
                      placeholder={clientId ? "••••••••  (set)" : "Enter secret"}
                      className={inputCls} />
                  </Field>
                  <div className="rounded-lg bg-white/[0.03] border border-white/[0.06] p-3 space-y-1">
                    <p className="text-[10px] font-semibold text-slate-400 uppercase tracking-wide">
                      Redirect URI (register this in your IdP)
                    </p>
                    <p className="text-xs font-mono text-slate-300">
                      {typeof window !== "undefined"
                        ? window.location.origin.replace(":3000", ":8080")
                        : "https://your-hub-domain"}/api/v1/enterprise/auth/oidc/callback
                    </p>
                  </div>
                </>
              ) : (
                <>
                  <p className="text-xs text-slate-500">
                    Configure NetScope as a SAML 2.0 service provider. Paste your IdP
                    metadata below (from Okta, Azure AD, ADFS, etc.).
                  </p>
                  <Field label="IdP Entity ID / Issuer">
                    <input value={entityId} onChange={e => setEntityId(e.target.value)}
                      placeholder="https://your-idp.example.com/saml"
                      className={inputCls} />
                  </Field>
                  <Field label="IdP SSO URL">
                    <input value={ssoUrl} onChange={e => setSsoUrl(e.target.value)}
                      placeholder="https://your-idp.example.com/sso/saml"
                      className={inputCls} />
                  </Field>
                  <Field label="IdP X.509 certificate (PEM)"
                    help="The signing certificate from your IdP metadata XML.">
                    <textarea value={certificate} onChange={e => setCertificate(e.target.value)}
                      rows={5}
                      placeholder={"-----BEGIN CERTIFICATE-----\nMIIC..."}
                      className={clsx(inputCls, "font-mono resize-none")} />
                  </Field>
                  <div className="rounded-lg bg-white/[0.03] border border-white/[0.06] p-3 space-y-1.5">
                    <p className="text-[10px] font-semibold text-slate-400 uppercase tracking-wide">
                      SP details (register these in your IdP)
                    </p>
                    {[
                      ["ACS URL", "/api/v1/enterprise/auth/saml/acs"],
                      ["Entity ID", "/api/v1/enterprise/auth/saml/metadata"],
                      ["Metadata URL", "/api/v1/enterprise/auth/saml/metadata"],
                    ].map(([label, path]) => (
                      <div key={label}>
                        <p className="text-[10px] text-slate-500">{label}</p>
                        <p className="text-xs font-mono text-slate-300">
                          {typeof window !== "undefined"
                            ? window.location.origin.replace(":3000", ":8080")
                            : "https://your-hub-domain"}{path}
                        </p>
                      </div>
                    ))}
                  </div>
                </>
              )}
            </div>
          </div>

          {/* Save */}
          <button
            onClick={handleSave}
            disabled={saving}
            className={clsx(
              "flex items-center gap-1.5 px-4 py-2 rounded-lg text-sm font-medium transition-colors",
              saved
                ? "bg-emerald-600/20 text-emerald-400 border border-emerald-500/30"
                : "bg-indigo-600 hover:bg-indigo-500 text-white disabled:opacity-40",
            )}
          >
            {saving ? <RefreshCw size={13} className="animate-spin" /> :
             saved   ? <CheckCircle size={13} /> : <Save size={13} />}
            {saved ? "Saved!" : "Save configuration"}
          </button>
        </>
      )}
    </div>
  );
}

const inputCls =
  "w-full bg-white/[0.04] border border-white/10 rounded-lg px-3 py-2 " +
  "text-xs text-white placeholder:text-slate-600 focus:outline-none focus:border-indigo-500/50";

function Field({
  label, help, children,
}: {
  label: string; help?: string; children: React.ReactNode;
}) {
  return (
    <div className="space-y-1.5">
      <label className="text-xs font-medium text-slate-300">{label}</label>
      {children}
      {help && <p className="text-[10px] text-slate-500">{help}</p>}
    </div>
  );
}
