import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { copyFileSync, mkdtempSync, mkdirSync, readFileSync, rmSync, symlinkSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import path from 'node:path';
import test from 'node:test';

const manifestPath = 'web/legacy/browser-release-inputs.json';
const commandPath = '.github/scripts/browser-release-inputs.mjs';

/** fixture creates a Docker-shaped web tree with real guard code and an isolated maintenance command. */
function fixture(t) {
  const root = mkdtempSync(path.join(tmpdir(), 'oneapi-manifest-'));
  t.after(() => rmSync(root, { recursive: true, force: true }));
  const manifest = {};
  for (const theme of ['modern', 'air', 'berry']) {
    for (const directory of ['src', 'public']) mkdirSync(path.join(root, 'web', theme, directory), { recursive: true });
    const source = 'web/' + theme + '/src/main.js';
    const asset = 'web/' + theme + '/public/logo.txt';
    writeFileSync(path.join(root, source), 'export const reviewed = true;\n');
    writeFileSync(path.join(root, asset), 'reviewed public asset');
    manifest[theme] = [manifestPath, source, asset].sort();
  }
  mkdirSync(path.join(root, 'web/legacy'), { recursive: true });
  mkdirSync(path.join(root, '.github/scripts'), { recursive: true });
  copyFileSync(new URL('../../web/legacy/browser-release.mjs', import.meta.url), path.join(root, 'web/legacy/browser-release.mjs'));
  copyFileSync(new URL('./browser-release-inputs.mjs', import.meta.url), path.join(root, commandPath));
  writeFileSync(path.join(root, manifestPath), JSON.stringify(manifest, null, 2) + '\n');
  return {
    root,
    manifest: () => JSON.parse(readFileSync(path.join(root, manifestPath))),
    bytes: () => readFileSync(path.join(root, manifestPath), 'utf8'),
    run: (...args) => spawnSync(process.execPath, [commandPath, ...args], { cwd: root, encoding: 'utf8' }),
    write(name, content = 'synthetic content') {
      mkdirSync(path.dirname(path.join(root, name)), { recursive: true });
      writeFileSync(path.join(root, name), content);
    },
  };
}

test('all shipped source/public inputs are reviewed and pass the unchanged production guard', () => {
  const result = spawnSync(process.execPath, [new URL('./browser-release-inputs.mjs', import.meta.url).pathname, 'check'], {
    encoding: 'utf8',
  });
  assert.equal(result.status, 0, result.stderr);
});

test('new source, test fixtures and public assets fail until explicitly reviewed, without altering other entries', (t) => {
  const f = fixture(t);
  const original = f.bytes();
  assert.equal(f.run('check').status, 0);
  const paths = [
    'web/modern/src/RegistrationPasswordFields.tsx',
    'web/modern/src/testdata/registration-passwords.json',
    'web/modern/public/.well-known/new.json',
  ];
  for (const name of paths) f.write(name);
  const missing = f.run('check');
  assert.equal(missing.status, 1);
  for (const name of paths) assert.ok(missing.stderr.includes(name));
  assert.equal(f.bytes(), original);
  const before = f.manifest();
  assert.equal(f.run('add', 'modern', ...paths).status, 0);
  assert.deepEqual(f.manifest().air, before.air);
  assert.deepEqual(f.manifest().berry, before.berry);
  assert.deepEqual(f.manifest().modern, [...before.modern, ...paths].sort());
  const approved = f.bytes();
  assert.equal(f.run('add', 'modern', ...paths).status, 0);
  assert.equal(f.bytes(), approved);
  assert.equal(f.run('check').status, 0);
  // The next new source must still fail; no directory wildcard is approved.
  f.write('web/modern/src/another.js');
  assert.equal(f.run('check').status, 1);
});

test('deleted reviewed files fail until deliberately removed; removal never deletes source files', (t) => {
  const f = fixture(t);
  const source = 'web/modern/src/main.js';
  assert.equal(f.run('remove', 'modern', source).status, 0);
  assert.equal(f.run('check').status, 1);
  assert.ok(readFileSync(path.join(f.root, source), 'utf8').includes('reviewed'));
  assert.equal(f.run('add', 'modern', source).status, 0);
  rmSync(path.join(f.root, source));
  assert.equal(f.run('check').status, 1);
  assert.equal(f.run('remove', 'modern', source).status, 0);
  assert.equal(f.run('check').status, 0);
});

test('invalid mixed updates, private paths, unsupported themes and malformed requests never rewrite approval', (t) => {
  const f = fixture(t);
  const original = f.bytes();
  f.write('web/modern/src/review-me.js');
  f.write('web/modern/.env', 'synthetic-private-secret');
  f.write('web/modern/src/.private.js', 'synthetic-private-secret');
  for (const name of [
    'web/modern/.env',
    'web/modern/src/.private.js',
    'web/modern/src/../package.json',
    'web/modern/src/./main.js',
    'web/modern/src//main.js',
    'web/modern/src\\main.js',
    '/etc/private',
    'web/air/src/main.js',
    'web/modern/node_modules/private',
    'web/modern/build/private',
  ]) {
    const result = f.run('add', 'modern', 'web/modern/src/review-me.js', name);
    assert.equal(result.status, 1, name);
    assert.doesNotMatch(result.stderr, /synthetic-private-secret/);
    assert.equal(f.bytes(), original);
  }
  for (const args of [
    ['add', 'default', 'web/modern/src/main.js'],
    ['add', 'modern'],
    ['check', 'modern'],
    ['update'],
    ['remove', 'modern', manifestPath],
    ['remove', 'modern', 'web/modern/src/unreviewed.js'],
  ]) {
    assert.equal(f.run(...args).status, 1);
    assert.equal(f.bytes(), original);
  }
});

test('file, directory and ancestor symlinks fail without approving or reading their private targets', (t) => {
  const f = fixture(t);
  const original = f.bytes();
  f.write('private/secret.js', 'synthetic-private-secret');
  for (const [target, link] of [
    ['private/secret.js', 'web/modern/src/link.js'],
    ['private', 'web/modern/src/linked-directory'],
  ]) {
    symlinkSync(path.join(f.root, target), path.join(f.root, link));
    assert.equal(f.run('check').status, 1);
    const result = f.run('add', 'modern', link + (link.endsWith('.js') ? '' : '/secret.js'));
    assert.equal(result.status, 1);
    assert.doesNotMatch(result.stderr, /synthetic-private-secret/);
    assert.equal(f.bytes(), original);
    rmSync(path.join(f.root, link));
  }
});

test('duplicate and unsupported manifest lists fail closed without mutation', (t) => {
  const f = fixture(t);
  const baseline = f.bytes();
  for (const mutate of [
    (manifest) => manifest.modern.push(manifest.modern[0]),
    (manifest) => {
      manifest.default = manifest.modern;
    },
    (manifest) => {
      manifest.air = 'invalid';
    },
  ]) {
    const manifest = JSON.parse(baseline);
    mutate(manifest);
    f.write(manifestPath, JSON.stringify(manifest));
    const original = f.bytes();
    assert.equal(f.run('check').status, 1);
    assert.equal(f.bytes(), original);
  }
});
