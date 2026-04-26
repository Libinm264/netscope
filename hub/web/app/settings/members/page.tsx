"use client";

import { useCallback, useEffect, useState } from "react";
import {
  Users, Plus, Trash2, RefreshCw, Shield, Eye, ChevronDown, Crown, BarChart2,
} from "lucide-react";
import { clsx } from "clsx";
import {
  fetchMembers, inviteMember, updateMemberRole, removeMember, fetchLicense,
} from "@/lib/api";
import type { OrgMember } from "@/lib/api";
import { EnterpriseGate } from "@/components/EnterpriseGate";

const ROLES = ["owner", "admin", "analyst", "viewer"] as const;
type Role = typeof ROLES[number];

const ROLE_META: Record<Role, { label: string; color: string; icon: React.ReactNode }> = {
  owner:   { label: "Owner",   color: "text-amber-400  bg-amber-500/10  border-amber-500/25",  icon: <Crown    size={11} /> },
  admin:   { label: "Admin",   color: "text-indigo-400 bg-indigo-500/10 border-indigo-500/25", icon: <Shield   size={11} /> },
  analyst: { label: "Analyst", color: "text-sky-400    bg-sky-500/10    border-sky-500/25",    icon: <BarChart2 size={11} /> },
  viewer:  { label: "Viewer",  color: "text-slate-400  bg-slate-700/40  border-white/10",      icon: <Eye      size={11} /> },
};

function RoleBadge({ role }: { role: string }) {
  const meta = ROLE_META[role as Role] ?? ROLE_META.viewer;
  return (
    <span className={clsx(
      "inline-flex items-center gap-1 px-1.5 py-0.5 rounded text-[10px] font-medium border",
      meta.color,
    )}>
      {meta.icon} {meta.label}
    </span>
  );
}

function InviteModal({ onClose, onDone }: { onClose: () => void; onDone: () => void }) {
  const [email, setEmail] = useState("");
  const [name,  setName]  = useState("");
  const [role,  setRole]  = useState<Role>("viewer");
  const [loading, setLoading] = useState(false);
  const [err, setErr]     = useState("");

  const submit = async () => {
    if (!email.trim()) { setErr("Email is required"); return; }
    setLoading(true);
    setErr("");
    try {
      await inviteMember({ email: email.trim(), name: name.trim(), role });
      onDone();
      onClose();
    } catch (e) {
      setErr(e instanceof Error ? e.message : "Failed to invite member");
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60">
      <div className="w-full max-w-sm bg-[#0d0d1a] border border-white/[0.08] rounded-2xl p-6 space-y-4">
        <div className="flex items-center justify-between">
          <p className="text-sm font-semibold text-white">Invite member</p>
          <button onClick={onClose} className="text-slate-500 hover:text-slate-300 text-lg leading-none">×</button>
        </div>

        <div className="space-y-3">
          <div>
            <label className="text-xs text-slate-400 mb-1 block">Email</label>
            <input value={email} onChange={(e) => setEmail(e.target.value)}
              placeholder="alice@acme.com"
              className="w-full bg-white/[0.04] border border-white/10 rounded-lg px-3 py-2
                         text-xs text-white placeholder:text-slate-600 focus:outline-none focus:border-indigo-500/50" />
          </div>
          <div>
            <label className="text-xs text-slate-400 mb-1 block">Display name</label>
            <input value={name} onChange={(e) => setName(e.target.value)}
              placeholder="Alice Smith"
              className="w-full bg-white/[0.04] border border-white/10 rounded-lg px-3 py-2
                         text-xs text-white placeholder:text-slate-600 focus:outline-none focus:border-indigo-500/50" />
          </div>
          <div>
            <label className="text-xs text-slate-400 mb-1 block">Role</label>
            <select value={role} onChange={(e) => setRole(e.target.value as Role)}
              className="w-full bg-white/[0.04] border border-white/10 rounded-lg px-3 py-2
                         text-xs text-slate-300 focus:outline-none">
              {ROLES.filter(r => r !== "owner").map(r => (
                <option key={r} value={r}>{ROLE_META[r].label}</option>
              ))}
            </select>
          </div>
          {err && <p className="text-xs text-red-400">{err}</p>}
        </div>

        <div className="flex gap-2 pt-1">
          <button onClick={onClose}
            className="flex-1 px-3 py-2 rounded-lg text-xs text-slate-400 border border-white/10
                       hover:bg-white/[0.04] transition-colors">
            Cancel
          </button>
          <button onClick={submit} disabled={loading}
            className="flex-1 px-3 py-2 rounded-lg text-xs text-white bg-indigo-600
                       hover:bg-indigo-500 disabled:opacity-40 transition-colors">
            {loading ? "Inviting…" : "Send invite"}
          </button>
        </div>
      </div>
    </div>
  );
}

export default function MembersPage() {
  const [members, setMembers]   = useState<OrgMember[]>([]);
  const [loading, setLoading]   = useState(true);
  const [showInvite, setShowInvite] = useState(false);
  const [locked, setLocked]     = useState(false);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const [data, lic] = await Promise.all([fetchMembers(), fetchLicense()]);
      setMembers(data.members ?? []);
      setLocked(lic.plan === "community");
    } catch { /* hub offline */ }
    finally { setLoading(false); }
  }, []);

  useEffect(() => { load(); }, [load]);

  const handleRoleChange = async (userId: string, role: string) => {
    try { await updateMemberRole(userId, role); load(); }
    catch (e) { alert(e instanceof Error ? e.message : "Update failed"); }
  };

  const handleRemove = async (userId: string, email: string) => {
    if (!confirm(`Remove ${email} from this organisation?`)) return;
    try { await removeMember(userId); load(); }
    catch (e) { alert(e instanceof Error ? e.message : "Remove failed"); }
  };

  return (
    <div className="p-6 space-y-6">
      {showInvite && (
        <InviteModal onClose={() => setShowInvite(false)} onDone={load} />
      )}

      {/* Header */}
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-3">
          <div className="p-2 rounded-lg bg-indigo-500/10">
            <Users size={20} className="text-indigo-400" />
          </div>
          <div>
            <h1 className="text-lg font-semibold text-white">Members & Roles</h1>
            <p className="text-xs text-slate-400 mt-0.5">
              Manage who has access and what they can do
            </p>
          </div>
        </div>
        {!locked && (
          <button
            onClick={() => setShowInvite(true)}
            className="flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs
                       bg-indigo-600 hover:bg-indigo-500 text-white transition-colors"
          >
            <Plus size={12} /> Invite member
          </button>
        )}
      </div>

      {/* Role legend */}
      {!locked && (
        <div className="grid grid-cols-4 gap-3">
          {ROLES.map(r => (
            <div key={r} className="bg-[#0d0d1a] border border-white/[0.06] rounded-lg px-3 py-2.5">
              <RoleBadge role={r} />
              <p className="text-[10px] text-slate-500 mt-1.5 leading-snug">
                {r === "owner"   && "Full access. Can manage billing and delete the org."}
                {r === "admin"   && "Manage members, agents, alerts, and policies."}
                {r === "analyst" && "View all data and create/modify alerts and policies."}
                {r === "viewer"  && "Read-only access to all dashboards."}
              </p>
            </div>
          ))}
        </div>
      )}

      {locked ? (
        <EnterpriseGate
          feature="multi_tenant"
          title="Team plan required"
          description="Member management and custom RBAC roles are available on the Team plan and above. Community plan supports a single implicit admin."
        />
      ) : loading ? (
        <div className="flex items-center justify-center h-32">
          <RefreshCw size={20} className="animate-spin text-slate-600" />
        </div>
      ) : members.length === 0 ? (
        <div className="text-center py-16 text-slate-500 text-sm">
          No members yet — invite your team.
        </div>
      ) : (
        <div className="bg-[#0d0d1a] border border-white/[0.06] rounded-xl overflow-hidden">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-white/[0.04]">
                {["Member", "Role", "SSO provider", "Last seen", ""].map(h => (
                  <th key={h} className="text-left px-4 py-2.5 text-xs font-medium text-slate-500">{h}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {members.map(m => (
                <tr key={m.user_id} className="border-b border-white/[0.03] hover:bg-white/[0.02]">
                  <td className="px-4 py-2.5">
                    <p className="text-xs font-medium text-white">{m.display_name || "—"}</p>
                    <p className="text-[10px] text-slate-500">{m.email}</p>
                  </td>
                  <td className="px-4 py-2.5">
                    <div className="relative group inline-block">
                      <button className="flex items-center gap-1 hover:opacity-80 transition-opacity">
                        <RoleBadge role={m.role} />
                        <ChevronDown size={10} className="text-slate-500" />
                      </button>
                      {/* Role dropdown */}
                      <div className="absolute top-full left-0 mt-1 w-28 bg-[#12121f] border border-white/[0.08]
                                      rounded-lg shadow-xl z-20 hidden group-hover:block py-1">
                        {ROLES.filter(r => r !== "owner").map(r => (
                          <button key={r} onClick={() => handleRoleChange(m.user_id, r)}
                            className="w-full text-left px-3 py-1.5 text-xs text-slate-300
                                       hover:bg-white/[0.04] transition-colors">
                            {ROLE_META[r].label}
                          </button>
                        ))}
                      </div>
                    </div>
                  </td>
                  <td className="px-4 py-2.5">
                    <span className="text-xs text-slate-500">
                      {m.sso_provider || "pending"}
                    </span>
                  </td>
                  <td className="px-4 py-2.5 text-xs text-slate-500">
                    {m.last_seen
                      ? new Date(m.last_seen).toLocaleDateString()
                      : "never"}
                  </td>
                  <td className="px-4 py-2.5">
                    {m.role !== "owner" && (
                      <button onClick={() => handleRemove(m.user_id, m.email)}
                        className="p-1 rounded hover:bg-red-500/10 text-slate-600
                                   hover:text-red-400 transition-colors">
                        <Trash2 size={12} />
                      </button>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
