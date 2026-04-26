import { NextResponse } from "next/server";
import type { NextRequest } from "next/server";

// Paths that are always public — no session required.
const PUBLIC_PREFIXES = [
  "/login",
  "/accept-invite",   // new-user invite acceptance (token in query param)
  "/forgot-password", // password reset request form
  "/reset-password",  // password reset confirmation form
  "/api/",            // Next.js API routes (proxy, auth callbacks)
  "/_next/",          // Next.js static assets
  "/favicon",
  "/icon.svg",
];

/**
 * Auth middleware — protects all hub pages with an ns_session cookie check.
 *
 * The ns_session cookie is issued by the Go backend after a successful
 * OIDC or SAML login. The cookie is httpOnly so it cannot be read by JS;
 * this middleware just checks for its presence.
 *
 * Actual session validation (expiry, user lookup) happens on the Go side
 * when the browser makes API calls — the proxy forwards the cookie upstream.
 *
 * When no session is present the user is redirected to /login with a
 * `from` query param so they land back on their intended page after signing in.
 */
export function middleware(request: NextRequest) {
  const { pathname } = request.nextUrl;

  // Always allow public paths through.
  if (PUBLIC_PREFIXES.some((p) => pathname.startsWith(p))) {
    return NextResponse.next();
  }

  const session = request.cookies.get("ns_session");

  if (!session?.value) {
    const loginUrl = new URL("/login", request.url);
    // Preserve the intended destination so the login page can redirect back.
    if (pathname !== "/") {
      loginUrl.searchParams.set("from", pathname);
    }
    return NextResponse.redirect(loginUrl);
  }

  return NextResponse.next();
}

export const config = {
  // Run on all routes except static files and Next.js internals.
  matcher: [
    "/((?!_next/static|_next/image|favicon\\.ico|icon\\.svg).*)",
  ],
};
