import react from "@vitejs/plugin-react";
import { defineConfig } from "vite-plus";

// The Go server owns /api and /auth; in development Vite proxies to it.
const server = process.env.EASYGO_SERVER ?? "http://127.0.0.1:8080";

export default defineConfig({
  plugins: [react()],
  server: {
    proxy: {
      "/api": { target: server, changeOrigin: false },
      "/auth": { target: server, changeOrigin: false },
    },
  },
  build: { outDir: "dist", emptyOutDir: true },
});
