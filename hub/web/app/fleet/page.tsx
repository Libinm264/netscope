"use client";

import { useEffect, useState } from "react";
import { Globe, Wifi, WifiOff, Activity, Search, RefreshCw, Tag } from "lucide-react";

interface ClusterSummary {
  cluster: string;
  agent_count: number;
  online_count: number;
  versions: string[];
  flows_1h: number;
}

interface FleetFlow {
  id: string;
  agent_id: string;
  hostname: string;
  ts: string;
  protocol: string;
  src_ip: string;
  src_port: number;
  dst_ip: string;
  dst_port: number;
  bytes_in: number;
  bytes_out: number;
  cluster?: string;
}

export default function FleetPage() {
  const [clusters, setClusters] = useState<ClusterSummary[]>([]);
  const [searchResults, setSearchResults] = useState<FleetFlow[]>([]);
  const [loading, setLoading] = useState(true);
  const [searching, setSearching] = useState(false);
  const [selectedCluster, setSelectedCluster] = useState<string>("");
  const [searchIP, setSearchIP] = useState("");
  const [lastRefresh, setLastRefresh] = useState(new Date());

  const fetchClusters = async () => {
    setLoading(true);
    try {
      const r = await fetch("/api/v1/fleet/clusters");
      const d = await r.json();
      setClusters(d.clusters ?? []);
      setLastRefresh(new Date());
    } finally {
      setLoading(false);
    }
  };

  const searchFlows = async () => {
    setSearching(true);
    try {
      const params = new URLSearchParams();
      if (selectedCluster) params.set("cluster", selectedCluster);
      if (searchIP) params.set("src_ip", searchIP);
      params.set("limit", "100");
      const r = await fetch(`/api/v1/fleet/search?${params}`);
      const d = await r.json();
      setSearchResults(d.flows ?? []);
    } finally {
      setSearching(false);
    }
  };

  useEffect(() => {
    fetchClusters();
    const t = setInterval(fetchClusters, 30_000);
    return () => clearInterval(t);
  }, []);

  const totalAgents  = clusters.reduce((s, c) => s + c.agent_count, 0);
  const totalOnline  = clusters.reduce((s, c) => s + c.online_count, 0);
  const totalFlows1h = clusters.reduce((s, c) => s + c.flows_1h, 0);

  return (
    <div className="p-6 space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-3">
          <Globe size={22} className="text-indigo-400" />
          <div>
            <h1 className="text-xl font-bold text-white">Fleet Overview</h1>
            <p className="text-xs text-slate-400 mt-0.5">
              Multi-cluster health grid · refreshed {lastRefresh.toLocaleTimeString()}
            </p>
          </div>
        </div>
        <button
          onClick={fetchClusters}
          className="flex items-center gap-2 px-3 py-1.5 rounded-md bg-white/[0.05]
                     text-slate-300 text-sm hover:bg-white/[0.08] transition-colors"
        >
          <RefreshCw size={14} className={loading ? "animate-spin" : ""} />
          Refresh
        </button>
      </div>

      {/* Stats bar */}
      <div className="grid grid-cols-3 gap-4">
        {[
          { label: "Total Agents",   value: totalAgents,                icon: Tag },
          { label: "Online Now",     value: `${totalOnline} / ${totalAgents}`, icon: Wifi },
          { label: "Flows (1h)",     value: totalFlows1h.toLocaleString(), icon: Activity },
        ].map(({ label, value, icon: Icon }) => (
          <div key={label} className="bg-white/[0.03] border border-white/[0.06] rounded-xl p-4">
            <div className="flex items-center gap-2 text-slate-400 text-xs mb-2">
              <Icon size={13} />
              {label}
            </div>
            <p className="text-2xl font-bold text-white">{value}</p>
          </div>
        ))}
      </div>

      {/* Cluster grid */}
      <div>
        <h2 className="text-sm font-semibold text-slate-300 mb-3">Clusters</h2>
        {loading && clusters.length === 0 ? (
          <p className="text-slate-500 text-sm">Loading clusters…</p>
        ) : clusters.length === 0 ? (
          <div className="bg-white/[0.03] border border-white/[0.06] rounded-xl p-8 text-center">
            <Globe size={32} className="mx-auto text-slate-600 mb-3" />
            <p className="text-slate-400 text-sm">No clusters found.</p>
            <p className="text-slate-500 text-xs mt-1">
              Assign cluster labels to agents via the <code className="text-indigo-400">AGENT_CLUSTER</code> env var.
            </p>
          </div>
        ) : (
          <div className="grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-3 gap-4">
            {clusters.map((c) => {
              const pct = c.agent_count > 0 ? Math.round((c.online_count / c.agent_count) * 100) : 0;
              const healthy = pct >= 80;
              const warn    = pct >= 50 && pct < 80;
              return (
                <button
                  key={c.cluster}
                  onClick={() => setSelectedCluster(c.cluster === selectedCluster ? "" : c.cluster)}
                  className={`text-left bg-white/[0.03] border rounded-xl p-5 transition-colors hover:bg-white/[0.05] ${
                    selectedCluster === c.cluster
                      ? "border-indigo-500/50"
                      : "border-white/[0.06]"
                  }`}
                >
                  <div className="flex items-start justify-between mb-3">
                    <span className="font-semibold text-white text-sm">{c.cluster}</span>
                    <span className={`text-xs px-2 py-0.5 rounded-full font-medium ${
                      healthy ? "bg-emerald-500/10 text-emerald-400" :
                      warn    ? "bg-amber-500/10 text-amber-400" :
                                "bg-red-500/10 text-red-400"
                    }`}>
                      {pct}% online
                    </span>
                  </div>
                  <div className="space-y-1.5 text-xs text-slate-400">
                    <div className="flex items-center gap-2">
                      {c.online_count > 0 ? <Wifi size={12} className="text-emerald-400" /> : <WifiOff size={12} className="text-red-400" />}
                      {c.online_count} / {c.agent_count} agents online
                    </div>
                    <div className="flex items-center gap-2">
                      <Activity size={12} className="text-indigo-400" />
                      {c.flows_1h.toLocaleString()} flows/h
                    </div>
                    {c.versions?.length > 0 && (
                      <div className="flex flex-wrap gap-1 pt-1">
                        {c.versions.map(v => (
                          <span key={v} className="px-1.5 py-0.5 rounded bg-slate-700/50 text-slate-300">{v}</span>
                        ))}
                      </div>
                    )}
                  </div>
                </button>
              );
            })}
          </div>
        )}
      </div>

      {/* Cross-cluster search */}
      <div className="bg-white/[0.03] border border-white/[0.06] rounded-xl p-5">
        <h2 className="text-sm font-semibold text-slate-300 mb-4 flex items-center gap-2">
          <Search size={15} className="text-indigo-400" />
          Cross-Cluster Flow Search
        </h2>
        <div className="flex gap-3 mb-4">
          <select
            value={selectedCluster}
            onChange={e => setSelectedCluster(e.target.value)}
            className="bg-white/[0.05] border border-white/[0.10] rounded-lg px-3 py-2
                       text-slate-200 text-sm focus:outline-none focus:border-indigo-500/50 flex-1"
          >
            <option value="">All clusters</option>
            {clusters.map(c => (
              <option key={c.cluster} value={c.cluster}>{c.cluster}</option>
            ))}
          </select>
          <input
            value={searchIP}
            onChange={e => setSearchIP(e.target.value)}
            placeholder="Filter by source IP…"
            className="bg-white/[0.05] border border-white/[0.10] rounded-lg px-3 py-2
                       text-slate-200 text-sm placeholder-slate-600 focus:outline-none
                       focus:border-indigo-500/50 flex-1"
          />
          <button
            onClick={searchFlows}
            disabled={searching}
            className="px-4 py-2 rounded-lg bg-indigo-500 hover:bg-indigo-600 text-white
                       text-sm font-medium transition-colors disabled:opacity-50"
          >
            {searching ? "Searching…" : "Search"}
          </button>
        </div>

        {searchResults.length > 0 && (
          <div className="overflow-x-auto rounded-lg border border-white/[0.06]">
            <table className="w-full text-xs text-slate-300">
              <thead>
                <tr className="bg-white/[0.03] text-slate-500">
                  {["Cluster","Hostname","Protocol","Source","Destination","Bytes↑","Time"].map(h => (
                    <th key={h} className="px-3 py-2 text-left font-medium">{h}</th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {searchResults.map(f => (
                  <tr key={f.id} className="border-t border-white/[0.04] hover:bg-white/[0.02]">
                    <td className="px-3 py-2">
                      <span className="px-1.5 py-0.5 rounded bg-indigo-500/10 text-indigo-400 text-[10px]">
                        {f.cluster || "default"}
                      </span>
                    </td>
                    <td className="px-3 py-2 font-mono text-slate-200">{f.hostname}</td>
                    <td className="px-3 py-2">
                      <span className="px-1.5 py-0.5 rounded bg-slate-700/50 text-slate-300">{f.protocol}</span>
                    </td>
                    <td className="px-3 py-2 font-mono">{f.src_ip}:{f.src_port}</td>
                    <td className="px-3 py-2 font-mono">{f.dst_ip}:{f.dst_port}</td>
                    <td className="px-3 py-2">{(f.bytes_out / 1024).toFixed(1)}KB</td>
                    <td className="px-3 py-2 text-slate-500">
                      {new Date(f.ts).toLocaleTimeString()}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </div>
  );
}
