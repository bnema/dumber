import { defineConfig } from "vite";
import { svelte } from "@sveltejs/vite-plugin-svelte";
import { resolve } from "path";
import { pageGenerator } from "./vite-plugin-pages";

export default defineConfig({
  plugins: [
    svelte({
      emitCss: false, // Inline component CSS for shadow DOM compatibility
    }),
    pageGenerator([
      {
        name: "blocked",
        title: "Page Blocked",
        script: "blocked.min.js",
        filename: "blocked.html",
      },
    ]),
  ],
  build: {
    rollupOptions: {
      input: resolve(__dirname, "src/pages/blocked.ts"),
      output: {
        dir: "../assets/gui",
        entryFileNames: "blocked.min.js",
        chunkFileNames: "[name].js",
        assetFileNames: "[name].[ext]",
        manualChunks: undefined,
        format: "iife",
        name: "DumberBlocked",
      },
    },
    emptyOutDir: false,
    target: ["es2020", "chrome91", "firefox90"],
    minify: true,
    sourcemap: false,
    cssCodeSplit: false,
    assetsInlineLimit: 4096,
  },
  resolve: {
    alias: {
      $lib: resolve(__dirname, "src/lib"),
      $components: resolve(__dirname, "src/components"),
    },
  },
});
