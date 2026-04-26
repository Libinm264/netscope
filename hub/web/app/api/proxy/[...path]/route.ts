/**
 * Catch-all server-side proxy for all NetScope Hub API calls.
 *
 * Responsibilities:
 *  1. Keeps HUB_API_KEY server-side (never exposed to the browser bundle).
 *  2. Forwards the ns_session cookie from the browser to the Go backend so
 *     enterprise auth endpoints (/me, /logout, SCIM, SSO config) can resolve
 *     the current user identity.
 *  3. Forwards Set-Cookie headers from the Go backend back to the browser so
 *     the ns_session cookie is set after email/password login.
 *  4. Transparently streams Server-Sent Events for /flows/stream.
 *
 * ENV VARS (server-side only — no NEXT_PUBLIC_ prefix):
 *   HUB_API_URL   URL of the hub Go API   (default: http://localhost:8080)
 *   HUB_API_KEY   Bootstrap API key       (required in production)
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

  // Forward query params (but strip any stale api_key the browser might have).
  req.nextUrl.searchParams.forEach((value, key) => {
    if (key !== "api_key") upstream.searchParams.set(key, value);
  });

  const isBodyMethod =
    req.method === "POST" ||
    req.method === "PUT"  ||
    req.method === "PATCH" ||
    req.method === "DELETE";

  // Forward the ns_session cookie so enterprise endpoints can identify the user.
  const cookieHeader = req.headers.get("Cookie") ?? "";

  let upstreamResp: Response;
  try {
    upstreamResp = await fetch(upstream.toString(), {
      method: req.method,
      headers: {
        "Content-Type":  "application/json",
        "X-Api-Key":     HUB_API_KEY,
        ...(cookieHeader ? { "Cookie": cookieHeader } : {}),
      },
      body:  isBodyMethod ? req.body : undefined,
      // Required for streaming request bodies in Node.js runtime.
      // @ts-ignore
      duplex: "half",
      cache: "no-store",
    });
  } catch (err) {
    console.error("[proxy] upstream request failed:", err);
    return NextResponse.json({ error: "hub backend unreachable" }, { status: 502 });
  }

  const ct = upstreamResp.headers.get("content-type") ?? "";

  // Transparently stream SSE — browser EventSource connects to
  // /api/proxy/flows/stream and receives the event stream directly.
  if (ct.includes("text/event-stream")) {
    return new NextResponse(upstreamResp.body, {
      status: 200,
      headers: {
        "Content-Type":      "text/event-stream",
        "Cache-Control":     "no-cache, no-transform",
        "X-Accel-Buffering": "no",
        "Connection":        "keep-alive",
      },
    });
  }

  // Regular JSON / text responses.
  const body = await upstreamResp.text();

  const responseHeaders: Record<string, string> = {
    "Content-Type": ct || "application/json",
  };

  // Forward Set-Cookie from the Go backend (e.g. after login sets ns_session).
  const setCookie = upstreamResp.headers.get("set-cookie");
  if (setCookie) {
    responseHeaders["Set-Cookie"] = setCookie;
  }

  return new NextResponse(body, {
    status:  upstreamResp.status,
    headers: responseHeaders,
  });
}

export const GET    = handler;
export const POST   = handler;
export const PUT    = handler;
export const PATCH  = handler;
export const DELETE = handler;
