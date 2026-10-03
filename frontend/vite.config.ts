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
    chunkSizeWarningLimit: 1200,
    rollupOptions: {
      output: {
        // Split framework/third-party chunks so the entry file stays small and
        // vendor files are cached independently across deploys.
        manualChunks: {
          vendor: ["react", "react-dom", "react-router-dom"],
          query: ["@tanstack/react-query"],
          charts: ["recharts"],
          mapgl: ["maplibre-gl"],
        },
      },
    },
  },
  server: {
    port: 5173,
    proxy: {
      "/api": "http://127.0.0.1:8080",
      "/internal": "http://127.0.0.1:8080",
    },
  },
});
