"use client";

import { useState, useEffect, useCallback } from "react";
import { fetchFlows, type Flow } from "@/lib/api";
import { FlowTable } from "@/components/FlowTable";
import { RefreshCw, Search, Clock, X, Download, Cpu, ChevronDown, ChevronUp } from "lucide-react";

const PAGE_SIZE = 100;

// Quick-range presets ──────────────────────────────────────────────────────────
const PRESETS = [
  { label: "Last 15 min", minutes: 15 },
  { label: "Last 1 hr",   minutes: 60 },
  { label: "Last 6 hrs",  minutes: 360 },
  { label: "Last 24 hrs", minutes: 1440 },
] as const;

function exportCSV(flows: Flow[]) {
  const header = "id,timestamp,protocol,src_ip,src_port,dst_ip,dst_port,bytes_in,bytes_out,info\n";
  const rows = flows.map(f =>
    [f.id, f.timestamp, f.protocol, f.src_ip, f.src_port,
     f.dst_ip, f.dst_port, f.bytes_in, f.bytes_out,
     `"${(f.info ?? "").replace(/"/g, '""')}"`].join(",")
  ).join("\n");
  const blob = new Blob([header + rows], { type: "text/csv" });
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = `netscope-flows-${new Date().toISOString().slice(0,10)}.csv`;
  a.click();
  URL.revokeObjectURL(url);
}

function toLocalDatetimeValue(d: Date) {
  // Returns "YYYY-MM-DDTHH:MM" for datetime-local inputs
  const pad = (n: number) => String(n).padStart(2, "0");
  return (
    d.getFullYear() +
    "-" + pad(d.getMonth() + 1) +
    "-" + pad(d.getDate()) +
    "T" + pad(d.getHours()) +
    ":" + pad(d.getMinutes())
  );
}

// ── Page ───────────────────────────────────────────────────────────────────────

export default function FlowsPage() {
  const [flows, setFlows] = useState<Flow[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(0);
  const [protocol, setProtocol] = useState("");
  const [srcIP, setSrcIP] = useState("");
  const [dstIP, setDstIP] = useState("");
  const [from, setFrom] = useState("");
  const [to, setTo] = useState("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [ebpfInfoOpen, setEbpfInfoOpen] = useState(false);

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const res = await fetchFlows({
        protocol:  protocol || undefined,
        src_ip:    srcIP    || undefined,
        dst_ip:    dstIP    || undefined,
        from:      from     || undefined,
        to:        to       || undefined,
        limit:     PAGE_SIZE,
        offset:    page * PAGE_SIZE,
      });
      setFlows(res.flows ?? []);
      setTotal(res.total ?? 0);
    } catch (e) {
      setError(String(e));
    } finally {
      setLoading(false);
    }
  }, [protocol, srcIP, dstIP, from, to, page]);

  useEffect(() => { load(); }, [load]);

  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE));

  function applyPreset(minutes: number) {
    const now = new Date();
    const start = new Date(now.getTime() - minutes * 60_000);
    setFrom(toLocalDatetimeValue(start));
    setTo(toLocalDatetimeValue(now));
    setPage(0);
  }

  function clearRange() {
    setFrom("");
    setTo("");
    setPage(0);
  }

  const hasRange    = Boolean(from || to);
  const hasEbpfData = flows.some((f) => f.process_name);

  return (
    <div className="p-6 space-y-4">
      {/* Header */}
      <div className="flex items-center justify-between">
        <h1 className="text-xl font-semibold text-white">Flow Explorer</h1>
        <div className="flex items-center gap-2">
          <button
            onClick={() => exportCSV(flows)}
            disabled={flows.length === 0}
            className="flex items-center gap-2 px-3 py-1.5 rounded bg-[#12121f] border border-white/10
                       text-sm text-slate-300 hover:text-white hover:border-white/20 transition-colors
                       disabled:opacity-40 disabled:cursor-not-allowed"
          >
            <Download size={14} />
            Export CSV
          </button>
          <button
            onClick={load}
            className="flex items-center gap-2 px-3 py-1.5 rounded bg-[#12121f] border border-white/10
                       text-sm text-slate-300 hover:text-white hover:border-white/20 transition-colors"
          >
            <RefreshCw size={14} className={loading ? "animate-spin" : ""} />
            Refresh
          </button>
        </div>
      </div>

      {/* Filters ── row 1: protocol + src IP */}
      <div className="flex flex-wrap gap-3">
        <div className="relative">
          <Search size={14} className="absolute left-3 top-1/2 -translate-y-1/2 text-slate-500 pointer-events-none" />
          <select
            value={protocol}
            onChange={(e) => { setProtocol(e.target.value); setPage(0); }}
            className="bg-[#0d0d1a] border border-white/10 rounded pl-8 pr-3 py-2 text-sm
                       text-slate-300 focus:outline-none focus:border-indigo-500/50 appearance-none"
          >
            <option value="">All protocols</option>
            <option value="HTTP">HTTP</option>
            <option value="DNS">DNS</option>
            <option value="TCP">TCP</option>
            <option value="UDP">UDP</option>
            <option value="TLS">TLS</option>
            <option value="HTTPS">HTTPS</option>
            <option value="HTTP/2">HTTP/2</option>
            <option value="gRPC">gRPC</option>
            <option value="ICMP">ICMP</option>
            <option value="ARP">ARP</option>
          </select>
        </div>
        <input
          type="text"
          placeholder="Filter by source IP…"
          value={srcIP}
          onChange={(e) => { setSrcIP(e.target.value); setPage(0); }}
          className="w-48 bg-[#0d0d1a] border border-white/10 rounded px-3 py-2
                     text-sm text-slate-300 placeholder-slate-600 focus:outline-none focus:border-indigo-500/50"
        />
        <input
          type="text"
          placeholder="Filter by dest IP…"
          value={dstIP}
          onChange={(e) => { setDstIP(e.target.value); setPage(0); }}
          className="w-48 bg-[#0d0d1a] border border-white/10 rounded px-3 py-2
                     text-sm text-slate-300 placeholder-slate-600 focus:outline-none focus:border-indigo-500/50"
        />
      </div>

      {/* Filters ── row 2: time range */}
      <div className="flex flex-wrap items-center gap-2">
        <Clock size={13} className="text-slate-600" />
        <span className="text-xs text-slate-600">Time range:</span>

        {/* Presets */}
        {PRESETS.map((p) => (
          <button
            key={p.minutes}
            onClick={() => applyPreset(p.minutes)}
            className="px-2.5 py-1 rounded text-xs bg-[#12121f] border border-white/[0.08]
                       text-slate-400 hover:text-white hover:border-white/20 transition-colors"
          >
            {p.label}
          </button>
        ))}

        {/* Custom datetime inputs */}
        <input
          type="datetime-local"
          value={from}
          onChange={(e) => { setFrom(e.target.value); setPage(0); }}
          className="bg-[#0d0d1a] border border-white/10 rounded px-2.5 py-1 text-xs
                     text-slate-300 focus:outline-none focus:border-indigo-500/50
                     [color-scheme:dark]"
        />
        <span className="text-slate-600 text-xs">→</span>
        <input
          type="datetime-local"
          value={to}
          onChange={(e) => { setTo(e.target.value); setPage(0); }}
          className="bg-[#0d0d1a] border border-white/10 rounded px-2.5 py-1 text-xs
                     text-slate-300 focus:outline-none focus:border-indigo-500/50
                     [color-scheme:dark]"
        />

        {hasRange && (
          <button
            onClick={clearRange}
            title="Clear time range"
            className="p-1 rounded text-slate-500 hover:text-white hover:bg-white/[0.06] transition-colors"
          >
            <X size={13} />
          </button>
        )}
      </div>

      {error && (
        <div className="rounded bg-red-900/20 border border-red-500/30 px-4 py-3 text-sm text-red-400">
          {error}
        </div>
      )}

      {/* ── eBPF mode callout ─────────────────────────────────────────────── */}
      {hasEbpfData ? (
        /* eBPF agent is running — show a green "active" badge */
        <div className="flex items-center gap-2 px-3 py-2 rounded-lg
                        bg-emerald-500/[0.08] border border-emerald-500/20 text-xs text-emerald-400">
          <Cpu size={13} className="shrink-0" />
          <span className="font-medium">eBPF mode active</span>
          <span className="text-emerald-500/60">·</span>
          <span className="text-emerald-500/80">
            Process attribution is enabled — each flow shows the exact process name and PID
            responsible for that connection.
          </span>
        </div>
      ) : (
        /* pcap-only agent — explain what eBPF mode unlocks */
        <div className="rounded-lg border border-indigo-500/20 bg-indigo-500/[0.05] overflow-hidden">
          <button
            onClick={() => setEbpfInfoOpen((o) => !o)}
            className="w-full flex items-center gap-2 px-3 py-2 text-left
                       text-xs text-indigo-400 hover:text-indigo-300 transition-colors"
          >
            <Cpu size={13} className="shrink-0" />
            <span className="font-medium">Unlock eBPF mode for process-level visibility</span>
            <span className="ml-auto text-indigo-500/50">
              {ebpfInfoOpen ? <ChevronUp size={13} /> : <ChevronDown size={13} />}
            </span>
          </button>

          {ebpfInfoOpen && (
            <div className="px-4 pb-4 pt-1 space-y-3 text-xs text-slate-400 border-t border-indigo-500/10">
              <p>
                <span className="text-white font-medium">What is eBPF mode?</span>{" "}
                NetScope can attach kernel-level probes to intercept TLS/SSL plaintext
                <em> before</em> it is encrypted and <em>after</em> it is decrypted — with zero
                code changes to your applications. Each captured flow is tagged with the
                originating <span className="text-emerald-400">process name</span> and{" "}
                <span className="text-emerald-400">PID</span>, letting you answer questions like
                "which service is talking to this IP?" instantly.
              </p>

              <p>
                <span className="text-white font-medium">What you get</span>
              </p>
              <ul className="list-disc list-inside space-y-1 text-slate-500">
                <li>Process name + PID on every TLS/HTTPS flow</li>
                <li>Plaintext HTTP bodies captured from encrypted connections</li>
                <li>TCP connection events with process attribution</li>
                <li>Works with any TLS library (OpenSSL, BoringSSL, LibreSSL)</li>
              </ul>

              <p>
                <span className="text-white font-medium">How to enable it</span>
              </p>
              <ol className="list-decimal list-inside space-y-1 text-slate-500">
                <li>
                  Download the{" "}
                  <span className="text-indigo-400 font-mono">netscope-agent-ebpf-…-linux</span>{" "}
                  binary from the GitHub Releases page (Linux x86_64 or aarch64).
                </li>
                <li>
                  Run it with root or{" "}
                  <span className="font-mono text-slate-300">CAP_BPF</span> capability:{" "}
                  <span className="font-mono text-slate-300">
                    sudo ./netscope-agent --hub http://&lt;hub&gt;:8080
                  </span>
                </li>
                <li>
                  Reload this page — the{" "}
                  <span className="text-emerald-400 font-medium">Process</span> column will
                  populate with live process names.
                </li>
              </ol>

              <p className="text-slate-600">
                Requires Linux kernel ≥ 5.8. macOS and Windows agents operate in pcap-only mode
                and show <span className="font-mono">—</span> in the Process column.
              </p>
            </div>
          )}
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
