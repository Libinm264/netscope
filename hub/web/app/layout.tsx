import type { Metadata } from "next";
import { cookies } from "next/headers";
import "./globals.css";
import { Sidebar } from "@/components/Sidebar";
import { CopilotPanel } from "@/components/CopilotPanel";

export const metadata: Metadata = {
  title: "NetScope Hub",
  description: "Network observability dashboard",
  icons: {
    icon:    "/icon.svg",
    shortcut:"/icon.svg",
    apple:   "/icon.svg",
  },
};

// Resolve the current user from the ns_session cookie.
// Calls the Go /me endpoint server-side so the Sidebar gets real user info
// without any client-side fetch on every page.
// Returns null when unauthenticated — middleware handles the redirect before
// this layout renders for protected pages.
async function getSessionUser() {
  const HUB_API_URL = process.env.HUB_API_URL ?? "http://localhost:8080";
  const HUB_API_KEY = process.env.HUB_API_KEY ?? "";

  const cookieStore = await cookies();
  const sessionToken = cookieStore.get("ns_session")?.value;
  if (!sessionToken || !HUB_API_KEY) return null;

  try {
    const res = await fetch(
      `${HUB_API_URL}/api/v1/enterprise/auth/me`,
      {
        headers: {
          "X-Api-Key": HUB_API_KEY,
          "Cookie":    `ns_session=${sessionToken}`,
        },
        cache: "no-store",
      },
    );
    if (!res.ok) return null;
    return await res.json() as {
      email: string;
      display_name: string;
      role: string;
      org_id: string;
    };
  } catch {
    return null;
  }
}

export default async function RootLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  const user = await getSessionUser();

  return (
    <html lang="en" className="dark">
      <body className="flex h-screen overflow-hidden bg-[#0a0a14] text-slate-200">
        <Sidebar
          user={user
            ? { name: user.display_name, email: user.email, picture: null }
            : null
          }
        />
        <main className="flex-1 overflow-auto ml-[220px]">
          {children}
        </main>
        <CopilotPanel />
      </body>
    </html>
  );
}
