import react from "@vitejs/plugin-react";
import { defineConfig } from "vitest/config";

// The daemon serves the built SPA from web/_ui/dist via go:embed. In dev the
// Vite server owns the page and proxies every /api call — INCLUDING the
// terminal websocket upgrade (`ws: true`) — to the daemon on 7781.
export default defineConfig({
  plugins: [react()],
  build: { outDir: "dist", emptyOutDir: true, assetsDir: "assets" },
  server: {
    port: 5173,
    proxy: {
      "/api": { target: "http://127.0.0.1:7781", changeOrigin: false, ws: true },
    },
  },
  test: {
    environment: "jsdom",
    globals: true,
    setupFiles: ["./src/test/setup.ts"],
  },
});
