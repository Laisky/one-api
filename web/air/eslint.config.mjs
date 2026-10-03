import globals from 'globals';
import reactHooks from 'eslint-plugin-react-hooks';

export default [{
  files: ['src/**/*.{js,jsx}'],
  languageOptions: {
    ecmaVersion: 'latest', sourceType: 'module',
    parserOptions: { ecmaFeatures: { jsx: true } },
    globals: { ...globals.browser, process: 'readonly', React: 'readonly' },
  },
  plugins: { 'react-hooks': reactHooks },
  rules: {
    'no-undef': 'error', 'no-dupe-args': 'error', 'no-dupe-keys': 'error',
    'no-unreachable': 'error', 'no-unsafe-finally': 'error',
    'react-hooks/rules-of-hooks': 'error',
  },
}];
