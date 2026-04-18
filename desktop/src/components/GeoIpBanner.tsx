import { useState } from "react";
import { invoke } from "@tauri-apps/api/core";
import { open as openDialog } from "@tauri-apps/plugin-dialog";
import { MapPin, X, FolderOpen, ChevronRight } from "lucide-react";
import { Button } from "@/components/ui/button";
import { useCaptureStore } from "@/store/captureStore";

export function GeoIpBanner() {
  const { geoipAvailable, setGeoipAvailable } = useCaptureStore();
  const [dismissed, setDismissed] = useState(false);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // Don't show if already loaded or user dismissed
  if (geoipAvailable || dismissed) return null;

  const handleAutoLoad = async () => {
    setLoading(true);
    setError(null);
    try {
      await invoke("load_geoip_db", { cityPath: "", asnPath: "" });
      setGeoipAvailable(true);
    } catch (e) {
      setError(String(e));
    } finally {
      setLoading(false);
    }
  };

  const handleBrowse = async () => {
    const city = await openDialog({
      title: "Select GeoLite2-City.mmdb",
      filters: [{ name: "MaxMind DB", extensions: ["mmdb"] }],
    });
    if (!city || Array.isArray(city)) return;

    const asn = await openDialog({
      title: "Select GeoLite2-ASN.mmdb",
      filters: [{ name: "MaxMind DB", extensions: ["mmdb"] }],
    });
    if (!asn || Array.isArray(asn)) return;

    setLoading(true);
    setError(null);
    try {
      await invoke("load_geoip_db", { cityPath: city, asnPath: asn });
      setGeoipAvailable(true);
    } catch (e) {
      setError(String(e));
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="flex items-center gap-3 border-b border-amber-700/30 bg-amber-950/20 px-3 py-2 text-xs text-amber-300">
      <MapPin className="h-3.5 w-3.5 shrink-0 text-amber-400" />
      <span className="flex-1">
        GeoIP databases not found. Place{" "}
        <code className="rounded bg-white/10 px-1">GeoLite2-City.mmdb</code> and{" "}
        <code className="rounded bg-white/10 px-1">GeoLite2-ASN.mmdb</code> in{" "}
        <code className="rounded bg-white/10 px-1">~/.netscope/</code> to see
        country flags &amp; ASN info.{" "}
        <a
          href="https://dev.maxmind.com/geoip/geolite2-free-geolocation-data"
          target="_blank"
          rel="noreferrer"
          className="inline-flex items-center gap-0.5 underline hover:text-amber-200"
        >
          Download free <ChevronRight className="h-2.5 w-2.5" />
        </a>
      </span>

      {error && <span className="text-red-400">{error}</span>}

      <Button
        size="sm"
        variant="ghost"
        className="h-6 px-2 text-amber-300 hover:text-white"
        onClick={handleAutoLoad}
        disabled={loading}
      >
        {loading ? "Loading…" : "Auto-detect"}
      </Button>
      <Button
        size="sm"
        variant="ghost"
        className="h-6 gap-1 px-2 text-amber-300 hover:text-white"
        onClick={handleBrowse}
        disabled={loading}
      >
        <FolderOpen className="h-3 w-3" /> Browse
      </Button>
      <button
        onClick={() => setDismissed(true)}
        className="text-amber-500 hover:text-amber-300"
      >
        <X className="h-3.5 w-3.5" />
      </button>
    </div>
  );
}
