"use client";

import { useEffect, useState } from "react";
import { Network, Server, Zap, ArrowRight, Terminal } from "lucide-react";
import Link from "next/link";
import { StatsCards } from "@/components/StatsCards";
import { LiveFeed } from "@/components/LiveFeed";
import { ProtocolChart } from "@/components/ProtocolChart";
import { FlowsChart } from "@/components/FlowsChart";
import { fetchStats } from "@/lib/api";

// ── Onboarding guide shown when no flows have been captured yet ───────────────

function OnboardingGuide() {
  const steps = [
    {
      num: "1",
      title: "Build the agent",
      description: "Compile the Rust agent from source",
      code: "cd agent && cargo build --release",
      icon: Terminal,
    },
    {
      num: "2",
      title: "Enrol a machine",
      description: "Generate an enrollment token in Settings, then run",
      code: 'curl -sSL "http://localhost:8080/install?token=<token>" | sh',
      icon: Server,
    },
    {
      num: "3",
      title: "Watch flows arrive",
      description: "Live traffic will appear here automatically",
      code: null,
      icon: Zap,
    },
  ];

  return (
    <div className="flex flex-col items-center justify-center min-h-[60vh] px-4">
      <div className="w-full max-w-2xl space-y-8 text-center">
        {/* Logo lockup */}
        <div className="flex flex-col items-center gap-3">
          <div className="p-4 rounded-2xl bg-indigo-500/10 ring-1 ring-indigo-500/20">
            <Network size={36} className="text-indigo-400" />
          </div>
          <div>
            <h1 className="text-2xl font-bold text-white">Welcome to NetScope Hub</h1>
            <p className="text-sm text-slate-400 mt-1">
              No flows yet — follow these steps to start capturing network traffic
            </p>
          </div>
        </div>

        {/* Steps */}
        <div className="grid gap-3 text-left">
          {steps.map((step, i) => {
            const Icon = step.icon;
            return (
              <div
                key={i}
                className="flex gap-4 p-4 rounded-xl bg-[#0d0d1a] border border-white/[0.06]"
              >
                <div className="flex flex-col items-center gap-2 shrink-0">
                  <div className="w-7 h-7 rounded-full bg-indigo-500/20 flex items-center justify-center
                                  text-xs font-bold text-indigo-400">
                    {step.num}
                  </div>
                  {i < steps.length - 1 && (
                    <div className="w-px flex-1 bg-white/[0.04]" />
                  )}
                </div>
                <div className="flex-1 pt-0.5 space-y-1.5 pb-1">
                  <div className="flex items-center gap-2">
                    <Icon size={13} className="text-slate-400" />
                    <p className="text-sm font-semibold text-white">{step.title}</p>
                  </div>
                  <p className="text-xs text-slate-500">{step.description}</p>
                  {step.code && (
                    <div className="font-mono text-[11px] text-emerald-300 bg-black/30
                                    rounded px-3 py-2 mt-1 border border-white/[0.04]">
                      {step.code}
                    </div>
                  )}
                </div>
              </div>
            );
          })}
        </div>

        {/* CTA */}
        <div className="flex items-center justify-center gap-3">
          <Link
            href="/agents"
            className="flex items-center gap-2 px-4 py-2 rounded-lg
                       bg-indigo-600 hover:bg-indigo-500 text-white text-sm font-medium transition-colors"
          >
            <Server size={14} /> Add your first agent
          </Link>
          <Link
            href="/settings"
            className="flex items-center gap-2 px-4 py-2 rounded-lg
                       border border-white/10 text-slate-300 hover:text-white
                       hover:bg-white/[0.04] text-sm transition-colors"
          >
            Settings <ArrowRight size={13} />
          </Link>
        </div>
      </div>
    </div>
  );
}

// ── Page ───────────────────────────────────────────────────────────────────────

export default function DashboardPage() {
  const [hasFlows, setHasFlows] = useState<boolean | null>(null); // null = loading

  useEffect(() => {
    fetchStats()
      .then((s) => setHasFlows(s.total_flows > 0))
      .catch(() => setHasFlows(true)); // on error, show the dashboard anyway
  }, []);

  // Loading state — avoid flash
  if (hasFlows === null) return null;

  if (!hasFlows) return <OnboardingGuide />;

  return (
    <div className="p-6 space-y-6">
      <h1 className="text-xl font-semibold text-white">Dashboard</h1>
      <StatsCards />
      <div className="grid grid-cols-1 xl:grid-cols-3 gap-6">
        <div className="xl:col-span-2 space-y-6">
          <FlowsChart />
          <LiveFeed />
        </div>
        <div>
          <ProtocolChart />
        </div>
      </div>
    </div>
  );
}
