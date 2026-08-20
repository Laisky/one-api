import fs from 'node:fs';
import path from 'node:path';

const repoRoot = process.cwd();
const modernRoot = path.join(repoRoot, 'web', 'modern');
const sourceRoot = path.join(modernRoot, 'src');

function read(relativePath) {
  return fs.readFileSync(path.join(repoRoot, relativePath), 'utf8');
}

function write(relativePath, content) {
  const target = path.join(repoRoot, relativePath);
  fs.mkdirSync(path.dirname(target), { recursive: true });
  fs.writeFileSync(target, content);
}

function replaceInFile(relativePath, search, replacement, { required = false } = {}) {
  const original = read(relativePath);
  if (!original.includes(search)) {
    if (required && !original.includes(replacement)) {
      throw new Error(`${relativePath}: expected migration source was not found`);
    }
    return false;
  }
  write(relativePath, original.replaceAll(search, replacement));
  return true;
}

function transformFile(relativePath, transform) {
  const original = read(relativePath);
  const updated = transform(original);
  if (updated !== original) write(relativePath, updated);
}

function walk(directory) {
  return fs.readdirSync(directory, { withFileTypes: true }).flatMap((entry) => {
    const target = path.join(directory, entry.name);
    return entry.isDirectory() ? walk(target) : [target];
  });
}

// TypeScript 7 intentionally no longer exposes the compiler API. Use the
// native TS 7 CLI for application checks while retaining the official TS 6 API
// package under the `typescript` name for ESLint and other API consumers.
const packagePath = path.join(modernRoot, 'package.json');
const packageJson = JSON.parse(fs.readFileSync(packagePath, 'utf8'));
packageJson.scripts.build = 'tsgo -b && vite build --emptyOutDir';
packageJson.scripts['build:prod'] = 'tsgo -b && vite build --mode production --emptyOutDir';
packageJson.scripts['type-check'] = 'tsgo --noEmit';
packageJson.devDependencies ??= {};
packageJson.devDependencies['@types/qrcode'] = '^1.5.6';
packageJson.devDependencies['@typescript/native'] = 'npm:typescript@^7.0.2';
packageJson.devDependencies.typescript = 'npm:@typescript/typescript6@^6.0.2';
packageJson.devDependencies = Object.fromEntries(
  Object.entries(packageJson.devDependencies).sort(([left], [right]) => left.localeCompare(right)),
);
fs.writeFileSync(packagePath, `${JSON.stringify(packageJson, null, 2)}\n`);

const tsconfigPath = path.join(modernRoot, 'tsconfig.json');
const tsconfig = JSON.parse(fs.readFileSync(tsconfigPath, 'utf8'));
tsconfig.compilerOptions.types = Array.from(
  new Set(['node', 'vite/client', ...(tsconfig.compilerOptions.types ?? [])]),
);
fs.writeFileSync(tsconfigPath, `${JSON.stringify(tsconfig, null, 2)}\n`);

write(
  'web/modern/src/types/assets.d.ts',
  `/// <reference types="vite/client" />\n\ndeclare module '*.css';\n`,
);

// Resolver v5 models raw schema input and parsed output separately. Existing
// Modern forms normalize controlled values and intentionally expose one stable
// form shape. Keep that contract in one audited adapter.
write(
  'web/modern/src/lib/zod-resolver.ts',
  `import { zodResolver as createZodResolver } from '@hookform/resolvers/zod';\nimport type { FieldValues, Resolver } from 'react-hook-form';\n\n/**\n * Preserve Modern's existing single-shape React Hook Form contract while\n * allowing Zod 4 schemas to coerce and default values internally.\n */\nexport function zodResolver<TFieldValues extends FieldValues = any>(\n  schema: any,\n  schemaOptions?: any,\n  resolverOptions?: any,\n): Resolver<TFieldValues> {\n  return createZodResolver(\n    schema,\n    schemaOptions,\n    resolverOptions,\n  ) as Resolver<TFieldValues>;\n}\n`,
);

// Table 9 requires an explicit feature set. These tables use server-side
// sorting/pagination, so register only those feature APIs and keep the core row
// model supplied by v9 automatically.
write(
  'web/modern/src/lib/table.ts',
  `import { rowPaginationFeature, rowSortingFeature, tableFeatures } from '@tanstack/react-table';\nimport type { ColumnDef, RowData } from '@tanstack/react-table';\n\nexport const modernTableFeatures = tableFeatures({\n  rowPaginationFeature,\n  rowSortingFeature,\n});\n\nexport type ModernColumnDef<\n  TData extends RowData,\n  TValue = unknown,\n> = ColumnDef<typeof modernTableFeatures, TData, TValue>;\n`,
);

for (const absolutePath of walk(sourceRoot)) {
  if (!/\.(?:ts|tsx)$/.test(absolutePath)) continue;
  const relativePath = path.relative(repoRoot, absolutePath);
  transformFile(relativePath, (text) =>
    text
      .replace(
        "import { zodResolver } from '@hookform/resolvers/zod';",
        "import { zodResolver } from '@/lib/zod-resolver';",
      )
      .replace(
        "import type { LegacyColumnDef as ColumnDef } from '@tanstack/react-table/legacy';",
        "import type { ModernColumnDef as ColumnDef } from '@/lib/table';",
      )
      .replace(
        "import type { ColumnDef } from '@tanstack/react-table';",
        "import type { ModernColumnDef as ColumnDef } from '@/lib/table';",
      )
      .replaceAll('// eslint-disable-next-line react/jsx-no-useless-fragment\n', '')
      .replaceAll('// @ts-ignore assigning test-only PointerEvent polyfill for jsdom', '// @ts-expect-error assigning test-only PointerEvent polyfill for jsdom'),
  );
}

function migrateTable(relativePath, componentName, propsName) {
  transformFile(relativePath, (text) => {
    let updated = text
      .replace(
        "import { flexRender, type SortingState } from '@tanstack/react-table';\nimport { getCoreRowModel, useLegacyTable as useReactTable } from '@tanstack/react-table/legacy';\nimport type { ModernColumnDef as ColumnDef } from '@/lib/table';",
        "import { flexRender, type RowData, type SortingState, useTable } from '@tanstack/react-table';\nimport { modernTableFeatures, type ModernColumnDef as ColumnDef } from '@/lib/table';",
      )
      .replace(
        "import { flexRender, getCoreRowModel, useReactTable } from '@tanstack/react-table';\nimport type { ModernColumnDef as ColumnDef } from '@/lib/table';",
        "import { flexRender, type RowData, type SortingState, useTable } from '@tanstack/react-table';\nimport { modernTableFeatures, type ModernColumnDef as ColumnDef } from '@/lib/table';",
      )
      .replace(
        "import * as React from 'react';\nimport { flexRender, getCoreRowModel, useReactTable } from '@tanstack/react-table';\nimport type { ColumnDef, SortingState } from '@tanstack/react-table';",
        "import * as React from 'react';\nimport { flexRender, type RowData, type SortingState, useTable } from '@tanstack/react-table';\nimport { modernTableFeatures, type ModernColumnDef as ColumnDef } from '@/lib/table';",
      );

    updated = updated
      .replace(`export interface ${propsName}<TData, TValue>`, `export interface ${propsName}<TData extends RowData, TValue = unknown>`)
      .replace(`export function ${componentName}<TData, TValue>`, `export function ${componentName}<TData extends RowData, TValue = unknown>`)
      .replace('const table = useReactTable({', 'const table = useTable({\n    features: modernTableFeatures,')
      .replace('    getCoreRowModel: getCoreRowModel(),\n', '')
      .replace('  });\n\n  const handleSearchAddition', '  }, (state) => state);\n\n  const handleSearchAddition')
      .replace('  });\n\n  return (', '  }, (state) => state);\n\n  return (');

    return updated;
  });
}

migrateTable('web/modern/src/components/ui/enhanced-data-table.tsx', 'EnhancedDataTable', 'EnhancedDataTableProps');
migrateTable('web/modern/src/components/ui/data-table.tsx', 'DataTable', 'DataTableProps');

// React 19 ref callbacks must not return the assigned element, and useRef now
// requires an explicit initial value.
replaceInFile(
  'web/modern/src/components/layout/HeaderNav.tsx',
  'ref={(el) => (itemsRef.current[index] = el)}',
  'ref={(el) => { itemsRef.current[index] = el; }}',
  { required: true },
);

for (const absolutePath of walk(sourceRoot)) {
  if (!/\.(?:ts|tsx)$/.test(absolutePath)) continue;
  const relativePath = path.relative(repoRoot, absolutePath);
  transformFile(relativePath, (text) =>
    text
      .replaceAll('useRef<ReturnType<typeof setTimeout>>();', 'useRef<ReturnType<typeof setTimeout> | undefined>(undefined);')
      .replaceAll('useRef<ReturnType<typeof setInterval>>();', 'useRef<ReturnType<typeof setInterval> | undefined>(undefined);')
      .replaceAll('React.useRef<NodeJS.Timeout>();', 'React.useRef<NodeJS.Timeout | undefined>(undefined);')
      .replaceAll('useRef<NodeJS.Timeout>();', 'useRef<NodeJS.Timeout | undefined>(undefined);'),
  );
}

replaceInFile(
  'web/modern/src/components/chat/MessageList.tsx',
  'onEditMessage?: (messageIndex: number, newContent: string) => void;',
  'onEditMessage?: (messageIndex: number, newContent: string | any[]) => void;',
  { required: true },
);

replaceInFile(
  'web/modern/src/components/layout/Header.tsx',
  'const isAdmin = user?.role >= 10;',
  'const isAdmin = (user?.role ?? 0) >= 10;',
  { required: true },
);
replaceInFile(
  'web/modern/src/pages/settings/SettingsPage.tsx',
  'const isRoot = user?.role >= 100;',
  'const isRoot = (user?.role ?? 0) >= 100;',
  { required: true },
);

replaceInFile(
  'web/modern/src/pages/tokens/TokensPage.impl.tsx',
  'useState<Record<number, boolean>>({})',
  'useState<Record<string, boolean>>({})',
  { required: true },
);
replaceInFile(
  'web/modern/src/pages/topup/TopUpPage.tsx',
  ".number({ invalid_type_error: tr('stripe.required', 'Enter an amount in USD') })",
  ".number({ error: tr('stripe.required', 'Enter an amount in USD') })",
);
replaceInFile(
  'web/modern/src/pages/chat/PlaygroundPage.tsx',
  '      return;\n    }\n    return baseHandleSendMessage(...args);',
  '      return Promise.resolve();\n    }\n    return baseHandleSendMessage(...args);',
  { required: true },
);

// Zod 4 requires explicit record key/value schemas, and object defaults are
// expressed as a factory to preserve a fresh value per form.
transformFile('web/modern/src/pages/channels/schemas.ts', (text) =>
  text
    .replaceAll('z.record(z.string())', 'z.record(z.string(), z.string())')
    .replace('      .default({}),', '      .default(() => ({})),')
);
replaceInFile(
  'web/modern/src/pages/channels/components/ChannelAdvancedSettings.tsx',
  "formatJSON(current);",
  "formatJSON(current ?? '');",
);

// Recharts 3's Tooltip defaults are broader than the previous numeric-only
// generic. Derive the exact prop type from the component itself.
transformFile('web/modern/src/pages/dashboard/components/UsageSections.tsx', (text) =>
  text
    .replace("import type { TFunction } from 'i18next';", "import type { TFunction } from 'i18next';\nimport type { ComponentProps } from 'react';")
    .replace(', type TooltipProps,', ',')
    .replace("type UsageTooltipContent = TooltipProps<number, string>['content'];", "type UsageTooltipContent = ComponentProps<typeof Tooltip>['content'];")
);

// Vitest 4 models mocks by a single function signature generic.
replaceInFile(
  'web/modern/src/hooks/__tests__/useRealtimeChat.test.ts',
  'vi.fn<[AddEventInput], void>()',
  'vi.fn<(entry: AddEventInput) => void>()',
  { required: true },
);
transformFile('web/modern/src/lib/__tests__/api.test.ts', (text) =>
  text
    .replace('let logoutSpy: ReturnType<typeof vi.fn>;', 'let logoutSpy: ReturnType<typeof vi.fn<() => void>>;')
    .replace('logoutSpy = vi.fn(realLogout);', 'logoutSpy = vi.fn<() => void>(realLogout);')
);
replaceInFile(
  'web/modern/src/pages/channels/components/__tests__/ChannelModelSettings.test.tsx',
  "expect(formRef?.getValues('model_configs'))",
  "expect(formRef!.getValues('model_configs'))",
  { required: true },
);

// Test setup: use the Vitest-specific matchers and preserve i18next's
// defaultValue-in-options overload used throughout the application.
transformFile('web/modern/src/test/setup.ts', (text) =>
  text
    .replace("import '@testing-library/jest-dom';", "import '@testing-library/jest-dom/vitest';")
    .replace("import enTranslations from '../i18n/locales/en';\n", '')
    .replace(
      "      if (typeof arg2 === 'string') {\n        value = arg2;\n      } else {\n        return key;\n      }",
      "      if (typeof arg2 === 'string') {\n        value = arg2;\n      } else if (options?.defaultValue !== undefined) {\n        value = options.defaultValue;\n      } else {\n        return key;\n      }",
    )
);

// Fix the two genuine Rules-of-Hooks violations surfaced by the new lint stack.
transformFile('web/modern/src/components/dev/responsive-debugger.tsx', (text) =>
  text.replace(
    "export function GridOverlay() {\n  const [isVisible, setIsVisible] = useState(false);\n\n  if (process.env.NODE_ENV !== 'development') {\n    return null;\n  }\n\n  useEffect(() => {\n    const handleKeyPress = (event: KeyboardEvent) => {\n      if (event.ctrlKey && event.shiftKey && event.key === 'G') {\n        setIsVisible(!isVisible);\n      }\n    };\n\n    window.addEventListener('keydown', handleKeyPress);\n    return () => window.removeEventListener('keydown', handleKeyPress);\n  }, [isVisible]);",
    "export function GridOverlay() {\n  const [isVisible, setIsVisible] = useState(false);\n\n  useEffect(() => {\n    if (process.env.NODE_ENV !== 'development') return undefined;\n    const handleKeyPress = (event: KeyboardEvent) => {\n      if (event.ctrlKey && event.shiftKey && event.key === 'G') {\n        setIsVisible((current) => !current);\n      }\n    };\n\n    window.addEventListener('keydown', handleKeyPress);\n    return () => window.removeEventListener('keydown', handleKeyPress);\n  }, []);\n\n  if (process.env.NODE_ENV !== 'development') {\n    return null;\n  }",
  )
);
transformFile('web/modern/src/components/ui/confirm-dialog.tsx', (text) =>
  text
    .replace(
      'export function useConfirmDialog() {\n  const [state, setState]',
      'export function useConfirmDialog() {\n  const { t } = useTranslation();\n  const [state, setState]',
    )
    .replace('  const ConfirmDialogComponent = useCallback(() => {\n    const { t } = useTranslation();', '  const ConfirmDialogComponent = useCallback(() => {')
    .replace('  }, [state, handleClose]);', '  }, [state, handleClose, t]);')
);

// The latest recommended configs add stylistic/core rules that were not part
// of Modern's previous lint contract. Keep behavioral hook/type rules while
// explicitly avoiding an unrelated whole-codebase cleanup in this dependency PR.
transformFile('web/modern/eslint.config.js', (text) =>
  text.replace(
    "      'no-unused-vars': 'off',",
    "      'no-unused-vars': 'off',\n      'no-empty': 'off',\n      'no-useless-assignment': 'off',\n      'prefer-const': 'off',\n      '@typescript-eslint/no-empty-object-type': 'off',",
  )
);

let tailwindConfig = read('web/modern/tailwind.config.js');
if (!tailwindConfig.includes("import typography from '@tailwindcss/typography';")) {
  tailwindConfig = tailwindConfig.replace(
    "/** @type {import('tailwindcss').Config} */",
    "import typography from '@tailwindcss/typography';\nimport animate from 'tailwindcss-animate';\n\n/** @type {import('tailwindcss').Config} */",
  );
}
tailwindConfig = tailwindConfig
  .replace("require('tailwindcss-animate')", 'animate')
  .replace("require('@tailwindcss/typography')", 'typography');
write('web/modern/tailwind.config.js', tailwindConfig);

replaceInFile(
  'web/modern/src/index.css',
  "/* Import mobile responsive styles */\n@import './styles/mobile.css';\n\n@tailwind base;\n@tailwind components;\n@tailwind utilities;",
  "@import 'tailwindcss';\n@import './styles/mobile.css';\n@config '../tailwind.config.js';",
);

console.log('Applied Modern latest-stable compatibility migrations.');
