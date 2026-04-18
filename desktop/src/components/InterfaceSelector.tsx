import { useEffect } from "react";
import { invoke } from "@tauri-apps/api/core";
import { useCaptureStore } from "@/store/captureStore";
import type { InterfaceDto } from "@/types/flow";

/**
 * Build a human-readable label for a network interface.
 *
 * macOS/Linux kernel names (en0, eth0, lo0, …) are cryptic; we prefer the
 * description string that comes from the Rust backend when it exists.  When
 * it's absent we fall back to well-known naming conventions so users can tell
 * Wi-Fi from Ethernet from a VPN tunnel at a glance.
 */
function friendlyLabel(iface: InterfaceDto): string {
  const desc = iface.description?.trim();
  const name = iface.name;
  const ip   = iface.addresses.length > 0 ? ` — ${iface.addresses[0]}` : "";

  // Prefer the description the OS provides (e.g. "Wi-Fi", "Ethernet")
  if (desc) return `${name} (${desc})${ip}`;

  // Fallback: infer from the well-known naming conventions
  let hint = "";
  if (/^lo\d*$/.test(name))              hint = "Loopback";
  else if (/^en0$/.test(name))           hint = "Wi-Fi";        // macOS primary wireless
  else if (/^en\d+$/.test(name))         hint = "Ethernet";     // macOS wired / Thunderbolt
  else if (/^eth\d+$/.test(name))        hint = "Ethernet";     // Linux wired
  else if (/^wlan\d+$/.test(name))       hint = "Wi-Fi";        // Linux wireless
  else if (/^wlp/.test(name))            hint = "Wi-Fi";        // Linux predictable names
  else if (/^enp/.test(name))            hint = "Ethernet";     // Linux predictable names
  else if (/^utun\d+$/.test(name))       hint = "VPN tunnel";   // macOS VPN / WireGuard
  else if (/^tun\d+$/.test(name))        hint = "VPN tunnel";   // Linux TUN
  else if (/^tap\d+$/.test(name))        hint = "TAP/VM";
  else if (/^docker/.test(name))         hint = "Docker bridge";
  else if (/^br[-\d]/.test(name))        hint = "Bridge/Docker";
  else if (/^veth/.test(name))           hint = "Virtual Ethernet";
  else if (/^awdl\d+$/.test(name))       hint = "AirDrop (AWDL)";
  else if (/^llw\d+$/.test(name))        hint = "Low-latency WLAN";
  else if (/^gif\d+$/.test(name))        hint = "IPv6-in-IPv4 tunnel";
  else if (/^stf\d+$/.test(name))        hint = "6to4 tunnel";

  return hint ? `${name} (${hint})${ip}` : `${name}${ip}`;
}

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
          {friendlyLabel(iface)}
        </option>
      ))}
    </select>
  );
}
