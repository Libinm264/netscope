import { useState } from "react";
import { invoke } from "@tauri-apps/api/core";
import { X, Link2, TestTube, Download, CheckCircle2, XCircle } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { useCaptureStore } from "@/store/captureStore";
import type { HubConfig } from "@/store/captureStore";
import type { FlowDto } from "@/types/flow";

interface Props {
  onClose: () => void;
}

export function HubConnectModal({ onClose }: Props) {
  const { hubConfig, hubConnected, setHubConfig, setHubConnected, setFlows } =
    useCaptureStore();

  const [url, setUrl] = useState(hubConfig?.url ?? "");
  const [token, setToken] = useState(hubConfig?.token ?? "");
  const [protocol, setProtocol] = useState("");
  const [srcIp, setSrcIp] = useState("");
  const [limit, setLimit] = useState("200");

  const [testing, setTesting] = useState(false);
  const [testResult, setTestResult] = useState<"ok" | "fail" | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const handleSave = async () => {
    await invoke("set_hub_config", { url: url.trim(), token: token.trim() });
    setHubConfig(
      url.trim() ? ({ url: url.trim(), token: token.trim() } as HubConfig) : null,
    );
    setHubConnected(false);
    setTestResult(null);
  };

  const handleTest = async () => {
    await handleSave();
    setTesting(true);
    setTestResult(null);
    try {
      await invoke("test_hub_connection");
      setTestResult("ok");
      setHubConnected(true);
    } catch {
      setTestResult("fail");
      setHubConnected(false);
    } finally {
      setTesting(false);
    }
  };

  const handleFetch = async () => {
    setLoading(true);
    setError(null);
    try {
      const flows = await invoke<FlowDto[]>("query_hub_flows", {
        filters: {
          protocol: protocol.trim() || null,
          srcIp: srcIp.trim() || null,
          limit: parseInt(limit, 10) || 200,
        },
      });
      setFlows(flows);
      onClose();
    } catch (e) {
      setError(String(e));
    } finally {
      setLoading(false);
    }
  };

  const handleDisconnect = async () => {
    await invoke("set_hub_config", { url: "", token: "" });
    setHubConfig(null);
    setHubConnected(false);
    setTestResult(null);
    setUrl("");
    setToken("");
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm">
      <div className="w-[480px] rounded-xl border border-white/10 bg-[#0d0d1a] shadow-2xl">
        {/* Header */}
        <div className="flex items-center justify-between border-b border-white/10 px-5 py-4">
          <div className="flex items-center gap-2">
            <Link2 className="h-4 w-4 text-blue-400" />
            <span className="font-semibold text-white">Connect to Hub</span>
          </div>
          <button onClick={onClose} className="text-gray-400 hover:text-white">
            <X className="h-4 w-4" />
          </button>
        </div>

        <div className="p-5 space-y-4">
          {/* Connection fields */}
          <div className="space-y-3">
            <div>
              <label className="mb-1 block text-xs text-gray-400">Hub URL</label>
              <Input
                placeholder="https://hub.example.com"
                value={url}
                onChange={(e) => setUrl(e.target.value)}
                className="font-mono text-xs"
              />
            </div>
            <div>
              <label className="mb-1 block text-xs text-gray-400">API Token</label>
              <Input
                type="password"
                placeholder="ns_••••••••"
                value={token}
                onChange={(e) => setToken(e.target.value)}
                className="font-mono text-xs"
              />
            </div>
          </div>

          {/* Test connection */}
          <div className="flex items-center gap-3">
            <Button
              size="sm"
              variant="outline"
              className="gap-1.5 text-xs"
              onClick={handleTest}
              disabled={testing || !url}
            >
              <TestTube className="h-3.5 w-3.5" />
              {testing ? "Testing…" : "Test connection"}
            </Button>

            {testResult === "ok" && (
              <span className="flex items-center gap-1 text-xs text-emerald-400">
                <CheckCircle2 className="h-3.5 w-3.5" /> Connected
              </span>
            )}
            {testResult === "fail" && (
              <span className="flex items-center gap-1 text-xs text-red-400">
                <XCircle className="h-3.5 w-3.5" /> Connection failed
              </span>
            )}
          </div>

          {/* Query filters */}
          {(hubConnected || testResult === "ok") && (
            <div className="space-y-3 rounded-lg border border-white/10 bg-white/5 p-4">
              <p className="text-xs font-semibold text-gray-300">Query filters</p>
              <div className="grid grid-cols-2 gap-3">
                <div>
                  <label className="mb-1 block text-xs text-gray-400">Protocol</label>
                  <Input
                    placeholder="HTTP, DNS, TLS…"
                    value={protocol}
                    onChange={(e) => setProtocol(e.target.value)}
                    className="text-xs"
                  />
                </div>
                <div>
                  <label className="mb-1 block text-xs text-gray-400">Source IP</label>
                  <Input
                    placeholder="192.168.1.1"
                    value={srcIp}
                    onChange={(e) => setSrcIp(e.target.value)}
                    className="text-xs font-mono"
                  />
                </div>
              </div>
              <div className="w-32">
                <label className="mb-1 block text-xs text-gray-400">Limit</label>
                <Input
                  type="number"
                  value={limit}
                  onChange={(e) => setLimit(e.target.value)}
                  className="text-xs"
                  min={1}
                  max={5000}
                />
              </div>
              {error && (
                <p className="text-xs text-red-400">{error}</p>
              )}
              <Button
                size="sm"
                variant="success"
                className="w-full gap-1.5 text-xs"
                onClick={handleFetch}
                disabled={loading}
              >
                <Download className="h-3.5 w-3.5" />
                {loading ? "Fetching flows…" : "Load hub flows"}
              </Button>
            </div>
          )}
        </div>

        {/* Footer */}
        <div className="flex items-center justify-between border-t border-white/10 px-5 py-3">
          {hubConfig ? (
            <button
              className="text-xs text-red-400 hover:text-red-300"
              onClick={handleDisconnect}
            >
              Disconnect
            </button>
          ) : (
            <span />
          )}
          <Button size="sm" variant="ghost" onClick={onClose} className="text-xs">
            Close
          </Button>
        </div>
      </div>
    </div>
  );
}
