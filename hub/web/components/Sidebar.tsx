"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { Network, LayoutDashboard, List, Server, Bell, GitFork, BarChart3 } from "lucide-react";
import { clsx } from "clsx";
import { UserMenu } from "@/components/UserMenu";

const NAV = [
  { href: "/",          label: "Dashboard",  icon: LayoutDashboard },
  { href: "/flows",     label: "Flows",      icon: List },
  { href: "/services",  label: "Services",   icon: GitFork },
  { href: "/analytics", label: "Analytics",  icon: BarChart3 },
  { href: "/agents",    label: "Agents",     icon: Server },
  { href: "/alerts",    label: "Alerts",     icon: Bell },
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
      <nav className="flex-1 px-3 py-4 space-y-0.5">
        {NAV.map(({ href, label, icon: Icon }) => {
          const active =
            href === "/" ? pathname === "/" : pathname.startsWith(href);
          return (
            <Link
              key={href}
              href={href}
              className={clsx(
                "flex items-center gap-3 px-3 py-2 rounded-md text-sm transition-colors",
                active
                  ? "bg-indigo-500/10 text-indigo-400"
                  : "text-slate-400 hover:text-slate-200 hover:bg-white/[0.04]"
              )}
            >
              <Icon size={16} />
              {label}
            </Link>
          );
        })}
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
          <p className="text-[11px] text-slate-600 px-2">NetScope v0.1.0</p>
        )}
      </div>
    </aside>
  );
}
