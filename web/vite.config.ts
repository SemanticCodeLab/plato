import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// The build output is embedded into the Go binary, so emit into cmd/plato/web_dist.
// During dev, proxy API + health calls to the local Plato server.
export default defineConfig({
  plugins: [react()],
  build: {
    outDir: "../cmd/plato/web_dist",
    emptyOutDir: true,
  },
  server: {
    proxy: {
      "/api": "http://localhost:8080",
      "/healthz": "http://localhost:8080",
    },
  },
});
