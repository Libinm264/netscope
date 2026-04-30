"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import {
  Network, LayoutDashboard, List, Server, Bell, GitFork, BarChart3,
  ShieldCheck, ClipboardList, Shield, ShieldAlert, ScanSearch,
  Settings, Building2, Users, Users2, KeyRound, Zap, ChevronDown,
  Plug, Map, Cloud, Globe, Siren, FileBarChart2, Activity, Sparkles,
} from "lucide-react";
import { clsx } from "clsx";
import { useState } from "react";
import { UserMenu } from "@/components/UserMenu";

const NAV_MAIN = [
  { href: "/",           label: "Dashboard",  icon: LayoutDashboard },
  { href: "/flows",      label: "Flows",      icon: List },
  { href: "/services",   label: "Services",   icon: GitFork },
  { href: "/analytics",  label: "Analytics",  icon: BarChart3 },
  { href: "/agents",     label: "Agents",     icon: Server },
  { href: "/certs",      label: "Certs",      icon: ShieldCheck },
  { href: "/alerts",     label: "Alerts",     icon: Bell },
  { href: "/threats",    label: "Threats",    icon: Shield },
  { href: "/policies",   label: "Policies",   icon: ShieldAlert },
  { href: "/compliance", label: "Compliance", icon: ClipboardList },
  { href: "/sigma",      label: "Detection",  icon: ScanSearch },
  { href: "/anomalies",  label: "Anomalies",  icon: Activity },
  { href: "/incidents",  label: "Incidents",  icon: Siren },
  { href: "/fleet",      label: "Fleet",      icon: Globe },
  { href: "/cloud",      label: "Cloud Flows", icon: Cloud },
  { href: "/roadmap",    label: "Roadmap",    icon: Map },
];

const NAV_SETTINGS = [
  { href: "/settings",                label: "Tokens & Audit", icon: Settings },
  { href: "/settings/org",            label: "Organisation",   icon: Building2 },
  { href: "/settings/members",        label: "Members",        icon: Users,    badge: "Enterprise" },
  { href: "/settings/teams",          label: "Teams",          icon: Users2,   badge: "Enterprise" },
  { href: "/settings/sso",            label: "SSO",            icon: KeyRound, badge: "Enterprise" },
  { href: "/settings/integrations",   label: "Integrations",   icon: Plug,          badge: "Enterprise" },
  { href: "/settings/storage",        label: "Storage",        icon: Cloud,         badge: "Enterprise" },
  { href: "/compliance/reports",      label: "Reports",        icon: FileBarChart2, badge: "Enterprise" },
  { href: "/settings/license",        label: "License",        icon: Zap },
];

interface SidebarProps {
  user?: {
    name?: string | null;
    email?: string | null;
    picture?: string | null;
  } | null;
}

export function Sidebar({ user }: SidebarProps) {
  const pathname = usePathname();
  const inSettings = pathname.startsWith("/settings");
  const [settingsOpen, setSettingsOpen] = useState(inSettings);

  return (
    <aside className="fixed left-0 top-0 bottom-0 w-[220px] flex flex-col
                      bg-[#0d0d1a] border-r border-white/[0.06] z-50">
      {/* Logo */}
      <div className="flex items-center gap-2.5 px-5 py-5 border-b border-white/[0.06]">
        <div className="p-1.5 rounded-md bg-indigo-500/10">
          <Network size={18} className="text-indigo-400" />
        </div>
        <div>
          <p className="text-sm font-semibold text-white leading-none">NetScope</p>
          <p className="text-[10px] text-slate-500 mt-0.5">Hub</p>
        </div>
      </div>

      {/* Navigation */}
      <nav className="flex-1 px-3 py-4 overflow-y-auto space-y-0.5">
        {/* Main nav */}
        {NAV_MAIN.map(({ href, label, icon: Icon }) => {
          const active = href === "/" ? pathname === "/" : pathname === href;
          return (
            <Link
              key={href}
              href={href}
              className={clsx(
                "flex items-center gap-3 px-3 py-2 rounded-md text-sm transition-colors",
                active
                  ? "bg-indigo-500/10 text-indigo-400"
                  : "text-slate-400 hover:text-slate-200 hover:bg-white/[0.04]",
              )}
            >
              <Icon size={16} />
              {label}
            </Link>
          );
        })}

        {/* AI Copilot button */}
        <div className="pt-2 pb-1">
          <button
            onClick={() => window.dispatchEvent(new CustomEvent("open-copilot"))}
            className="w-full flex items-center gap-3 px-3 py-2 rounded-md text-sm
                       text-indigo-400 hover:text-indigo-300
                       bg-indigo-500/[0.08] hover:bg-indigo-500/[0.14]
                       border border-indigo-500/[0.15] hover:border-indigo-500/30
                       transition-all"
          >
            <Sparkles size={16} />
            <span>AI Copilot</span>
            <span className="ml-auto text-[9px] px-1.5 py-0.5 rounded
                             bg-indigo-500/20 text-indigo-300 font-medium border border-indigo-500/20">
              NEW
            </span>
          </button>
        </div>

        {/* Settings group */}
        <div className="pt-1">
          <button
            onClick={() => setSettingsOpen(o => !o)}
            className={clsx(
              "w-full flex items-center justify-between px-3 py-2 rounded-md text-sm transition-colors",
              inSettings
                ? "bg-indigo-500/10 text-indigo-400"
                : "text-slate-400 hover:text-slate-200 hover:bg-white/[0.04]",
            )}
          >
            <span className="flex items-center gap-3">
              <Settings size={16} />
              Settings
            </span>
            <ChevronDown
              size={13}
              className={clsx("transition-transform", settingsOpen && "rotate-180")}
            />
          </button>

          {settingsOpen && (
            <div className="mt-0.5 ml-3 pl-3 border-l border-white/[0.06] space-y-0.5">
              {NAV_SETTINGS.map(({ href, label, icon: Icon, badge }) => {
                const active = pathname === href;
                return (
                  <Link
                    key={href}
                    href={href}
                    className={clsx(
                      "flex items-center justify-between px-2.5 py-1.5 rounded-md text-xs transition-colors",
                      active
                        ? "bg-indigo-500/10 text-indigo-300"
                        : "text-slate-400 hover:text-slate-200 hover:bg-white/[0.04]",
                    )}
                  >
                    <span className="flex items-center gap-2">
                      <Icon size={13} />
                      {label}
                    </span>
                    {badge && (
                      <span className="text-[9px] px-1 py-0.5 rounded bg-amber-500/10 text-amber-400 font-medium border border-amber-500/20">
                        {badge}
                      </span>
                    )}
                  </Link>
                );
              })}
            </div>
          )}
        </div>
      </nav>

      {/* Footer */}
      <div className="px-3 py-3 border-t border-white/[0.06]">
        {user ? (
          <UserMenu
            name={user.name}
            email={user.email}
            picture={user.picture}
          />
        ) : (
          <p className="text-[11px] text-slate-600 px-2">NetScope v0.6.0</p>
        )}
      </div>
    </aside>
  );
}
