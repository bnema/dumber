import { defineConfig } from 'tsup'

export default defineConfig([
  // Browser bundle
  {
    entry: ['src/main.ts'],
    format: ['esm'],
    outDir: 'dist',
    outExtension: () => ({ js: '.js' }),
    target: 'es2024',
    platform: 'browser',
    minify: false,
    sourcemap: true,
    clean: true,
    bundle: true,
    splitting: false,
    external: [],
    // noExternal: [],
  },
  // Injectable controls script (minified for injection)
  {
    entry: ['src/injected-controls.ts'],
    format: ['iife'],
    outDir: 'dist',
    outExtension: () => ({ js: '.min.js' }),
    target: 'es2024',
    platform: 'browser',
    minify: true,
    sourcemap: false,
    bundle: true,
    splitting: false,
    globalName: '__dumberControls',
    external: [],
    onSuccess: async () => {
      console.log('✓ Injectable controls script compiled successfully')
    }
  },
  // Node.js build script
  {
    entry: ['src/build-html.ts', 'src/utils/html-builder.ts'],
    format: ['esm'],
    outDir: 'dist',
    outExtension: () => ({ js: '.js' }),
    target: 'es2024',
    platform: 'node',
    minify: false,
    sourcemap: false,
    bundle: true,
    splitting: false,
    onSuccess: async () => {
      console.log('✓ Frontend TypeScript compiled successfully')
    }
  }
])
