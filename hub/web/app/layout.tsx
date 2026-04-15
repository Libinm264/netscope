import type { Metadata } from "next";
import "./globals.css";
import { Sidebar } from "@/components/Sidebar";

export const metadata: Metadata = {
  title: "NetScope Hub",
  description: "Network observability dashboard",
};

export default function RootLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <html lang="en" className="dark">
      <body className="flex h-screen overflow-hidden bg-[#0a0a14] text-slate-200">
        <Sidebar />
        <main className="flex-1 overflow-auto ml-[220px]">
          {children}
        </main>
      </body>
    </html>
  );
}
