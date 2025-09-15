import { defineConfig } from 'vite';
import { svelte } from '@sveltejs/vite-plugin-svelte';
import { resolve } from 'path';

export default defineConfig({
  plugins: [
    svelte(),
  ],
  build: {
    rollupOptions: {
      input: resolve(__dirname, 'src/injected/gui.ts'),
      output: {
        dir: '../assets/gui',
        format: 'iife',
        entryFileNames: 'gui.min.js',
        name: '__dumberGUI',
        inlineDynamicImports: true,
        assetFileNames: '[name].[ext]', // Don't hash asset names
      },
    },
    emptyOutDir: false,
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