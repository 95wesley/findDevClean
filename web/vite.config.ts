import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// The build output is embedded into the Go binary (see web/embed.go).
export default defineConfig({
  plugins: [react()],
  base: "/",
  build: {
    outDir: "dist",
    emptyOutDir: true,
  },
});
