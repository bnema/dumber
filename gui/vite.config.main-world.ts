import { defineConfig } from "vite";
import { resolve } from "path";

export default defineConfig({
  build: {
    rollupOptions: {
      input: resolve(__dirname, "src/injected/main-world.ts"),
      output: {
        dir: "../assets/gui",
        format: "iife",
        entryFileNames: "main-world.min.js",
        name: "__dumberMainWorld",
        inlineDynamicImports: true,
        manualChunks: undefined,
        assetFileNames: "[name].[ext]",
      },
    },
    emptyOutDir: false,
    target: ["es2020", "chrome91", "firefox90"],
    minify: true,
    sourcemap: false,
    cssCodeSplit: false,
    assetsInlineLimit: 100000000,
  },
  define: {
    __DOM_ZOOM_DEFAULT__: "1.0", // Default placeholder, will be replaced by Go
  },
});
