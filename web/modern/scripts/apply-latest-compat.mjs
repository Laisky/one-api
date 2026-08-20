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

function walk(directory) {
  return fs.readdirSync(directory, { withFileTypes: true }).flatMap((entry) => {
    const target = path.join(directory, entry.name);
    return entry.isDirectory() ? walk(target) : [target];
  });
}

// TypeScript 7 intentionally no longer exposes the compiler API. Keep the
// native TS 7 CLI while providing the official TS 6 API package to ESLint and
// other compiler-API consumers.
const packagePath = path.join(modernRoot, 'package.json');
const packageJson = JSON.parse(fs.readFileSync(packagePath, 'utf8'));
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
  new Set(['node', ...(tsconfig.compilerOptions.types ?? [])]),
);
fs.writeFileSync(tsconfigPath, `${JSON.stringify(tsconfig, null, 2)}\n`);

// Resolver v5 models raw schema input and parsed output separately. Existing
// Modern forms already normalize controlled values and deliberately expose one
// stable form shape. Keep that contract in one audited adapter instead of
// scattering unsafe casts throughout every form.
write(
  'web/modern/src/lib/zod-resolver.ts',
  `import { zodResolver as createZodResolver } from '@hookform/resolvers/zod';\nimport type { FieldValues, Resolver } from 'react-hook-form';\n\n/**\n * Preserve Modern's existing single-shape React Hook Form contract while\n * allowing Zod 4 schemas to coerce and default values internally.\n */\nexport function zodResolver<TFieldValues extends FieldValues = any>(\n  schema: any,\n  schemaOptions?: any,\n  resolverOptions?: any,\n): Resolver<TFieldValues> {\n  return createZodResolver(\n    schema,\n    schemaOptions,\n    resolverOptions,\n  ) as Resolver<TFieldValues>;\n}\n`,
);

for (const absolutePath of walk(sourceRoot)) {
  if (!/\.(?:ts|tsx)$/.test(absolutePath)) continue;
  const relativePath = path.relative(repoRoot, absolutePath);
  replaceInFile(
    relativePath,
    "import { zodResolver } from '@hookform/resolvers/zod';",
    "import { zodResolver } from '@/lib/zod-resolver';",
  );
  replaceInFile(
    relativePath,
    "import type { ColumnDef } from '@tanstack/react-table';",
    "import type { LegacyColumnDef as ColumnDef } from '@tanstack/react-table/legacy';",
  );
}

replaceInFile(
  'web/modern/src/components/ui/enhanced-data-table.tsx',
  "import type { ColumnDef, SortingState } from '@tanstack/react-table';\nimport { flexRender, getCoreRowModel, useReactTable } from '@tanstack/react-table';",
  "import { flexRender, type SortingState } from '@tanstack/react-table';\nimport { getCoreRowModel, useLegacyTable as useReactTable } from '@tanstack/react-table/legacy';\nimport type { LegacyColumnDef as ColumnDef } from '@tanstack/react-table/legacy';",
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
  { required: true },
);

replaceInFile(
  'web/modern/src/pages/settings/SettingsPage.tsx',
  'user.role',
  '(user?.role ?? 0)',
);

const setupPath = 'web/modern/src/test/setup.ts';
const setup = read(setupPath).replace(/\bglobal\./g, 'globalThis.');
write(setupPath, setup);

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
  { required: true },
);

console.log('Applied Modern latest-stable compatibility migrations.');
