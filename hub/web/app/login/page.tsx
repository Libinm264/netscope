import { Network } from "lucide-react";

export default function LoginPage() {
  return (
    <div className="min-h-screen flex items-center justify-center bg-[#0a0a14]">
      <div className="w-full max-w-sm space-y-8">
        {/* Logo */}
        <div className="flex flex-col items-center gap-3">
          <div className="p-3 rounded-xl bg-indigo-500/10 border border-indigo-500/20">
            <Network size={28} className="text-indigo-400" />
          </div>
          <div className="text-center">
            <h1 className="text-2xl font-bold text-white">NetScope Hub</h1>
            <p className="mt-1 text-sm text-slate-500">
              Network observability dashboard
            </p>
          </div>
        </div>

        {/* Card */}
        <div className="rounded-2xl bg-[#0d0d1a] border border-white/[0.08] p-8 space-y-6">
          <div className="space-y-1">
            <h2 className="text-lg font-semibold text-white">Sign in</h2>
            <p className="text-sm text-slate-500">
              Authenticate with your organisation account to continue.
            </p>
          </div>

          <a
            href="/api/auth/login"
            className="flex items-center justify-center w-full px-4 py-2.5 rounded-lg
                       bg-indigo-600 hover:bg-indigo-500 active:bg-indigo-700
                       text-white font-medium text-sm transition-colors"
          >
            Continue with Auth0
          </a>

          <p className="text-center text-xs text-slate-600">
            Access is restricted to authorised users only.
          </p>
        </div>
      </div>
    </div>
  );
}
