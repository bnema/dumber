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
        name: "homepage",
        title: "Dumber Browser",
        script: "homepage.min.js",
        filename: "index.html",
      },
    ]),
  ],
  build: {
    rollupOptions: {
      input: resolve(__dirname, "src/pages/homepage.ts"),
      output: {
        dir: "../assets/webui",
        entryFileNames: "homepage.min.js",
        chunkFileNames: "[name].js",
        assetFileNames: "[name].[ext]",
        manualChunks: undefined,
        format: "iife",
        name: "DumberHomepage",
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
