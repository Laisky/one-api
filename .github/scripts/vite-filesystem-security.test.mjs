import assert from 'node:assert/strict';
import { after, before, test } from 'node:test';
import { mkdtemp, writeFile, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import path from 'node:path';
import { createRequire } from 'node:module';
import { pathToFileURL } from 'node:url';

const theme = process.env.FRONTEND_THEME || path.basename(process.cwd());
assert.ok(['air', 'berry', 'modern'].includes(theme), 'Run from a frontend or set FRONTEND_THEME');
const themeRequire = createRequire(path.resolve(import.meta.dirname, '../../web', theme, 'package.json'));
const { createServer } = await import(pathToFileURL(themeRequire.resolve('vite')).href);
let fixture;
let server;
let origin;

before(async () => {
  fixture = await mkdtemp(path.join(tmpdir(), 'oneapi-vite-fs-'));
  await writeFile(path.join(fixture, '.env'), 'LEAK-synthetic-fixture-only');
  await writeFile(path.join(fixture, 'ok.js'), 'export const fixture = "allowed-fixture";');
  await writeFile(path.join(fixture, 'ok.wasm'), Buffer.from([0, 97, 115, 109, 1, 0, 0, 0]));
  server = await createServer({
    configFile: false, envFile: false, root: fixture, publicDir: false, logLevel: 'silent',
    optimizeDeps: { noDiscovery: true },
    server: { host: '127.0.0.1', port: 0, strictPort: true,
      fs: { strict: true, allow: [fixture], deny: ['**/.env'] } },
  });
  await server.listen();
  const address = server.httpServer.address();
  assert.equal(address.address, '127.0.0.1');
  origin = 'http://127.0.0.1:' + address.port;
});

after(async () => {
  if (server) await server.close();
  if (fixture) {
    assert.equal(path.dirname(fixture), tmpdir());
    assert.ok(path.basename(fixture).startsWith('oneapi-vite-fs-'));
    await rm(fixture, { recursive: true });
  }
});

test(theme + ': allowed JavaScript remains available', async () => {
  const response = await fetch(origin + '/ok.js', { signal: AbortSignal.timeout(3000) });
  assert.equal(response.status, 200);
  assert.match(await response.text(), /allowed-fixture/);
});

test(theme + ': ordinary denied-file request stays forbidden', async () => {
  const response = await fetch(origin + '/.env', { signal: AbortSignal.timeout(3000) });
  assert.equal(response.status, 403);
  assert.doesNotMatch(await response.text(), /LEAK-synthetic/);
});

test(theme + ': allowed Wasm instance remains available', async () => {
  const response = await fetch(origin + '/ok.wasm?import&vite-wasm-instance',
    { signal: AbortSignal.timeout(3000) });
  assert.equal(response.status, 200);
});

test(theme + ': Wasm transform cannot expose bytes of a denied file', async () => {
  const response = await fetch(origin + '/.env?import&vite-wasm-instance',
    { signal: AbortSignal.timeout(3000) });
  const body = await response.text();
  const leaksMarker = /4c 45 41 4b|LEAK-synthetic/i.test(body);
  assert.equal(response.status, 403, 'Denied Wasm request must be forbidden; synthetic byte leak=' + leaksMarker);
  assert.equal(leaksMarker, false);
});
