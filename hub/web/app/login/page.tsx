"use client";

import { Suspense, useState } from "react";
import { useSearchParams } from "next/navigation";
import { Network, LogIn, RefreshCw, ArrowRight } from "lucide-react";
import { clsx } from "clsx";

// Build the SSO initiation URL — points directly at the Go backend so the
// browser follows the IdP redirect without going through the Next.js proxy.
function ssoInitURL(provider: "oidc" | "saml", redirectAfter: string): string {
  if (typeof window === "undefined") return "#";
  const hubBase = window.location.origin.replace(":3000", ":8080");
  const encoded = encodeURIComponent(window.location.origin + redirectAfter);
  return `${hubBase}/api/v1/enterprise/auth/${provider}/initiate?redirect_uri=${encoded}`;
}

function LoginInner() {
  const searchParams  = useSearchParams();
  const from          = searchParams.get("from") ?? "/";

  const [email,    setEmail]    = useState("");
  const [password, setPassword] = useState("");
  const [loading,  setLoading]  = useState(false);
  const [error,    setError]    = useState("");

  // Email/password submit — active once Pass B (OIDC) and Pass C (local login) land.
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

        {/* Card */}
        <div className="rounded-2xl bg-[#0d0d1a] border border-white/[0.08] p-8 space-y-5">
          <div>
            <h2 className="text-base font-semibold text-white">Sign in</h2>
            <p className="text-xs text-slate-500 mt-0.5">
              Access your organisation&apos;s dashboard
            </p>
          </div>

          {/* SSO buttons */}
          <div className="space-y-2">
            {/* Primary: OIDC (Okta, Azure AD, Dex, Google) */}
            <a
              href={ssoInitURL("oidc", from)}
              className="flex items-center justify-between w-full px-4 py-2.5 rounded-lg
                         bg-indigo-600 hover:bg-indigo-500 text-white text-sm font-medium
                         transition-colors group"
            >
              <span>Continue with SSO (OIDC)</span>
              <ArrowRight size={14} className="opacity-60 group-hover:opacity-100 transition-opacity" />
            </a>

            {/* Secondary: SAML (Okta, Azure AD, ADFS, OneLogin) */}
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

          {/* Divider */}
          <div className="relative">
            <div className="absolute inset-0 flex items-center">
              <div className="w-full border-t border-white/[0.07]" />
            </div>
            <div className="relative flex justify-center">
              <span className="bg-[#0d0d1a] px-3 text-[11px] text-slate-600">
                or sign in with email
              </span>
            </div>
          </div>

          {/* Email / password form */}
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
                         bg-white/[0.05] border border-white/10 text-white text-sm font-medium
                         hover:bg-white/[0.09] disabled:opacity-40 transition-colors"
            >
              {loading
                ? <RefreshCw size={14} className="animate-spin" />
                : <LogIn size={14} />
              }
              {loading ? "Signing in…" : "Sign in"}
            </button>
          </form>

          <p className="text-center text-[11px] text-slate-600">
            Access is restricted to authorised users only.
          </p>
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
