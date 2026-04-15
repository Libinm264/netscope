"use client";

import { useState, useEffect, useCallback } from "react";
import { fetchFlows, type Flow } from "@/lib/api";
import { FlowTable } from "@/components/FlowTable";
import { RefreshCw, Search } from "lucide-react";

const PAGE_SIZE = 100;

export default function FlowsPage() {
  const [flows, setFlows] = useState<Flow[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(0);
  const [protocol, setProtocol] = useState("");
  const [srcIP, setSrcIP] = useState("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const res = await fetchFlows({
        protocol: protocol || undefined,
        src_ip: srcIP || undefined,
        limit: PAGE_SIZE,
        offset: page * PAGE_SIZE,
      });
      setFlows(res.flows ?? []);
      setTotal(res.total ?? 0);
    } catch (e) {
      setError(String(e));
    } finally {
      setLoading(false);
    }
  }, [protocol, srcIP, page]);

  useEffect(() => {
    load();
  }, [load]);

  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE));

  return (
    <div className="p-6 space-y-4">
      <div className="flex items-center justify-between">
        <h1 className="text-xl font-semibold text-white">Flow Explorer</h1>
        <button
          onClick={load}
          className="flex items-center gap-2 px-3 py-1.5 rounded bg-[#12121f] border border-white/10
                     text-sm text-slate-300 hover:text-white hover:border-white/20 transition-colors"
        >
          <RefreshCw size={14} className={loading ? "animate-spin" : ""} />
          Refresh
        </button>
      </div>

      {/* Filters */}
      <div className="flex gap-3">
        <div className="relative flex-1 max-w-xs">
          <Search size={14} className="absolute left-3 top-1/2 -translate-y-1/2 text-slate-500" />
          <select
            value={protocol}
            onChange={(e) => { setProtocol(e.target.value); setPage(0); }}
            className="w-full bg-[#0d0d1a] border border-white/10 rounded px-3 py-2 text-sm
                       text-slate-300 focus:outline-none focus:border-indigo-500/50"
          >
            <option value="">All protocols</option>
            <option value="HTTP">HTTP</option>
            <option value="DNS">DNS</option>
            <option value="TCP">TCP</option>
            <option value="UDP">UDP</option>
          </select>
        </div>
        <input
          type="text"
          placeholder="Filter by source IP…"
          value={srcIP}
          onChange={(e) => { setSrcIP(e.target.value); setPage(0); }}
          className="flex-1 max-w-xs bg-[#0d0d1a] border border-white/10 rounded px-3 py-2
                     text-sm text-slate-300 placeholder-slate-600 focus:outline-none focus:border-indigo-500/50"
        />
      </div>

      {error && (
        <div className="rounded bg-red-900/20 border border-red-500/30 px-4 py-3 text-sm text-red-400">
          {error}
        </div>
      )}

      <FlowTable flows={flows} loading={loading} />

      {/* Pagination */}
      <div className="flex items-center justify-between text-sm text-slate-500">
        <span>{total.toLocaleString()} total flows</span>
        <div className="flex items-center gap-3">
          <button
            onClick={() => setPage((p) => Math.max(0, p - 1))}
            disabled={page === 0}
            className="px-3 py-1 rounded bg-[#12121f] border border-white/10 hover:border-white/20
                       disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
          >
            ← Prev
          </button>
          <span className="text-slate-400">
            {page + 1} / {totalPages}
          </span>
          <button
            onClick={() => setPage((p) => Math.min(totalPages - 1, p + 1))}
            disabled={page >= totalPages - 1}
            className="px-3 py-1 rounded bg-[#12121f] border border-white/10 hover:border-white/20
                       disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
          >
            Next →
          </button>
        </div>
      </div>
    </div>
  );
}
