import { defineConfig } from 'vitest/config';
import preact from '@preact/preset-vite';
import { mockApi } from './mock/api.ts';

// The Go host embeds dist/ and serves it with no-store caching, so asset names
// stay simple and stable. That also keeps the committed build easy to review.
export default defineConfig({
  base: '/',
  // The mock only hooks the dev and preview servers. It is never bundled.
  plugins: [preact(), mockApi()],
  build: {
    outDir: 'dist',
    emptyOutDir: true,
    assetsDir: 'assets',
    sourcemap: false,
    cssCodeSplit: false,
    assetsInlineLimit: 0,
    rollupOptions: {
      output: {
        entryFileNames: 'assets/app.js',
        chunkFileNames: 'assets/[name].js',
        assetFileNames: 'assets/[name][extname]',
      },
    },
  },
  test: {
    environment: 'node',
    include: ['src/**/*.test.ts'],
  },
});
