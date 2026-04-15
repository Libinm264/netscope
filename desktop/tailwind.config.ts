import type { Config } from "tailwindcss";

export default {
  darkMode: "class",
  content: ["./index.html", "./src/**/*.{ts,tsx}"],
  theme: {
    extend: {
      colors: {
        // Protocol colors
        "proto-http": "#3b82f6",
        "proto-https": "#0ea5e9",
        "proto-dns": "#8b5cf6",
        "proto-tls": "#10b981",
        "proto-tcp": "#6b7280",
        "proto-udp": "#06b6d4",
        "proto-error": "#ef4444",
        // App chrome
        border: "hsl(var(--border))",
        background: "hsl(var(--background))",
        foreground: "hsl(var(--foreground))",
        primary: {
          DEFAULT: "hsl(var(--primary))",
          foreground: "hsl(var(--primary-foreground))",
        },
        muted: {
          DEFAULT: "hsl(var(--muted))",
          foreground: "hsl(var(--muted-foreground))",
        },
        accent: {
          DEFAULT: "hsl(var(--accent))",
          foreground: "hsl(var(--accent-foreground))",
        },
      },
      fontFamily: {
        mono: ["JetBrains Mono", "Cascadia Code", "Fira Code", "monospace"],
      },
    },
  },
  plugins: [],
} satisfies Config;
