import fs from 'node:fs';
import path from 'node:path';

const repoRoot = process.cwd();
const modernRoot = path.join(repoRoot, 'web', 'modern');

function write(relativePath, content) {
  const target = path.join(repoRoot, relativePath);
  fs.mkdirSync(path.dirname(target), { recursive: true });
  fs.writeFileSync(target, content);
}

const packagePath = path.join(modernRoot, 'package.json');
const packageJson = JSON.parse(fs.readFileSync(packagePath, 'utf8'));

// The @typescript/native alias publishes the TypeScript 7 CLI under the
// standard `tsc` binary. Keep the TS 6 package only as the compiler-API
// sidecar required by ESLint and other tooling.
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

console.log('Finalized Modern latest-stable toolchain configuration.');
