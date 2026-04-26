"use client";

import { useState } from "react";
import { Network, Mail, RefreshCw, CheckCircle } from "lucide-react";
import { clsx } from "clsx";
import Link from "next/link";

export default function ForgotPasswordPage() {
  const [email,   setEmail]   = useState("");
  const [loading, setLoading] = useState(false);
  const [done,    setDone]    = useState(false);
  const [error,   setError]   = useState("");

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!email) return;
    setLoading(true);
    setError("");
    try {
      const res = await fetch("/api/proxy/enterprise/auth/forgot-password", {
        method:  "POST",
        headers: { "Content-Type": "application/json" },
        body:    JSON.stringify({ email: email.trim() }),
      });
      if (res.ok) {
        setDone(true);
      } else {
        const data = await res.json().catch(() => ({}));
        setError(data.error ?? "Something went wrong — please try again.");
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
            <p className="text-sm text-slate-500 mt-1">Reset your password</p>
          </div>
        </div>

        <div className="rounded-2xl bg-[#0d0d1a] border border-white/[0.08] p-8 space-y-5">
          {done ? (
            <div className="flex flex-col items-center gap-3 py-4 text-center">
              <CheckCircle size={32} className="text-emerald-400" />
              <p className="text-white font-medium">Check your inbox</p>
              <p className="text-xs text-slate-500">
                If that email is registered you&apos;ll receive a reset link shortly.
              </p>
              <Link href="/login" className="text-xs text-indigo-400 hover:text-indigo-300 transition-colors">
                Back to sign in
              </Link>
            </div>
          ) : (
            <>
              <div>
                <h2 className="text-base font-semibold text-white">Forgot your password?</h2>
                <p className="text-xs text-slate-500 mt-0.5">
                  Enter your email and we&apos;ll send you a reset link.
                </p>
              </div>

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
                {error && (
                  <p className="text-xs text-red-400 bg-red-500/10 border border-red-500/20
                                rounded-lg px-3 py-2">
                    {error}
                  </p>
                )}
                <button
                  type="submit"
                  disabled={loading || !email}
                  className="flex items-center justify-center gap-2 w-full px-4 py-2.5 rounded-lg
                             bg-indigo-600 hover:bg-indigo-500 disabled:opacity-40 transition-colors
                             text-white text-sm font-medium"
                >
                  {loading ? <RefreshCw size={14} className="animate-spin" /> : <Mail size={14} />}
                  {loading ? "Sending…" : "Send reset link"}
                </button>
              </form>

              <p className="text-center text-xs">
                <Link href="/login" className="text-slate-500 hover:text-slate-400 transition-colors">
                  Back to sign in
                </Link>
              </p>
            </>
          )}
        </div>
      </div>
    </div>
  );
}
