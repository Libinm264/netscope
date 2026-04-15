import type { Config } from "tailwindcss";

const config: Config = {
  content: [
    "./app/**/*.{ts,tsx}",
    "./components/**/*.{ts,tsx}",
    "./lib/**/*.{ts,tsx}",
  ],
  theme: {
    extend: {
      colors: {
        bg: {
          base:    "#0a0a14",
          surface: "#0d0d1a",
          raised:  "#12121f",
          hover:   "#1a1a2e",
        },
        border: "rgba(255,255,255,0.08)",
        accent: {
          DEFAULT: "#6366f1",
          hover:   "#818cf8",
        },
      },
      fontFamily: {
        mono: ["ui-monospace", "SFMono-Regular", "Menlo", "monospace"],
      },
    },
  },
  plugins: [],
};

export default config;
