"use client";

import { useState } from "react";
import {
  Plus, Pencil, Check, X, Trash2, ChevronLeft, ChevronRight,
  Maximize2, Minimize2,
} from "lucide-react";
import type { Widget, WidgetSize, WidgetType } from "@/lib/dashboard";
import {
  WIDGET_CATALOGUE, sizeToColSpan, newWidget,
} from "@/lib/dashboard";
import { StatWidget }        from "@/components/widgets/StatWidget";
import { TimeseriesWidget }  from "@/components/widgets/TimeseriesWidget";
import { ProtocolPieWidget } from "@/components/widgets/ProtocolPieWidget";
import { TopTalkersWidget }  from "@/components/widgets/TopTalkersWidget";
import { AlertFeedWidget }   from "@/components/widgets/AlertFeedWidget";
import { AnomalyFeedWidget } from "@/components/widgets/AnomalyFeedWidget";

// ── Widget heights by type ────────────────────────────────────────────────────
const WIDGET_H: Record<WidgetType, string> = {
  stat:          "h-32",
  timeseries:    "h-64",
  protocol_pie:  "h-64",
  top_talkers:   "h-64",
  alert_feed:    "h-72",
  anomaly_feed:  "h-72",
};

// ── Renderer ──────────────────────────────────────────────────────────────────
function WidgetRenderer({ widget }: { widget: Widget }) {
  switch (widget.type) {
    case "stat":
      return <StatWidget title={widget.title} config={widget.config as any} />;
    case "timeseries":
      return <TimeseriesWidget title={widget.title} config={widget.config as any} />;
    case "protocol_pie":
      return <ProtocolPieWidget title={widget.title} config={widget.config as any} />;
    case "top_talkers":
      return <TopTalkersWidget title={widget.title} config={widget.config as any} />;
    case "alert_feed":
      return <AlertFeedWidget title={widget.title} config={widget.config as any} />;
    case "anomaly_feed":
      return <AnomalyFeedWidget title={widget.title} config={widget.config as any} />;
  }
}

// ── Widget config form ────────────────────────────────────────────────────────
function ConfigForm({
  widget,
  onChange,
}: {
  widget: Widget;
  onChange: (updated: Widget) => void;
}) {
  const set = (key: string, value: unknown) =>
    onChange({ ...widget, config: { ...widget.config, [key]: value } } as Widget);

  return (
    <div className="space-y-3 text-xs">
      {/* title */}
      <label className="block">
        <span className="text-slate-400">Title</span>
        <input
          className="mt-1 w-full rounded-lg bg-white/[0.06] border border-white/[0.08]
                     px-3 py-1.5 text-slate-200 outline-none focus:border-indigo-500
                     placeholder:text-slate-600"
          value={widget.title}
          onChange={(e) => onChange({ ...widget, title: e.target.value })}
        />
      </label>

      {/* size */}
      <label className="block">
        <span className="text-slate-400">Width</span>
        <select
          className="mt-1 w-full rounded-lg bg-[#0d0d1a] border border-white/[0.08]
                     px-3 py-1.5 text-slate-200 outline-none focus:border-indigo-500"
          value={widget.size}
          onChange={(e) => onChange({ ...widget, size: e.target.value as WidgetSize })}
        >
          <option value="sm">Small (1/3)</option>
          <option value="md">Medium (1/2)</option>
          <option value="lg">Full width</option>
        </select>
      </label>

      {/* type-specific config */}
      {widget.type === "stat" && (
        <label className="block">
          <span className="text-slate-400">Metric</span>
          <select
            className="mt-1 w-full rounded-lg bg-[#0d0d1a] border border-white/[0.08]
                       px-3 py-1.5 text-slate-200 outline-none focus:border-indigo-500"
            value={(widget.config as any).metric}
            onChange={(e) => set("metric", e.target.value)}
          >
            <option value="total_flows">Total Flows</option>
            <option value="active_agents">Active Agents</option>
            <option value="total_bytes">Total Bytes (24h)</option>
            <option value="anomaly_count">Anomaly Count (24h)</option>
            <option value="alert_count">Alert Count (24h)</option>
          </select>
        </label>
      )}

      {widget.type === "timeseries" && (
        <label className="block">
          <span className="text-slate-400">Time window</span>
          <select
            className="mt-1 w-full rounded-lg bg-[#0d0d1a] border border-white/[0.08]
                       px-3 py-1.5 text-slate-200 outline-none focus:border-indigo-500"
            value={(widget.config as any).window}
            onChange={(e) => set("window", e.target.value)}
          >
            <option value="1h">Last 1 hour</option>
            <option value="6h">Last 6 hours</option>
            <option value="24h">Last 24 hours</option>
          </select>
        </label>
      )}

      {widget.type === "top_talkers" && (
        <>
          <label className="block">
            <span className="text-slate-400">Time window</span>
            <select
              className="mt-1 w-full rounded-lg bg-[#0d0d1a] border border-white/[0.08]
                         px-3 py-1.5 text-slate-200 outline-none focus:border-indigo-500"
              value={(widget.config as any).window}
              onChange={(e) => set("window", e.target.value)}
            >
              <option value="1h">Last 1 hour</option>
              <option value="6h">Last 6 hours</option>
              <option value="24h">Last 24 hours</option>
              <option value="7d">Last 7 days</option>
            </select>
          </label>
          <label className="block">
            <span className="text-slate-400">Rank by</span>
            <select
              className="mt-1 w-full rounded-lg bg-[#0d0d1a] border border-white/[0.08]
                         px-3 py-1.5 text-slate-200 outline-none focus:border-indigo-500"
              value={(widget.config as any).by}
              onChange={(e) => set("by", e.target.value)}
            >
              <option value="flows">Flow count</option>
              <option value="bytes">Bytes transferred</option>
            </select>
          </label>
          <label className="block">
            <span className="text-slate-400">Show top</span>
            <select
              className="mt-1 w-full rounded-lg bg-[#0d0d1a] border border-white/[0.08]
                         px-3 py-1.5 text-slate-200 outline-none focus:border-indigo-500"
              value={(widget.config as any).limit}
              onChange={(e) => set("limit", Number(e.target.value))}
            >
              <option value={5}>5</option>
              <option value={10}>10</option>
              <option value={20}>20</option>
            </select>
          </label>
        </>
      )}

      {(widget.type === "alert_feed" || widget.type === "anomaly_feed") && (
        <label className="block">
          <span className="text-slate-400">Max rows</span>
          <select
            className="mt-1 w-full rounded-lg bg-[#0d0d1a] border border-white/[0.08]
                       px-3 py-1.5 text-slate-200 outline-none focus:border-indigo-500"
            value={(widget.config as any).limit}
            onChange={(e) => set("limit", Number(e.target.value))}
          >
            <option value={5}>5</option>
            <option value={8}>8</option>
            <option value={15}>15</option>
          </select>
        </label>
      )}

      {widget.type === "anomaly_feed" && (
        <label className="block">
          <span className="text-slate-400">Severity filter</span>
          <select
            className="mt-1 w-full rounded-lg bg-[#0d0d1a] border border-white/[0.08]
                       px-3 py-1.5 text-slate-200 outline-none focus:border-indigo-500"
            value={(widget.config as any).severity ?? "all"}
            onChange={(e) => set("severity", e.target.value)}
          >
            <option value="all">All</option>
            <option value="high">High only</option>
            <option value="medium">Medium+</option>
            <option value="low">Low+</option>
          </select>
        </label>
      )}
    </div>
  );
}

// ── Add-widget picker ─────────────────────────────────────────────────────────
function WidgetPicker({ onAdd }: { onAdd: (w: Widget) => void }) {
  return (
    <div className="space-y-2">
      {WIDGET_CATALOGUE.map((tpl) => (
        <button
          key={tpl.type}
          onClick={() => onAdd(newWidget(tpl))}
          className="w-full text-left rounded-lg p-3 border border-white/[0.06]
                     bg-white/[0.02] hover:bg-white/[0.05] transition-colors"
        >
          <p className="text-xs font-medium text-slate-200">{tpl.label}</p>
          <p className="text-[10px] text-slate-500 mt-0.5">{tpl.description}</p>
        </button>
      ))}
    </div>
  );
}

// ── DashboardGrid ─────────────────────────────────────────────────────────────

interface Props {
  widgets: Widget[];
  editing: boolean;
  onChange: (widgets: Widget[]) => void;
}

export function DashboardGrid({ widgets, editing, onChange }: Props) {
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [showPicker, setShowPicker] = useState(false);

  const selectedWidget = widgets.find((w) => w.id === selectedId) ?? null;

  const move = (id: string, dir: -1 | 1) => {
    const idx = widgets.findIndex((w) => w.id === id);
    if (idx < 0) return;
    const next = idx + dir;
    if (next < 0 || next >= widgets.length) return;
    const arr = [...widgets];
    [arr[idx], arr[next]] = [arr[next], arr[idx]];
    onChange(arr);
  };

  const resize = (id: string, dir: -1 | 1) => {
    const ORDER: WidgetSize[] = ["sm", "md", "lg"];
    onChange(
      widgets.map((w) => {
        if (w.id !== id) return w;
        const i = ORDER.indexOf(w.size);
        const j = Math.max(0, Math.min(ORDER.length - 1, i + dir));
        return { ...w, size: ORDER[j] };
      })
    );
  };

  const remove = (id: string) => {
    onChange(widgets.filter((w) => w.id !== id));
    if (selectedId === id) setSelectedId(null);
  };

  const updateWidget = (updated: Widget) => {
    onChange(widgets.map((w) => (w.id === updated.id ? updated : w)));
  };

  const addWidget = (w: Widget) => {
    onChange([...widgets, w]);
    setShowPicker(false);
    setSelectedId(w.id);
  };

  if (widgets.length === 0 && !editing) {
    return (
      <div className="flex flex-col items-center justify-center py-24 text-center">
        <p className="text-slate-500 text-sm">This dashboard has no widgets yet.</p>
        <p className="text-slate-600 text-xs mt-1">Click "Edit" to add some.</p>
      </div>
    );
  }

  return (
    <div className={`flex gap-6 ${editing ? "items-start" : ""}`}>
      {/* ── Grid ── */}
      <div className="flex-1 min-w-0">
        {editing && (
          <div className="flex items-center justify-between mb-4">
            <p className="text-xs text-slate-500">
              Click a widget to configure it. Use arrows to reorder.
            </p>
            <button
              onClick={() => setShowPicker(true)}
              className="flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs
                         bg-indigo-600 hover:bg-indigo-500 text-white transition-colors"
            >
              <Plus size={12} /> Add widget
            </button>
          </div>
        )}

        <div className="grid grid-cols-12 gap-4">
          {widgets.map((widget, idx) => {
            const isSelected = editing && selectedId === widget.id;
            return (
              <div
                key={widget.id}
                className={`${sizeToColSpan(widget.size)} ${WIDGET_H[widget.type]}
                            relative transition-all
                            ${editing ? "cursor-pointer" : ""}
                            ${isSelected ? "ring-2 ring-indigo-500 rounded-xl" : ""}`}
                onClick={() => editing && setSelectedId(widget.id)}
              >
                <WidgetRenderer widget={widget} />

                {/* edit-mode overlay controls */}
                {editing && (
                  <div className="absolute top-2 right-2 flex gap-1 opacity-0
                                  group-hover:opacity-100 hover:opacity-100
                                  [.relative:hover_&]:opacity-100">
                    {/* move left */}
                    {idx > 0 && (
                      <button
                        onClick={(e) => { e.stopPropagation(); move(widget.id, -1); }}
                        className="p-1 rounded bg-black/60 text-slate-400 hover:text-white"
                        title="Move left"
                      >
                        <ChevronLeft size={10} />
                      </button>
                    )}
                    {/* move right */}
                    {idx < widgets.length - 1 && (
                      <button
                        onClick={(e) => { e.stopPropagation(); move(widget.id, 1); }}
                        className="p-1 rounded bg-black/60 text-slate-400 hover:text-white"
                        title="Move right"
                      >
                        <ChevronRight size={10} />
                      </button>
                    )}
                    {/* shrink */}
                    {widget.size !== "sm" && (
                      <button
                        onClick={(e) => { e.stopPropagation(); resize(widget.id, -1); }}
                        className="p-1 rounded bg-black/60 text-slate-400 hover:text-white"
                        title="Shrink"
                      >
                        <Minimize2 size={10} />
                      </button>
                    )}
                    {/* grow */}
                    {widget.size !== "lg" && (
                      <button
                        onClick={(e) => { e.stopPropagation(); resize(widget.id, 1); }}
                        className="p-1 rounded bg-black/60 text-slate-400 hover:text-white"
                        title="Grow"
                      >
                        <Maximize2 size={10} />
                      </button>
                    )}
                    {/* delete */}
                    <button
                      onClick={(e) => { e.stopPropagation(); remove(widget.id); }}
                      className="p-1 rounded bg-black/60 text-slate-400 hover:text-red-400"
                      title="Remove"
                    >
                      <Trash2 size={10} />
                    </button>
                  </div>
                )}
              </div>
            );
          })}
        </div>
      </div>

      {/* ── Side panel (config or picker) ── */}
      {editing && (selectedWidget || showPicker) && (
        <div className="w-64 shrink-0 rounded-xl bg-[#0d0d1a] border border-white/[0.06] p-4">
          {showPicker ? (
            <>
              <div className="flex items-center justify-between mb-3">
                <p className="text-xs font-semibold text-slate-200">Add widget</p>
                <button onClick={() => setShowPicker(false)}>
                  <X size={13} className="text-slate-500 hover:text-white" />
                </button>
              </div>
              <WidgetPicker onAdd={addWidget} />
            </>
          ) : selectedWidget ? (
            <>
              <div className="flex items-center justify-between mb-3">
                <p className="text-xs font-semibold text-slate-200">Configure widget</p>
                <button onClick={() => setSelectedId(null)}>
                  <X size={13} className="text-slate-500 hover:text-white" />
                </button>
              </div>
              <ConfigForm widget={selectedWidget} onChange={updateWidget} />
            </>
          ) : null}
        </div>
      )}
    </div>
  );
}
