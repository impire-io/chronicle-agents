import react from "@vitejs/plugin-react";
import { defineConfig } from "vitest/config";

// The build lands in ../dist, which web/server.go embeds; the Go binary
// serves it at every address, so the base is the root.
export default defineConfig({
  plugins: [react()],
  base: "/",
  build: {
    outDir: "../dist",
    emptyOutDir: true,
    sourcemap: false,
  },
  test: {
    environment: "node",
    include: ["src/**/*.test.ts"],
  },
});
