import { defineConfig } from "vite";
import { resolve } from "path";

export default defineConfig({
  build: {
    rollupOptions: {
      input: resolve(__dirname, "src/injected/modules/color-scheme.ts"),
      output: {
        dir: "../assets/webui",
        format: "iife",
        entryFileNames: "color-scheme.js",
        inlineDynamicImports: true,
        manualChunks: undefined, // Prevent chunk splitting
      },
    },
    emptyOutDir: false, // Don't clear the directory since GUI bundle is also built here
    target: ["es2020", "chrome91", "firefox90"],
    minify: true,
    sourcemap: false,
  },
  resolve: {
    alias: {
      // Alias for easier imports
      $lib: resolve(__dirname, "src/lib"),
      $components: resolve(__dirname, "src/components"),
    },
  },
});
