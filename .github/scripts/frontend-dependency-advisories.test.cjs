const assert = require('node:assert/strict');
const { spawnSync } = require('node:child_process');
const crypto = require('node:crypto');
const fs = require('node:fs');
const http = require('node:http');
const { createRequire } = require('node:module');
const path = require('node:path');
const { test } = require('node:test');

// Behavioural regressions for the advisories cleared on 2026-11-04. Each test drives the
// copy of the package that its real consumer resolves in the selected theme, so a
// lockfile or resolution change that reintroduces a vulnerable copy fails here even if
// the registry audit is unavailable or incomplete.
const theme = process.env.FRONTEND_THEME || path.basename(process.cwd());
assert.ok(['air', 'berry', 'modern'].includes(theme), 'Run from a frontend or set FRONTEND_THEME');
const themeRequire = createRequire(path.resolve(__dirname, '../../web', theme, 'package.json'));

// consumerRequire resolves modules exactly as the installed consumer package does.
function consumerRequire(...chain) {
  let current = themeRequire;
  for (const name of chain) current = createRequire(current.resolve(name));
  return current;
}

// zodCopies returns every installed zod the theme's consumers load, labelled by consumer.
function zodCopies() {
  const copies = [['eslint-plugin-react-hooks', consumerRequire('eslint-plugin-react-hooks')('zod')]];
  if (theme === 'modern') copies.push(['modern', themeRequire('zod')]);
  return copies;
}

// minimatchConsumerChain maps each lint-time minimatch consumer to its resolution path.
const minimatchConsumerChain = {
  eslint: ['eslint'],
  '@eslint/config-array': ['eslint', '@eslint/config-array'],
  ...(theme === 'modern' ? { '@typescript-eslint/typescript-estree': ['typescript-eslint', '@typescript-eslint/typescript-estree'] } : {}),
};

// minimatchCopies returns every installed minimatch the theme's lint toolchain loads.
function minimatchCopies() {
  return Object.entries(minimatchConsumerChain).map(([consumer, chain]) => [consumer, consumerRequire(...chain)('minimatch')]);
}

test(`${theme}: zod rejects oversized bounded collections before parsing their elements (GHSA-mrhx-c7xc-m3w5)`, () => {
  for (const [consumer, { z }] of zodCopies()) {
    const failing = Array.from({ length: 20000 }, (_, index) => index);
    const collections = [
      ['array', z.array(z.string()).max(10), failing],
      ['set', z.set(z.string()).max(10), new Set(failing)],
      ['map', z.map(z.string(), z.string()).max(10), new Map(failing.map(value => [value, value]))],
    ];
    for (const [kind, schema, oversized] of collections) {
      const result = schema.safeParse(oversized);
      assert.equal(result.success, false, `${consumer}: an oversized ${kind} must fail`);
      assert.deepEqual(result.error.issues.map(issue => issue.code), ['too_big'],
        `${consumer}: an oversized ${kind} must fail on its bound without one issue per element ` +
        `(kept ${result.error.issues.length})`);
    }

    // Collections within their bound are still parsed element by element.
    const bounded = z.array(z.string()).max(10);
    assert.deepEqual(bounded.parse(['a', 'b']), ['a', 'b'], `${consumer}: valid arrays still parse`);
    assert.deepEqual(bounded.safeParse(['a', 1]).error.issues.map(issue => issue.path), [[1]],
      `${consumer}: an invalid element within the bound is still reported`);
    assert.equal(z.set(z.string()).min(3).safeParse(new Set(['a'])).error.issues[0].code, 'too_small',
      `${consumer}: lower bounds still apply`);
  }
});

// reviewedMinimatchConsumers are the packages whose minimatch use was checked against the
// v11 API, and which package.json therefore resolves to minimatch 11. Any other consumer
// must be reviewed before it may share the override.
const reviewedMinimatchConsumers = theme === 'modern'
  ? ['@eslint/config-array', '@typescript-eslint/typescript-estree', 'eslint']
  : ['@eslint/config-array', 'eslint'];

// minimatchV11RemovedApi matches the minimatch 10 APIs that 11.0 removed.
const minimatchV11RemovedApi = /\bmakeRe\b|\bmatchOne\b|\bslashSplit\b|\blevelTwoFileOptimize\b|\bmaxExtglobRecursion\b|\.regexp\b|minimatch\.AST\b/;

// installedPackages yields the directory and manifest of every installed package.
function* installedPackages(dir) {
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    if (!entry.isDirectory() || entry.name.startsWith('.')) continue;
    const full = path.join(dir, entry.name);
    if (entry.name.startsWith('@')) {
      yield* installedPackages(full);
      continue;
    }
    const manifest = path.join(full, 'package.json');
    if (fs.existsSync(manifest)) yield [full, JSON.parse(fs.readFileSync(manifest, 'utf8'))];
    const nested = path.join(full, 'node_modules');
    if (fs.existsSync(nested)) yield* installedPackages(nested);
  }
}

// sourceFiles yields a package's own JavaScript files, excluding nested dependencies.
function* sourceFiles(dir) {
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory() && entry.name !== 'node_modules') yield* sourceFiles(full);
    else if (entry.isFile() && /\.(?:c|m)?js$/.test(entry.name)) yield full;
  }
}

test(`${theme}: only reviewed packages consume the minimatch 11 override, through APIs it keeps`, () => {
  const consumers = new Map();
  for (const [dir, manifest] of installedPackages(path.resolve(__dirname, '../../web', theme, 'node_modules'))) {
    if ({ ...manifest.dependencies, ...manifest.optionalDependencies }.minimatch) consumers.set(manifest.name, dir);
  }
  assert.deepEqual([...consumers.keys()].sort(), reviewedMinimatchConsumers,
    'A new minimatch consumer must be reviewed against the v11 API before sharing the resolution');
  for (const [name, dir] of consumers) {
    for (const file of sourceFiles(dir)) {
      const source = fs.readFileSync(file, 'utf8');
      if (!/require\(["']minimatch["']\)|from ["']minimatch["']/.test(source)) continue;
      assert.doesNotMatch(source, minimatchV11RemovedApi, `${name}: ${path.relative(dir, file)} uses a minimatch API removed in 11`);
    }
  }
});

test(`${theme}: lint-time minimatch bounds **/.. amplification and keeps its consumer API (GHSA-g3hv-75wg-m249)`, () => {
  for (const [consumer, { Minimatch, minimatch }] of minimatchCopies()) {
    // Twenty `**/../x/y` portions double the glob set twenty times. Run it in a child
    // process: the vulnerable versions spin synchronously, which no test timeout interrupts.
    const entry = consumerRequire(...minimatchConsumerChain[consumer]).resolve('minimatch');
    const probe = spawnSync(process.execPath, ['-e', `
      const { Minimatch } = require(${JSON.stringify(entry)});
      const started = process.hrtime.bigint();
      const matcher = new Minimatch('**/../x/y/'.repeat(20) + 'z', { optimizationLevel: 2 });
      console.log(JSON.stringify({ set: matcher.set.length, ms: Number(process.hrtime.bigint() - started) / 1e6 }));
    `], { encoding: 'utf8', timeout: 20000 });
    assert.equal(probe.status, 0, `${consumer}: **/.. preprocessing must finish (signal ${probe.signal})`);
    const { set } = JSON.parse(probe.stdout);
    assert.ok(set <= 100000, `${consumer}: **/.. preprocessing must stop at braceExpandMax (produced ${set})`);

    // The exact calls eslint, @eslint/config-array and typescript-estree make.
    const files = new Minimatch('src/**/*.{js,jsx}', { dot: true });
    assert.equal(files.match('src/pages/a/b.jsx'), true, `${consumer}: nested files match`);
    assert.equal(files.match('src/.hidden/c.js'), true, `${consumer}: dot directories match with dot`);
    assert.equal(files.match('test/a.js'), false, `${consumer}: unrelated files do not match`);
    assert.equal(files.match('src/pages', true), true, `${consumer}: partial matching admits parent directories`);
    assert.equal(files.match('test', true), false, `${consumer}: partial matching rejects unrelated directories`);
    assert.equal(new Minimatch('!dist/**', { dot: true }).negate, true, `${consumer}: negation is reported`);
    assert.equal(files.pattern, 'src/**/*.{js,jsx}', `${consumer}: the source pattern is retained`);
    assert.equal(minimatch('scripts/x.ts', 'scripts/*.ts', { dot: true }), true, `${consumer}: minimatch() matches`);
  }
});

// serveBody answers every request with body and runs fn against the loopback URL.
async function serveBody(body, fn) {
  const server = http.createServer((_, res) => {
    res.setHeader('Content-Type', 'text/plain');
    res.end(body);
  });
  await new Promise(resolve => server.listen(0, '127.0.0.1', resolve));
  try {
    await fn(`http://127.0.0.1:${server.address().port}/asset.js`);
  } finally {
    await new Promise(resolve => server.close(resolve));
  }
}

test(`${theme}: jsdom's undici rejects a tampered body pinned with an uppercase SRI token (GHSA-f72v-6w45-q472)`, async () => {
  const { fetch } = consumerRequire('jsdom')('undici');
  const digest = body => crypto.createHash('sha256').update(body).digest('base64');
  await serveBody('tampered', async url => {
    for (const token of ['sha256', 'SHA256', 'Sha256']) {
      await assert.rejects(fetch(url, { integrity: `${token}-${digest('expected')}` }),
        `a ${token} digest of different bytes must be rejected`);
      const accepted = await fetch(url, { integrity: `${token}-${digest('tampered')}` });
      assert.equal(await accepted.text(), 'tampered', `a matching ${token} digest is still accepted`);
    }
  });
});
