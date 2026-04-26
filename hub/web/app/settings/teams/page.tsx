"use client";

import { useCallback, useEffect, useState } from "react";
import { Users2, Plus, Trash2, RefreshCw, UserPlus } from "lucide-react";
import {
  fetchTeams, createTeam, deleteTeam,
  fetchTeamMembers, addTeamMember, removeTeamMember,
  fetchMembers, fetchLicense,
} from "@/lib/api";
import type { Team, TeamMember, OrgMember } from "@/lib/api";
import { EnterpriseGate } from "@/components/EnterpriseGate";

function AddMemberModal({
  teamId, orgMembers, onClose, onDone,
}: {
  teamId: string;
  orgMembers: OrgMember[];
  onClose: () => void;
  onDone: () => void;
}) {
  const [userId, setUserId] = useState(orgMembers[0]?.user_id ?? "");
  const [loading, setLoading] = useState(false);

  const submit = async () => {
    if (!userId) return;
    setLoading(true);
    try { await addTeamMember(teamId, userId); onDone(); onClose(); }
    catch (e) { alert(e instanceof Error ? e.message : "Failed"); }
    finally { setLoading(false); }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60">
      <div className="w-full max-w-sm bg-[#0d0d1a] border border-white/[0.08] rounded-2xl p-6 space-y-4">
        <div className="flex items-center justify-between">
          <p className="text-sm font-semibold text-white">Add team member</p>
          <button onClick={onClose} className="text-slate-500 hover:text-slate-300 text-lg leading-none">×</button>
        </div>
        <select value={userId} onChange={(e) => setUserId(e.target.value)}
          className="w-full bg-white/[0.04] border border-white/10 rounded-lg px-3 py-2
                     text-xs text-slate-300 focus:outline-none">
          {orgMembers.map(m => (
            <option key={m.user_id} value={m.user_id}>{m.display_name || m.email}</option>
          ))}
        </select>
        <div className="flex gap-2">
          <button onClick={onClose}
            className="flex-1 px-3 py-2 rounded-lg text-xs text-slate-400 border border-white/10
                       hover:bg-white/[0.04] transition-colors">Cancel</button>
          <button onClick={submit} disabled={loading}
            className="flex-1 px-3 py-2 rounded-lg text-xs text-white bg-indigo-600
                       hover:bg-indigo-500 disabled:opacity-40 transition-colors">
            {loading ? "Adding…" : "Add"}
          </button>
        </div>
      </div>
    </div>
  );
}

function TeamCard({
  team, orgMembers, onDelete,
}: {
  team: Team;
  orgMembers: OrgMember[];
  onDelete: () => void;
}) {
  const [members, setMembers]   = useState<TeamMember[]>([]);
  const [expanded, setExpanded] = useState(false);
  const [addOpen, setAddOpen]   = useState(false);

  const loadMembers = useCallback(async () => {
    try { setMembers((await fetchTeamMembers(team.team_id)).members ?? []); }
    catch { /* ignore */ }
  }, [team.team_id]);

  useEffect(() => { if (expanded) loadMembers(); }, [expanded, loadMembers]);

  return (
    <>
      {addOpen && (
        <AddMemberModal
          teamId={team.team_id}
          orgMembers={orgMembers}
          onClose={() => setAddOpen(false)}
          onDone={loadMembers}
        />
      )}
      <div className="bg-[#0d0d1a] border border-white/[0.06] rounded-xl overflow-hidden">
        <div className="flex items-center justify-between px-4 py-3">
          <button
            onClick={() => setExpanded(e => !e)}
            className="flex items-center gap-2 text-left flex-1"
          >
            <Users2 size={14} className="text-indigo-400 shrink-0" />
            <div>
              <p className="text-sm font-medium text-white">{team.name}</p>
              {team.description && (
                <p className="text-xs text-slate-500">{team.description}</p>
              )}
            </div>
            <span className="text-xs text-slate-600 ml-2">
              {team.member_count ?? 0} member{team.member_count !== 1 ? "s" : ""}
            </span>
          </button>
          <div className="flex items-center gap-2">
            <button
              onClick={() => { setExpanded(true); setAddOpen(true); }}
              className="p-1.5 rounded hover:bg-indigo-500/10 text-slate-500
                         hover:text-indigo-400 transition-colors"
              title="Add member"
            >
              <UserPlus size={13} />
            </button>
            <button
              onClick={onDelete}
              className="p-1.5 rounded hover:bg-red-500/10 text-slate-600
                         hover:text-red-400 transition-colors"
              title="Delete team"
            >
              <Trash2 size={13} />
            </button>
          </div>
        </div>

        {expanded && (
          <div className="border-t border-white/[0.04] divide-y divide-white/[0.03]">
            {members.length === 0 ? (
              <p className="px-4 py-3 text-xs text-slate-600">No members yet.</p>
            ) : (
              members.map(m => (
                <div key={m.user_id} className="flex items-center justify-between px-4 py-2.5">
                  <div>
                    <p className="text-xs text-slate-200">{m.display_name || "—"}</p>
                    <p className="text-[10px] text-slate-500">{m.email}</p>
                  </div>
                  <button
                    onClick={async () => {
                      await removeTeamMember(team.team_id, m.user_id);
                      loadMembers();
                    }}
                    className="p-1 rounded hover:bg-red-500/10 text-slate-600 hover:text-red-400 transition-colors"
                  >
                    <Trash2 size={11} />
                  </button>
                </div>
              ))
            )}
          </div>
        )}
      </div>
    </>
  );
}

export default function TeamsPage() {
  const [teams, setTeams]         = useState<Team[]>([]);
  const [members, setMembers]     = useState<OrgMember[]>([]);
  const [loading, setLoading]     = useState(true);
  const [locked, setLocked]       = useState(false);
  const [newName, setNewName]     = useState("");
  const [newDesc, setNewDesc]     = useState("");
  const [creating, setCreating]   = useState(false);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const [t, m, lic] = await Promise.all([
        fetchTeams(), fetchMembers(), fetchLicense(),
      ]);
      setTeams(t.teams ?? []);
      setMembers(m.members ?? []);
      setLocked(lic.plan === "community");
    } catch { /* hub offline */ }
    finally { setLoading(false); }
  }, []);

  useEffect(() => { load(); }, [load]);

  const handleCreate = async () => {
    if (!newName.trim()) return;
    setCreating(true);
    try {
      await createTeam({ name: newName.trim(), description: newDesc.trim() });
      setNewName(""); setNewDesc("");
      load();
    } catch (e) {
      alert(e instanceof Error ? e.message : "Failed to create team");
    } finally { setCreating(false); }
  };

  const handleDelete = async (teamId: string) => {
    if (!confirm("Delete this team? Members will not be removed from the org.")) return;
    try { await deleteTeam(teamId); load(); }
    catch (e) { alert(e instanceof Error ? e.message : "Failed to delete team"); }
  };

  return (
    <div className="p-6 space-y-6">
      {/* Header */}
      <div className="flex items-center gap-3">
        <div className="p-2 rounded-lg bg-indigo-500/10">
          <Users2 size={20} className="text-indigo-400" />
        </div>
        <div>
          <h1 className="text-lg font-semibold text-white">Teams</h1>
          <p className="text-xs text-slate-400 mt-0.5">
            Group members to scope agent access and alert routing
          </p>
        </div>
      </div>

      {locked ? (
        <EnterpriseGate
          feature="multi_tenant"
          title="Team plan required"
          description="Team management is available on the Team plan and above."
        />
      ) : (
        <>
          {/* Create form */}
          <div className="bg-[#0d0d1a] border border-white/[0.06] rounded-xl p-4 space-y-3">
            <p className="text-xs font-semibold text-slate-300">New team</p>
            <div className="flex gap-2">
              <input value={newName} onChange={(e) => setNewName(e.target.value)}
                placeholder="Team name"
                className="flex-1 bg-white/[0.04] border border-white/10 rounded-lg px-3 py-2
                           text-xs text-white placeholder:text-slate-600 focus:outline-none focus:border-indigo-500/50" />
              <input value={newDesc} onChange={(e) => setNewDesc(e.target.value)}
                placeholder="Description (optional)"
                className="flex-1 bg-white/[0.04] border border-white/10 rounded-lg px-3 py-2
                           text-xs text-white placeholder:text-slate-600 focus:outline-none focus:border-indigo-500/50" />
              <button onClick={handleCreate} disabled={!newName.trim() || creating}
                className="flex items-center gap-1.5 px-3 py-2 rounded-lg text-xs text-white
                           bg-indigo-600 hover:bg-indigo-500 disabled:opacity-40 transition-colors">
                <Plus size={12} /> Create
              </button>
            </div>
          </div>

          {loading ? (
            <div className="flex items-center justify-center h-24">
              <RefreshCw size={18} className="animate-spin text-slate-600" />
            </div>
          ) : teams.length === 0 ? (
            <p className="text-center text-sm text-slate-500 py-12">
              No teams yet. Create one to start scoping access.
            </p>
          ) : (
            <div className="space-y-3">
              {teams.map(t => (
                <TeamCard
                  key={t.team_id}
                  team={t}
                  orgMembers={members}
                  onDelete={() => handleDelete(t.team_id)}
                />
              ))}
            </div>
          )}
        </>
      )}
    </div>
  );
}
