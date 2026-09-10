// package: vite / build
// type:    config
// job:     build a single IIFE bundle with its assets inlined
// limits:  build configuration; the one-file fold is scripts/inline.mjs

import react from '@vitejs/plugin-react';
import { defineConfig } from 'vite';

/**
 * Built as a single IIFE bundle with assets inlined, so `run.sh single` can fold it
 * into one double-clickable file (see scripts/inline.mjs). A module bundle would be
 * rejected by `file://`; a classic script is not.
 */
// The build stamps its own version, as the server stamps its binary: EXPLORER_VERSION
// comes from `git describe` in the Makefile, and a bare `vite build` says "dev" rather
// than claiming a release it is not.
const version = process.env.EXPLORER_VERSION || 'dev';

export default defineConfig({
  plugins: [react()],
  define: {
    __EXPLORER_VERSION__: JSON.stringify(version),
  },
  build: {
    outDir: '.build', // scratch — never committed, never served; inline.mjs folds it into explorer.html
    target: 'es2022',
    assetsInlineLimit: 1024 * 1024 * 100,
    cssCodeSplit: false,
    modulePreload: false,
    rollupOptions: {
      output: {
        format: 'iife',
        inlineDynamicImports: true,
        entryFileNames: 'assets/app.js',
        assetFileNames: 'assets/app.[ext]',
      },
    },
  },
});
