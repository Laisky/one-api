const assert = require('node:assert/strict');
const { createRequire } = require('node:module');
const path = require('node:path');
const { test } = require('node:test');

const theme = process.env.LEGACY_THEME || path.basename(process.cwd());
assert.ok(['air', 'berry'].includes(theme), 'Run from air/berry or set LEGACY_THEME');
const themeRequire = createRequire(path.resolve(__dirname, '../../web', theme, 'package.json'));

// jsonpath pins an old underscore patch in the CRA toolchain. Resolve from
// that actual consumer instead of accidentally testing an unrelated hoist.
test(`${theme}: jsonpath uses the patched underscore without changing queries`, () => {
  const consumerRequire = createRequire(themeRequire.resolve('jsonpath'));
  const version = consumerRequire('underscore/package.json').version.split('.').map(Number);
  assert.equal(version[0], 1, 'The resolution must not silently cross a major version');
  assert.ok(version[1] > 13 || (version[1] === 13 && version[2] >= 8), 'underscore must include the 1.13.8 security fix');
  const underscore = consumerRequire('underscore');
  assert.equal(underscore.template('Hello <%= name %>')({ name: 'One API' }), 'Hello One API');
  const jsonpath = themeRequire('jsonpath');
  assert.deepEqual(jsonpath.query({ items: [{ id: 1 }, { id: 2 }] }, '$.items[*].id'), [1, 2]);
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
