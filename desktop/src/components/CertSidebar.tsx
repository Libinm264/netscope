import { X, Shield, ShieldX, ShieldAlert, ShieldCheck } from "lucide-react";
import { useCaptureStore } from "@/store/captureStore";
import { cn } from "@/lib/utils";
import type { CertEntry } from "@/store/captureStore";

interface Props {
  onClose: () => void;
}

function daysUntil(dateStr?: string): number | null {
  if (!dateStr) return null;
  return Math.floor((new Date(dateStr).getTime() - Date.now()) / 86_400_000);
}

function CertCard({ cert }: { cert: CertEntry }) {
  const days = daysUntil(cert.expiry);

  const status: "expired" | "critical" | "warning" | "valid" = cert.expired
    ? "expired"
    : days !== null && days < 7
    ? "critical"
    : days !== null && days < 30
    ? "warning"
    : "valid";

  const statusConfig = {
    expired: {
      icon: ShieldX,
      cls: "text-red-400",
      bg: "border-red-700/30 bg-red-950/20",
      label: "EXPIRED",
      labelCls: "bg-red-900/50 text-red-400",
    },
    critical: {
      icon: ShieldAlert,
      cls: "text-orange-400",
      bg: "border-orange-700/30 bg-orange-950/10",
      label: `${days}d`,
      labelCls: "bg-orange-900/50 text-orange-400",
    },
    warning: {
      icon: ShieldAlert,
      cls: "text-amber-400",
      bg: "border-amber-700/20 bg-amber-950/10",
      label: `${days}d`,
      labelCls: "bg-amber-900/40 text-amber-400",
    },
    valid: {
      icon: ShieldCheck,
      cls: "text-emerald-400",
      bg: "border-white/5 bg-white/[0.02]",
      label: days !== null ? `${days}d` : "valid",
      labelCls: "bg-emerald-900/30 text-emerald-400",
    },
  }[status];

  const Icon = statusConfig.icon;

  return (
    <div
      className={cn(
        "rounded-lg border p-3 mb-2 text-xs space-y-1.5",
        statusConfig.bg,
      )}
    >
      <div className="flex items-start justify-between gap-2">
        <div className="flex items-center gap-1.5 min-w-0">
          <Icon className={cn("h-3.5 w-3.5 shrink-0", statusConfig.cls)} />
          <span className="font-semibold text-white truncate">{cert.cn}</span>
        </div>
        <span
          className={cn(
            "shrink-0 rounded px-1.5 py-0.5 text-[9px] font-bold",
            statusConfig.labelCls,
          )}
        >
          {statusConfig.label}
        </span>
      </div>

      {cert.issuer && (
        <div className="text-gray-400 truncate">
          <span className="text-gray-500">Issuer: </span>
          {cert.issuer}
        </div>
      )}

      {cert.expiry && (
        <div className="text-gray-400">
          <span className="text-gray-500">Expires: </span>
          {cert.expiry}
        </div>
      )}

      {cert.sans.length > 0 && (
        <div className="text-gray-500 truncate">
          {cert.sans.slice(0, 3).join(", ")}
          {cert.sans.length > 3 && ` +${cert.sans.length - 3} more`}
        </div>
      )}

      <div className="flex items-center gap-3 text-gray-500 pt-0.5 border-t border-white/5">
        <span>{cert.dstIp}</span>
        <span className="text-white/20">·</span>
        <span>{cert.seenCount}×</span>
        <span className="text-white/20">·</span>
        <span>last {cert.lastSeen}</span>
      </div>
    </div>
  );
}

export function CertSidebar({ onClose }: Props) {
  const { certInventory } = useCaptureStore();

  const expired = certInventory.filter((c) => c.expired);
  const critical = certInventory.filter(
    (c) => !c.expired && (daysUntil(c.expiry) ?? 999) < 7,
  );
  const warning = certInventory.filter(
    (c) =>
      !c.expired &&
      (daysUntil(c.expiry) ?? 999) >= 7 &&
      (daysUntil(c.expiry) ?? 999) < 30,
  );
  const valid = certInventory.filter(
    (c) => !c.expired && (daysUntil(c.expiry) ?? 999) >= 30,
  );

  return (
    <div className="flex h-full w-72 shrink-0 flex-col border-l border-white/10 bg-[#0a0a14]">
      {/* Header */}
      <div className="flex shrink-0 items-center justify-between border-b border-white/10 px-3 py-2">
        <div className="flex items-center gap-2">
          <Shield className="h-3.5 w-3.5 text-indigo-400" />
          <span className="text-xs font-semibold text-white">
            TLS Certificates
          </span>
          <span className="rounded bg-white/10 px-1.5 py-0.5 text-[10px] text-gray-400">
            {certInventory.length}
          </span>
        </div>
        <button onClick={onClose} className="text-gray-500 hover:text-white">
          <X className="h-3.5 w-3.5" />
        </button>
      </div>

      {/* Summary row */}
      {certInventory.length > 0 && (
        <div className="flex shrink-0 gap-2 border-b border-white/10 px-3 py-2 text-[11px]">
          {expired.length > 0 && (
            <span className="flex items-center gap-1 text-red-400">
              <ShieldX className="h-3 w-3" /> {expired.length} expired
            </span>
          )}
          {critical.length > 0 && (
            <span className="flex items-center gap-1 text-orange-400">
              <ShieldAlert className="h-3 w-3" /> {critical.length} critical
            </span>
          )}
          {warning.length > 0 && (
            <span className="flex items-center gap-1 text-amber-400">
              <ShieldAlert className="h-3 w-3" /> {warning.length} expiring
            </span>
          )}
          {valid.length > 0 && (
            <span className="flex items-center gap-1 text-emerald-400">
              <ShieldCheck className="h-3 w-3" /> {valid.length} valid
            </span>
          )}
        </div>
      )}

      {/* Cert list */}
      <div className="flex-1 overflow-y-auto p-3">
        {certInventory.length === 0 ? (
          <div className="flex flex-col items-center justify-center h-full gap-2 text-center text-xs text-gray-500">
            <Shield className="h-8 w-8 text-gray-700" />
            <p>No TLS certificates seen yet.</p>
            <p className="text-gray-600">They appear as you capture TLS handshakes.</p>
          </div>
        ) : (
          <>
            {[...expired, ...critical, ...warning, ...valid].map((cert) => (
              <CertCard key={cert.cn} cert={cert} />
            ))}
          </>
        )}
      </div>
    </div>
  );
}
