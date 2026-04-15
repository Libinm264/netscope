import { StatsCards } from "@/components/StatsCards";
import { LiveFeed } from "@/components/LiveFeed";
import { ProtocolChart } from "@/components/ProtocolChart";

export default function DashboardPage() {
  return (
    <div className="p-6 space-y-6">
      <h1 className="text-xl font-semibold text-white">Dashboard</h1>

      {/* Stats row */}
      <StatsCards />

      {/* Live feed + chart */}
      <div className="grid grid-cols-1 xl:grid-cols-3 gap-6">
        <div className="xl:col-span-2">
          <LiveFeed />
        </div>
        <div>
          <ProtocolChart />
        </div>
      </div>
    </div>
  );
}
