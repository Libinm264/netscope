/**
 * UpdateBanner — slim notification bar shown when a new NetScope version is available.
 *
 * Flow:
 *   1. On mount (3 s delay) calls tauri-plugin-updater `check()`.
 *   2. If an update is available, renders a dismissible banner with version + notes.
 *   3. On "Update & Restart": streams download progress, then installs and relaunches.
 *   4. Any network / signing error is surfaced inline; banner stays visible so the
 *      user can retry or dismiss.
 */

import { useState, useEffect } from "react";
import { check, type Update } from "@tauri-apps/plugin-updater";
import { relaunch } from "@tauri-apps/plugin-process";
import { Download, X, RefreshCw, AlertTriangle } from "lucide-react";
import { cn } from "@/lib/utils";

interface UpdateState {
  update: Update;
  version: string;
  notes: string;
}

type Phase = "idle" | "downloading" | "error";

const CHECK_DELAY_MS = 3_000;   // Wait for app to fully load before checking
const MAX_NOTES_LEN  = 180;     // Truncate long changelogs in the banner

export function UpdateBanner() {
  const [info,      setInfo]      = useState<UpdateState | null>(null);
  const [dismissed, setDismissed] = useState(false);
  const [phase,     setPhase]     = useState<Phase>("idle");
  const [progress,  setProgress]  = useState(0);          // 0–100
  const [errorMsg,  setErrorMsg]  = useState<string>("");

  // ── Check for update once on mount ───────────────────────────────────────────
  useEffect(() => {
    const timer = setTimeout(async () => {
      try {
        const result = await check();
        if (result?.available) {
          setInfo({
            update:  result,
            version: result.version,
            notes:   result.body?.trim() ?? "",
          });
        }
      } catch {
        // Silently swallow — no internet, dev build, etc.
      }
    }, CHECK_DELAY_MS);

    return () => clearTimeout(timer);
  }, []);

  // ── Install handler ───────────────────────────────────────────────────────────
  const handleInstall = async () => {
    if (!info || phase === "downloading") return;

    setPhase("downloading");
    setProgress(0);
    setErrorMsg("");

    let downloaded  = 0;
    let totalBytes  = 0;

    try {
      await info.update.downloadAndInstall((event) => {
        switch (event.event) {
          case "Started":
            totalBytes = event.data.contentLength ?? 0;
            break;
          case "Progress":
            downloaded += event.data.chunkLength;
            if (totalBytes > 0) {
              setProgress(Math.round((downloaded / totalBytes) * 100));
            }
            break;
          case "Finished":
            setProgress(100);
            break;
        }
      });
      await relaunch();
    } catch (err) {
      setPhase("error");
      setErrorMsg(String(err).replace(/^Error: /, ""));
    }
  };

  if (!info || dismissed) return null;

  const shortNotes =
    info.notes.length > MAX_NOTES_LEN
      ? info.notes.slice(0, MAX_NOTES_LEN).trimEnd() + "…"
      : info.notes;

  return (
    <div
      className={cn(
        "flex shrink-0 items-center gap-3 px-3 py-2",
        "border-b border-blue-500/30 bg-blue-950/40 text-sm",
        phase === "error" && "border-red-500/30 bg-red-950/30",
      )}
    >
      {/* Icon */}
      <div className="flex shrink-0 items-center gap-1.5">
        {phase === "error" ? (
          <AlertTriangle className="h-3.5 w-3.5 text-red-400" />
        ) : phase === "downloading" ? (
          <RefreshCw className="h-3.5 w-3.5 animate-spin text-blue-400" />
        ) : (
          <Download className="h-3.5 w-3.5 text-blue-400" />
        )}
      </div>

      {/* Message */}
      <div className="flex flex-1 items-center gap-2 overflow-hidden">
        {phase === "error" ? (
          <span className="text-red-300">
            Update failed: <span className="text-red-400">{errorMsg}</span>
          </span>
        ) : phase === "downloading" ? (
          <div className="flex flex-1 items-center gap-3">
            <span className="shrink-0 text-blue-300">
              Downloading v{info.version}…
            </span>
            {/* Progress bar */}
            <div className="flex-1 max-w-48 h-1.5 rounded-full bg-white/10 overflow-hidden">
              <div
                className="h-full bg-blue-500 transition-all duration-150"
                style={{ width: `${progress}%` }}
              />
            </div>
            <span className="shrink-0 text-xs text-blue-400/70">{progress}%</span>
          </div>
        ) : (
          <>
            <span className="shrink-0 font-medium text-blue-300">
              NetScope v{info.version} is available
            </span>
            {shortNotes && (
              <>
                <span className="text-white/20">·</span>
                <span className="truncate text-white/50">{shortNotes}</span>
              </>
            )}
          </>
        )}
      </div>

      {/* Actions */}
      <div className="flex shrink-0 items-center gap-2">
        {phase === "error" ? (
          <button
            onClick={() => { setPhase("idle"); setErrorMsg(""); }}
            className="rounded px-2 py-0.5 text-xs text-red-300 hover:bg-red-500/20 transition-colors"
          >
            Retry
          </button>
        ) : phase !== "downloading" ? (
          <button
            onClick={handleInstall}
            className="rounded px-2.5 py-0.5 text-xs font-medium bg-blue-600 hover:bg-blue-500 text-white transition-colors"
          >
            Update &amp; Restart
          </button>
        ) : null}

        {phase !== "downloading" && (
          <button
            onClick={() => setDismissed(true)}
            className="rounded p-0.5 text-white/40 hover:text-white/80 hover:bg-white/10 transition-colors"
            title="Dismiss"
          >
            <X className="h-3.5 w-3.5" />
          </button>
        )}
      </div>
    </div>
  );
}
