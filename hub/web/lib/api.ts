// ── Types ─────────────────────────────────────────────────────────────────────

export interface HttpFlow {
  method: string;
  path: string;
  status: number;
  latency_ms: number;
}

export interface DnsFlow {
  query_name: string;
  query_type: string;
  is_response: boolean;
  answers: string[];
  rcode: number;
}

export interface Flow {
  id: string;
  agent_id: string;
  hostname: string;
  timestamp: string;
  protocol: string;
  src_ip: string;
  src_port: number;
  dst_ip: string;
  dst_port: number;
  bytes_in: number;
  bytes_out: number;
  duration_ms: number;
  info: string;
  http?: HttpFlow;
  dns?: DnsFlow;
}

export interface StatsResponse {
  total_flows: number;
  flows_per_minute: number;
  top_protocols: { protocol: string; count: number }[];
  top_talkers: { ip: string; flows: number }[];
  active_agents: number;
}

export interface Agent {
  agent_id: string;
  hostname: string;
  version: string;
  interface: string;
  last_seen: string;
  registered_at: string;
}

// ── Client ────────────────────────────────────────────────────────────────────

const BASE_URL =
  process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080";

const API_KEY =
  process.env.NEXT_PUBLIC_API_KEY ?? "changeme";

function headers(): HeadersInit {
  return {
    "Content-Type": "application/json",
    "X-Api-Key": API_KEY,
  };
}

async function get<T>(path: string, params?: Record<string, string | number>): Promise<T> {
  const url = new URL(`${BASE_URL}${path}`);
  if (params) {
    Object.entries(params).forEach(([k, v]) => {
      if (v !== undefined && v !== "") url.searchParams.set(k, String(v));
    });
  }
  const res = await fetch(url.toString(), { headers: headers(), cache: "no-store" });
  if (!res.ok) {
    const body = await res.text().catch(() => "");
    throw new Error(`API ${path} → ${res.status}: ${body}`);
  }
  return res.json() as Promise<T>;
}

// ── API functions ─────────────────────────────────────────────────────────────

export async function fetchFlows(params?: {
  protocol?: string;
  src_ip?: string;
  dst_ip?: string;
  hostname?: string;
  limit?: number;
  offset?: number;
}): Promise<{ flows: Flow[]; total: number }> {
  return get("/api/v1/flows", params as Record<string, string | number>);
}

export async function fetchStats(): Promise<StatsResponse> {
  return get("/api/v1/stats");
}

export async function fetchAgents(): Promise<{ agents: Agent[] }> {
  return get("/api/v1/agents");
}

/**
 * Open a Server-Sent Events connection to the live flow stream.
 * Returns a cleanup function — call it to close the connection.
 */
export function createFlowStream(
  onFlow: (flow: Flow) => void,
  onError?: () => void,
  onOpen?: () => void,
): () => void {
  const url = `${BASE_URL}/api/v1/flows/stream?api_key=${encodeURIComponent(API_KEY)}`;
  const es = new EventSource(url);

  es.onopen = () => onOpen?.();

  es.onmessage = (e) => {
    try {
      const flow = JSON.parse(e.data) as Flow;
      onFlow(flow);
    } catch {
      // Ignore malformed events (e.g. keep-alive comments)
    }
  };

  es.onerror = () => {
    onError?.();
    es.close();
  };

  return () => es.close();
}
