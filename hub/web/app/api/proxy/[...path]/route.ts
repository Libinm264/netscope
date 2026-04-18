/**
 * Catch-all server-side proxy for all NetScope Hub API calls.
 *
 * WHY THIS EXISTS
 * ───────────────
 * Putting HUB_API_KEY in a NEXT_PUBLIC_* env var bakes the secret into the
 * browser JavaScript bundle, where any visitor can read it.  This proxy keeps
 * the key server-side only: the browser calls /api/proxy/<path> (same origin,
 * no auth header needed), and this route injects the real key before forwarding
 * to the hub backend.
 *
 * REQUIRED ENV VARS (server-side only — NO "NEXT_PUBLIC_" prefix)
 * ───────────────────────────────────────────────────────────────
 *   HUB_API_URL   URL of the hub Go API  (default: http://localhost:8080)
 *   HUB_API_KEY   Bootstrap API key      (required in production)
 *
 * ROUTING
 * ───────
 *   Browser  →  /api/proxy/flows          →  hub :8080/api/v1/flows
 *   Browser  →  /api/proxy/flows/stream   →  hub :8080/api/v1/flows/stream  (SSE)
 *   Browser  →  /api/proxy/alerts/abc     →  hub :8080/api/v1/alerts/abc
 */

import { type NextRequest, NextResponse } from "next/server";

const HUB_API_URL = process.env.HUB_API_URL ?? "http://localhost:8080";
const HUB_API_KEY = process.env.HUB_API_KEY ?? "";

type RouteContext = { params: Promise<{ path: string[] }> };

async function handler(req: NextRequest, context: RouteContext): Promise<NextResponse> {
  if (!HUB_API_KEY) {
    return NextResponse.json(
      { error: "Hub API key is not configured on the server" },
      { status: 503 },
    );
  }

  const { path: segments } = await context.params;
  const upstreamPath = `/api/v1/${(segments ?? []).join("/")}`;

  let upstream: URL;
  try {
    upstream = new URL(upstreamPath, HUB_API_URL);
  } catch {
    return NextResponse.json({ error: "invalid upstream path" }, { status: 400 });
  }

  // Forward all query params from the browser, but never leak or forward any
  // stale api_key the browser might have cached from older versions.
  req.nextUrl.searchParams.forEach((value, key) => {
    if (key !== "api_key") upstream.searchParams.set(key, value);
  });

  const isBodyMethod =
    req.method === "POST" ||
    req.method === "PUT" ||
    req.method === "PATCH" ||
    req.method === "DELETE";

  let upstreamResp: Response;
  try {
    upstreamResp = await fetch(upstream.toString(), {
      method: req.method,
      headers: {
        "Content-Type": "application/json",
        "X-Api-Key": HUB_API_KEY,
      },
      body: isBodyMethod ? req.body : undefined,
      // Required for streaming request bodies in Node.js runtime
      // @ts-ignore
      duplex: "half",
      cache: "no-store",
    });
  } catch (err) {
    console.error("[proxy] upstream request failed:", err);
    return NextResponse.json({ error: "hub backend unreachable" }, { status: 502 });
  }

  const ct = upstreamResp.headers.get("content-type") ?? "";

  // Transparently stream SSE connections — the browser EventSource connects to
  // /api/proxy/flows/stream and receives the event stream directly.
  if (ct.includes("text/event-stream")) {
    return new NextResponse(upstreamResp.body, {
      status: 200,
      headers: {
        "Content-Type":    "text/event-stream",
        "Cache-Control":   "no-cache, no-transform",
        "X-Accel-Buffering": "no",
        "Connection":      "keep-alive",
      },
    });
  }

  // Regular JSON / text responses
  const body = await upstreamResp.text();
  return new NextResponse(body, {
    status: upstreamResp.status,
    headers: {
      "Content-Type": ct || "application/json",
    },
  });
}

export const GET    = handler;
export const POST   = handler;
export const PUT    = handler;
export const PATCH  = handler;
export const DELETE = handler;
