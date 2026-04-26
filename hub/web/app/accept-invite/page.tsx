"use client";

import { Suspense, useState } from "react";
import { useSearchParams, useRouter } from "next/navigation";
import { Network, KeyRound, RefreshCw, CheckCircle } from "lucide-react";
import { clsx } from "clsx";

function AcceptInviteInner() {
  const searchParams = useSearchParams();
  const router       = useRouter();
  const token        = searchParams.get("token") ?? "";

  const [password,  setPassword]  = useState("");
  const [confirm,   setConfirm]   = useState("");
  const [loading,   setLoading]   = useState(false);
  const [error,     setError]     = useState("");
  const [done,      setDone]      = useState(false);

  const mismatch = confirm.length > 0 && password !== confirm;
  const tooShort = password.length > 0 && password.length < 12;
  const canSubmit = token && password.length >= 12 && password === confirm && !loading;

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!canSubmit) return;
    setLoading(true);
    setError("");
    try {
      const res = await fetch("/api/proxy/enterprise/auth/invite/accept", {
        method:  "POST",
        headers: { "Content-Type": "application/json" },
        body:    JSON.stringify({ token, password }),
      });
      if (res.ok) {
        setDone(true);
        setTimeout(() => router.push("/"), 1500);
      } else {
        const data = await res.json().catch(() => ({}));
        setError(data.error ?? "Something went wrong — the link may have expired.");
      }
    } catch {
      setError("Unable to reach the hub. Check your connection.");
    } finally {
      setLoading(false);
    }
  };

  if (!token) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-[#0a0a14] px-4">
        <p className="text-red-400 text-sm">Invalid invite link — token is missing.</p>
      </div>
    );
  }

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
            <p className="text-sm text-slate-500 mt-1">Set your password to get started</p>
          </div>
        </div>

        <div className="rounded-2xl bg-[#0d0d1a] border border-white/[0.08] p-8 space-y-5">
          {done ? (
            <div className="flex flex-col items-center gap-3 py-4">
              <CheckCircle size={32} className="text-emerald-400" />
              <p className="text-white font-medium">Password set — signing you in…</p>
            </div>
          ) : (
            <>
              <div>
                <h2 className="text-base font-semibold text-white">Accept your invitation</h2>
                <p className="text-xs text-slate-500 mt-0.5">
                  Choose a password with at least 12 characters.
                </p>
              </div>

              <form onSubmit={handleSubmit} className="space-y-3">
                <input
                  type="password"
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  placeholder="New password (min 12 chars)"
                  autoComplete="new-password"
                  className={clsx(
                    "w-full bg-white/[0.04] border rounded-lg px-3 py-2.5 text-sm text-white",
                    "placeholder:text-slate-600 focus:outline-none transition-colors",
                    tooShort ? "border-amber-500/40" : "border-white/10 focus:border-indigo-500/50",
                  )}
                />
                <input
                  type="password"
                  value={confirm}
                  onChange={(e) => setConfirm(e.target.value)}
                  placeholder="Confirm password"
                  autoComplete="new-password"
                  className={clsx(
                    "w-full bg-white/[0.04] border rounded-lg px-3 py-2.5 text-sm text-white",
                    "placeholder:text-slate-600 focus:outline-none transition-colors",
                    mismatch ? "border-red-500/40" : "border-white/10 focus:border-indigo-500/50",
                  )}
                />
                {mismatch && (
                  <p className="text-xs text-red-400">Passwords do not match.</p>
                )}
                {tooShort && !mismatch && (
                  <p className="text-xs text-amber-400">Password must be at least 12 characters.</p>
                )}
                {error && (
                  <p className="text-xs text-red-400 bg-red-500/10 border border-red-500/20
                                rounded-lg px-3 py-2">
                    {error}
                  </p>
                )}
                <button
                  type="submit"
                  disabled={!canSubmit}
                  className="flex items-center justify-center gap-2 w-full px-4 py-2.5 rounded-lg
                             bg-indigo-600 hover:bg-indigo-500 disabled:opacity-40 transition-colors
                             text-white text-sm font-medium"
                >
                  {loading ? <RefreshCw size={14} className="animate-spin" /> : <KeyRound size={14} />}
                  {loading ? "Setting password…" : "Set password & sign in"}
                </button>
              </form>
            </>
          )}
        </div>
      </div>
    </div>
  );
}

export default function AcceptInvitePage() {
  return (
    <Suspense>
      <AcceptInviteInner />
    </Suspense>
  );
}
