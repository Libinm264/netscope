"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { Network, RefreshCw, CheckCircle2, ArrowRight } from "lucide-react";
import { clsx } from "clsx";

// First-run setup page — shown when no admin account exists yet.
// The backend's POST /api/v1/auth/setup endpoint creates the owner account and
// returns a session cookie, so the user lands on the dashboard automatically.
//
// If setup is already complete (needs_setup === false) we redirect to /login.

export default function SetupPage() {
  const router = useRouter();

  const [name,     setName]     = useState("");
  const [email,    setEmail]    = useState("");
  const [password, setPassword] = useState("");
  const [confirm,  setConfirm]  = useState("");
  const [loading,  setLoading]  = useState(false);
  const [checking, setChecking] = useState(true);
  const [error,    setError]    = useState("");
  const [done,     setDone]     = useState(false);

  // Guard: if setup is already done, bounce to /login immediately.
  useEffect(() => {
    fetch("/api/proxy/auth/setup")
      .then((r) => r.json())
      .then((d) => {
        if (!d.needs_setup) {
          router.replace("/login");
        } else {
          setChecking(false);
        }
      })
      .catch(() => setChecking(false));
  }, [router]);

  const passwordsMatch = password === confirm;
  const canSubmit = name.trim() && email.trim() && password.length >= 12 && passwordsMatch;

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!canSubmit) return;
    setLoading(true);
    setError("");
    try {
      const res = await fetch("/api/proxy/auth/setup", {
        method:  "POST",
        headers: { "Content-Type": "application/json" },
        body:    JSON.stringify({
          name:     name.trim(),
          email:    email.trim(),
          password,
        }),
      });
      if (res.ok) {
        setDone(true);
        // Brief pause so the success state is visible, then go to dashboard.
        setTimeout(() => router.push("/"), 1500);
      } else {
        const data = await res.json().catch(() => ({}));
        setError(data.error ?? "Setup failed — please try again.");
      }
    } catch {
      setError("Unable to reach the hub. Check your connection.");
    } finally {
      setLoading(false);
    }
  };

  // ── Spinner while checking setup status ───────────────────────────────────
  if (checking) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-[#0a0a14]">
        <RefreshCw size={20} className="animate-spin text-slate-500" />
      </div>
    );
  }

  // ── Success state ─────────────────────────────────────────────────────────
  if (done) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-[#0a0a14] px-4">
        <div className="flex flex-col items-center gap-4 text-center">
          <CheckCircle2 size={48} className="text-emerald-400" />
          <h2 className="text-xl font-semibold text-white">You&apos;re all set!</h2>
          <p className="text-sm text-slate-400">Taking you to the dashboard…</p>
        </div>
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
            <p className="text-sm text-slate-500 mt-1">First-run setup</p>
          </div>
        </div>

        {/* Setup card */}
        <div className="rounded-2xl bg-[#0d0d1a] border border-white/[0.08] p-8 space-y-5">
          <div>
            <h2 className="text-base font-semibold text-white">Create your admin account</h2>
            <p className="text-xs text-slate-500 mt-0.5">
              This account will be the organisation owner — it can invite team members and manage all settings.
            </p>
          </div>

          <form onSubmit={handleSubmit} className="space-y-3">
            {/* Full name */}
            <div>
              <label className="text-xs text-slate-400 mb-1 block">Full name</label>
              <input
                type="text"
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder="Jane Smith"
                autoComplete="name"
                required
                className={clsx(
                  "w-full bg-white/[0.04] border rounded-lg px-3 py-2.5 text-sm text-white",
                  "placeholder:text-slate-600 focus:outline-none transition-colors",
                  "border-white/10 focus:border-indigo-500/50",
                )}
              />
            </div>

            {/* Email */}
            <div>
              <label className="text-xs text-slate-400 mb-1 block">Email address</label>
              <input
                type="email"
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                placeholder="jane@company.com"
                autoComplete="email"
                required
                className={clsx(
                  "w-full bg-white/[0.04] border rounded-lg px-3 py-2.5 text-sm text-white",
                  "placeholder:text-slate-600 focus:outline-none transition-colors",
                  error ? "border-red-500/40" : "border-white/10 focus:border-indigo-500/50",
                )}
              />
            </div>

            {/* Password */}
            <div>
              <label className="text-xs text-slate-400 mb-1 block">Password</label>
              <input
                type="password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                placeholder="At least 12 characters"
                autoComplete="new-password"
                required
                className={clsx(
                  "w-full bg-white/[0.04] border rounded-lg px-3 py-2.5 text-sm text-white",
                  "placeholder:text-slate-600 focus:outline-none transition-colors",
                  error ? "border-red-500/40" : "border-white/10 focus:border-indigo-500/50",
                )}
              />
            </div>

            {/* Confirm password */}
            <div>
              <label className="text-xs text-slate-400 mb-1 block">Confirm password</label>
              <input
                type="password"
                value={confirm}
                onChange={(e) => setConfirm(e.target.value)}
                placeholder="Re-enter password"
                autoComplete="new-password"
                required
                className={clsx(
                  "w-full bg-white/[0.04] border rounded-lg px-3 py-2.5 text-sm text-white",
                  "placeholder:text-slate-600 focus:outline-none transition-colors",
                  confirm && !passwordsMatch
                    ? "border-red-500/40"
                    : "border-white/10 focus:border-indigo-500/50",
                )}
              />
              {confirm && !passwordsMatch && (
                <p className="text-[11px] text-red-400 mt-1">Passwords don&apos;t match</p>
              )}
            </div>

            {error && (
              <p className="text-xs text-red-400 bg-red-500/10 border border-red-500/20
                            rounded-lg px-3 py-2">
                {error}
              </p>
            )}

            <button
              type="submit"
              disabled={loading || !canSubmit}
              className="flex items-center justify-center gap-2 w-full px-4 py-2.5 rounded-lg
                         bg-indigo-600 hover:bg-indigo-500 disabled:opacity-40 transition-colors
                         text-white text-sm font-medium mt-1"
            >
              {loading
                ? <RefreshCw size={14} className="animate-spin" />
                : <ArrowRight size={14} />
              }
              {loading ? "Creating account…" : "Create admin account"}
            </button>
          </form>

          <p className="text-center text-[11px] text-slate-600">
            Already have an account?{" "}
            <a href="/login" className="text-slate-500 hover:text-slate-400 transition-colors">
              Sign in
            </a>
          </p>
        </div>
      </div>
    </div>
  );
}
