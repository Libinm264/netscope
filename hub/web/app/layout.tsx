import type { Metadata } from "next";
import "./globals.css";
import { Sidebar } from "@/components/Sidebar";

export const metadata: Metadata = {
  title: "NetScope Hub",
  description: "Network observability dashboard",
  icons: {
    icon: "/icon.svg",
    shortcut: "/icon.svg",
    apple: "/icon.svg",
  },
};

// Resolve the current Auth0 user if auth is configured.
// Falls back gracefully when AUTH0 env vars are absent or on error.
async function getUser() {
  const authEnabled =
    process.env.AUTH0_SECRET &&
    process.env.AUTH0_ISSUER_BASE_URL &&
    process.env.AUTH0_CLIENT_ID &&
    process.env.AUTH0_CLIENT_SECRET;

  if (!authEnabled) return null;

  try {
    const { getSession } = await import("@auth0/nextjs-auth0");
    const session = await getSession();
    return session?.user ?? null;
  } catch {
    return null;
  }
}

export default async function RootLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  const user = await getUser();

  return (
    <html lang="en" className="dark">
      <body className="flex h-screen overflow-hidden bg-[#0a0a14] text-slate-200">
        <Sidebar user={user} />
        <main className="flex-1 overflow-auto ml-[220px]">
          {children}
        </main>
      </body>
    </html>
  );
}
