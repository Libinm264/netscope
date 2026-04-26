"use client";

import { usePathname } from "next/navigation";
import Link from "next/link";
import { clsx } from "clsx";

const NAV = [
  { href: "/settings",          label: "Tokens & Audit" },
  { href: "/settings/org",      label: "Organisation" },
  { href: "/settings/members",  label: "Members & Roles",  badge: "Enterprise" },
  { href: "/settings/teams",    label: "Teams",            badge: "Enterprise" },
  { href: "/settings/sso",      label: "SSO",              badge: "Enterprise" },
  { href: "/settings/license",  label: "License & Plan" },
];

export default function SettingsLayout({ children }: { children: React.ReactNode }) {
  const path = usePathname();

  return (
    <div className="flex h-full min-h-0">
      {/* Sub-nav sidebar */}
      <aside className="w-48 shrink-0 border-r border-white/[0.06] p-4 overflow-y-auto">
        <p className="text-[10px] font-semibold text-slate-500 uppercase tracking-wider mb-3">
          Settings
        </p>
        <nav className="space-y-0.5">
          {NAV.map(({ href, label, badge }) => (
            <Link
              key={href}
              href={href}
              className={clsx(
                "flex items-center justify-between px-2.5 py-1.5 rounded-md text-xs transition-colors",
                path === href
                  ? "bg-indigo-500/10 text-indigo-300"
                  : "text-slate-400 hover:text-slate-200 hover:bg-white/[0.04]",
              )}
            >
              {label}
              {badge && (
                <span className="text-[9px] px-1 py-0.5 rounded bg-amber-500/10 text-amber-400 font-medium border border-amber-500/20">
                  {badge}
                </span>
              )}
            </Link>
          ))}
        </nav>
      </aside>

      {/* Page content */}
      <div className="flex-1 overflow-auto">
        {children}
      </div>
    </div>
  );
}
