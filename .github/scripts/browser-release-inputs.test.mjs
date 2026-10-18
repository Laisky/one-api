import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { copyFileSync, mkdtempSync, mkdirSync, readFileSync, renameSync, rmSync, symlinkSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import path from 'node:path';
import test from 'node:test';
import { fileURLToPath, pathToFileURL } from 'node:url';

const manifestPath = 'web/legacy/browser-release-inputs.json';
const commandPath = '.github/scripts/browser-release-inputs.mjs';

/** fixture creates a Git-free Docker-shaped tree with the real discovery code and isolated public inputs. */
async function fixture(t) {
  const root = mkdtempSync(path.join(tmpdir(), 'oneapi-manifest-'));
  t.after(() => rmSync(root, { recursive: true, force: true }));
  const manifest = {};
  for (const theme of ['modern', 'air', 'berry']) {
    for (const directory of ['src', 'public']) mkdirSync(path.join(root, 'web', theme, directory), { recursive: true });
    writeFileSync(path.join(root, 'web', theme, 'src/main.js'), 'export const reviewed = true;\n');
    writeFileSync(path.join(root, 'web', theme, 'public/logo.txt'), 'public asset');
    writeFileSync(path.join(root, 'web', theme, 'package.json'), '{}');
    writeFileSync(path.join(root, 'web', theme, 'yarn.lock'), '# locked dependencies');
    manifest[theme] = {
      roots: ['web/' + theme + '/src', 'web/' + theme + '/public'],
      files: [manifestPath, 'web/' + theme + '/package.json', 'web/' + theme + '/yarn.lock'],
    };
  }
  mkdirSync(path.join(root, 'web/legacy'), { recursive: true });
  mkdirSync(path.join(root, '.github/scripts'), { recursive: true });
  copyFileSync(new URL('../../web/legacy/browser-release.mjs', import.meta.url), path.join(root, 'web/legacy/browser-release.mjs'));
  copyFileSync(new URL('./browser-release-inputs.mjs', import.meta.url), path.join(root, commandPath));
  writeFileSync(path.join(root, manifestPath), JSON.stringify(manifest, null, 2) + '\n');
  const api = await import(pathToFileURL(path.join(root, 'web/legacy/browser-release.mjs')).href);
  return {
    root, api, manifest,
    bytes: () => readFileSync(path.join(root, manifestPath), 'utf8'),
    run: (...args) => spawnSync(process.execPath, [commandPath, ...args], { cwd: root, encoding: 'utf8' }),
    write(name, content = 'synthetic content') {
      mkdirSync(path.dirname(path.join(root, name)), { recursive: true });
      writeFileSync(path.join(root, name), content);
    },
  };
}

test('all shipped declared inputs pass the production guard without frontend dependencies or Git reads', () => {
  const result = spawnSync(process.execPath, [fileURLToPath(new URL('./browser-release-inputs.mjs', import.meta.url)), 'check'], { encoding: 'utf8' });
  assert.equal(result.status, 0, result.stderr);
});

test('new ordinary source, tests, nonimported Tailwind/Sass inputs and public assets need no per-file registration', async (t) => {
  const f = await fixture(t);
  const original = f.bytes();
  const before = Object.fromEntries(['modern', 'air', 'berry'].map((theme) => [theme, f.api.browserRelease(theme, {})]));
  const paths = [
    'web/modern/src/RegistrationPasswordFields.tsx',
    'web/modern/src/testdata/registration-passwords.json',
    'web/modern/src/registration-password.test.ts',
    'web/modern/src/nonimported-tailwind.tsx',
    'web/modern/src/new-sass-partial.scss',
    'web/modern/public/.well-known/new.json',
  ];
  for (const name of paths) {
    const release = f.api.browserRelease('modern', {});
    f.write(name);
    assert.notEqual(f.api.browserRelease('modern', {}), release, name);
  }
  assert.equal(f.run('check').status, 0);
  assert.equal(f.bytes(), original);
  assert.notEqual(f.api.browserRelease('modern', {}), before.modern);
  for (const theme of ['air', 'berry']) assert.equal(f.api.browserRelease(theme, {}), before[theme]);
  const inputs = f.api.browserReleaseInputs('modern');
  assert.deepEqual(inputs, [...inputs].sort());
  for (const name of paths) {
    assert.ok(inputs.includes(name));
    assert.equal(f.api.releaseInputGuard('modern').load(path.join(f.root, name)), null);
  }
});

test('source deletion is discovered but input additions, deletions or mutations after a snapshot fail before emit', async (t) => {
  for (const action of ['add', 'delete', 'mutate', 'lock', 'config']) {
    const f = await fixture(t);
    const contract = f.api.browserBuildContract('modern', { mode: 'production' });
    assert.doesNotThrow(() => contract.plugin.buildStart());
    if (action === 'add') f.write('web/modern/src/new.ts');
    if (action === 'delete') rmSync(path.join(f.root, 'web/modern/src/main.js'));
    if (action === 'mutate') f.write('web/modern/src/main.js');
    if (action === 'lock') f.write('web/modern/yarn.lock');
    if (action === 'config') f.write('web/modern/package.json');
    assert.throws(() => contract.plugin.generateBundle(), /changed after release snapshot/);
    assert.notEqual(f.api.browserRelease('modern', { mode: 'production' }), contract.release);
    assert.equal(f.run('check').status, 0);
  }
});

test('private segments, credential files, outside-root imports and unsupported maintenance commands fail without disclosure or mutation', async (t) => {
  const f = await fixture(t);
  const original = f.bytes();
  for (const name of ['web/modern/.env', 'private/secret.js', 'web/modern/unlisted.js']) f.write(name, 'synthetic-private-secret');
  const guard = f.api.releaseInputGuard('modern');
  for (const name of ['web/modern/.env', 'private/secret.js', 'web/modern/unlisted.js'])
    assert.throws(() => guard.load(path.join(f.root, name)), /Unlisted browser build input/);
  for (const name of ['web/modern/src/.private.js', 'web/modern/src/.env.production', 'web/modern/src/server.key', 'web/modern/public/credentials.json', 'web/modern/public/private.pem', 'web/modern/src/id_dsa', 'web/modern/src/id_ecdsa', 'web/modern/public/id_ed25519_sk']) {
    f.write(name, 'synthetic-private-secret');
    const result = f.run('check');
    assert.equal(result.status, 1, name);
    assert.doesNotMatch(result.stderr, /synthetic-private-secret/);
    assert.equal(f.bytes(), original);
    rmSync(path.join(f.root, name));
  }
  for (const args of [['add', 'modern', 'web/modern/src/main.js'], ['remove', 'modern', manifestPath], ['check', 'modern'], ['update']]) {
    assert.equal(f.run(...args).status, 1);
    assert.equal(f.bytes(), original);
  }
  assert.equal(f.run('check').status, 0);
});

test('private directories fail before enumerating their filenames or reading contents', async (t) => {
  for (const directory of ['web/modern/src/private', 'web/modern/public/private']) {
    const f = await fixture(t);
    const original = f.bytes();
    f.write(directory + '/synthetic-private-filename.js', 'synthetic-private-secret');
    const result = f.run('check');
    assert.equal(result.status, 1);
    assert.match(result.stderr, /Invalid browser release input/);
    assert.doesNotMatch(result.stderr, /synthetic-private-filename|synthetic-private-secret/);
    assert.equal(f.bytes(), original);
  }
});

test('file, directory and theme-ancestor symlinks fail before exposing private target names or contents', async (t) => {
  for (const kind of ['file', 'directory', 'ancestor']) {
    const f = await fixture(t);
    const original = f.bytes();
    f.write('private/src/synthetic-private-filename.js', 'synthetic-private-secret');
    mkdirSync(path.join(f.root, 'private/public'));
    if (kind === 'file') symlinkSync(path.join(f.root, 'private/src/synthetic-private-filename.js'), path.join(f.root, 'web/modern/src/link.js'));
    if (kind === 'directory') symlinkSync(path.join(f.root, 'private'), path.join(f.root, 'web/modern/src/link'));
    if (kind === 'ancestor') {
      renameSync(path.join(f.root, 'web/modern'), path.join(f.root, 'original-modern'));
      symlinkSync(path.join(f.root, 'private'), path.join(f.root, 'web/modern'));
    }
    const result = f.run('check');
    assert.equal(result.status, 1);
    assert.match(result.stderr, /Symbolic browser release input/);
    assert.doesNotMatch(result.stderr, /synthetic-private-filename|synthetic-private-secret/);
    assert.equal(f.bytes(), original);
  }
});

test('malformed declarations cannot expand roots, cross themes, omit policy or add private build files', async (t) => {
  const f = await fixture(t);
  const original = f.bytes();
  for (const mutate of [
    (m) => m.modern.files.push(m.modern.files[0]),
    (m) => m.modern.roots.push('private'),
    (m) => m.modern.roots = ['web/modern/src', 'web/modern/src'],
    (m) => m.modern.files = ['web/modern/package.json'],
    (m) => m.modern.files.push('web/air/package.json'),
    (m) => m.modern.files.push('web/modern/src/main.js'),
    (m) => m.modern.files.push('web/modern/.env'),
    (m) => m.default = m.modern,
    (m) => m.air = 'invalid',
  ]) {
    const manifest = JSON.parse(original);
    mutate(manifest);
    f.write(manifestPath, JSON.stringify(manifest));
    const before = f.bytes();
    assert.equal(f.run('check').status, 1);
    assert.equal(f.bytes(), before);
  }
});
