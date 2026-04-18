import { create } from "zustand";
import type { FlowDto, InterfaceDto, CaptureStatus, TlsFlowDto } from "@/types/flow";

// ── Hub config ────────────────────────────────────────────────────────────────

export interface HubConfig {
  url: string;
  token: string;
}

// ── Certificate inventory ─────────────────────────────────────────────────────

export interface CertEntry {
  cn: string;
  issuer?: string;
  expiry?: string;
  expired: boolean;
  sans: string[];
  seenCount: number;
  firstSeen: string;    // timeStr
  lastSeen: string;
  srcIp: string;
  dstIp: string;
}

// ── HTTP endpoint analytics ───────────────────────────────────────────────────

export interface EndpointStats {
  host: string;
  path: string;
  method: string;
  count: number;
  errorCount: number;
  latencies: number[];   // all recorded latency_ms values
  p50: number;
  p95: number;
  p99: number;
  errorRate: number;     // 0–100 %
}

// ── Store interface ───────────────────────────────────────────────────────────

interface CaptureStore {
  flows: FlowDto[];
  filteredFlows: FlowDto[];
  selectedFlow: FlowDto | null;
  filter: string;
  status: CaptureStatus;
  interface_: string;
  interfaces: InterfaceDto[];
  sessionPath: string | null;
  sessionName: string;
  privilegeGranted: boolean;
  geoipAvailable: boolean;
  hubConfig: HubConfig | null;
  hubConnected: boolean;
  certInventory: CertEntry[];
  endpointStats: EndpointStats[];

  addFlow: (flow: FlowDto) => void;
  setFlows: (flows: FlowDto[]) => void;
  setFilter: (filter: string) => void;
  setSelectedFlow: (flow: FlowDto | null) => void;
  setStatus: (status: CaptureStatus) => void;
  setInterface: (iface: string) => void;
  setInterfaces: (ifaces: InterfaceDto[]) => void;
  clearFlows: () => void;
  setSessionPath: (path: string | null) => void;
  setSessionName: (name: string) => void;
  setPrivilegeGranted: (granted: boolean) => void;
  setGeoipAvailable: (available: boolean) => void;
  setHubConfig: (config: HubConfig | null) => void;
  setHubConnected: (connected: boolean) => void;
}

// ── Filter logic ──────────────────────────────────────────────────────────────

function applyFilter(flows: FlowDto[], filter: string): FlowDto[] {
  if (!filter.trim()) return flows;
  const lower = filter.toLowerCase();

  // Named quick filters
  if (lower === "http") return flows.filter((f) => f.protocol.toLowerCase().startsWith("http"));
  if (lower === "dns")  return flows.filter((f) => f.protocol.toLowerCase() === "dns");
  if (lower === "tls")  return flows.filter((f) => f.protocol.toLowerCase() === "tls");
  if (lower === "errors") return flows.filter((f) => (f.http?.statusCode ?? 0) >= 400);
  if (lower === "threats") return flows.filter((f) => f.threat && f.threat.score > 0);
  if (lower === "hub")  return flows.filter((f) => f.source === "hub");

  // General text search
  return flows.filter(
    (f) =>
      f.srcIp.includes(lower) ||
      f.dstIp.includes(lower) ||
      f.protocol.toLowerCase().includes(lower) ||
      f.info.toLowerCase().includes(lower) ||
      String(f.srcPort).includes(lower) ||
      String(f.dstPort).includes(lower) ||
      f.geoSrc?.countryName.toLowerCase().includes(lower) ||
      f.geoDst?.countryName.toLowerCase().includes(lower) ||
      f.geoDst?.asOrg.toLowerCase().includes(lower)
  );
}

// ── Certificate inventory helpers ─────────────────────────────────────────────

function updateCertInventory(
  inventory: CertEntry[],
  flow: FlowDto,
): CertEntry[] {
  const tls = flow.tls as TlsFlowDto | undefined;
  if (!tls?.certCn) return inventory;

  const existing = inventory.find((c) => c.cn === tls.certCn);
  if (existing) {
    existing.seenCount++;
    existing.lastSeen = flow.timeStr;
    return [...inventory];
  }
  return [
    ...inventory,
    {
      cn: tls.certCn!,
      issuer: tls.certIssuer,
      expiry: tls.certExpiry,
      expired: tls.certExpired,
      sans: tls.certSans,
      seenCount: 1,
      firstSeen: flow.timeStr,
      lastSeen: flow.timeStr,
      srcIp: flow.srcIp,
      dstIp: flow.dstIp,
    },
  ];
}

// ── HTTP endpoint analytics helpers ──────────────────────────────────────────

function percentile(sorted: number[], p: number): number {
  if (sorted.length === 0) return 0;
  const idx = Math.ceil((p / 100) * sorted.length) - 1;
  return sorted[Math.max(0, idx)];
}

function updateEndpointStats(
  stats: EndpointStats[],
  flow: FlowDto,
): EndpointStats[] {
  const http = flow.http;
  if (!http?.method || !http.path) return stats;

  const host = http.host ?? flow.dstIp;
  const existing = stats.find(
    (s) => s.host === host && s.path === http.path && s.method === http.method,
  );

  if (existing) {
    existing.count++;
    if ((http.statusCode ?? 0) >= 400) existing.errorCount++;
    if (http.latencyMs != null) {
      existing.latencies.push(http.latencyMs);
      const sorted = [...existing.latencies].sort((a, b) => a - b);
      existing.p50 = percentile(sorted, 50);
      existing.p95 = percentile(sorted, 95);
      existing.p99 = percentile(sorted, 99);
    }
    existing.errorRate = (existing.errorCount / existing.count) * 100;
    return [...stats];
  }

  const latencies = http.latencyMs != null ? [http.latencyMs] : [];
  const sorted = [...latencies].sort((a, b) => a - b);
  return [
    ...stats,
    {
      host,
      path: http.path,
      method: http.method,
      count: 1,
      errorCount: (http.statusCode ?? 0) >= 400 ? 1 : 0,
      latencies,
      p50: percentile(sorted, 50),
      p95: percentile(sorted, 95),
      p99: percentile(sorted, 99),
      errorRate: (http.statusCode ?? 0) >= 400 ? 100 : 0,
    },
  ];
}

// ── Store ─────────────────────────────────────────────────────────────────────

export const useCaptureStore = create<CaptureStore>((set) => ({
  flows: [],
  filteredFlows: [],
  selectedFlow: null,
  filter: "",
  status: "idle",
  interface_: "en0",
  interfaces: [],
  sessionPath: null,
  sessionName: "New Session",
  privilegeGranted: true,
  geoipAvailable: false,
  hubConfig: null,
  hubConnected: false,
  certInventory: [],
  endpointStats: [],

  addFlow: (flow) =>
    set((state) => {
      const flows = [...state.flows, flow];
      const filteredFlows = state.filter ? applyFilter(flows, state.filter) : flows;
      const certInventory = updateCertInventory(state.certInventory, flow);
      const endpointStats = updateEndpointStats(state.endpointStats, flow);
      return { flows, filteredFlows, certInventory, endpointStats };
    }),

  setFlows: (flows) =>
    set((state) => {
      // Rebuild derived state from scratch
      const certInventory = flows.reduce(
        (inv, f) => updateCertInventory(inv, f),
        [] as CertEntry[],
      );
      const endpointStats = flows.reduce(
        (stats, f) => updateEndpointStats(stats, f),
        [] as EndpointStats[],
      );
      return {
        flows,
        filteredFlows: applyFilter(flows, state.filter),
        certInventory,
        endpointStats,
      };
    }),

  setFilter: (filter) =>
    set((state) => ({
      filter,
      filteredFlows: applyFilter(state.flows, filter),
    })),

  setSelectedFlow: (flow) => set({ selectedFlow: flow }),
  setStatus: (status) => set({ status }),
  setInterface: (iface) => set({ interface_: iface }),
  setInterfaces: (interfaces) => set({ interfaces }),
  clearFlows: () =>
    set({ flows: [], filteredFlows: [], selectedFlow: null, certInventory: [], endpointStats: [] }),
  setSessionPath: (path) => set({ sessionPath: path }),
  setSessionName: (name) => set({ sessionName: name }),
  setPrivilegeGranted: (granted) => set({ privilegeGranted: granted }),
  setGeoipAvailable: (available) => set({ geoipAvailable: available }),
  setHubConfig: (config) => set({ hubConfig: config }),
  setHubConnected: (connected) => set({ hubConnected: connected }),
}));
