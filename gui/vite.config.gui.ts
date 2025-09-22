import { defineConfig } from "vite";
import { svelte } from "@sveltejs/vite-plugin-svelte";
import { resolve } from "path";

export default defineConfig({
  plugins: [
    svelte({
      emitCss: false, // Inline component CSS into JS bundle
    }),
  ],
  build: {
    rollupOptions: {
      input: resolve(__dirname, "src/injected/gui.ts"),
      output: {
        dir: "../assets/gui",
        format: "iife",
        entryFileNames: "gui.min.js",
        name: "__dumberGUI",
        inlineDynamicImports: true,
        manualChunks: undefined, // Prevent chunk splitting
        assetFileNames: "[name].[ext]", // Don't hash asset names since they're inlined
      },
    },
    emptyOutDir: false,
    target: ["es2020", "chrome91", "firefox90"],
    minify: true,
    sourcemap: false,
    cssCodeSplit: false, // Prevent CSS code splitting
    assetsInlineLimit: 100000000, // Inline all assets as data URLs (100MB limit)
  },
  resolve: {
    alias: {
      // Alias for easier imports
      $lib: resolve(__dirname, "src/lib"),
      $components: resolve(__dirname, "src/components"),
    },
  },
});
