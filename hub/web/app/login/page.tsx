"use client";

import { Suspense, useEffect, useState } from "react";
import { useSearchParams } from "next/navigation";
import { Network, LogIn, RefreshCw, ArrowRight, FlaskConical, ChevronDown, ChevronUp } from "lucide-react";
import { clsx } from "clsx";
import Link from "next/link";

// Build the SSO initiation URL — points directly at the Go backend so the
// browser follows the IdP redirect without going through the Next.js proxy.
function ssoInitURL(provider: "oidc" | "saml", redirectAfter: string): string {
  if (typeof window === "undefined") return "#";
  const hubBase = window.location.origin.replace(":3000", ":8080");
  const encoded = encodeURIComponent(window.location.origin + redirectAfter);
  return `${hubBase}/api/v1/enterprise/auth/${provider}/initiate?redirect_uri=${encoded}`;
}

interface SetupStatus {
  needs_setup:   boolean;
  demo_enabled:  boolean;
}

function LoginInner() {
  const searchParams  = useSearchParams();
  const from          = searchParams.get("from") ?? "/";

  const [email,       setEmail]       = useState("");
  const [password,    setPassword]    = useState("");
  const [loading,     setLoading]     = useState(false);
  const [demoLoading, setDemoLoading] = useState(false);
  const [error,       setError]       = useState("");
  const [showSSO,     setShowSSO]     = useState(false);
  const [setup,       setSetup]       = useState<SetupStatus | null>(null);

  // Fetch setup status once — determines whether to show demo / setup nudges.
  useEffect(() => {
    fetch("/api/proxy/auth/setup")
      .then((r) => r.json())
      .then((d: SetupStatus) => setSetup(d))
      .catch(() => { /* non-fatal; UI degrades gracefully */ });
  }, []);

  // Email/password submit
  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!email || !password) return;
    setLoading(true);
    setError("");
    try {
      const res = await fetch("/api/proxy/enterprise/auth/login", {
        method:  "POST",
        headers: { "Content-Type": "application/json" },
        body:    JSON.stringify({ email: email.trim(), password }),
      });
      if (res.ok) {
        window.location.href = from;
      } else {
        const data = await res.json().catch(() => ({}));
        setError(data.error ?? "Sign-in failed — check your credentials.");
      }
    } catch {
      setError("Unable to reach the hub. Check your connection.");
    } finally {
      setLoading(false);
    }
  };

  // Demo login — creates a sandboxed read-only session
  const handleDemo = async () => {
    setDemoLoading(true);
    setError("");
    try {
      const res = await fetch("/api/proxy/auth/demo", { method: "POST" });
      if (res.ok) {
        window.location.href = from;
      } else {
        const data = await res.json().catch(() => ({}));
        setError(data.error ?? "Could not start demo session.");
      }
    } catch {
      setError("Unable to reach the hub. Check your connection.");
    } finally {
      setDemoLoading(false);
    }
  };

  return (
    <div className="min-h-screen flex items-center justify-center bg-[#0a0a14] px-4">
      <div className="w-full max-w-sm space-y-8">

        {/* Logo */}
        <div className="flex flex-col items-center gap-3">
          <div className="p-3 rounded-xl bg-indigo-500/10 border border-indigo-500/20">
            <Network size={28} className="text-indigo-400" />
          </div>
          <div className="text-center">
            <h1 className="text-2xl font-bold text-white">NetScope Hub</h1>
            <p className="text-sm text-slate-500 mt-1">Network observability dashboard</p>
          </div>
        </div>

        {/* First-run nudge — only shown when no admin account exists yet */}
        {setup?.needs_setup && (
          <div className="rounded-xl bg-amber-500/10 border border-amber-500/20 px-4 py-3 flex items-start gap-3">
            <span className="text-amber-400 mt-0.5 text-lg">⚙</span>
            <div className="flex-1 min-w-0">
              <p className="text-sm font-medium text-amber-300">First-run setup required</p>
              <p className="text-xs text-amber-400/70 mt-0.5">
                No admin account exists yet. Create one to get started.
              </p>
              <Link
                href="/setup"
                className="inline-flex items-center gap-1 mt-2 text-xs font-medium
                           text-amber-300 hover:text-amber-200 transition-colors"
              >
                Set up your account <ArrowRight size={12} />
              </Link>
            </div>
          </div>
        )}

        {/* Card */}
        <div className="rounded-2xl bg-[#0d0d1a] border border-white/[0.08] p-8 space-y-5">
          <div>
            <h2 className="text-base font-semibold text-white">Sign in</h2>
            <p className="text-xs text-slate-500 mt-0.5">
              Access your organisation&apos;s dashboard
            </p>
          </div>

          {/* Email / password form — primary for community installs */}
          <form onSubmit={handleSubmit} className="space-y-3">
            <input
              type="email"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              placeholder="you@company.com"
              autoComplete="email"
              required
              className={clsx(
                "w-full bg-white/[0.04] border rounded-lg px-3 py-2.5 text-sm text-white",
                "placeholder:text-slate-600 focus:outline-none transition-colors",
                error ? "border-red-500/40" : "border-white/10 focus:border-indigo-500/50",
              )}
            />
            <input
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              placeholder="Password"
              autoComplete="current-password"
              className={clsx(
                "w-full bg-white/[0.04] border rounded-lg px-3 py-2.5 text-sm text-white",
                "placeholder:text-slate-600 focus:outline-none transition-colors",
                error ? "border-red-500/40" : "border-white/10 focus:border-indigo-500/50",
              )}
            />

            {error && (
              <p className="text-xs text-red-400 bg-red-500/10 border border-red-500/20
                            rounded-lg px-3 py-2">
                {error}
              </p>
            )}

            <button
              type="submit"
              disabled={loading || !email || !password}
              className="flex items-center justify-center gap-2 w-full px-4 py-2.5 rounded-lg
                         bg-indigo-600 hover:bg-indigo-500 disabled:opacity-40 transition-colors
                         text-white text-sm font-medium"
            >
              {loading
                ? <RefreshCw size={14} className="animate-spin" />
                : <LogIn size={14} />
              }
              {loading ? "Signing in…" : "Sign in"}
            </button>
          </form>

          {/* Demo button — only shown when hub has DEMO_ENABLED=true */}
          {setup?.demo_enabled && (
            <>
              <div className="relative">
                <div className="absolute inset-0 flex items-center">
                  <div className="w-full border-t border-white/[0.07]" />
                </div>
                <div className="relative flex justify-center">
                  <span className="bg-[#0d0d1a] px-3 text-[11px] text-slate-600">
                    or explore without an account
                  </span>
                </div>
              </div>

              <button
                type="button"
                onClick={handleDemo}
                disabled={demoLoading}
                className="flex items-center justify-center gap-2 w-full px-4 py-2.5 rounded-lg
                           bg-emerald-600/20 border border-emerald-500/30 text-emerald-300
                           text-sm font-medium hover:bg-emerald-600/30 disabled:opacity-40
                           transition-colors"
              >
                {demoLoading
                  ? <RefreshCw size={14} className="animate-spin" />
                  : <FlaskConical size={14} />
                }
                {demoLoading ? "Starting demo…" : "Try live demo"}
              </button>
              <p className="text-center text-[11px] text-slate-600">
                Read-only sandbox · no account required · resets on logout
              </p>
            </>
          )}

          {/* SSO — collapsed by default; enterprise users can expand */}
          <div className="space-y-2">
            <button
              type="button"
              onClick={() => setShowSSO((v) => !v)}
              className="flex items-center gap-1.5 text-xs text-slate-500 hover:text-slate-400
                         transition-colors w-full"
            >
              {showSSO ? <ChevronUp size={12} /> : <ChevronDown size={12} />}
              Enterprise SSO (OIDC / SAML)
            </button>

            {showSSO && (
              <div className="space-y-2 pt-1">
                <a
                  href={ssoInitURL("oidc", from)}
                  className="flex items-center justify-between w-full px-4 py-2.5 rounded-lg
                             bg-white/[0.04] border border-white/10 text-slate-300 text-sm font-medium
                             hover:bg-white/[0.08] transition-colors group"
                >
                  <span>Continue with OIDC</span>
                  <ArrowRight size={14} className="opacity-40 group-hover:opacity-80 transition-opacity" />
                </a>

                <a
                  href={ssoInitURL("saml", from)}
                  className="flex items-center justify-between w-full px-4 py-2.5 rounded-lg
                             bg-white/[0.04] border border-white/10 text-slate-300 text-sm font-medium
                             hover:bg-white/[0.08] transition-colors group"
                >
                  <span>Continue with SAML 2.0</span>
                  <ArrowRight size={14} className="opacity-40 group-hover:opacity-80 transition-opacity" />
                </a>
              </div>
            )}
          </div>

          <div className="flex justify-between text-[11px]">
            <Link href="/forgot-password" className="text-slate-500 hover:text-slate-400 transition-colors">
              Forgot password?
            </Link>
            {setup?.needs_setup && (
              <Link href="/setup" className="text-indigo-400 hover:text-indigo-300 transition-colors">
                First-run setup →
              </Link>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}

// Suspense boundary required because useSearchParams() needs it in Next.js 14.
export default function LoginPage() {
  return (
    <Suspense>
      <LoginInner />
    </Suspense>
  );
}
