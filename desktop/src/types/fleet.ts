/// Fleet types for the desktop app.
/// Mirrors the JSON shapes returned by the hub's /api/v1/fleet/* endpoints.

export interface ClusterSummary {
  cluster: string;
  totalAgents: number;
  onlineAgents: number;
  versions: string[];
  flowsPerHour: number;
}

export interface AgentInfo {
  agentId: string;
  hostname: string;
  cluster: string;
  version: string;
  mode: string;         // "pcap" | "ebpf"
  ebpfEnabled: boolean;
  lastSeen: string;     // ISO 8601 timestamp
  flowCount1h: number;
}
