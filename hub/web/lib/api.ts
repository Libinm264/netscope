// ── Types ─────────────────────────────────────────────────────────────────────

export interface TimeseriesPoint {
  ts: string;
  count: number;
  bytes_in: number;
  bytes_out: number;
}

export interface ProtocolCount {
  protocol: string;
  count: number;
}

export interface AuditEvent {
  id: string;
  token_id: string;
  role: string;
  method: string;
  path: string;
  status: number;
  client_ip: string;
  latency_ms: number;
  ts: string;
}

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
  /** eBPF process attribution — only present for flows captured in eBPF mode */
  process_name?: string;
  pid?: number;
  /** Kubernetes pod name — populated when agent runs inside a K8s pod */
  pod_name?: string;
  /** Kubernetes namespace — populated when agent runs inside a K8s pod */
  k8s_namespace?: string;
  /** Threat intelligence score 0–100 from AbuseIPDB / blocklists (0 = clean/unscored) */
  threat_score?: number;
  /** Threat level: "high" | "medium" | "low" | "" */
  threat_level?: string;
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
  os?: string;
  capture_mode?: string;    // "pcap" | "ebpf"
  ebpf_enabled?: boolean;
  flow_count_1h?: number;
}

// ── Proxy client ──────────────────────────────────────────────────────────────
//
// All requests go to the Next.js server-side proxy at /api/proxy/*, which
// injects the hub API key from a server-only environment variable.
//
// The API key is NEVER present in the browser bundle.  Do NOT add
// NEXT_PUBLIC_API_KEY or NEXT_PUBLIC_API_URL — use HUB_API_KEY and HUB_API_URL
// in your server environment instead (see hub/web/.env.example).

const PROXY = "/api/proxy";

function stdHeaders(): HeadersInit {
  return { "Content-Type": "application/json" };
}

async function get<T>(path: string, params?: Record<string, string | number>): Promise<T> {
  const url = new URL(`${PROXY}${path}`, typeof window !== "undefined" ? window.location.origin : "http://localhost:3000");
  if (params) {
    Object.entries(params).forEach(([k, v]) => {
      if (v !== undefined && v !== "") url.searchParams.set(k, String(v));
    });
  }
  const res = await fetch(url.toString(), { headers: stdHeaders(), cache: "no-store" });
  if (!res.ok) {
    const body = await res.text().catch(() => "");
    throw new Error(`API ${path} → ${res.status}: ${body}`);
  }
  return res.json() as Promise<T>;
}

async function api<T>(
  method: string,
  path: string,
  body?: unknown,
): Promise<T> {
  const res = await fetch(`${PROXY}${path}`, {
    method,
    headers: stdHeaders(),
    cache: "no-store",
    body: body !== undefined ? JSON.stringify(body) : undefined,
  });
  if (!res.ok) {
    const text = await res.text().catch(() => "");
    throw new Error(`API ${method} ${path} → ${res.status}: ${text}`);
  }
  return res.json() as Promise<T>;
}

// ── API functions ─────────────────────────────────────────────────────────────

export async function fetchFlows(params?: {
  protocol?: string;
  src_ip?: string;
  dst_ip?: string;
  hostname?: string;
  from?: string;
  to?: string;
  limit?: number;
  offset?: number;
}): Promise<{ flows: Flow[]; total: number }> {
  return get("/flows", params as Record<string, string | number>);
}

export async function fetchStats(): Promise<StatsResponse> {
  return get("/stats");
}

export async function fetchAgents(): Promise<{ agents: Agent[] }> {
  return get("/agents");
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
  | "teams"
  | "email";

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
  enabled: number;
  cooldown_minutes: number;
  created_at: string;
  webhook_secret?: string;
  email_to?: string;
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
  email_to?: string;
}

export async function fetchAlertRules(): Promise<{ rules: AlertRule[] }> {
  return get("/alerts");
}

export async function createAlertRule(req: CreateAlertRuleRequest): Promise<AlertRule> {
  return api<AlertRule>("POST", "/alerts", req);
}

export async function updateAlertRule(
  id: string,
  patch: Partial<Pick<AlertRule, "enabled" | "webhook_url">>,
): Promise<AlertRule> {
  return api<AlertRule>("PATCH", `/alerts/${id}`, patch);
}

export async function deleteAlertRule(id: string): Promise<void> {
  await api<unknown>("DELETE", `/alerts/${id}`);
}

export async function fetchAlertEvents(): Promise<{ events: AlertEvent[] }> {
  return get("/alerts/events");
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
  return get("/enrollment-tokens");
}

export async function createEnrollmentToken(name: string, expires_in = "7d"): Promise<EnrollmentToken> {
  return api<EnrollmentToken>("POST", "/enrollment-tokens", { name, expires_in });
}

export async function revokeEnrollmentToken(id: string): Promise<void> {
  await api<unknown>("DELETE", `/enrollment-tokens/${id}`);
}

// ── TLS certificate fleet ─────────────────────────────────────────────────────

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
  return get("/certs");
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
  return get("/tokens");
}

export async function createAPIToken(name: string, role: "admin" | "viewer"): Promise<APIToken> {
  return api<APIToken>("POST", "/tokens", { name, role });
}

export async function revokeAPIToken(id: string): Promise<void> {
  await api<unknown>("DELETE", `/tokens/${id}`);
}

// ── Service dependency graph ──────────────────────────────────────────────────

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
  return get("/services/graph", { window });
}

// ── HTTP endpoint analytics ───────────────────────────────────────────────────

export interface EndpointStat {
  method: string;
  path: string;
  count: number;
  success_count: number;
  error_count: number;
  error_rate: number;
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
  return get("/analytics/endpoints", { window });
}

// ── Compliance reporting ──────────────────────────────────────────────────────

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
  /** eBPF process attribution — only present for flows captured in eBPF mode */
  process_name?: string;
  pid?: number;
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
  return get("/compliance/summary", { window });
}

export async function fetchComplianceConnections(params?: {
  window?: string;
  src_ip?: string;
  dst_ip?: string;
  protocol?: string;
  external_only?: string;
  limit?: number;
}): Promise<{ connections: ConnectionRecord[]; window: string; total: number }> {
  return get("/compliance/connections", params as Record<string, string | number>);
}

export async function fetchTLSAudit(): Promise<{ certs: TLSAuditRecord[]; total: number }> {
  return get("/compliance/tls");
}

export async function fetchTopTalkers(window = "24h"): Promise<{ talkers: TopTalker[]; window: string }> {
  return get("/compliance/top-talkers", { window });
}

export async function fetchExternalConnections(window = "24h"): Promise<{ destinations: ExternalDest[]; window: string }> {
  return get("/compliance/external", { window });
}

// ── Geo enrichment ────────────────────────────────────────────────────────────

export interface GeoCountry {
  code: string;
  name: string;
  connections: number;
  bytes_out: number;
  unique_sources: number;
  max_threat_score: number;
}

export async function fetchGeoSummary(window = "24h"): Promise<{ countries: GeoCountry[]; window: string; total: number }> {
  return get("/compliance/geo", { window });
}

export async function fetchTimeseries(hours = 1): Promise<{ points: TimeseriesPoint[]; hours: number }> {
  return get("/metrics/timeseries", { hours });
}

export async function fetchProtocolBreakdown(hours = 1): Promise<{ protocols: ProtocolCount[]; hours: number }> {
  return get("/metrics/protocols", { hours });
}

export async function fetchAuditEvents(params?: { limit?: number; token?: string; status?: number }): Promise<{ events: AuditEvent[]; count: number }> {
  return get("/audit", params as Record<string, string | number>);
}

// ── Live flow stream (SSE) ────────────────────────────────────────────────────
//
// The EventSource connects to the Next.js proxy at /api/proxy/flows/stream
// (same-origin, no credentials in the URL).  The proxy injects the hub API key
// server-side before forwarding to the backend.

export function createFlowStream(
  onFlow: (flow: Flow) => void,
  onError?: () => void,
  onOpen?: () => void,
): () => void {
  // Same-origin proxy URL — no API key in the browser URL
  const es = new EventSource(`${PROXY}/flows/stream`);

  es.onopen = () => onOpen?.();

  es.onmessage = (e) => {
    try {
      const flow = JSON.parse(e.data) as Flow;
      onFlow(flow);
    } catch {
      // Ignore malformed events (keep-alive comments, etc.)
    }
  };

  es.onerror = () => {
    onError?.();
    es.close();
  };

  return () => es.close();
}

// ── Process policies ──────────────────────────────────────────────────────────

export interface ProcessPolicy {
  id: string;
  name: string;
  process_name: string;
  action: "alert" | "deny";
  dst_ip_cidr: string;
  dst_port: number;
  description: string;
  enabled: boolean;
  created_at: string;
}

export interface PolicyViolation {
  id: string;
  policy_id: string;
  policy_name: string;
  process_name: string;
  pid: number;
  src_ip: string;
  dst_ip: string;
  dst_port: number;
  protocol: string;
  agent_id: string;
  hostname: string;
  violated_at: string;
}

export interface CreatePolicyRequest {
  name: string;
  process_name: string;
  action: "alert" | "deny";
  dst_ip_cidr?: string;
  dst_port?: number;
  description?: string;
}

export async function fetchPolicies(): Promise<{ policies: ProcessPolicy[] }> {
  return get("/policies");
}
export async function createPolicy(req: CreatePolicyRequest): Promise<{ id: string }> {
  return api("POST", "/policies", req);
}
export async function updatePolicy(id: string, patch: { enabled?: boolean; action?: string }): Promise<void> {
  await api("PATCH", `/policies/${id}`, patch);
}
export async function deletePolicy(id: string): Promise<void> {
  await api("DELETE", `/policies/${id}`);
}
export async function fetchPolicyViolations(params?: { window?: string; limit?: number }): Promise<{ violations: PolicyViolation[]; window: string }> {
  return get("/policies/violations", params as Record<string, string | number>);
}

// ── Threat intelligence ───────────────────────────────────────────────────────

export interface ThreatIP {
  dst_ip: string;
  threat_score: number;
  threat_level: string;
  country_code: string;
  country_name: string;
  as_org: string;
  flow_count: number;
  last_seen: string;
  processes: string[];
}

export interface ThreatSummary {
  total: number;
  high: number;
  medium: number;
  low: number;
}

export async function fetchThreats(params?: { window?: string; limit?: number }): Promise<{ threats: ThreatIP[]; summary: ThreatSummary; window: string }> {
  return get("/threats", params as Record<string, string | number>);
}

// ── Alert test delivery ───────────────────────────────────────────────────────

export async function testAlertDelivery(id: string): Promise<{ ok: boolean; message: string }> {
  return api("POST", `/alerts/${id}/test`);
}

// ── Enterprise: Organisation ──────────────────────────────────────────────────

export interface OrgInfo {
  org_id: string;
  name: string;
  slug: string;
  agent_quota: number;
  retention_days: number;
  plan: string;
  created_at?: string;
}

export async function fetchOrg(): Promise<OrgInfo> {
  return get("/enterprise/org");
}

export async function updateOrg(body: { name: string; agent_quota: number; retention_days: number }): Promise<{ ok: boolean }> {
  return api("PUT", "/enterprise/org", body);
}

// ── Enterprise: Members ───────────────────────────────────────────────────────

export interface OrgMember {
  user_id: string;
  org_id: string;
  email: string;
  display_name: string;
  role: string;
  sso_provider?: string;
  is_active: boolean;
  created_at: string;
  last_seen?: string;
}

// ── Current-user identity ─────────────────────────────────────────────────────

export interface MeResponse {
  authenticated: boolean;
  user_id:       string;
  email:         string;
  display_name:  string;
  role:          string;
  org_id:        string;
  sso_provider?: string;
  expires_at:    string;
}

/** Returns the identity of the currently authenticated session user. */
export async function fetchMe(): Promise<MeResponse> {
  return get("/enterprise/auth/me");
}

// ── Members ───────────────────────────────────────────────────────────────────

export async function fetchMembers(): Promise<{ members: OrgMember[] }> {
  return get("/enterprise/members");
}

export async function inviteMember(
  body: { email: string; name: string; role: string },
): Promise<{ user_id: string; invite_url?: string }> {
  return api("POST", "/enterprise/members", body);
}

export async function updateMemberRole(userId: string, role: string): Promise<{ ok: boolean }> {
  return api("PATCH", `/enterprise/members/${userId}/role`, { role });
}

export async function removeMember(userId: string): Promise<{ ok: boolean }> {
  return api("DELETE", `/enterprise/members/${userId}`);
}

// ── Enterprise: Teams ─────────────────────────────────────────────────────────

export interface Team {
  team_id: string;
  org_id: string;
  name: string;
  description?: string;
  member_count: number;
  created_at: string;
}

export interface TeamMember {
  team_id: string;
  user_id: string;
  email?: string;
  display_name?: string;
  role?: string;
  added_at: string;
}

export async function fetchTeams(): Promise<{ teams: Team[] }> {
  return get("/enterprise/teams");
}

export async function createTeam(body: { name: string; description: string }): Promise<{ team_id: string }> {
  return api("POST", "/enterprise/teams", body);
}

export async function deleteTeam(id: string): Promise<{ ok: boolean }> {
  return api("DELETE", `/enterprise/teams/${id}`);
}

export async function fetchTeamMembers(teamId: string): Promise<{ members: TeamMember[] }> {
  return get(`/enterprise/teams/${teamId}/members`);
}

export async function addTeamMember(teamId: string, userId: string): Promise<{ ok: boolean }> {
  return api("POST", `/enterprise/teams/${teamId}/members`, { user_id: userId });
}

export async function removeTeamMember(teamId: string, userId: string): Promise<{ ok: boolean }> {
  return api("DELETE", `/enterprise/teams/${teamId}/members/${userId}`);
}

// ── Enterprise: SSO config ────────────────────────────────────────────────────

export interface SSOConfig {
  org_id: string;
  provider?: string;
  enabled?: boolean;
  entity_id?: string;
  sso_url?: string;
  certificate?: string;
  issuer_url?: string;
  client_id?: string;
  updated_at?: string;
}

export async function fetchSSOConfig(): Promise<SSOConfig> {
  return get("/enterprise/sso/config");
}

export async function updateSSOConfig(body: {
  provider: string;
  enabled: boolean;
  entity_id?: string;
  sso_url?: string;
  certificate?: string;
  issuer_url?: string;
  client_id?: string;
  client_secret?: string;
}): Promise<{ ok: boolean }> {
  return api("PUT", "/enterprise/sso/config", body);
}

// ── Enterprise: License ───────────────────────────────────────────────────────

export interface LicenseInfo {
  valid: boolean;
  expired: boolean;
  plan: string;
  plan_badge: string;
  org_id: string;
  org_name: string;
  agent_quota: number;
  features: string[];
  expires_at?: string;
}

export async function fetchLicense(): Promise<LicenseInfo> {
  return get("/enterprise/license");
}

// ── Enterprise: Integrations (SIEM sinks) ────────────────────────────────────

export type SinkType = "splunk" | "elastic" | "datadog" | "loki";

export interface IntegrationConfig {
  type:          SinkType;
  enabled:       boolean;
  config:        Record<string, string>;   // secrets are redacted to "***" by the API
  last_shipped?: string;                   // RFC3339, absent if never shipped
  updated_at?:   string;
}

export interface TestResult {
  ok:          boolean;
  latency_ms?: number;
  error?:      string;
}

export async function fetchIntegrations(): Promise<{ integrations: IntegrationConfig[] }> {
  return get("/enterprise/integrations");
}

export async function upsertIntegration(
  type: SinkType,
  body: { enabled: boolean; config: Record<string, string> },
): Promise<{ ok: boolean }> {
  return api("PUT", `/enterprise/integrations/${type}`, body);
}

export async function deleteIntegration(type: SinkType): Promise<{ ok: boolean }> {
  return api("DELETE", `/enterprise/integrations/${type}`);
}

export async function testIntegration(
  type: SinkType,
  config: Record<string, string>,
): Promise<TestResult> {
  return api("POST", `/enterprise/integrations/${type}/test`, { config });
}

// ── Enterprise: Audit Export ──────────────────────────────────────────────────

/** Builds a download URL for the audit log export endpoint. */
export function auditExportURL(params: {
  format: "json" | "cef" | "leef";
  from:   string;   // RFC3339
  to:     string;   // RFC3339
  limit?: number;
}): string {
  const base = "/api/proxy/enterprise/audit/export";
  const qs = new URLSearchParams({
    format: params.format,
    from:   params.from,
    to:     params.to,
  });
  if (params.limit) qs.set("limit", String(params.limit));
  return `${base}?${qs.toString()}`;
}
