import { AgentList } from "@/components/AgentList";

export default function AgentsPage() {
  return (
    <div className="p-6 space-y-4">
      <h1 className="text-xl font-semibold text-white">Agents</h1>
      <p className="text-sm text-slate-500">
        All registered NetScope agents. An agent is considered online if it has
        reported a flow within the last 5 minutes.
      </p>
      <AgentList />
    </div>
  );
}
