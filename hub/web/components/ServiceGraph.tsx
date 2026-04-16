"use client";

import { useEffect, useRef, useState } from "react";
import type { ServiceGraph, ServiceEdge } from "@/lib/api";

// ── Protocol colour palette ────────────────────────────────────────────────────
const PROTO_COLOR: Record<string, string> = {
  HTTP:    "#60a5fa",
  HTTPS:   "#818cf8",
  DNS:     "#a78bfa",
  TLS:     "#22d3ee",
  ICMP:    "#34d399",
  ARP:     "#fbbf24",
  TCP:     "#64748b",
  UDP:     "#94a3b8",
};
const DEFAULT_COLOR = "#475569";

function protoColor(p: string) {
  return PROTO_COLOR[p.toUpperCase()] ?? DEFAULT_COLOR;
}

// ── Layout helpers ─────────────────────────────────────────────────────────────

interface NodePos {
  id: string;
  ip: string;
  label: string;
  x: number;
  y: number;
  r: number;         // circle radius
  isKnown: boolean;
  flowCount: number;
}

/**
 * Lay out nodes in concentric rings centred on (cx, cy).
 * Highest-traffic nodes sit in the innermost ring.
 */
function layoutNodes(
  graph: ServiceGraph,
  cx: number,
  cy: number,
): NodePos[] {
  const sorted = [...graph.nodes].sort((a, b) => b.flow_count - a.flow_count);
  const rings = [1, 6, 12, 20, 999]; // max nodes per ring
  const radii  = [0, 120, 220, 310, 400];

  const positions: NodePos[] = [];
  let offset = 0;

  for (let ri = 0; ri < rings.length; ri++) {
    const ring  = sorted.slice(offset, offset + rings[ri]);
    if (ring.length === 0) break;
    offset += rings[ri];

    const r = radii[ri];
    for (let i = 0; i < ring.length; i++) {
      const angle = (2 * Math.PI * i) / Math.max(ring.length, 1) - Math.PI / 2;
      const n = ring[i];
      const nodeR = n.is_known ? 18 : Math.max(8, Math.min(14, 8 + Math.log10(n.flow_count + 1) * 4));
      positions.push({
        id:        n.id,
        ip:        n.ip,
        label:     n.hostname || n.ip,
        x:         r === 0 ? cx : cx + r * Math.cos(angle),
        y:         r === 0 ? cy : cy + r * Math.sin(angle),
        r:         nodeR,
        isKnown:   n.is_known,
        flowCount: n.flow_count,
      });
    }
  }
  return positions;
}

// ── Tooltip ────────────────────────────────────────────────────────────────────

interface TooltipState {
  x: number;
  y: number;
  lines: string[];
}

// ── Main component ─────────────────────────────────────────────────────────────

interface ServiceGraphProps {
  data: ServiceGraph;
}

export function ServiceGraphViz({ data }: ServiceGraphProps) {
  const svgRef = useRef<SVGSVGElement>(null);
  const [dims, setDims] = useState({ w: 800, h: 560 });
  const [tooltip, setTooltip] = useState<TooltipState | null>(null);
  const [hover, setHover] = useState<string | null>(null);

  // Responsive resize
  useEffect(() => {
    const el = svgRef.current?.parentElement;
    if (!el) return;
    const obs = new ResizeObserver((entries) => {
      const { width } = entries[0].contentRect;
      setDims({ w: width, h: Math.max(400, width * 0.65) });
    });
    obs.observe(el);
    return () => obs.disconnect();
  }, []);

  const cx = dims.w / 2;
  const cy = dims.h / 2;
  const positions = layoutNodes(data, cx, cy);
  const posMap = new Map(positions.map((p) => [p.id, p]));

  // Deduplicate edges: keep highest-count edge per (src, dst) pair
  const edgeMap = new Map<string, ServiceEdge>();
  for (const e of data.edges) {
    const key = [e.source, e.target].sort().join("|");
    const existing = edgeMap.get(key);
    if (!existing || e.count > existing.count) edgeMap.set(key, e);
  }
  const edges = Array.from(edgeMap.values());

  const maxCount = edges.reduce((m, e) => Math.max(m, e.count), 1);

  return (
    <div className="relative w-full select-none">
      <svg
        ref={svgRef}
        width={dims.w}
        height={dims.h}
        className="w-full overflow-visible"
        onMouseLeave={() => { setTooltip(null); setHover(null); }}
      >
        <defs>
          {/* Subtle radial glow for the centre */}
          <radialGradient id="centre-glow" cx="50%" cy="50%" r="50%">
            <stop offset="0%"   stopColor="#6366f1" stopOpacity="0.15" />
            <stop offset="100%" stopColor="#6366f1" stopOpacity="0" />
          </radialGradient>
        </defs>

        {/* Background centre glow */}
        <ellipse cx={cx} cy={cy} rx={100} ry={100} fill="url(#centre-glow)" />

        {/* Edges */}
        {edges.map((e) => {
          const src = posMap.get(e.source);
          const dst = posMap.get(e.target);
          if (!src || !dst) return null;

          const isFaded = hover !== null && hover !== e.source && hover !== e.target;
          const thickness = 0.5 + (e.count / maxCount) * 3.5;
          const color = protoColor(e.protocol);

          return (
            <line
              key={`${e.source}-${e.target}-${e.protocol}`}
              x1={src.x} y1={src.y}
              x2={dst.x} y2={dst.y}
              stroke={color}
              strokeWidth={thickness}
              strokeOpacity={isFaded ? 0.06 : 0.45}
              className="transition-all duration-150"
              onMouseEnter={(ev) => {
                setTooltip({
                  x: (src.x + dst.x) / 2,
                  y: (src.y + dst.y) / 2,
                  lines: [
                    `${e.source} → ${e.target}`,
                    `Protocol: ${e.protocol}`,
                    `Flows: ${e.count.toLocaleString()}`,
                    `Avg latency: ${e.avg_latency_ms.toFixed(1)} ms`,
                    `Bytes: ${formatBytes(e.bytes_total)}`,
                  ],
                });
              }}
              onMouseLeave={() => setTooltip(null)}
            />
          );
        })}

        {/* Nodes */}
        {positions.map((p) => {
          const isFaded = hover !== null && hover !== p.id;
          const isHovered = hover === p.id;

          return (
            <g
              key={p.id}
              transform={`translate(${p.x},${p.y})`}
              style={{ cursor: "pointer" }}
              onMouseEnter={() => {
                setHover(p.id);
                setTooltip({
                  x: p.x,
                  y: p.y - p.r - 8,
                  lines: [
                    p.label !== p.ip ? p.label : "",
                    `IP: ${p.ip}`,
                    `Flows: ${p.flowCount.toLocaleString()}`,
                    p.isKnown ? "Agent (registered)" : "External",
                  ].filter(Boolean) as string[],
                });
              }}
              onMouseLeave={() => { setHover(null); setTooltip(null); }}
            >
              {/* Outer ring for known agents */}
              {p.isKnown && (
                <circle
                  r={p.r + 5}
                  fill="none"
                  stroke="#6366f1"
                  strokeWidth={1.5}
                  strokeOpacity={isFaded ? 0.1 : 0.6}
                  strokeDasharray="3 2"
                />
              )}

              {/* Main circle */}
              <circle
                r={p.r}
                fill={p.isKnown ? "#312e81" : "#1e293b"}
                stroke={p.isKnown ? "#6366f1" : "#334155"}
                strokeWidth={isHovered ? 2 : 1}
                fillOpacity={isFaded ? 0.3 : 1}
                strokeOpacity={isFaded ? 0.2 : 1}
                className="transition-all duration-150"
              />

              {/* IP label */}
              <text
                y={p.r + 13}
                textAnchor="middle"
                fontSize={10}
                fill="#94a3b8"
                fillOpacity={isFaded ? 0.2 : 0.9}
                className="pointer-events-none"
              >
                {abbreviateLabel(p.label)}
              </text>
            </g>
          );
        })}

        {/* Tooltip */}
        {tooltip && (
          <TooltipBox x={tooltip.x} y={tooltip.y} lines={tooltip.lines} dims={dims} />
        )}
      </svg>

      {/* Legend */}
      <div className="absolute bottom-3 left-3 flex flex-wrap gap-2">
        {Object.entries(PROTO_COLOR).map(([proto, color]) => (
          <span key={proto} className="flex items-center gap-1 text-[10px] text-slate-400">
            <span className="inline-block w-3 h-1 rounded" style={{ background: color }} />
            {proto}
          </span>
        ))}
      </div>
    </div>
  );
}

// ── Tooltip box ────────────────────────────────────────────────────────────────

function TooltipBox({
  x, y, lines, dims,
}: { x: number; y: number; lines: string[]; dims: { w: number; h: number } }) {
  const W = 180;
  const H = lines.length * 16 + 12;
  // Clamp so tooltip stays within SVG
  const tx = Math.min(x + 8, dims.w - W - 4);
  const ty = Math.max(4, Math.min(y - H / 2, dims.h - H - 4));

  return (
    <g pointerEvents="none">
      <rect
        x={tx} y={ty} width={W} height={H}
        rx={4}
        fill="#0f172a"
        stroke="#334155"
        strokeWidth={1}
        fillOpacity={0.95}
      />
      {lines.map((line, i) => (
        <text
          key={i}
          x={tx + 8}
          y={ty + 14 + i * 16}
          fontSize={10}
          fill={i === 0 ? "#e2e8f0" : "#94a3b8"}
          fontWeight={i === 0 ? "600" : "400"}
        >
          {line}
        </text>
      ))}
    </g>
  );
}

// ── Utilities ──────────────────────────────────────────────────────────────────

function abbreviateLabel(label: string): string {
  if (label.length <= 15) return label;
  // For IPs keep as-is; for hostnames truncate
  return label.slice(0, 13) + "…";
}

function formatBytes(b: number): string {
  if (b < 1024) return `${b} B`;
  if (b < 1024 ** 2) return `${(b / 1024).toFixed(1)} KB`;
  if (b < 1024 ** 3) return `${(b / 1024 ** 2).toFixed(1)} MB`;
  return `${(b / 1024 ** 3).toFixed(1)} GB`;
}
