import { create } from "zustand";
import type { FlowDto, InterfaceDto, CaptureStatus } from "@/types/flow";

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
}

function applyFilter(flows: FlowDto[], filter: string): FlowDto[] {
  if (!filter.trim()) return flows;
  const lower = filter.toLowerCase();
  return flows.filter((f) => {
    if (lower === "http") return f.protocol.toLowerCase().startsWith("http");
    if (lower === "dns") return f.protocol.toLowerCase() === "dns";
    if (lower === "errors") return (f.http?.statusCode ?? 0) >= 400;
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

  addFlow: (flow) =>
    set((state) => {
      const flows = [...state.flows, flow];
      const filteredFlows = state.filter
        ? applyFilter(flows, state.filter)
        : flows;
      return { flows, filteredFlows };
    }),

  setFlows: (flows) =>
    set((state) => ({
      flows,
      filteredFlows: applyFilter(flows, state.filter),
    })),

  setFilter: (filter) =>
    set((state) => ({
      filter,
      filteredFlows: applyFilter(state.flows, filter),
    })),

  setSelectedFlow: (flow) => set({ selectedFlow: flow }),
  setStatus: (status) => set({ status }),
  setInterface: (iface) => set({ interface_: iface }),
  setInterfaces: (interfaces) => set({ interfaces }),
  clearFlows: () => set({ flows: [], filteredFlows: [], selectedFlow: null }),
  setSessionPath: (path) => set({ sessionPath: path }),
  setSessionName: (name) => set({ sessionName: name }),
  setPrivilegeGranted: (granted) => set({ privilegeGranted: granted }),
}));
