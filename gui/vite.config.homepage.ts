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
        filename: 'index.html',
      }
    ]),
  ],
  build: {
    rollupOptions: {
      input: resolve(__dirname, 'src/pages/homepage.ts'),
      output: {
        dir: '../assets/gui',
        entryFileNames: 'homepage.min.js',
        chunkFileNames: '[name].js',
        assetFileNames: '[name].[ext]',
      },
    },
    emptyOutDir: false,
    target: ['es2020', 'chrome91', 'firefox90'],
    minify: true,
    sourcemap: false,
    cssCodeSplit: true,
    assetsInlineLimit: 4096,
  },
  resolve: {
    alias: {
      '$lib': resolve(__dirname, 'src/lib'),
      '$components': resolve(__dirname, 'src/components'),
    },
  },
});