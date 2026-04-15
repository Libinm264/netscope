// Handles all Auth0 routes:
//   /api/auth/login    → redirect to Auth0
//   /api/auth/logout   → clear session + redirect
//   /api/auth/callback → exchange code for tokens
//   /api/auth/me       → return current user JSON
import { handleAuth } from "@auth0/nextjs-auth0";

export const GET = handleAuth();
