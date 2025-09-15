import { vitePreprocess } from '@sveltejs/vite-plugin-svelte';

export default {
  // Enable rune mode for Svelte 5
  compilerOptions: {
    runes: true,
  },
  // Use Vite's preprocessor for TypeScript and PostCSS
  preprocess: vitePreprocess(),
};