import { defineConfig } from "vite";
import { svelte } from "@sveltejs/vite-plugin-svelte";
import { resolve } from "path";
import { pageGenerator } from "./vite-plugin-pages";

// Page configurations for dumb:// protocol pages
const pages = {
  homepage: {
    entry: "src/pages/homepage.ts",
    output: "homepage.min.js",
    global: "DumberHomepage",
    html: {
      name: "homepage",
      title: "Dumber Browser",
      script: "homepage.min.js",
      css: "style.css",
      filename: "index.html",
    },
  },
  error: {
    entry: "src/pages/error.ts",
    output: "error.min.js",
    global: "DumberError",
    html: {
      name: "error",
      title: "Error",
      script: "error.min.js",
      css: "style.css",
      filename: "error.html",
    },
  },
  config: {
    entry: "src/pages/config.ts",
    output: "config.min.js",
    global: "DumberConfig",
    html: {
      name: "config",
      title: "Settings",
      script: "config.min.js",
      css: "style.css",
      filename: "config.html",
    },
  },
} as const;

// Get page from VITE_PAGE env var, default to building all
const targetPage = process.env.VITE_PAGE as keyof typeof pages | undefined;

// Build single page config
function buildPageConfig(pageName: keyof typeof pages) {
  const page = pages[pageName];
  return defineConfig({
    plugins: [
      svelte({
        emitCss: false, // Inline component CSS for shadow DOM compatibility
      }),
      pageGenerator([page.html]),
    ],
    build: {
      rollupOptions: {
        input: resolve(__dirname, page.entry),
        output: {
          dir: "../assets/webui",
          entryFileNames: page.output,
          chunkFileNames: "[name].js",
          assetFileNames: "[name].[ext]",
          manualChunks: undefined,
          format: "iife",
          name: page.global,
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
}

// Export config for specified page or default to homepage
// Use VITE_PAGE=homepage or VITE_PAGE=blocked to select
export default buildPageConfig(targetPage || "homepage");
