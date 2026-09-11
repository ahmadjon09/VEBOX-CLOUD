/// <reference types="vitest/config" />
import { defineConfig, type Plugin } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";

const csp = [
  "default-src 'self'",
  "script-src 'self'",
  "style-src 'self' 'unsafe-inline'",
  "img-src 'self' data: blob: https:",
  "media-src 'self' blob: https:",
  "connect-src 'self' https:",
  "font-src 'self'",
  "object-src 'none'",
  "base-uri 'self'",
  "form-action 'self'",
  "frame-ancestors 'none'",
].join("; ");

function cspMeta(): Plugin {
  return {
    name: "vebox:csp",
    apply: "build",
    transformIndexHtml() {
      return [
        {
          tag: "meta",
          attrs: { "http-equiv": "Content-Security-Policy", content: csp },
          injectTo: "head-prepend" as const,
        },
      ];
    },
  };
}

const apiProxy = {
  target: process.env.VITE_DEV_PROXY || "http://127.0.0.1:8080",
  changeOrigin: true,
};

export default defineConfig({
  plugins: [react(), tailwindcss(), cspMeta()],
  server: {
    host: "0.0.0.0",
    port: 5173,
    allowedHosts: true,
    proxy: {
      "/v1": apiProxy,
      "/auth": apiProxy,
      "/healthz": apiProxy,
      "/openapi.json": apiProxy,
      "/i/": apiProxy,
      "/cdn/": apiProxy,
      "/preview/": apiProxy,
      "/d/": apiProxy,
      "/s/": apiProxy,
    },
  },
  build: {
    outDir: "dist",
    rollupOptions: {
      output: {
        manualChunks: {
          react: ["react", "react-dom", "react-router-dom"],
          charts: ["recharts"],
          http: ["axios", "swr"],
        },
      },
    },
  },
  test: {
    environment: "jsdom",
    setupFiles: "./test/setup.ts",
    css: false,
    include: ["test/**/*.test.{ts,tsx}"],
  },
});
