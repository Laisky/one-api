import assert from 'node:assert/strict';
import { test } from 'node:test';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { legacyConfig, removeInvalidSemanticRule } from '../../web/legacy/vite-config.mjs';

const repository = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '../..');
/** config constructs the real build configuration with harmless plugin factories. */
function config(theme, runtime = {}, publicEnv = {}) {
  return legacyConfig({
    root: path.join(repository, 'web', theme), mode: 'production', runtime, publicEnv,
    react: () => ({ name: 'react' }), svgr: () => ({ name: 'svgr' }), transformWithOxc: () => ({}),
  });
}
for (const theme of ['air', 'berry']) {
  test(`${theme}: build outputs cannot erase another theme`, () => {
    const value = config(theme);
    assert.equal(value.build.outDir, path.join(repository, 'web/build', theme));
    assert.equal(value.build.emptyOutDir, true);
    assert.equal(value.server.host, '127.0.0.1');
    assert.equal(value.server.strictPort, true);
    assert.ok(value.resolve.dedupe.includes('react'));
  });
  test(`${theme}: only intended public constants enter client bundles`, () => {
    const value = config(theme, { REACT_APP_VERSION: 'release-42', DATABASE_PASSWORD: 'server-only-secret', PORT: '3010' },
      { REACT_APP_SERVER: 'https://api.example.test', REACT_APP_PRIVATE: 'not-automatically-exposed' });
    assert.equal(value.define['process.env.REACT_APP_VERSION'], '"release-42"');
    assert.equal(value.define['process.env.REACT_APP_SERVER'], '"https://api.example.test"');
    assert.ok(!JSON.stringify(value.define).includes('server-only-secret'));
    assert.ok(!JSON.stringify(value.define).includes('not-automatically-exposed'));
    assert.equal(value.server.port, 3010);
    assert.equal(value.define['process.env.PUBLIC_URL'], '""');
  });
}
test('build rejects invalid port configurations', () => {
  for (const PORT of ['NaN', '-1', '0', '65536', '3.5']) assert.throws(() => config('air', { PORT }));
});
test('Semantic UI correction removes only the known invalid third-party rule', () => {
  let removed = 0;
  const selector = '[data-tooltip][data-inverted]:after .header';
  const rule = (file, value = selector) => ({ selector: value, source: { input: { file } }, remove: () => removed++ });
  removeInvalidSemanticRule(rule('/app/node_modules/semantic-ui-css/semantic.min.css'));
  assert.equal(removed, 1);
  removeInvalidSemanticRule(rule('/app/src/own.css'));
  removeInvalidSemanticRule(rule('/app/node_modules/semantic-ui-css/semantic.min.css', '[data-tooltip]:after'));
  assert.equal(removed, 1, 'Never weaken other selectors or first-party CSS validation');
});
