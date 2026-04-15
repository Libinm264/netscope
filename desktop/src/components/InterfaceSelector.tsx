import { useEffect } from "react";
import { invoke } from "@tauri-apps/api/core";
import { useCaptureStore } from "@/store/captureStore";
import type { InterfaceDto } from "@/types/flow";

export function InterfaceSelector() {
  const { interfaces, interface_, setInterfaces, setInterface } = useCaptureStore();

  useEffect(() => {
    invoke<InterfaceDto[]>("list_interfaces")
      .then(setInterfaces)
      .catch(console.error);
  }, [setInterfaces]);

  return (
    <select
      value={interface_}
      onChange={(e) => setInterface(e.target.value)}
      className="h-8 rounded-md border border-white/20 bg-[#1a1a2e] px-2 text-sm text-white
                 focus:outline-none focus:ring-1 focus:ring-blue-500"
    >
      {interfaces.length === 0 && (
        <option value={interface_}>{interface_}</option>
      )}
      {interfaces.map((iface) => (
        <option key={iface.name} value={iface.name}>
          {iface.name}
          {iface.addresses.length > 0 ? ` — ${iface.addresses[0]}` : ""}
        </option>
      ))}
    </select>
  );
}
