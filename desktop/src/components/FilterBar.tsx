import { useState } from "react";
import { Search, ChevronDown, X } from "lucide-react";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { useCaptureStore } from "@/store/captureStore";

const QUICK_FILTERS = [
  { label: "All traffic", value: "" },
  { label: "HTTP only", value: "http" },
  { label: "DNS only", value: "dns" },
  { label: "Errors (4xx/5xx)", value: "errors" },
];

export function FilterBar() {
  const { filter, setFilter } = useCaptureStore();
  const [showQuick, setShowQuick] = useState(false);

  return (
    <div className="relative flex items-center gap-1 flex-1">
      <div className="relative flex-1">
        <Search className="absolute left-2.5 top-1/2 -translate-y-1/2 h-3.5 w-3.5 text-gray-400 pointer-events-none" />
        <Input
          className="pl-8 pr-8 font-mono text-xs"
          placeholder="Filter: host 192.168.1.1   or   http   or   dns"
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

      {/* Quick filter dropdown */}
      <div className="relative">
        <Button
          variant="outline"
          size="sm"
          className="gap-1 text-xs"
          onClick={() => setShowQuick((v) => !v)}
        >
          Quick <ChevronDown className="h-3 w-3" />
        </Button>
        {showQuick && (
          <div className="absolute right-0 top-full z-50 mt-1 w-44 rounded-md border border-white/20 bg-[#16213e] py-1 shadow-xl">
            {QUICK_FILTERS.map((f) => (
              <button
                key={f.value}
                className="w-full px-3 py-1.5 text-left text-xs text-gray-300 hover:bg-white/10 hover:text-white"
                onClick={() => {
                  setFilter(f.value);
                  setShowQuick(false);
                }}
              >
                {f.label}
              </button>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}
