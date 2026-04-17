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

export interface TlsFlow {
  record_type: string;          // "ClientHello" | "ServerHello" | "Certificate" | "Alert"
  version: string;
  sni?: string;
  cipher_suites?: string[];
  has_weak_cipher: boolean;
  chosen_cipher?: string;
  negotiated_version?: string;
  cert_cn?: string;
  cert_sans?: string[];
  cert_expiry?: string;         // "YYYY-MM-DD"
  cert_expired: boolean;
  cert_issuer?: string;
  alert_level?: string;
  alert_description?: string;
}

export interface IcmpFlow {
  icmp_type: number;
  icmp_code: number;
  type_str: string;
  echo_id?: number;
  echo_seq?: number;
  rtt_ms?: number;
}

export interface ArpFlow {
  operation: string;
  sender_ip: string;
  sender_mac: string;
  target_ip: string;
  target_mac: string;
}

export interface TcpStats {
  retransmissions: number;
  out_of_order: number;
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
  tls?: TlsFlow;
  icmp?: IcmpFlow;
  arp?: ArpFlow;
  tcp_stats?: TcpStats;
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
  | "dns_nxdomain_rate"
  | "anomaly_flow_rate"
  | "anomaly_http_latency";

export type AlertIntegrationType =
  | "webhook"
  | "slack"
  | "pagerduty"
  | "opsgenie"
  | "teams";

export type AlertCondition = "gt" | "lt";

export interface AlertRule {
  id: string;
  name: string;
  metric: AlertMetric;
  condition: AlertCondition;
  threshold: number;
  window_minutes: number;
  integration_type: AlertIntegrationType;
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
  integration_type?: AlertIntegrationType;
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

// ── Enrollment tokens ─────────────────────────────────────────────────────────

export interface EnrollmentToken {
  id: string;
  name: string;
  token: string;
  created_at: string;
  expires_at: string;
  used_count: number;
  revoked: boolean;
}

export async function fetchEnrollmentTokens(): Promise<{ tokens: EnrollmentToken[] }> {
  return get("/api/v1/enrollment-tokens");
}

export async function createEnrollmentToken(name: string, expires_in = "7d"): Promise<EnrollmentToken> {
  return api<EnrollmentToken>("POST", "/api/v1/enrollment-tokens", { name, expires_in });
}

export async function revokeEnrollmentToken(id: string): Promise<void> {
  await api<unknown>("DELETE", `/api/v1/enrollment-tokens/${id}`);
}

// ── TLS certificate fleet ──────────────────────────────────────────────────────

export interface TlsCert {
  fingerprint: string;
  cn: string;
  issuer: string;
  expiry: string;
  expired: boolean;
  days_left: number;
  sans: string[];
  agent_id: string;
  hostname: string;
  src_ip: string;
  dst_ip: string;
  first_seen: string;
  last_seen: string;
}

export interface CertSummary {
  expired: number;
  critical: number;
  warning: number;
  ok: number;
}

export async function fetchCerts(): Promise<{ certs: TlsCert[]; total: number; summary: CertSummary }> {
  return get("/api/v1/certs");
}

// ── API tokens (RBAC) ─────────────────────────────────────────────────────────

export interface APIToken {
  id: string;
  name: string;
  role: "admin" | "viewer";
  token: string;
  created_at: string;
  last_used: string;
  revoked: boolean;
}

export async function fetchAPITokens(): Promise<{ tokens: APIToken[] }> {
  return get("/api/v1/tokens");
}

export async function createAPIToken(name: string, role: "admin" | "viewer"): Promise<APIToken> {
  return api<APIToken>("POST", "/api/v1/tokens", { name, role });
}

export async function revokeAPIToken(id: string): Promise<void> {
  await api<unknown>("DELETE", `/api/v1/tokens/${id}`);
}

// ── Service dependency graph ───────────────────────────────────────────────────

export interface ServiceNode {
  id: string;
  ip: string;
  flow_count: number;
  is_known: boolean;
  hostname: string;
}

export interface ServiceEdge {
  source: string;
  target: string;
  protocol: string;
  count: number;
  avg_latency_ms: number;
  bytes_total: number;
}

export interface ServiceGraph {
  nodes: ServiceNode[];
  edges: ServiceEdge[];
  window: string;
}

export async function fetchServiceGraph(window = "1h"): Promise<ServiceGraph> {
  return get("/api/v1/services/graph", { window });
}

// ── HTTP endpoint analytics ────────────────────────────────────────────────────

export interface EndpointStat {
  method: string;
  path: string;
  count: number;
  success_count: number;
  error_count: number;
  error_rate: number;    // percent 0-100
  avg_latency_ms: number;
  p50_ms: number;
  p95_ms: number;
  p99_ms: number;
}

export interface EndpointStatsResponse {
  endpoints: EndpointStat[];
  window: string;
  total: number;
}

export async function fetchEndpointStats(window = "1h"): Promise<EndpointStatsResponse> {
  return get("/api/v1/analytics/endpoints", { window });
}

// ── Compliance reporting ───────────────────────────────────────────────────────

export interface ComplianceSummary {
  total_connections: number;
  external_connections: number;
  tls_issues: number;
  window: string;
}

export interface ConnectionRecord {
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
  is_external: boolean;
}

export interface TLSAuditRecord {
  fingerprint: string;
  cn: string;
  issuer: string;
  expiry: string;
  expired: boolean;
  days_left: number;
  hostname: string;
  dst_ip: string;
  last_seen: string;
  issue: "expired" | "expiring_critical" | "expiring_soon" | "self_signed" | "ok";
}

export interface TopTalker {
  ip: string;
  hostname: string;
  bytes_out: number;
  bytes_in: number;
  flow_count: number;
  unique_destinations: number;
}

export interface ExternalDest {
  dst_ip: string;
  dst_port: number;
  protocol: string;
  flow_count: number;
  bytes_out: number;
  last_seen: string;
  src_ips: string[];
}

export async function fetchComplianceSummary(window = "24h"): Promise<ComplianceSummary> {
  return get("/api/v1/compliance/summary", { window });
}

export async function fetchComplianceConnections(params?: {
  window?: string;
  src_ip?: string;
  dst_ip?: string;
  protocol?: string;
  external_only?: string;
  limit?: number;
}): Promise<{ connections: ConnectionRecord[]; window: string; total: number }> {
  return get("/api/v1/compliance/connections", params as Record<string, string | number>);
}

export async function fetchTLSAudit(): Promise<{ certs: TLSAuditRecord[]; total: number }> {
  return get("/api/v1/compliance/tls");
}

export async function fetchTopTalkers(window = "24h"): Promise<{ talkers: TopTalker[]; window: string }> {
  return get("/api/v1/compliance/top-talkers", { window });
}

export async function fetchExternalConnections(window = "24h"): Promise<{ destinations: ExternalDest[]; window: string }> {
  return get("/api/v1/compliance/external", { window });
}

// ── Geo enrichment ─────────────────────────────────────────────────────────────

export interface GeoCountry {
  code: string;
  name: string;
  connections: number;
  bytes_out: number;
  unique_sources: number;
  max_threat_score: number;
}

export async function fetchGeoSummary(window = "24h"): Promise<{ countries: GeoCountry[]; window: string; total: number }> {
  return get("/api/v1/compliance/geo", { window });
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
