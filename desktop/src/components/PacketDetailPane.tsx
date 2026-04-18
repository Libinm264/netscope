import { useState } from "react";
import { ChevronRight, ChevronDown, AlertTriangle, ShieldCheck, ShieldX, MapPin, ShieldAlert } from "lucide-react";
import { useCaptureStore } from "@/store/captureStore";
import { cn } from "@/lib/utils";
import type { FlowDto } from "@/types/flow";

// ── Generic tree types ────────────────────────────────────────────────────────

interface TreeNode {
  label: string;
  value?: string;
  children?: TreeNode[];
  warn?: boolean;
  error?: boolean;
}

// ── Tree builder ──────────────────────────────────────────────────────────────

function buildTree(flow: FlowDto): TreeNode[] {
  const nodes: TreeNode[] = [];

  // Frame
  nodes.push({
    label: "Frame",
    children: [
      { label: "Arrival time", value: flow.timestamp },
      { label: "Length",       value: `${flow.length} bytes` },
      { label: "Source",       value: flow.source === "hub" ? "NetScope Hub" : "Local capture" },
    ],
  });

  // Network layer + optional GeoIP
  const ipChildren: TreeNode[] = [
    { label: "Source",      value: flow.srcIp },
    { label: "Destination", value: flow.dstIp },
  ];

  if (flow.geoSrc) {
    const g = flow.geoSrc;
    ipChildren.push({
      label: "Source geo",
      children: [
        { label: "Country",  value: `${g.countryCode} ${g.countryName}` },
        ...(g.city ? [{ label: "City", value: g.city }] : []),
        ...(g.asOrg ? [{ label: "ASN", value: `AS${g.asn} ${g.asOrg}` }] : []),
      ],
    });
  }
  if (flow.geoDst) {
    const g = flow.geoDst;
    ipChildren.push({
      label: "Destination geo",
      children: [
        { label: "Country",  value: `${g.countryCode} ${g.countryName}` },
        ...(g.city ? [{ label: "City", value: g.city }] : []),
        ...(g.asOrg ? [{ label: "ASN", value: `AS${g.asn} ${g.asOrg}` }] : []),
      ],
    });
  }

  nodes.push({ label: "Internet Protocol", children: ipChildren });

  // Threat intelligence
  if (flow.threat) {
    const t = flow.threat;
    const isHigh = t.level === "high";
    const isMed  = t.level === "medium";
    nodes.push({
      label: "Threat Intelligence",
      error: isHigh,
      warn:  isMed,
      children: [
        {
          label: "Score",
          value: `${t.score}/100 — ${t.level.toUpperCase()}`,
          error: isHigh,
          warn:  isMed,
        },
        ...t.reasons.map((r) => ({ label: "Reason", value: r, error: isHigh, warn: isMed })),
      ],
    });
  }

  // Transport layer
  if (flow.arp == null && flow.icmp == null) {
    const transportProto =
      flow.protocol.startsWith("HTTP") || flow.protocol === "TLS"
        ? "TCP"
        : flow.protocol === "DNS"
        ? "UDP"
        : flow.protocol;

    nodes.push({
      label: `${transportProto} Segment`,
      children: [
        { label: "Source port",      value: String(flow.srcPort) },
        { label: "Destination port", value: String(flow.dstPort) },
        ...(flow.tcpStats && flow.tcpStats.retransmissions > 0
          ? [{ label: "Retransmissions", value: String(flow.tcpStats.retransmissions), warn: true }]
          : []),
        ...(flow.tcpStats && flow.tcpStats.outOfOrder > 0
          ? [{ label: "Out-of-order segments", value: String(flow.tcpStats.outOfOrder), warn: true }]
          : []),
      ],
    });
  }

  // ── HTTP ──────────────────────────────────────────────────────────────────
  if (flow.http) {
    const h = flow.http;
    const httpChildren: TreeNode[] = [];

    if (h.method) {
      httpChildren.push({
        label: "Request",
        children: [
          { label: "Method", value: h.method },
          { label: "Path",   value: h.path ?? "/" },
          ...(h.host ? [{ label: "Host", value: h.host }] : []),
          {
            label: `Headers (${h.reqHeaders.length})`,
            children: h.reqHeaders.map(([k, v]) => ({ label: k, value: v })),
          },
          ...(h.reqBodyPreview ? [{ label: "Body preview", value: h.reqBodyPreview }] : []),
        ],
      });
    }

    if (h.statusCode !== undefined) {
      const isError = h.statusCode >= 400;
      httpChildren.push({
        label: "Response",
        error: isError,
        children: [
          { label: "Status",  value: `${h.statusCode} ${h.statusText ?? ""}`, error: isError },
          ...(h.latencyMs !== undefined ? [{ label: "Latency", value: `${h.latencyMs} ms` }] : []),
          {
            label: `Headers (${h.respHeaders.length})`,
            children: h.respHeaders.map(([k, v]) => ({ label: k, value: v })),
          },
          ...(h.respBodyPreview ? [{ label: "Body preview", value: h.respBodyPreview }] : []),
        ],
      });
    }

    nodes.push({ label: "Hypertext Transfer Protocol", children: httpChildren });
  }

  // ── DNS ───────────────────────────────────────────────────────────────────
  if (flow.dns) {
    const d = flow.dns;
    const isNxdomain = d.rcode === "NXDOMAIN";
    nodes.push({
      label: "Domain Name System",
      children: [
        { label: "Transaction ID", value: `0x${d.transactionId.toString(16).padStart(4, "0")}` },
        { label: "Type",       value: d.isResponse ? "Response" : "Query" },
        { label: "Query name", value: d.queryName },
        { label: "Query type", value: d.queryType },
        ...(d.rcode ? [{ label: "Response code", value: d.rcode, error: isNxdomain, warn: !isNxdomain && d.rcode !== "NOERROR" }] : []),
        ...(d.answers.length > 0
          ? [{
              label: `Answers (${d.answers.length})`,
              children: d.answers.map((a) => ({
                label: `${a.recordType} ${a.name}`,
                value: `${a.data} (TTL ${a.ttl}s)`,
              })),
            }]
          : []),
      ],
    });
  }

  // ── TLS ───────────────────────────────────────────────────────────────────
  if (flow.tls) {
    const t = flow.tls;
    const children: TreeNode[] = [
      { label: "Message type", value: t.recordType },
      { label: "Version",      value: t.version },
      ...(t.negotiatedVersion && t.negotiatedVersion !== t.version
        ? [{ label: "Negotiated version", value: t.negotiatedVersion }]
        : []),
    ];

    if (t.sni)
      children.push({ label: "Server Name (SNI)", value: t.sni });
    if (t.cipherSuites.length > 0) {
      children.push({
        label: `Cipher suites offered (${t.cipherSuites.length})${t.hasWeakCipher ? " ⚠" : ""}`,
        warn: t.hasWeakCipher,
        children: t.cipherSuites.map((c) => ({ label: c, warn: isWeakCipherName(c) })),
      });
    }
    if (t.chosenCipher)
      children.push({ label: "Chosen cipher", value: t.chosenCipher, warn: isWeakCipherName(t.chosenCipher) });
    if (t.certCn) {
      children.push({
        label: "Certificate",
        error: t.certExpired,
        children: [
          { label: "Common Name", value: t.certCn },
          ...(t.certIssuer ? [{ label: "Issuer", value: t.certIssuer }] : []),
          ...(t.certExpiry
            ? [{ label: "Expires", value: t.certExpiry, error: t.certExpired, warn: !t.certExpired && daysUntil(t.certExpiry) < 30 }]
            : []),
          ...(t.certSans.length > 0
            ? [{ label: `SANs (${t.certSans.length})`, children: t.certSans.map((s) => ({ label: s })) }]
            : []),
        ],
      });
    }
    if (t.alertLevel) {
      children.push(
        { label: "Alert level",       value: t.alertLevel,       error: t.alertLevel === "fatal", warn: t.alertLevel === "warning" },
        { label: "Alert description", value: t.alertDescription ?? "unknown", error: t.alertLevel === "fatal" },
      );
    }
    nodes.push({ label: "Transport Layer Security", children });
  }

  // ── ICMP ──────────────────────────────────────────────────────────────────
  if (flow.icmp) {
    const i = flow.icmp;
    const isUnreachable = i.icmpType === 3;
    nodes.push({
      label: "ICMP",
      error: isUnreachable,
      children: [
        { label: "Type",  value: `${i.icmpType} (${i.typeStr})`, error: isUnreachable },
        { label: "Code",  value: String(i.icmpCode) },
        ...(i.echoId  !== undefined ? [{ label: "Identifier", value: `0x${i.echoId.toString(16).padStart(4, "0")}` }] : []),
        ...(i.echoSeq !== undefined ? [{ label: "Sequence",   value: String(i.echoSeq) }] : []),
        ...(i.rttMs   !== undefined ? [{ label: "Round-trip time", value: `${i.rttMs.toFixed(2)} ms` }] : []),
      ],
    });
  }

  // ── ARP ───────────────────────────────────────────────────────────────────
  if (flow.arp) {
    const a = flow.arp;
    nodes.push({
      label: "Address Resolution Protocol",
      children: [
        { label: "Operation",  value: a.operation === "who-has" ? "1 (Request)" : "2 (Reply)" },
        { label: "Sender IP",  value: a.senderIp },
        { label: "Sender MAC", value: a.senderMac },
        { label: "Target IP",  value: a.targetIp },
        { label: "Target MAC", value: a.targetMac },
      ],
    });
  }

  return nodes;
}

// ── Helper functions ──────────────────────────────────────────────────────────

function isWeakCipherName(name: string): boolean {
  return (
    name.includes("RC4") ||
    name.includes("3DES") ||
    name.includes("_NULL_") ||
    name.includes("EXPORT") ||
    name.includes("MD5") ||
    (name.includes("RSA_WITH") && !name.includes("GCM") && !name.includes("ECDHE"))
  );
}

function daysUntil(dateStr: string): number {
  return Math.floor((new Date(dateStr).getTime() - Date.now()) / 86_400_000);
}

// ── Tree node renderer ────────────────────────────────────────────────────────

function TreeNodeRow({ node, depth = 0, defaultOpen = false }: {
  node: TreeNode;
  depth?: number;
  defaultOpen?: boolean;
}) {
  const [open, setOpen] = useState(defaultOpen || depth === 0);
  const hasChildren = node.children && node.children.length > 0;

  const labelCls = node.error ? "text-red-400" : node.warn ? "text-amber-400" : "text-blue-300";
  const valueCls = node.error ? "text-red-300" : node.warn ? "text-amber-300" : "text-gray-200";

  return (
    <div>
      <div
        className={cn(
          "flex items-start gap-1 py-0.5 text-xs font-mono cursor-default hover:bg-white/5 select-text",
          (node.error || node.warn) && "bg-red-950/10 hover:bg-red-950/20",
        )}
        style={{ paddingLeft: `${depth * 16 + 4}px` }}
        onClick={() => hasChildren && setOpen((v) => !v)}
      >
        {hasChildren ? (
          <span className="mt-0.5 shrink-0 text-gray-400">
            {open ? <ChevronDown className="h-3 w-3" /> : <ChevronRight className="h-3 w-3" />}
          </span>
        ) : (
          <span className="w-3 shrink-0" />
        )}
        <span className={cn("shrink-0", labelCls)}>{node.label}</span>
        {node.value !== undefined && (
          <>
            <span className="text-gray-500 shrink-0">: </span>
            <span className={cn("break-all", valueCls)}>{node.value}</span>
          </>
        )}
      </div>
      {hasChildren && open && (
        <div>
          {node.children!.map((child, i) => (
            <TreeNodeRow key={i} node={child} depth={depth + 1} />
          ))}
        </div>
      )}
    </div>
  );
}

// ── Banners ───────────────────────────────────────────────────────────────────

function TlsBanner({ flow }: { flow: FlowDto }) {
  const t = flow.tls;
  if (!t) return null;
  if (t.certExpired)
    return (
      <div className="flex items-center gap-2 mx-1 mb-1 px-3 py-2 rounded bg-red-950/40 border border-red-700/40 text-xs text-red-400">
        <ShieldX size={13} />Certificate expired on {t.certExpiry}
      </div>
    );
  if (t.alertDescription === "certificate_expired" || t.alertDescription === "handshake_failure")
    return (
      <div className="flex items-center gap-2 mx-1 mb-1 px-3 py-2 rounded bg-red-950/40 border border-red-700/40 text-xs text-red-400">
        <ShieldX size={13} />TLS {t.alertLevel}: {t.alertDescription}
      </div>
    );
  if (t.hasWeakCipher)
    return (
      <div className="flex items-center gap-2 mx-1 mb-1 px-3 py-2 rounded bg-amber-950/30 border border-amber-700/30 text-xs text-amber-400">
        <AlertTriangle size={13} />Weak cipher suite advertised
      </div>
    );
  if (t.certCn && !t.certExpired) {
    const days = t.certExpiry ? daysUntil(t.certExpiry) : null;
    if (days !== null && days < 30)
      return (
        <div className="flex items-center gap-2 mx-1 mb-1 px-3 py-2 rounded bg-amber-950/30 border border-amber-700/30 text-xs text-amber-400">
          <AlertTriangle size={13} />Certificate expires in {days} day{days !== 1 ? "s" : ""}
        </div>
      );
    return (
      <div className="flex items-center gap-2 mx-1 mb-1 px-3 py-2 rounded bg-emerald-950/30 border border-emerald-700/30 text-xs text-emerald-400">
        <ShieldCheck size={13} />Valid certificate — {t.certCn}
      </div>
    );
  }
  return null;
}

function ThreatBanner({ flow }: { flow: FlowDto }) {
  const threat = flow.threat;
  if (!threat || threat.level === "clean") return null;

  const isHigh = threat.level === "high";
  return (
    <div
      className={cn(
        "flex items-start gap-2 mx-1 mb-1 px-3 py-2 rounded border text-xs",
        isHigh
          ? "bg-red-950/40 border-red-700/40 text-red-400"
          : threat.level === "medium"
          ? "bg-orange-950/30 border-orange-700/30 text-orange-400"
          : "bg-yellow-950/20 border-yellow-700/20 text-yellow-400",
      )}
    >
      <ShieldAlert size={13} className="mt-0.5 shrink-0" />
      <div>
        <span className="font-semibold">Threat score {threat.score}/100 ({threat.level.toUpperCase()})</span>
        <ul className="mt-1 space-y-0.5 list-disc list-inside">
          {threat.reasons.map((r, i) => <li key={i}>{r}</li>)}
        </ul>
      </div>
    </div>
  );
}

function GeoBanner({ flow }: { flow: FlowDto }) {
  const geo = flow.geoDst;
  if (!geo || geo.countryCode === "??") return null;
  return (
    <div className="flex items-center gap-2 mx-1 mb-1 px-3 py-2 rounded bg-slate-900/40 border border-slate-700/30 text-xs text-slate-400">
      <MapPin size={13} />
      Destination: {geo.countryName}{geo.city ? `, ${geo.city}` : ""}
      {geo.asOrg ? ` · ${geo.asOrg}` : ""}
    </div>
  );
}

// ── Detail pane ───────────────────────────────────────────────────────────────

export function PacketDetailPane() {
  const selectedFlow = useCaptureStore((s) => s.selectedFlow);

  if (!selectedFlow) {
    return (
      <div className="flex h-full items-center justify-center text-gray-500 text-xs">
        Select a packet to see decoded fields
      </div>
    );
  }

  const tree = buildTree(selectedFlow);

  return (
    <div className="h-full overflow-auto bg-[#0a0a14] p-1">
      <ThreatBanner flow={selectedFlow} />
      <TlsBanner flow={selectedFlow} />
      <GeoBanner flow={selectedFlow} />
      {tree.map((node, i) => (
        <TreeNodeRow key={i} node={node} depth={0} defaultOpen />
      ))}
    </div>
  );
}
