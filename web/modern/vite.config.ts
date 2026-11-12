/// <reference types="vitest" />
import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { browserBuildContract } from '../legacy/browser-release.mjs';

const __dirname = path.dirname(fileURLToPath(import.meta.url));

const manualChunkGroups: Record<string, string[]> = {
  // use-sync-external-store is shared by react-i18next, zustand, recharts and TanStack Table; keeping it
  // here stops whichever feature group reaches it first from becoming a dependency of every page.
  vendor: ['react', 'react-dom', 'use-sync-external-store'],
  router: ['react-router-dom'],
  'tanstack-query': ['@tanstack/react-query'],
  'tanstack-table': ['@tanstack/react-table'],
  'radix-ui-core': ['@radix-ui/react-dialog', '@radix-ui/react-dropdown-menu', '@radix-ui/react-popover', '@radix-ui/react-tooltip'],
  'radix-ui-forms': ['@radix-ui/react-checkbox', '@radix-ui/react-label', '@radix-ui/react-select', '@radix-ui/react-switch'],
  'radix-ui-layout': [
    '@radix-ui/react-scroll-area',
    '@radix-ui/react-separator',
    '@radix-ui/react-slot',
    '@radix-ui/react-tabs',
    '@radix-ui/react-toast',
    '@radix-ui/react-hover-card',
  ],
  'markdown-core': ['react-markdown', 'marked'],
  'markdown-remark': ['remark-gfm', 'remark-math', 'remark-emoji', 'remark-breaks'],
  'markdown-rehype-highlight': ['rehype-highlight'],
  'markdown-rehype-katex': ['rehype-katex', 'katex'],
  'markdown-rehype-sanitize': ['rehype-sanitize'],
  charts: ['recharts'],
  'ui-utils': ['lucide-react', 'class-variance-authority', 'clsx', 'tailwind-merge', 'cmdk'],
  forms: ['react-hook-form', '@hookform/resolvers', 'zod'],
  network: ['axios'],
  // qrcode is deliberately not grouped here: only the 2FA setup flow in personal settings uses it, so it
  // belongs in that lazy route's chunks rather than in a chunk the app shell loads.
  'misc-utils': ['zustand'],
};

/** packagePattern builds a cross-platform module-id matcher for the supplied package names and returns it. */
function packagePattern(packages: string[]): RegExp {
  const packagePaths = packages.map((packageName) =>
    packageName
      .split('/')
      .map((segment) => segment.replace(/[.*+?^${}()|[\]\\]/g, '\\$&'))
      .join('[\\\\/]')
  );

  return new RegExp(`[\\\\/]node_modules[\\\\/](?:${packagePaths.join('|')})(?:[\\\\/]|$)`);
}

// Groups that only feature routes (dashboard charts, Markdown pages, data tables) need. Every other group
// holds packages shared across the app shell.
const featureChunkGroups = new Set([
  'tanstack-table',
  'markdown-core',
  'markdown-remark',
  'markdown-rehype-highlight',
  'markdown-rehype-katex',
  'markdown-rehype-sanitize',
  'charts',
]);

// Rolldown groups capture their modules' dependencies recursively by default, so a feature group can swallow
// a small shared package (recharts pulled in clsx; a table/chart group pulled in use-sync-external-store) and
// make every chunk that needs it statically import the whole feature chunk. Shared groups therefore get a
// higher priority: Rolldown resolves them first and removes their modules from lower-priority groups.
const SHARED_GROUP_PRIORITY = 20;
const FEATURE_GROUP_PRIORITY = 10;

const codeSplittingGroups = Object.entries(manualChunkGroups).map(([name, packages]) => ({
  name,
  test: packagePattern(packages),
  priority: featureChunkGroups.has(name) ? FEATURE_GROUP_PRIORITY : SHARED_GROUP_PRIORITY,
}));

// The build uses the latest locked caniuse-lite data. Suppress Browserslist's
// age warning so CI stays deterministic when the system date is ahead of the
// newest published browser metadata.
process.env.BROWSERSLIST_IGNORE_OLD_DATA ??= '1';

export default defineConfig(({ mode }) => {
  const browserBuild = browserBuildContract('modern', { mode });
  return {
    plugins: [browserBuild.plugin, react()],
    define: { __ONEAPI_BROWSER_RELEASE__: JSON.stringify(browserBuild.release) },
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
      rolldownOptions: {
        treeshake: {
          moduleSideEffects: 'no-external',
          propertyReadSideEffects: false,
        },
        external: [],
        output: {
          chunkFileNames: '[name].[hash].js',
          codeSplitting: {
            groups: codeSplittingGroups,
          },
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
      // Vitest's 5s default is a unit-test budget. This suite renders whole pages
      // (TanStack Table, Radix portals, react-hook-form) into jsdom and runs its 80
      // files across every core, so the page-level tests cost far more wall clock
      // under that contention than they do alone: the channel pagination and email
      // whitelist cases measure ~1.3-2.1s in isolation but up to ~8.1s in a full
      // parallel run, which used to time them out. The bound stays well above that
      // worst case while still failing a genuinely hung test promptly.
      testTimeout: 30000,
    },
  };
});
