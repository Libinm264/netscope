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
  from?: string;       // ISO datetime or datetime-local string
  to?: string;         // ISO datetime or datetime-local string
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

// ── Alert types ───────────────────────────────────────────────────────────────

export type AlertMetric =
  | "flows_per_minute"
  | "http_error_rate"
  | "dns_nxdomain_rate";

export type AlertCondition = "gt" | "lt";

export interface AlertRule {
  id: string;
  name: string;
  metric: AlertMetric;
  condition: AlertCondition;
  threshold: number;
  window_minutes: number;
  webhook_url: string;
  enabled: number;        // 0 | 1 (ClickHouse UInt8)
  cooldown_minutes: number;
  created_at: string;
}

export interface AlertEvent {
  id: string;
  rule_id: string;
  rule_name: string;
  metric: string;
  value: number;
  threshold: number;
  webhook_delivered: number;
  fired_at: string;
}

export interface CreateAlertRuleRequest {
  name: string;
  metric: AlertMetric;
  condition: AlertCondition;
  threshold: number;
  window_minutes?: number;
  webhook_url?: string;
  cooldown_minutes?: number;
}

// ── Alert API helpers ─────────────────────────────────────────────────────────

async function api<T>(
  method: string,
  path: string,
  body?: unknown,
): Promise<T> {
  const res = await fetch(`${BASE_URL}${path}`, {
    method,
    headers: headers(),
    cache: "no-store",
    body: body !== undefined ? JSON.stringify(body) : undefined,
  });
  if (!res.ok) {
    const text = await res.text().catch(() => "");
    throw new Error(`API ${method} ${path} → ${res.status}: ${text}`);
  }
  return res.json() as Promise<T>;
}

export async function fetchAlertRules(): Promise<{ rules: AlertRule[] }> {
  return get("/api/v1/alerts");
}

export async function createAlertRule(
  req: CreateAlertRuleRequest,
): Promise<AlertRule> {
  return api<AlertRule>("POST", "/api/v1/alerts", req);
}

export async function updateAlertRule(
  id: string,
  patch: Partial<Pick<AlertRule, "enabled" | "webhook_url">>,
): Promise<AlertRule> {
  return api<AlertRule>("PATCH", `/api/v1/alerts/${id}`, patch);
}

export async function deleteAlertRule(id: string): Promise<void> {
  await api<unknown>("DELETE", `/api/v1/alerts/${id}`);
}

export async function fetchAlertEvents(): Promise<{ events: AlertEvent[] }> {
  return get("/api/v1/alerts/events");
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
