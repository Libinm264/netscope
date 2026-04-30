"use client";

/**
 * CopilotPanel — AI Security Copilot slide-in chat panel.
 *
 * Self-contained: renders a fixed floating button (bottom-right) that opens
 * a 440px right-side panel.  No external state required — drop it anywhere
 * in the layout tree.
 *
 * SSE protocol from POST /api/v1/copilot/chat:
 *   {"type":"text","text":"..."}        — stream into current assistant bubble
 *   {"type":"query","sql":"...","description":"..."} — show SQL card
 *   {"type":"result","columns":[...],"rows":[...],"total":n} — data table
 *   {"type":"error","error":"..."}      — inline error card
 *   {"type":"done"}                     — mark turn complete
 */

import { useCallback, useEffect, useRef, useState } from "react";
import {
  Bot, X, Send, ChevronDown, ChevronRight,
  Database, AlertTriangle, Loader2, Sparkles,
} from "lucide-react";
import { clsx } from "clsx";

// ── Types ─────────────────────────────────────────────────────────────────────

type SSEEventType = "text" | "query" | "result" | "error" | "done";

interface SSEEvent {
  type: SSEEventType;
  text?:        string;
  sql?:         string;
  description?: string;
  columns?:     string[];
  rows?:        unknown[][];
  total?:       number;
  error?:       string;
}

// A "block" is one piece of an assistant turn rendered in the UI.
type Block =
  | { kind: "text"; content: string }
  | { kind: "query"; sql: string; description: string }
  | { kind: "result"; columns: string[]; rows: unknown[][]; total: number }
  | { kind: "error"; message: string };

interface Message {
  role: "user" | "assistant";
  /** For user messages, a simple string.  For assistant, an array of blocks. */
  content: string | Block[];
  streaming?: boolean; // true while assistant is still typing
}

// ── Sub-components ────────────────────────────────────────────────────────────

function QueryCard({ sql, description }: { sql: string; description: string }) {
  const [expanded, setExpanded] = useState(false);

  return (
    <div className="rounded-lg border border-indigo-500/20 bg-indigo-500/5 overflow-hidden text-xs mt-2">
      <button
        onClick={() => setExpanded(e => !e)}
        className="w-full flex items-center gap-2 px-3 py-2 text-left
                   hover:bg-indigo-500/10 transition-colors"
      >
        <Database size={11} className="text-indigo-400 shrink-0" />
        <span className="text-indigo-300 font-medium truncate flex-1">
          {description || "ClickHouse query"}
        </span>
        {expanded
          ? <ChevronDown size={11} className="text-indigo-400 shrink-0" />
          : <ChevronRight size={11} className="text-indigo-400 shrink-0" />}
      </button>
      {expanded && (
        <pre className="px-3 pb-3 pt-1 text-emerald-300 font-mono text-[10px] leading-relaxed
                        whitespace-pre-wrap break-all border-t border-indigo-500/20">
          {sql}
        </pre>
      )}
    </div>
  );
}

function ResultTable({ columns, rows, total }: { columns: string[]; rows: unknown[][]; total: number }) {
  const [showAll, setShowAll] = useState(false);
  const preview = showAll ? rows : rows.slice(0, 5);
  const hasMore = rows.length > 5 && !showAll;

  if (rows.length === 0) {
    return (
      <div className="mt-2 rounded-lg border border-white/[0.06] bg-white/[0.02]
                      px-3 py-2 text-xs text-slate-500 italic">
        Query returned 0 rows
      </div>
    );
  }

  return (
    <div className="mt-2 rounded-lg border border-white/[0.06] bg-white/[0.02] overflow-hidden text-xs">
      <div className="overflow-x-auto">
        <table className="w-full text-left">
          <thead>
            <tr className="border-b border-white/[0.06] bg-white/[0.03]">
              {columns.map(c => (
                <th key={c} className="px-3 py-1.5 font-medium text-slate-400 whitespace-nowrap">
                  {c}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {preview.map((row, ri) => (
              <tr key={ri} className="border-b border-white/[0.03] hover:bg-white/[0.02]">
                {(row as unknown[]).map((cell, ci) => (
                  <td key={ci} className="px-3 py-1.5 text-slate-300 font-mono whitespace-nowrap max-w-[200px] truncate">
                    {String(cell ?? "")}
                  </td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      <div className="flex items-center justify-between px-3 py-1.5 text-[10px] text-slate-500 border-t border-white/[0.06]">
        <span>{total.toLocaleString()} row{total !== 1 ? "s" : ""}</span>
        {hasMore && (
          <button
            onClick={() => setShowAll(true)}
            className="text-indigo-400 hover:text-indigo-300 transition-colors"
          >
            Show all {rows.length} rows
          </button>
        )}
      </div>
    </div>
  );
}

function ErrorCard({ message }: { message: string }) {
  return (
    <div className="mt-2 rounded-lg border border-red-500/20 bg-red-500/5 px-3 py-2
                    flex items-start gap-2 text-xs text-red-300">
      <AlertTriangle size={12} className="mt-0.5 shrink-0 text-red-400" />
      <span>{message}</span>
    </div>
  );
}

function AssistantBlocks({ blocks, streaming }: { blocks: Block[]; streaming?: boolean }) {
  return (
    <div>
      {blocks.map((b, i) => {
        if (b.kind === "text") {
          return (
            <span key={i} className="text-sm text-slate-200 leading-relaxed whitespace-pre-wrap">
              {b.content}
            </span>
          );
        }
        if (b.kind === "query") {
          return <QueryCard key={i} sql={b.sql} description={b.description} />;
        }
        if (b.kind === "result") {
          return <ResultTable key={i} columns={b.columns} rows={b.rows} total={b.total} />;
        }
        if (b.kind === "error") {
          return <ErrorCard key={i} message={b.message} />;
        }
        return null;
      })}
      {streaming && (
        <span className="inline-block w-1.5 h-3.5 bg-indigo-400 rounded-sm animate-pulse ml-0.5 align-text-bottom" />
      )}
    </div>
  );
}

// ── Suggested prompts shown on empty state ────────────────────────────────────

const SUGGESTIONS = [
  "What are the top 10 source IPs by flow count in the last hour?",
  "Are there any high-severity anomalies in the past 24 hours?",
  "Which processes are making outbound connections on unusual ports?",
  "Show me DNS queries to domains longer than 50 characters",
  "Which agents have seen the most threat activity this week?",
];

// ── Main panel ────────────────────────────────────────────────────────────────

export function CopilotPanel() {
  const [open,     setOpen]     = useState(false);
  const [messages, setMessages] = useState<Message[]>([]);
  const [input,    setInput]    = useState("");
  const [busy,     setBusy]     = useState(false);

  const bottomRef  = useRef<HTMLDivElement>(null);
  const inputRef   = useRef<HTMLTextAreaElement>(null);
  const abortRef   = useRef<AbortController | null>(null);

  // Auto-scroll to bottom when messages change.
  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [messages]);

  // Resize textarea to content (max 5 lines).
  const handleInputChange = (e: React.ChangeEvent<HTMLTextAreaElement>) => {
    setInput(e.target.value);
    e.target.style.height = "auto";
    e.target.style.height = Math.min(e.target.scrollHeight, 120) + "px";
  };

  // Close with Escape key; open via custom "open-copilot" event (from Sidebar).
  useEffect(() => {
    const keyHandler = (e: KeyboardEvent) => {
      if (e.key === "Escape" && open) setOpen(false);
    };
    const openHandler = () => setOpen(true);
    window.addEventListener("keydown", keyHandler);
    window.addEventListener("open-copilot", openHandler);
    return () => {
      window.removeEventListener("keydown", keyHandler);
      window.removeEventListener("open-copilot", openHandler);
    };
  }, [open]);

  // Focus input when panel opens.
  useEffect(() => {
    if (open) setTimeout(() => inputRef.current?.focus(), 50);
  }, [open]);

  const sendMessage = useCallback(async (text: string) => {
    if (!text.trim() || busy) return;

    const userMsg: Message = { role: "user", content: text };

    // Build history for the API (previous user/assistant text pairs).
    const history = [
      ...messages.map(m => ({
        role: m.role,
        content: typeof m.content === "string"
          ? m.content
          : (m.content as Block[])
              .filter(b => b.kind === "text")
              .map(b => (b as { kind: "text"; content: string }).content)
              .join(""),
      })),
      { role: "user" as const, content: text },
    ];

    setMessages(prev => [...prev, userMsg]);
    setInput("");
    // Reset textarea height.
    if (inputRef.current) {
      inputRef.current.style.height = "auto";
    }
    setBusy(true);

    // Append a blank assistant message that we'll fill in as tokens arrive.
    const assistantIdx = messages.length + 1;
    setMessages(prev => [...prev, { role: "assistant", content: [], streaming: true }]);

    const ctrl = new AbortController();
    abortRef.current = ctrl;

    try {
      const res = await fetch("/api/proxy/copilot/chat", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ messages: history }),
        signal: ctrl.signal,
        cache: "no-store",
      });

      if (!res.ok) {
        const err = await res.text().catch(() => "Unknown error");
        setMessages(prev =>
          prev.map((m, i) =>
            i === assistantIdx
              ? { ...m, content: [{ kind: "error", message: `API error ${res.status}: ${err}` }] as Block[], streaming: false }
              : m,
          ),
        );
        return;
      }

      const reader = res.body!.getReader();
      const decoder = new TextDecoder();
      let buf = "";

      // Helper: append a block to the current assistant message.
      const pushBlock = (block: Block) => {
        setMessages(prev =>
          prev.map((m, i) =>
            i === assistantIdx
              ? { ...m, content: [...(m.content as Block[]), block] }
              : m,
          ),
        );
      };

      // Helper: append text to the last text block (or create one).
      const appendText = (text: string) => {
        setMessages(prev =>
          prev.map((m, i) => {
            if (i !== assistantIdx) return m;
            const blocks = m.content as Block[];
            if (blocks.length > 0 && blocks[blocks.length - 1].kind === "text") {
              const updated = [...blocks];
              const last = updated[updated.length - 1] as { kind: "text"; content: string };
              updated[updated.length - 1] = { kind: "text", content: last.content + text };
              return { ...m, content: updated };
            }
            return { ...m, content: [...blocks, { kind: "text", content: text }] };
          }),
        );
      };

      while (true) {
        const { done, value } = await reader.read();
        if (done) break;

        buf += decoder.decode(value, { stream: true });
        const lines = buf.split("\n");
        buf = lines.pop() ?? "";

        for (const line of lines) {
          if (!line.startsWith("data: ")) continue;
          const raw = line.slice(6).trim();
          if (!raw) continue;

          let ev: SSEEvent;
          try {
            ev = JSON.parse(raw);
          } catch {
            continue;
          }

          switch (ev.type) {
            case "text":
              if (ev.text) appendText(ev.text);
              break;
            case "query":
              pushBlock({ kind: "query", sql: ev.sql ?? "", description: ev.description ?? "" });
              break;
            case "result":
              pushBlock({ kind: "result", columns: ev.columns ?? [], rows: ev.rows ?? [], total: ev.total ?? 0 });
              break;
            case "error":
              pushBlock({ kind: "error", message: ev.error ?? "Unknown error" });
              break;
            case "done":
              // Mark streaming complete.
              setMessages(prev =>
                prev.map((m, i) => (i === assistantIdx ? { ...m, streaming: false } : m)),
              );
              break;
          }
        }
      }
    } catch (err: unknown) {
      if (err instanceof Error && err.name === "AbortError") return;
      const msg = err instanceof Error ? err.message : "Unknown error";
      setMessages(prev =>
        prev.map((m, i) =>
          i === assistantIdx
            ? { ...m, content: [{ kind: "error", message: msg }] as Block[], streaming: false }
            : m,
        ),
      );
    } finally {
      setBusy(false);
      abortRef.current = null;
    }
  }, [busy, messages]);

  const handleKeyDown = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === "Enter" && !e.shiftKey) {
      e.preventDefault();
      sendMessage(input);
    }
  };

  const stopStreaming = () => {
    abortRef.current?.abort();
    setBusy(false);
  };

  const clearHistory = () => {
    setMessages([]);
    setBusy(false);
    abortRef.current?.abort();
  };

  return (
    <>
      {/* ── Floating toggle button ──────────────────────────────────────────── */}
      <button
        onClick={() => setOpen(o => !o)}
        title="AI Security Copilot"
        className={clsx(
          "fixed bottom-5 right-5 z-50 w-12 h-12 rounded-full shadow-lg",
          "flex items-center justify-center transition-all duration-200",
          open
            ? "bg-indigo-600 text-white scale-95"
            : "bg-indigo-600 hover:bg-indigo-500 text-white",
        )}
      >
        {open ? <X size={18} /> : <Sparkles size={18} />}
      </button>

      {/* ── Panel ──────────────────────────────────────────────────────────── */}
      <div
        className={clsx(
          "fixed top-0 right-0 bottom-0 z-40 w-[440px]",
          "flex flex-col bg-[#0d0d1a] border-l border-white/[0.06]",
          "transition-transform duration-300 ease-in-out shadow-2xl",
          open ? "translate-x-0" : "translate-x-full",
        )}
      >
        {/* Header */}
        <div className="flex items-center justify-between px-4 py-3 border-b border-white/[0.06]">
          <div className="flex items-center gap-2.5">
            <div className="p-1 rounded-md bg-indigo-500/10">
              <Bot size={16} className="text-indigo-400" />
            </div>
            <div>
              <p className="text-sm font-semibold text-white">AI Security Copilot</p>
              <p className="text-[10px] text-slate-500">Powered by Claude</p>
            </div>
          </div>
          <div className="flex items-center gap-1">
            {messages.length > 0 && (
              <button
                onClick={clearHistory}
                className="text-xs text-slate-500 hover:text-slate-300 px-2 py-1 rounded
                           transition-colors hover:bg-white/[0.04]"
              >
                Clear
              </button>
            )}
            <button
              onClick={() => setOpen(false)}
              className="p-1 text-slate-500 hover:text-slate-300 rounded
                         hover:bg-white/[0.04] transition-colors"
            >
              <X size={15} />
            </button>
          </div>
        </div>

        {/* Messages */}
        <div className="flex-1 overflow-y-auto px-4 py-4 space-y-4">
          {messages.length === 0 && (
            <div className="space-y-5">
              {/* Welcome */}
              <div className="text-center pt-6">
                <div className="inline-flex p-3 rounded-xl bg-indigo-500/10 ring-1 ring-indigo-500/20 mb-3">
                  <Bot size={24} className="text-indigo-400" />
                </div>
                <p className="text-sm font-semibold text-white">AI Security Copilot</p>
                <p className="text-xs text-slate-400 mt-1 max-w-[280px] mx-auto leading-relaxed">
                  Ask anything about your network traffic. I can query ClickHouse in real time.
                </p>
              </div>

              {/* Suggestions */}
              <div className="space-y-1.5">
                <p className="text-[10px] text-slate-600 uppercase tracking-wider font-medium px-1">
                  Suggested
                </p>
                {SUGGESTIONS.map((s, i) => (
                  <button
                    key={i}
                    onClick={() => sendMessage(s)}
                    className="w-full text-left px-3 py-2.5 rounded-lg text-xs text-slate-400
                               border border-white/[0.06] hover:border-indigo-500/30
                               hover:text-slate-200 hover:bg-white/[0.03] transition-all"
                  >
                    {s}
                  </button>
                ))}
              </div>
            </div>
          )}

          {messages.map((msg, i) => (
            <div
              key={i}
              className={clsx(
                "flex",
                msg.role === "user" ? "justify-end" : "justify-start",
              )}
            >
              {msg.role === "user" ? (
                <div className="max-w-[85%] px-3 py-2 rounded-xl rounded-tr-sm
                                bg-indigo-600 text-white text-sm leading-relaxed">
                  {msg.content as string}
                </div>
              ) : (
                <div className="max-w-[95%] text-slate-200">
                  <AssistantBlocks
                    blocks={msg.content as Block[]}
                    streaming={msg.streaming}
                  />
                </div>
              )}
            </div>
          ))}

          <div ref={bottomRef} />
        </div>

        {/* Input area */}
        <div className="px-3 pb-4 pt-2 border-t border-white/[0.06]">
          <div className="flex items-end gap-2 bg-white/[0.04] rounded-xl
                          border border-white/[0.08] focus-within:border-indigo-500/40
                          transition-colors px-3 py-2">
            <textarea
              ref={inputRef}
              value={input}
              onChange={handleInputChange}
              onKeyDown={handleKeyDown}
              placeholder="Ask about your network..."
              rows={1}
              disabled={busy}
              className="flex-1 bg-transparent text-sm text-white placeholder-slate-500
                         resize-none outline-none leading-relaxed min-h-[24px]
                         disabled:opacity-50"
              style={{ overflow: "hidden" }}
            />
            {busy ? (
              <button
                onClick={stopStreaming}
                title="Stop"
                className="p-1.5 rounded-lg text-slate-400 hover:text-white
                           hover:bg-white/[0.06] transition-colors shrink-0"
              >
                <Loader2 size={15} className="animate-spin" />
              </button>
            ) : (
              <button
                onClick={() => sendMessage(input)}
                disabled={!input.trim()}
                title="Send (Enter)"
                className="p-1.5 rounded-lg bg-indigo-600 hover:bg-indigo-500
                           text-white transition-colors disabled:opacity-40
                           disabled:cursor-not-allowed shrink-0"
              >
                <Send size={14} />
              </button>
            )}
          </div>
          <p className="text-[10px] text-slate-600 text-center mt-1.5">
            Enter to send · Shift+Enter for newline · Esc to close
          </p>
        </div>
      </div>

      {/* Backdrop (mobile / narrow screens) */}
      {open && (
        <div
          onClick={() => setOpen(false)}
          className="fixed inset-0 z-30 bg-black/20 backdrop-blur-[1px] lg:hidden"
        />
      )}
    </>
  );
}
