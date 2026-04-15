import { ShieldAlert } from "lucide-react";
import { Button } from "@/components/ui/button";

interface PrivilegeModalProps {
  interface_: string;
  onDismiss: () => void;
}

export function PrivilegeModal({ interface_, onDismiss }: PrivilegeModalProps) {
  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/70 backdrop-blur-sm">
      <div className="w-full max-w-lg rounded-xl border border-red-500/30 bg-[#16213e] p-6 shadow-2xl">
        <div className="flex items-start gap-4">
          <div className="rounded-full bg-red-500/20 p-2.5">
            <ShieldAlert className="h-6 w-6 text-red-400" />
          </div>
          <div className="flex-1">
            <h2 className="text-base font-semibold text-white">
              Capture Permission Required
            </h2>
            <p className="mt-1.5 text-sm text-gray-400">
              Packet capture requires elevated privileges. NetScope cannot read
              network traffic without them.
            </p>

            <div className="mt-4 rounded-lg bg-black/40 p-3 font-mono text-xs text-gray-300">
              <p className="mb-1 text-gray-500"># macOS / Linux — run with sudo:</p>
              <p className="text-emerald-400">
                sudo netscope-agent capture --interface {interface_}
              </p>
              <p className="mt-2 text-gray-500"># Linux — grant capability (no sudo required after):</p>
              <p className="text-emerald-400">
                sudo setcap cap_net_raw+eip $(which netscope-agent)
              </p>
            </div>

            <p className="mt-3 text-xs text-gray-500">
              On macOS, the desktop app will request permission via an OS
              authentication dialog automatically on first launch.
            </p>
          </div>
        </div>

        <div className="mt-5 flex justify-end">
          <Button variant="outline" onClick={onDismiss}>
            Got it, I'll fix this
          </Button>
        </div>
      </div>
    </div>
  );
}
