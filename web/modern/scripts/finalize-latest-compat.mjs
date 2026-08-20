import fs from 'node:fs';
import path from 'node:path';

const repoRoot = process.cwd();
const modernRoot = path.join(repoRoot, 'web', 'modern');

function read(relativePath) {
  return fs.readFileSync(path.join(repoRoot, relativePath), 'utf8');
}

function write(relativePath, content) {
  const target = path.join(repoRoot, relativePath);
  fs.mkdirSync(path.dirname(target), { recursive: true });
  fs.writeFileSync(target, content);
}

function transform(relativePath, update) {
  const original = read(relativePath);
  const next = update(original);
  if (next !== original) write(relativePath, next);
}

const packagePath = path.join(modernRoot, 'package.json');
const packageJson = JSON.parse(fs.readFileSync(packagePath, 'utf8'));

// The @typescript/native alias publishes the TypeScript 7 CLI under the
// standard `tsc` binary. Keep TS 6 only as the compiler-API sidecar required
// by ESLint and other tools that have not migrated to the native API yet.
packageJson.scripts.build = 'tsc -b && vite build --emptyOutDir';
packageJson.scripts['build:prod'] = 'tsc -b && vite build --mode production --emptyOutDir';
packageJson.scripts['type-check'] = 'tsc --noEmit';
fs.writeFileSync(packagePath, `${JSON.stringify(packageJson, null, 2)}\n`);

write(
  'web/modern/eslint.config.js',
  `import js from '@eslint/js';
import { defineConfig, globalIgnores } from 'eslint/config';
import globals from 'globals';
import reactHooks from 'eslint-plugin-react-hooks';
import reactRefresh from 'eslint-plugin-react-refresh';
import tseslint from 'typescript-eslint';

export default defineConfig(
  globalIgnores([
    'node_modules/**',
    'coverage/**',
    'dist/**',
    'tsconfig.tsbuildinfo',
  ]),
  js.configs.recommended,
  tseslint.configs.recommended,
  {
    files: ['**/*.{js,mjs,cjs,jsx,ts,tsx}'],
    languageOptions: {
      ecmaVersion: 'latest',
      sourceType: 'module',
      globals: {
        ...globals.browser,
        ...globals.node,
      },
    },
    plugins: {
      'react-hooks': reactHooks,
      'react-refresh': reactRefresh,
    },
    rules: {
      'no-unused-vars': 'off',
      'no-empty': 'off',
      'no-useless-assignment': 'off',
      'prefer-const': 'off',
      '@typescript-eslint/no-empty-object-type': 'off',
      '@typescript-eslint/no-explicit-any': 'off',
      '@typescript-eslint/no-unused-vars': [
        'warn',
        {
          argsIgnorePattern: '^_',
          caughtErrorsIgnorePattern: '^_',
          varsIgnorePattern: '^_',
        },
      ],
      'react-hooks/rules-of-hooks': 'error',
      'react-hooks/exhaustive-deps': 'warn',
      'react-refresh/only-export-components': [
        'warn',
        { allowConstantExport: true },
      ],
    },
  },
);
`,
);

// Table 9 makes optional behavior explicit. The shared data tables use
// visibility, pagination, sorting and row-selection APIs, so register exactly
// those features while retaining server-controlled pagination/sorting.
write(
  'web/modern/src/lib/table.ts',
  `import {
  columnVisibilityFeature,
  rowPaginationFeature,
  rowSelectionFeature,
  rowSortingFeature,
  tableFeatures,
} from '@tanstack/react-table';
import type { ColumnDef, RowData } from '@tanstack/react-table';

export const modernTableFeatures = tableFeatures({
  columnVisibilityFeature,
  rowPaginationFeature,
  rowSelectionFeature,
  rowSortingFeature,
});

export type ModernColumnDef<
  TData extends RowData,
  TValue = unknown,
> = ColumnDef<typeof modernTableFeatures, TData, TValue>;
`,
);

for (const relativePath of [
  'web/modern/src/components/ui/data-table.tsx',
  'web/modern/src/components/ui/enhanced-data-table.tsx',
]) {
  transform(relativePath, (text) =>
    text.replace(
      '    columns: enhancedColumns,',
      '    columns: enhancedColumns as ColumnDef<TData, unknown>[],',
    ),
  );
}

transform('web/modern/src/components/chat/ChatInterface.tsx', (text) =>
  text.replace(
    'onEditMessage?: (messageIndex: number, newContent: string) => void;',
    'onEditMessage?: (messageIndex: number, newContent: string | any[]) => void;',
  ),
);

// Zod 4 applies defaults before parsing through prefault; the persisted source
// already uses this behavior. Keep the older intermediate form migratable if
// this script is re-run against a partially upgraded branch.
transform('web/modern/src/pages/channels/schemas.ts', (text) =>
  text
    .replace(
      '.default(() => ({})),',
      '.prefault({}),',
    )
    .replace(
      ".default(() => ({ auth_type: 'personal_access_token', api_format: 'chat_completion' as const })),",
      '.prefault({}),',
    ),
);

// Keep the Recharts 3 ComponentProps import idempotent across validation runs.
transform('web/modern/src/pages/dashboard/components/UsageSections.tsx', (text) => {
  const withoutDuplicates = text.replace(/^import type \{ ComponentProps \} from 'react';\n/gm, '');
  return withoutDuplicates.replace(
    "import type { TFunction } from 'i18next';\n",
    "import type { TFunction } from 'i18next';\nimport type { ComponentProps } from 'react';\n",
  );
});

// Vitest 4 no longer treats vi.fn().mockImplementation(...) as a constructable
// class. Floating UI, Radix and cmdk instantiate ResizeObserver with `new`.
transform('web/modern/src/test/setup.ts', (text) =>
  text
    .replace(
      `// Mock ResizeObserver
globalThis.ResizeObserver = vi.fn().mockImplementation(() => ({
  observe: vi.fn(),
  unobserve: vi.fn(),
  disconnect: vi.fn(),
}));`,
      `// Mock ResizeObserver with a real constructable class for Vitest 4.
class MockResizeObserver implements ResizeObserver {
  constructor(_callback: ResizeObserverCallback) {}

  observe(_target: Element, _options?: ResizeObserverOptions) {}

  unobserve(_target: Element) {}

  disconnect() {}
}

globalThis.ResizeObserver = MockResizeObserver;`,
    )
    .replace(
      '  // @ts-expect-error assigning test-only PointerEvent polyfill for jsdom\n',
      '',
    ),
);

// Testing Library 16 correctly requires a label/control relationship. Add an
// explicit stable ID instead of relying on wrapper structure. Zod 4's unified
// error API also requires an explicit error parameter for numeric bounds.
transform('web/modern/src/pages/topup/TopUpPage.tsx', (text) =>
  text
    .replace(
      "<FormLabel>{tr('stripe.label', 'Amount (USD)')}</FormLabel>",
      "<FormLabel htmlFor=\"stripe-amount-usd\">{tr('stripe.label', 'Amount (USD)')}</FormLabel>",
    )
    .replace(
      '<Input\n                                type="number"',
      '<Input\n                                id="stripe-amount-usd"\n                                type="number"',
    )
    .replace(
      ".min(minTopUpUSD, tr('stripe.min', `Minimum is $${minTopUpUSD}`, { value: minTopUpUSD }))",
      ".min(minTopUpUSD, { error: tr('stripe.min', `Minimum is $${minTopUpUSD}`, { value: minTopUpUSD }) })",
    )
    .replace(
      ".max(100000, tr('stripe.max', 'Amount too large'))",
      ".max(100000, { error: tr('stripe.max', 'Amount too large') })",
    ),
);

// Tailwind 4 cannot @apply an arbitrary custom component selector. The actual
// responsive card rules remain in mobile.css for .mobile-table, while the
// generic .data-table class retains its own explicit component declarations.
transform('web/modern/src/index.css', (text) =>
  text.replace(
    `
  /* Mobile table enhancements */
  @media (max-width: 768px) {
    .data-table {
      @apply mobile-table;
    }
  }
`,
    '\n',
  ),
);

// Vite 8 uses Rolldown. Preserve the existing chunk strategy with the supported
// function form and remove Rollup's obsolete treeshake preset property.
write(
  'web/modern/vite.config.ts',
  `/// <reference types="vitest" />
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
  const normalizedId = moduleId.replaceAll('\\\\', '/');
  if (!normalizedId.includes('/node_modules/')) return undefined;

  for (const [chunkName, packages] of Object.entries(manualChunkGroups)) {
    if (packages.some((packageName) => normalizedId.includes(`/node_modules/${packageName}/`))) {
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
`,
);

console.log('Finalized Modern latest-stable compatibility configuration.');
