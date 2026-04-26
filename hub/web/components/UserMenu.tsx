"use client";

import { LogOut, User } from "lucide-react";
import Image from "next/image";
import { useState } from "react";
import { useRouter } from "next/navigation";

interface Props {
  name?: string | null;
  email?: string | null;
  picture?: string | null;
}

export function UserMenu({ name, email, picture }: Props) {
  const [open, setOpen] = useState(false);
  const router = useRouter();

  const handleLogout = async () => {
    try {
      await fetch("/api/proxy/enterprise/auth/logout", { method: "POST" });
    } finally {
      router.push("/login");
    }
  };

  return (
    <div className="relative">
      <button
        onClick={() => setOpen((o) => !o)}
        className="flex items-center gap-2 px-2 py-1.5 rounded-lg hover:bg-white/[0.04]
                   transition-colors text-left"
      >
        {picture ? (
          <Image
            src={picture}
            alt={name ?? "User"}
            width={24}
            height={24}
            className="rounded-full"
          />
        ) : (
          <div className="w-6 h-6 rounded-full bg-indigo-500/20 flex items-center justify-center">
            <User size={12} className="text-indigo-400" />
          </div>
        )}
        <span className="text-xs text-slate-400 max-w-[120px] truncate">
          {name ?? email ?? "Account"}
        </span>
      </button>

      {open && (
        <>
          {/* Backdrop */}
          <div
            className="fixed inset-0 z-40"
            onClick={() => setOpen(false)}
          />
          {/* Dropdown */}
          <div
            className="absolute bottom-full left-0 mb-1 w-52 rounded-xl
                       bg-[#12121f] border border-white/[0.08] shadow-xl z-50 py-1"
          >
            <div className="px-3 py-2 border-b border-white/[0.06]">
              <p className="text-xs font-medium text-white truncate">
                {name ?? "User"}
              </p>
              {email && (
                <p className="text-[11px] text-slate-500 truncate">{email}</p>
              )}
            </div>
            <button
              onClick={handleLogout}
              className="flex items-center gap-2.5 w-full px-3 py-2 text-sm text-slate-400
                         hover:text-white hover:bg-white/[0.04] transition-colors text-left"
            >
              <LogOut size={14} />
              Sign out
            </button>
          </div>
        </>
      )}
    </div>
  );
}
