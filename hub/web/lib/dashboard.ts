// Dashboard builder — types and API helpers.

export type WidgetSize = "sm" | "md" | "lg"; // 1/3 | 1/2 | full of 12-col grid
export type WidgetType =
  | "stat"
  | "timeseries"
  | "protocol_pie"
  | "top_talkers"
  | "alert_feed"
  | "anomaly_feed";

// ── Per-widget configuration payloads ────────────────────────────────────────

export interface StatConfig {
  metric:
    | "total_flows"
    | "active_agents"
    | "total_bytes"
    | "anomaly_count"
    | "alert_count";
}

export interface TimeseriesConfig {
  window: "1h" | "6h" | "24h";
}

export interface ProtocolPieConfig {
  // intentionally empty — uses the standard protocol-breakdown endpoint
}

export interface TopTalkersConfig {
  window: "1h" | "6h" | "24h" | "7d";
  by: "flows" | "bytes";
  limit: number;
}

export interface AlertFeedConfig {
  limit: number;
}

export interface AnomalyFeedConfig {
  limit: number;
  severity?: "high" | "medium" | "low" | "all";
}

export type WidgetConfig =
  | StatConfig
  | TimeseriesConfig
  | ProtocolPieConfig
  | TopTalkersConfig
  | AlertFeedConfig
  | AnomalyFeedConfig;

// ── Widget ────────────────────────────────────────────────────────────────────

export interface Widget {
  id: string;
  type: WidgetType;
  title: string;
  size: WidgetSize; // sm=col-span-4, md=col-span-6, lg=col-span-12
  config: WidgetConfig;
}

// ── Dashboard ─────────────────────────────────────────────────────────────────

export interface Dashboard {
  id: string;
  name: string;
  description: string;
  widgets: Widget[];
  created_at: string;
  updated_at: string;
}

// ── Widget catalogue (shown in the Add Widget picker) ────────────────────────

export interface WidgetTemplate {
  type: WidgetType;
  label: string;
  description: string;
  defaultSize: WidgetSize;
  defaultConfig: WidgetConfig;
}

export const WIDGET_CATALOGUE: WidgetTemplate[] = [
  {
    type: "stat",
    label: "Stat Card",
    description: "Single metric number (flows, agents, bytes…)",
    defaultSize: "sm",
    defaultConfig: { metric: "total_flows" } as StatConfig,
  },
  {
    type: "timeseries",
    label: "Flow Rate Chart",
    description: "Flows per minute over a time window",
    defaultSize: "lg",
    defaultConfig: { window: "1h" } as TimeseriesConfig,
  },
  {
    type: "protocol_pie",
    label: "Protocol Breakdown",
    description: "Donut chart of traffic by protocol",
    defaultSize: "md",
    defaultConfig: {} as ProtocolPieConfig,
  },
  {
    type: "top_talkers",
    label: "Top Talkers",
    description: "Top source IPs by flow count or bytes",
    defaultSize: "md",
    defaultConfig: { window: "1h", by: "flows", limit: 10 } as TopTalkersConfig,
  },
  {
    type: "alert_feed",
    label: "Alert Events",
    description: "Recent alert rule triggers",
    defaultSize: "md",
    defaultConfig: { limit: 8 } as AlertFeedConfig,
  },
  {
    type: "anomaly_feed",
    label: "Anomaly Feed",
    description: "Recent anomaly detections",
    defaultSize: "md",
    defaultConfig: { limit: 8, severity: "all" } as AnomalyFeedConfig,
  },
];

// ── API helpers ───────────────────────────────────────────────────────────────

const BASE = "/api/proxy";

export async function listDashboards(): Promise<Dashboard[]> {
  const res = await fetch(`${BASE}/dashboards`);
  if (!res.ok) throw new Error("Failed to load dashboards");
  const data = await res.json();
  return (data.dashboards ?? []) as Dashboard[];
}

export async function getDashboard(id: string): Promise<Dashboard> {
  const res = await fetch(`${BASE}/dashboards/${id}`);
  if (!res.ok) throw new Error("Dashboard not found");
  return res.json();
}

export async function createDashboard(
  name: string,
  description: string,
  widgets: Widget[]
): Promise<Dashboard> {
  const res = await fetch(`${BASE}/dashboards`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ name, description, widgets }),
  });
  if (!res.ok) throw new Error("Failed to create dashboard");
  return res.json();
}

export async function updateDashboard(
  id: string,
  name: string,
  description: string,
  widgets: Widget[]
): Promise<Dashboard> {
  const res = await fetch(`${BASE}/dashboards/${id}`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ name, description, widgets }),
  });
  if (!res.ok) throw new Error("Failed to save dashboard");
  return res.json();
}

export async function deleteDashboard(id: string): Promise<void> {
  const res = await fetch(`${BASE}/dashboards/${id}`, { method: "DELETE" });
  if (!res.ok) throw new Error("Failed to delete dashboard");
}

// ── Widget-specific data fetches ──────────────────────────────────────────────

export interface Talker {
  src_ip: string;
  flow_count: number;
  total_bytes: number;
}

export async function fetchTopTalkers(
  window: string,
  by: string,
  limit: number
): Promise<Talker[]> {
  const res = await fetch(
    `${BASE}/flows/top-talkers?window=${window}&by=${by}&limit=${limit}`
  );
  if (!res.ok) return [];
  const data = await res.json();
  return (data.talkers ?? []) as Talker[];
}

// ── Layout helpers ────────────────────────────────────────────────────────────

export function sizeToColSpan(size: WidgetSize): string {
  switch (size) {
    case "sm":  return "col-span-12 md:col-span-4";
    case "md":  return "col-span-12 md:col-span-6";
    case "lg":  return "col-span-12";
  }
}

export function newWidget(template: WidgetTemplate): Widget {
  return {
    id: crypto.randomUUID(),
    type: template.type,
    title: template.label,
    size: template.defaultSize,
    config: { ...template.defaultConfig },
  };
}
