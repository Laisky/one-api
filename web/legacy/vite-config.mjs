import { readdirSync, readFileSync } from 'node:fs';
import path from 'node:path';

/** removeInvalidSemanticRule removes browser-discarded rules from Semantic UI 2.5. */
export function removeInvalidSemanticRule(rule) {
  const file = rule.source?.input?.file?.replaceAll('\\', '/') || '';
  const invalidSelectors = new Set([
    '[data-tooltip][data-inverted]:after.header',
    '.ui.labels a.active.label:ActiveHover:before,a.ui.active.label:ActiveHover:before'.replaceAll(' ', ''),
  ]);
  if (file.includes('/node_modules/semantic-ui-css/') &&
      invalidSelectors.has(rule.selector.replace(/\s+/g, ''))) {
    // These selectors are already discarded by browsers. Keep strict CSS
    // processing and leave first-party CSS and all valid vendor rules intact.
    rule.remove();
  }
}

/** legacyConfig returns the shared build contract without exposing server secrets. */
export function legacyConfig({ root, mode, react, svgr, transformWithOxc, publicEnv = {}, runtime = process.env }) {
  const theme = path.basename(root);
  if (!['air', 'berry'].includes(theme)) throw new Error(`Unsupported legacy theme: ${theme}`);
  const manifest = JSON.parse(readFileSync(path.join(root, 'package.json'), 'utf8'));
  const src = path.join(root, 'src');
  const alias = readdirSync(src, { withFileTypes: true }).map(entry => ({
    find: entry.isDirectory() ? entry.name : entry.name.replace(/\.[^.]+$/, ''),
    replacement: path.join(src, entry.name),
  }));
  if (theme === 'air') {
    // VRender imports the ESM default, but roughjs's browser entry is UMD.
    // Select its published ESM bundle without replacing or externalizing it.
    alias.unshift({ find: /^roughjs$/, replacement: 'roughjs/bundled/rough.esm.js' });
  }
  const port = Number(runtime.PORT || (theme === 'air' ? 3002 : 3003));
  if (!Number.isInteger(port) || port < 1 || port > 65535) throw new Error('PORT must be an integer from 1 to 65535');
  const publicURL = runtime.PUBLIC_URL || '/';
  const defines = Object.fromEntries(['REACT_APP_SERVER', 'REACT_APP_VERSION'].map(name => [
    `process.env.${name}`, JSON.stringify(runtime[name] ?? publicEnv[name] ?? ''),
  ]));
  return {
    root,
    base: publicURL,
    plugins: [
      {
        name: 'legacy-jsx-source',
        enforce: 'pre',
        // Preserve CRA's JSX-in-.js contract, paths, and source maps.
        async transform(code, id) {
          const file = id.split('?')[0];
          if (!file.startsWith(src + path.sep) || !file.endsWith('.js')) return null;
          return transformWithOxc(code, `${file}.jsx`, { jsx: { runtime: 'automatic' } });
        },
      },
      react(),
      svgr(),
    ],
    resolve: { alias, dedupe: ['react', 'react-dom', 'react-router', 'react-router-dom'] },
    define: { ...defines, 'process.env.PUBLIC_URL': JSON.stringify(publicURL.replace(/\/$/, '')) },
    css: { postcss: { plugins: [{ postcssPlugin: 'semantic-ui-invalid-selector', Rule: removeInvalidSemanticRule }] } },
    build: {
      outDir: path.resolve(root, '../build', theme),
      emptyOutDir: true,
      sourcemap: runtime.GENERATE_SOURCEMAP === 'true' || mode === 'development',
    },
    optimizeDeps: { rolldownOptions: { moduleTypes: { '.js': 'jsx' } } },
    server: {
      host: runtime.HOST || '127.0.0.1',
      port,
      strictPort: true,
      proxy: { '/api': { target: runtime.PROXY_TARGET || manifest.proxy, changeOrigin: true } },
      watch: runtime.CHOKIDAR_USEPOLLING === '1' ? { usePolling: true } : undefined,
    },
    preview: { host: '127.0.0.1', strictPort: true },
  };
}
