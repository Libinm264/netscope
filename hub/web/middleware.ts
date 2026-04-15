import { NextResponse } from "next/server";
import type { NextRequest } from "next/server";

// Auth is only enforced when all four Auth0 env vars are present.
// In local dev (or Docker without Auth0 configured) the middleware is a no-op.
const AUTH_ENABLED =
  !!process.env.AUTH0_SECRET &&
  !!process.env.AUTH0_ISSUER_BASE_URL &&
  !!process.env.AUTH0_CLIENT_ID &&
  !!process.env.AUTH0_CLIENT_SECRET;

export async function middleware(request: NextRequest) {
  if (!AUTH_ENABLED) {
    return NextResponse.next();
  }

  // Dynamically import so the build doesn't fail when Auth0 vars are absent
  const { withMiddlewareAuthRequired } = await import(
    "@auth0/nextjs-auth0/edge"
  );
  return withMiddlewareAuthRequired()(request);
}

export const config = {
  // Protect all pages except auth routes, static assets, and the login page
  matcher: [
    "/((?!api/auth|_next/static|_next/image|favicon\\.ico|login).*)",
  ],
};
