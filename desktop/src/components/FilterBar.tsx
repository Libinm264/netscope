import { useState, useRef, useEffect } from "react";
import { Search, ChevronDown, X, Check } from "lucide-react";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { useCaptureStore } from "@/store/captureStore";

const QUICK_FILTERS = [
  { label: "All traffic",      value: "" },
  { label: "HTTP only",        value: "http" },
  { label: "DNS only",         value: "dns" },
  { label: "TLS only",         value: "tls" },
  { label: "Errors (4xx/5xx)", value: "errors" },
  { label: "Threats",          value: "threats" },
  { label: "Hub flows",        value: "hub" },
];

export function FilterBar() {
  const { filter, setFilter } = useCaptureStore();
  const [showQuick, setShowQuick] = useState(false);
  const dropdownRef = useRef<HTMLDivElement>(null);

  // Close dropdown on outside click
  useEffect(() => {
    if (!showQuick) return;
    function handleClick(e: MouseEvent) {
      if (dropdownRef.current && !dropdownRef.current.contains(e.target as Node)) {
        setShowQuick(false);
      }
    }
    document.addEventListener("mousedown", handleClick);
    return () => document.removeEventListener("mousedown", handleClick);
  }, [showQuick]);

  // Which quick filter is currently active (exact match only)
  const activeQuick = QUICK_FILTERS.find((f) => f.value === filter);
  const isQuickActive = activeQuick !== undefined && activeQuick.value !== "";

  return (
    <div className="relative flex items-center gap-1 flex-1">
      <div className="relative flex-1">
        <Search className="absolute left-2.5 top-1/2 -translate-y-1/2 h-3.5 w-3.5 text-gray-400 pointer-events-none" />
        <Input
          className="pl-8 pr-8 font-mono text-xs"
          placeholder="Filter: 192.168.1.1   http   dns   threats"
          value={filter}
          onChange={(e) => setFilter(e.target.value)}
        />
        {filter && (
          <button
            className="absolute right-2 top-1/2 -translate-y-1/2 text-gray-400 hover:text-white"
            onClick={() => setFilter("")}
          >
            <X className="h-3.5 w-3.5" />
          </button>
        )}
      </div>

      <div className="relative" ref={dropdownRef}>
        <Button
          variant="outline"
          size="sm"
          className={`gap-1 text-xs max-w-[140px] ${
            isQuickActive ? "border-blue-500/60 text-blue-400" : ""
          }`}
          onClick={() => setShowQuick((v) => !v)}
        >
          <span className="truncate">
            {activeQuick ? activeQuick.label : "Quick"}
          </span>
          <ChevronDown className="h-3 w-3 shrink-0" />
        </Button>

        {showQuick && (
          <div className="absolute right-0 top-full z-50 mt-1 w-48 rounded-md border border-white/20 bg-[#16213e] py-1 shadow-xl">
            {QUICK_FILTERS.map((f) => {
              const isActive = f.value === filter;
              return (
                <button
                  key={f.value}
                  className={`w-full px-3 py-1.5 text-left text-xs flex items-center justify-between gap-2 ${
                    isActive
                      ? "bg-blue-500/20 text-blue-300"
                      : "text-gray-300 hover:bg-white/10 hover:text-white"
                  }`}
                  onClick={() => {
                    setFilter(f.value);
                    setShowQuick(false);
                  }}
                >
                  <span>{f.label}</span>
                  {isActive && <Check className="h-3 w-3 text-blue-400 shrink-0" />}
                </button>
              );
            })}
          </div>
        )}
      </div>
    </div>
  );
}
