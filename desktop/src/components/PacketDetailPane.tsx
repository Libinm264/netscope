import { useState } from "react";
import { ChevronRight, ChevronDown } from "lucide-react";
import { useCaptureStore } from "@/store/captureStore";
import { cn } from "@/lib/utils";

interface TreeNode {
  label: string;
  value?: string;
  children?: TreeNode[];
}

function buildTree(flow: import("@/types/flow").FlowDto): TreeNode[] {
  const nodes: TreeNode[] = [];

  // Frame info
  nodes.push({
    label: "Frame",
    children: [
      { label: "Arrival time", value: flow.timestamp },
      { label: "Length", value: `${flow.length} bytes` },
    ],
  });

  // Network layer
  nodes.push({
    label: "Internet Protocol",
    children: [
      { label: "Source", value: flow.srcIp },
      { label: "Destination", value: flow.dstIp },
    ],
  });

  // Transport layer
  const proto = flow.protocol.startsWith("HTTP") ? "TCP" : flow.protocol === "DNS" ? "UDP" : flow.protocol;
  nodes.push({
    label: `${proto} Segment`,
    children: [
      { label: "Source port", value: String(flow.srcPort) },
      { label: "Destination port", value: String(flow.dstPort) },
    ],
  });

  // HTTP
  if (flow.http) {
    const h = flow.http;
    const httpChildren: TreeNode[] = [];

    if (h.method) {
      httpChildren.push({
        label: "Request",
        children: [
          { label: "Method", value: h.method },
          { label: "Path", value: h.path ?? "/" },
          ...(h.host ? [{ label: "Host", value: h.host }] : []),
          {
            label: `Headers (${h.reqHeaders.length})`,
            children: h.reqHeaders.map(([k, v]) => ({ label: k, value: v })),
          },
          ...(h.reqBodyPreview
            ? [{ label: "Body preview", value: h.reqBodyPreview }]
            : []),
        ],
      });
    }

    if (h.statusCode !== undefined) {
      httpChildren.push({
        label: "Response",
        children: [
          {
            label: "Status",
            value: `${h.statusCode} ${h.statusText ?? ""}`,
          },
          ...(h.latencyMs !== undefined
            ? [{ label: "Latency", value: `${h.latencyMs} ms` }]
            : []),
          {
            label: `Headers (${h.respHeaders.length})`,
            children: h.respHeaders.map(([k, v]) => ({ label: k, value: v })),
          },
          ...(h.respBodyPreview
            ? [{ label: "Body preview", value: h.respBodyPreview }]
            : []),
        ],
      });
    }

    nodes.push({ label: "Hypertext Transfer Protocol", children: httpChildren });
  }

  // DNS
  if (flow.dns) {
    const d = flow.dns;
    nodes.push({
      label: "Domain Name System",
      children: [
        { label: "Transaction ID", value: `0x${d.transactionId.toString(16).padStart(4, "0")}` },
        { label: "Type", value: d.isResponse ? "Response" : "Query" },
        { label: "Query name", value: d.queryName },
        { label: "Query type", value: d.queryType },
        ...(d.rcode ? [{ label: "Response code", value: d.rcode }] : []),
        ...(d.answers.length > 0
          ? [
              {
                label: `Answers (${d.answers.length})`,
                children: d.answers.map((a) => ({
                  label: `${a.recordType} ${a.name}`,
                  value: `${a.data} (TTL ${a.ttl}s)`,
                })),
              },
            ]
          : []),
      ],
    });
  }

  return nodes;
}

function TreeNodeRow({
  node,
  depth = 0,
  defaultOpen = false,
}: {
  node: TreeNode;
  depth?: number;
  defaultOpen?: boolean;
}) {
  const [open, setOpen] = useState(defaultOpen || depth === 0);
  const hasChildren = node.children && node.children.length > 0;

  return (
    <div>
      <div
        className={cn(
          "flex items-start gap-1 py-0.5 text-xs font-mono cursor-default hover:bg-white/5",
          "select-text"
        )}
        style={{ paddingLeft: `${depth * 16 + 4}px` }}
        onClick={() => hasChildren && setOpen((v) => !v)}
      >
        {hasChildren ? (
          <span className="mt-0.5 shrink-0 text-gray-400">
            {open ? (
              <ChevronDown className="h-3 w-3" />
            ) : (
              <ChevronRight className="h-3 w-3" />
            )}
          </span>
        ) : (
          <span className="w-3 shrink-0" />
        )}

        <span className="text-blue-300 shrink-0">{node.label}</span>

        {node.value !== undefined && (
          <>
            <span className="text-gray-500 shrink-0">: </span>
            <span className="text-gray-200 break-all">{node.value}</span>
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
      {tree.map((node, i) => (
        <TreeNodeRow key={i} node={node} depth={0} defaultOpen />
      ))}
    </div>
  );
}
