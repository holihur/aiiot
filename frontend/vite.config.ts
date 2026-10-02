import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import path from "path";

export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: { "@": path.resolve(__dirname, "./src") },
  },
  build: {
    // Build straight into the directory the Go core serves as static files.
    // Override with VITE_OUT_DIR (used by the Docker build).
    outDir: process.env.VITE_OUT_DIR
      ? path.resolve(__dirname, process.env.VITE_OUT_DIR)
      : path.resolve(__dirname, "../backend/web/dist"),
    emptyOutDir: true,
    chunkSizeWarningLimit: 1500,
  },
  server: {
    port: 5173,
    proxy: {
      "/api": "http://127.0.0.1:8080",
      "/internal": "http://127.0.0.1:8080",
    },
  },
});
