import { useMemo, useRef, useState, useEffect, useCallback } from "react";
import { Network } from "lucide-react";
import { useCaptureStore } from "@/store/captureStore";
import type { FlowDto } from "@/types/flow";

// ── Graph data types ──────────────────────────────────────────────────────────

interface GraphNode {
  id: string;
  ip: string;
  flowCount: number;
  bytes: number;
  protocols: Set<string>;
  country?: string;
  asOrg?: string;
  x: number;
  y: number;
  vx: number;
  vy: number;
}

interface GraphEdge {
  source: string;  // node id
  target: string;
  count: number;
  bytes: number;
  protocol: string;
}

// ── Force-directed layout (simple spring simulation) ─────────────────────────

function runLayout(nodes: GraphNode[], edges: GraphEdge[], iterations = 80) {
  const nodeMap = new Map(nodes.map((n) => [n.id, n]));
  const k = 120;         // spring rest length
  const repulsion = 8000;
  const damping = 0.8;

  for (let iter = 0; iter < iterations; iter++) {
    // Repulsion between all node pairs
    for (let i = 0; i < nodes.length; i++) {
      for (let j = i + 1; j < nodes.length; j++) {
        const a = nodes[i];
        const b = nodes[j];
        const dx = b.x - a.x || 0.01;
        const dy = b.y - a.y || 0.01;
        const dist = Math.sqrt(dx * dx + dy * dy) || 1;
        const force = repulsion / (dist * dist);
        a.vx -= (force * dx) / dist;
        a.vy -= (force * dy) / dist;
        b.vx += (force * dx) / dist;
        b.vy += (force * dy) / dist;
      }
    }

    // Attraction along edges
    for (const edge of edges) {
      const a = nodeMap.get(edge.source);
      const b = nodeMap.get(edge.target);
      if (!a || !b) continue;
      const dx = b.x - a.x;
      const dy = b.y - a.y;
      const dist = Math.sqrt(dx * dx + dy * dy) || 1;
      const force = (dist - k) * 0.05;
      a.vx += (force * dx) / dist;
      a.vy += (force * dy) / dist;
      b.vx -= (force * dx) / dist;
      b.vy -= (force * dy) / dist;
    }

    // Apply velocity + damping
    for (const n of nodes) {
      n.x += n.vx;
      n.y += n.vy;
      n.vx *= damping;
      n.vy *= damping;
    }
  }
}

// ── Protocol colour ───────────────────────────────────────────────────────────

function edgeColor(protocol: string): string {
  const p = protocol.toUpperCase();
  if (p === "HTTP" || p.startsWith("HTTP ")) return "#38bdf8"; // sky
  if (p === "DNS") return "#a78bfa";   // violet
  if (p === "TLS") return "#6366f1";   // indigo
  if (p === "ICMP") return "#22d3ee";  // cyan
  return "#6b7280";                     // gray
}

// ── Node radius by traffic weight ─────────────────────────────────────────────

function nodeRadius(flowCount: number, maxFlows: number): number {
  return 8 + (flowCount / Math.max(1, maxFlows)) * 18;
}

// ── Build graph from flows ────────────────────────────────────────────────────

function buildGraph(
  flows: FlowDto[],
  posCache: Map<string, { x: number; y: number }>,
) {
  const nodes = new Map<string, GraphNode>();
  const edgeMap = new Map<string, GraphEdge>();

  const ensureNode = (ip: string, flow: (typeof flows)[0], isSrc: boolean) => {
    if (!nodes.has(ip)) {
      // Reuse cached position so existing nodes don't jump when new flows arrive
      const cached = posCache.get(ip);
      nodes.set(ip, {
        id: ip,
        ip,
        flowCount: 0,
        bytes: 0,
        protocols: new Set(),
        country: isSrc ? flow.geoSrc?.countryCode : flow.geoDst?.countryCode,
        asOrg: isSrc ? flow.geoSrc?.asOrg : flow.geoDst?.asOrg,
        x: cached?.x ?? (Math.random() - 0.5) * 400,
        y: cached?.y ?? (Math.random() - 0.5) * 300,
        vx: 0,
        vy: 0,
      });
    }
    const n = nodes.get(ip)!;
    n.flowCount++;
    n.bytes += flow.length;
    n.protocols.add(flow.protocol);
    if (!n.country) {
      n.country = isSrc ? flow.geoSrc?.countryCode : flow.geoDst?.countryCode;
    }
  };

  for (const flow of flows) {
    ensureNode(flow.srcIp, flow, true);
    ensureNode(flow.dstIp, flow, false);

    const edgeKey = `${flow.srcIp}|${flow.dstIp}|${flow.protocol}`;
    if (edgeMap.has(edgeKey)) {
      const e = edgeMap.get(edgeKey)!;
      e.count++;
      e.bytes += flow.length;
    } else {
      edgeMap.set(edgeKey, {
        source: flow.srcIp,
        target: flow.dstIp,
        count: 1,
        bytes: flow.length,
        protocol: flow.protocol,
      });
    }
  }

  const nodeArr = Array.from(nodes.values());
  const edgeArr = Array.from(edgeMap.values());
  return { nodes: nodeArr, edges: edgeArr };
}

// ── Component ─────────────────────────────────────────────────────────────────

export function ServiceMapPane() {
  const { flows } = useCaptureStore();
  const svgRef = useRef<SVGSVGElement>(null);
  const [size, setSize] = useState({ w: 800, h: 300 });
  const [tooltip, setTooltip] = useState<{ node: GraphNode; x: number; y: number } | null>(null);
  const [pan, setPan] = useState({ x: 0, y: 0 });
  const [zoom, setZoom] = useState(1);
  const dragging = useRef<{ startX: number; startY: number; panX: number; panY: number } | null>(null);

  // Persistent position cache — survives graph rebuilds so nodes don't jump
  const positionCache = useRef<Map<string, { x: number; y: number }>>(new Map());

  // Responsive sizing
  useEffect(() => {
    const el = svgRef.current?.parentElement;
    if (!el) return;
    const ro = new ResizeObserver((entries) => {
      const { width, height } = entries[0].contentRect;
      setSize({ w: width, h: height });
    });
    ro.observe(el);
    return () => ro.disconnect();
  }, []);

  // Compute a stable key that only changes when the set of *unique IPs* changes.
  // Depends on flows.length so it runs when new flows arrive, but produces a
  // stable string value when no new IPs are introduced, preventing unnecessary
  // graph rebuilds on every packet.
  const ipSetKey = useMemo(() => {
    const ips = new Set<string>();
    for (const f of flows) {
      ips.add(f.srcIp);
      ips.add(f.dstIp);
    }
    return [...ips].sort().join("|");
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [flows.length]);

  // Rebuild graph and run layout only when the IP set changes (new nodes),
  // not on every new packet. Existing node positions come from positionCache.
  const graph = useMemo(() => {
    if (flows.length === 0) {
      positionCache.current.clear();
      return { nodes: [], edges: [] };
    }
    const g = buildGraph(flows, positionCache.current);
    // Only run the spring layout if there are genuinely new nodes
    const hasNewNodes = g.nodes.some((n) => !positionCache.current.has(n.id));
    if (hasNewNodes) {
      runLayout(g.nodes, g.edges, 120);
    }
    // Persist updated positions for next rebuild
    for (const n of g.nodes) {
      positionCache.current.set(n.id, { x: n.x, y: n.y });
    }
    return g;
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [ipSetKey]);

  // O(1) node lookup map for edge rendering (replaces O(n) .find() in render loop)
  const nodeMap = useMemo(
    () => new Map(graph.nodes.map((n) => [n.id, n])),
    [graph.nodes],
  );

  const maxFlows = useMemo(
    () => Math.max(1, ...graph.nodes.map((n) => n.flowCount)),
    [graph.nodes],
  );

  const cx = size.w / 2 + pan.x;
  const cy = size.h / 2 + pan.y;

  // Pan handlers
  const onMouseDown = useCallback((e: React.MouseEvent) => {
    dragging.current = { startX: e.clientX, startY: e.clientY, panX: pan.x, panY: pan.y };
    const onMove = (ev: MouseEvent) => {
      if (!dragging.current) return;
      setPan({
        x: dragging.current.panX + ev.clientX - dragging.current.startX,
        y: dragging.current.panY + ev.clientY - dragging.current.startY,
      });
    };
    const onUp = () => {
      dragging.current = null;
      window.removeEventListener("mousemove", onMove);
      window.removeEventListener("mouseup", onUp);
    };
    window.addEventListener("mousemove", onMove);
    window.addEventListener("mouseup", onUp);
  }, [pan]);

  const onWheel = useCallback((e: React.WheelEvent) => {
    e.preventDefault();
    setZoom((z) => Math.min(3, Math.max(0.3, z - e.deltaY * 0.001)));
  }, []);

  if (flows.length === 0) {
    return (
      <div className="flex h-full items-center justify-center gap-2 text-xs text-gray-500">
        <Network className="h-4 w-4 text-gray-700" />
        No flows yet — the service map will build as you capture traffic
      </div>
    );
  }

  return (
    <div className="relative h-full w-full overflow-hidden bg-[#070710]">
      {/* Controls */}
      <div className="absolute top-2 right-2 z-10 flex gap-1">
        <button
          className="rounded bg-white/10 px-2 py-0.5 text-[10px] text-gray-300 hover:bg-white/20"
          onClick={() => { setPan({ x: 0, y: 0 }); setZoom(1); }}
        >
          Reset
        </button>
        <span className="rounded bg-white/5 px-2 py-0.5 text-[10px] text-gray-500">
          {graph.nodes.length} nodes · {graph.edges.length} edges
        </span>
      </div>

      <svg
        ref={svgRef}
        width={size.w}
        height={size.h}
        className="cursor-grab active:cursor-grabbing"
        onMouseDown={onMouseDown}
        onWheel={onWheel}
      >
        <g transform={`translate(${cx},${cy}) scale(${zoom})`}>
          {/* Edges */}
          {graph.edges.map((edge, i) => {
            const src = nodeMap.get(edge.source);
            const tgt = nodeMap.get(edge.target);
            if (!src || !tgt) return null;
            const strokeW = Math.min(4, 0.5 + Math.log2(edge.count + 1) * 0.6);
            return (
              <line
                key={i}
                x1={src.x} y1={src.y}
                x2={tgt.x} y2={tgt.y}
                stroke={edgeColor(edge.protocol)}
                strokeWidth={strokeW}
                strokeOpacity={0.35}
              />
            );
          })}

          {/* Nodes */}
          {graph.nodes.map((node) => {
            const r = nodeRadius(node.flowCount, maxFlows);
            return (
              <g
                key={node.id}
                transform={`translate(${node.x},${node.y})`}
                className="cursor-pointer"
                onMouseEnter={(e) => {
                  const rect = svgRef.current!.getBoundingClientRect();
                  setTooltip({ node, x: e.clientX - rect.left + 12, y: e.clientY - rect.top - 8 });
                }}
                onMouseLeave={() => setTooltip(null)}
              >
                <circle
                  r={r}
                  fill="#1e1e3a"
                  stroke={node.protocols.has("TLS") ? "#6366f1" : "#334155"}
                  strokeWidth={1.5}
                />
                <text
                  textAnchor="middle"
                  dominantBaseline="middle"
                  fontSize={Math.max(7, r * 0.55)}
                  fill="#94a3b8"
                  className="select-none pointer-events-none"
                >
                  {node.country ? `${node.country}` : node.ip.split(".").slice(-2).join(".")}
                </text>
                <text
                  y={r + 9}
                  textAnchor="middle"
                  fontSize={8}
                  fill="#475569"
                  className="select-none pointer-events-none"
                >
                  {node.ip}
                </text>
              </g>
            );
          })}
        </g>
      </svg>

      {/* Tooltip */}
      {tooltip && (
        <div
          className="pointer-events-none absolute z-20 max-w-[200px] rounded-lg border border-white/10 bg-[#0d0d1a] p-2.5 text-xs shadow-xl"
          style={{ left: tooltip.x, top: tooltip.y }}
        >
          <p className="font-mono font-semibold text-white mb-1">{tooltip.node.ip}</p>
          {tooltip.node.country && (
            <p className="text-gray-400">Country: {tooltip.node.country}</p>
          )}
          {tooltip.node.asOrg && (
            <p className="text-gray-400 truncate">AS: {tooltip.node.asOrg}</p>
          )}
          <p className="text-gray-400">Flows: {tooltip.node.flowCount}</p>
          <p className="text-gray-400">
            Protocols: {[...tooltip.node.protocols].join(", ")}
          </p>
        </div>
      )}
    </div>
  );
}
