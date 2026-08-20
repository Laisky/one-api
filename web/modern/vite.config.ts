/// <reference types="vitest" />
import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));

const manualChunkGroups: Record<string, string[]> = {
  vendor: ['react', 'react-dom'],
  router: ['react-router-dom'],
  'tanstack-query': ['@tanstack/react-query'],
  'tanstack-table': ['@tanstack/react-table'],
  'radix-ui-core': [
    '@radix-ui/react-dialog',
    '@radix-ui/react-dropdown-menu',
    '@radix-ui/react-popover',
    '@radix-ui/react-tooltip',
  ],
  'radix-ui-forms': [
    '@radix-ui/react-checkbox',
    '@radix-ui/react-label',
    '@radix-ui/react-select',
    '@radix-ui/react-switch',
  ],
  'radix-ui-layout': [
    '@radix-ui/react-scroll-area',
    '@radix-ui/react-separator',
    '@radix-ui/react-slot',
    '@radix-ui/react-tabs',
    '@radix-ui/react-toast',
    '@radix-ui/react-hover-card',
  ],
  'markdown-core': ['react-markdown', 'marked'],
  'markdown-remark': ['remark-gfm', 'remark-math', 'remark-emoji'],
  'markdown-rehype-highlight': ['rehype-highlight'],
  'markdown-rehype-katex': ['rehype-katex', 'katex'],
  'markdown-rehype-sanitize': ['rehype-sanitize'],
  charts: ['recharts'],
  'ui-utils': ['lucide-react', 'class-variance-authority', 'clsx', 'tailwind-merge', 'cmdk'],
  forms: ['react-hook-form', '@hookform/resolvers', 'zod'],
  network: ['axios'],
  'misc-utils': ['qrcode', 'zustand'],
};

function manualChunks(moduleId: string): string | undefined {
  const normalizedId = moduleId.replaceAll('\\', '/');
  if (!normalizedId.includes('/node_modules/')) return undefined;

  for (const [chunkName, packages] of Object.entries(manualChunkGroups)) {
    if (packages.some((packageName) => normalizedId.includes('/node_modules/' + packageName + '/'))) {
      return chunkName;
    }
  }

  return undefined;
}

// The build uses the latest locked caniuse-lite data. Suppress Browserslist's
// age warning so CI stays deterministic when the system date is ahead of the
// newest published browser metadata.
process.env.BROWSERSLIST_IGNORE_OLD_DATA ??= '1';

export default defineConfig(({ mode }) => ({
  plugins: [react()],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
    },
  },
  build: {
    outDir: '../build/modern',
    sourcemap: mode === 'development',
    chunkSizeWarningLimit: 500,
    minify: 'esbuild',
    target: 'esnext',
    cssCodeSplit: true,
    assetsInlineLimit: 4096,
    reportCompressedSize: true,
    esbuild: {
      legalComments: 'none',
      treeShaking: true,
      minifyIdentifiers: true,
      minifySyntax: true,
      minifyWhitespace: true,
    },
    rollupOptions: {
      treeshake: {
        moduleSideEffects: 'no-external',
        propertyReadSideEffects: false,
        tryCatchDeoptimization: false,
      },
      external: [],
      output: {
        chunkFileNames: '[name].[hash].js',
        manualChunks,
      },
    },
  },
  server: {
    port: 3001,
    proxy: {
      '/api': { target: 'http://localhost:3000', changeOrigin: true },
    },
  },
  test: {
    globals: true,
    environment: 'jsdom',
    setupFiles: ['./src/test/setup.ts'],
  },
}));
