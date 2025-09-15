import { defineConfig } from 'vite';
import { svelte } from '@sveltejs/vite-plugin-svelte';
import { resolve } from 'path';
import { pageGenerator } from './vite-plugin-pages';

export default defineConfig({
  plugins: [
    svelte(),
    pageGenerator([
      {
        name: 'homepage',
        title: 'Dumber Browser',
        script: 'homepage.min.js',
        css: 'homepage.css',
        filename: 'index.html', // Special case: homepage uses index.html
      }
    ]),
  ],
  build: {
    rollupOptions: {
      input: resolve(__dirname, 'src/pages/homepage.ts'),
      output: {
        dir: '../assets/gui',
        format: 'iife',
        entryFileNames: 'homepage.min.js',
        name: '__dumberHomepage',
        inlineDynamicImports: true,
        assetFileNames: (assetInfo) => {
          // Keep CSS as homepage.css to avoid conflicts with the main GUI
          if (assetInfo.name?.endsWith('.css')) {
            return 'homepage.[ext]';
          }
          return '[name].[ext]';
        },
      },
    },
    emptyOutDir: false, // Don't clear the directory (shared with GUI build)
    target: ['es2020', 'chrome91', 'firefox90'],
    minify: true,
    sourcemap: false,
    cssCodeSplit: false, // Keep CSS in single file
    assetsInlineLimit: 0, // Don't inline assets by default
  },
  resolve: {
    alias: {
      // Alias for easier imports
      '$lib': resolve(__dirname, 'src/lib'),
      '$components': resolve(__dirname, 'src/components'),
    },
  },
});