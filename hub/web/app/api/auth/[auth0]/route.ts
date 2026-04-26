/**
 * This route previously handled Auth0 callbacks.
 * Auth0 has been replaced by the native NetScope session-based auth
 * (ns_session cookie, OIDC/SAML SSO, local email+password).
 *
 * The Go backend handles all auth flows directly:
 *   POST /api/v1/enterprise/auth/login       — email/password
 *   GET  /api/v1/enterprise/auth/oidc/initiate — OIDC SSO
 *   POST /api/v1/enterprise/auth/saml/callback — SAML ACS
 *
 * This file is kept as a stub to avoid 404s from stale bookmarks.
 */
import { type NextRequest, NextResponse } from "next/server";

export function GET(_req: NextRequest) {
  return NextResponse.redirect(new URL("/login", _req.url));
}

export function POST(_req: NextRequest) {
  return NextResponse.redirect(new URL("/login", _req.url));
}
