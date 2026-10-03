const assert = require('node:assert/strict');
const { createRequire } = require('node:module');
const path = require('node:path');
const { test } = require('node:test');

const theme = process.env.LEGACY_THEME || path.basename(process.cwd());
assert.ok(['air', 'berry'].includes(theme), 'Run from air/berry or set LEGACY_THEME');
const themeRequire = createRequire(path.resolve(__dirname, '../../web', theme, 'package.json'));

// Retiring CRA removes its vulnerable transitive build chain entirely.
test(`${theme}: the retired CRA dependency graph cannot return`, () => {
  for (const name of ['react-scripts', 'webpack-dev-server', 'jsonpath']) {
    assert.throws(() => themeRequire.resolve(name), { code: 'MODULE_NOT_FOUND' });
  }
});

if (theme === 'air') {
  test('air: the chart dependency minimist cannot pollute object prototypes', () => {
    const consumerRequire = createRequire(themeRequire.resolve('geojson-flatten'));
    const minimist = consumerRequire('minimist');
    const marker = 'oneApiDependencyPollution';
    for (const prefix of ['__proto__', 'constructor.prototype']) {
      try {
        minimist([`--${prefix}.${marker}`, 'yes']);
        assert.equal(Object.prototype[marker], undefined, 'minimist must not modify Object.prototype');
        assert.equal(({})[marker], undefined, 'ordinary objects must remain unpolluted');
      } finally {
        delete Object.prototype[marker];
      }
    }
    assert.deepEqual(minimist(['--name', 'One API', '--count', '2']), { _: [], name: 'One API', count: 2 });
  });
}
