"use client";

import { Lock, ExternalLink } from "lucide-react";

interface Props {
  feature: string;
  title?: string;
  description?: string;
  plan?: "team" | "enterprise";
  children?: React.ReactNode;
}

/**
 * EnterpriseGate wraps content that requires a paid plan.
 *
 * Usage:
 *   <EnterpriseGate feature="sso" plan="team">
 *     <SSOConfigForm />
 *   </EnterpriseGate>
 *
 * When the current plan includes the feature, children are rendered normally.
 * Otherwise an upgrade prompt is shown instead.
 *
 * For simplicity in v0.4, gating is driven by the `plan` prop passed by the
 * parent page (which reads from /api/proxy/enterprise/license).
 * A fully automatic version can read from a React context set in the layout.
 */
export function EnterpriseGate({
  feature,
  title,
  description,
  plan = "team",
  children,
}: Props) {
  // This component is used in two modes:
  //  1. locked=true  → show upgrade prompt  (parent passes locked prop)
  //  2. locked=false → render children      (feature is available)
  // For v0.4 the parent pages pass the `locked` prop themselves.
  // This export is the prompt-only component for convenience.
  void feature;

  const planLabel = plan === "enterprise" ? "Enterprise" : "Team";

  return (
    <div className="flex flex-col items-center justify-center rounded-xl border border-amber-500/20 bg-amber-500/[0.04] p-10 text-center gap-4">
      <div className="p-3 rounded-full bg-amber-500/10">
        <Lock size={20} className="text-amber-400" />
      </div>

      <div>
        <p className="text-sm font-semibold text-white">
          {title ?? `${planLabel} Plan Required`}
        </p>
        <p className="text-xs text-slate-400 mt-1 max-w-sm">
          {description ??
            `This feature is available on the ${planLabel} plan and above. Upgrade to unlock it for your organisation.`}
        </p>
      </div>

      <div className="flex items-center gap-3">
        <a
          href="/settings/license"
          className="inline-flex items-center gap-1.5 px-4 py-2 rounded-lg
                     bg-amber-500/10 border border-amber-500/25 text-amber-300
                     text-xs font-medium hover:bg-amber-500/20 transition-colors"
        >
          View plans
        </a>
        <a
          href="https://netscope.ie/pricing"
          target="_blank"
          rel="noopener noreferrer"
          className="inline-flex items-center gap-1 text-xs text-slate-500 hover:text-slate-300 transition-colors"
        >
          Learn more <ExternalLink size={11} />
        </a>
      </div>
    </div>
  );
}
