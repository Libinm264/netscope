import { type ClassValue, clsx } from "clsx";
import { twMerge } from "tailwind-merge";
import type { FlowDto } from "@/types/flow";

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}

/** Return a Tailwind text-color class for a protocol string. */
export function protocolColor(protocol: string): string {
  const p = protocol.toUpperCase();
  if (p.startsWith("HTTP 4") || p.startsWith("HTTP 5")) return "text-proto-error";
  if (p === "HTTP" || p === "HTTPS") return "text-proto-http";
  if (p === "DNS") return "text-proto-dns";
  if (p === "TLS") return "text-proto-tls";
  if (p === "TCP") return "text-proto-tcp";
  if (p === "UDP") return "text-proto-udp";
  return "text-gray-400";
}

/** Return a Tailwind bg-color class for a row highlight. */
export function rowBgColor(flow: FlowDto, selected: boolean): string {
  if (selected) return "bg-blue-600/30 border-l-2 border-blue-500";
  const p = flow.protocol.toUpperCase();
  if (p.startsWith("HTTP 4") || p.startsWith("HTTP 5")) return "bg-red-950/20 hover:bg-red-950/40";
  if (p === "DNS") return "bg-purple-950/20 hover:bg-purple-950/30";
  if (p === "HTTP" || p === "HTTPS") return "hover:bg-blue-950/20";
  return "hover:bg-white/5";
}

/** Format bytes into a human-readable string. */
export function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(2)} MB`;
}

/** Apply a simple client-side filter to a list of flows. */
export function filterFlows(flows: FlowDto[], filter: string): FlowDto[] {
  if (!filter.trim()) return flows;
  const lower = filter.toLowerCase();

  return flows.filter((f) => {
    // Quick-filter shortcuts
    if (lower === "http") return f.protocol.toLowerCase().startsWith("http");
    if (lower === "dns") return f.protocol.toLowerCase() === "dns";
    if (lower === "errors") {
      const code = f.http?.statusCode ?? 0;
      return code >= 400;
    }

    // General text search across key fields
    return (
      f.srcIp.includes(lower) ||
      f.dstIp.includes(lower) ||
      f.protocol.toLowerCase().includes(lower) ||
      f.info.toLowerCase().includes(lower) ||
      String(f.srcPort).includes(lower) ||
      String(f.dstPort).includes(lower)
    );
  });
}
