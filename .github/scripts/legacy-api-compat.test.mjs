import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import { createServer } from 'node:http';
import { createRequire } from 'node:module';
import path from 'node:path';
import { test } from 'node:test';
import { fileURLToPath } from 'node:url';
import { createContext, SourceTextModule, SyntheticModule } from 'node:vm';

// Exercise the real legacy helpers and installed Axios without importing their
// UI notification libraries. Each theme runs this through its pretest hook.
// Standalone: LEGACY_THEME=berry node --experimental-vm-modules --test .github/scripts/legacy-api-compat.test.mjs
const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '../..');
const theme = process.env.LEGACY_THEME || path.basename(process.cwd());
assert.ok(['air', 'berry'].includes(theme), 'Run from a legacy theme or set LEGACY_THEME to air or berry');
const require = createRequire(path.join(root, 'web', theme, 'package.json'));
const axios = require('axios');
const helper = theme === 'air' ? 'src/helpers/api.js' : 'src/utils/api.js';

async function fixture(t, pathname = '/dashboard') {
  const server = createServer(async (request, response) => {
    const url = new URL(request.url, 'http://localhost');
    const status = url.pathname === '/api/private' ? 401 : url.pathname === '/api/error' ? 422 : 200;
    let body = '';
    for await (const chunk of request) body += chunk;
    response.writeHead(status, { 'Content-Type': 'application/json' });
    response.end(JSON.stringify({
      ok: status === 200,
      message: status === 422 ? 'validation failed' : 'authentication required',
      method: request.method,
      headers: request.headers,
      query: Object.fromEntries(url.searchParams),
      body: body ? JSON.parse(body) : null,
    }));
  });
  await new Promise((resolve, reject) => {
    server.once('error', reject);
    server.listen(0, '127.0.0.1', resolve);
  });
  t.after(async () => {
    const closed = new Promise((resolve, reject) => server.close(error => error ? reject(error) : resolve()));
    server.closeAllConnections();
    await closed;
  });
  const origin = `http://127.0.0.1:${server.address().port}`;
  const errors = [];
  const redirects = [];
  const actions = [];
  const storage = new Map([['user', 'user'], ['token', 'token'], ['theme', 'dark']]);
  const context = createContext({
    URL,
    Date,
    console,
    process: { env: { REACT_APP_SERVER: origin } },
    window: { location: { origin, pathname, replace: value => redirects.push(value) } },
    localStorage: { removeItem: key => storage.delete(key) },
  });
  const imports = {
    axios: { default: axios },
    './utils': { showError: error => errors.push(error) },
    './common': { showError: error => errors.push(error) },
    'store/index': { store: { dispatch: action => actions.push(action) } },
    'store/actions': { LOGIN: 'LOGIN' },
    config: { default: { basename: '/' } },
  };
  // The override allows a red/green check against the original helper while
  // keeping the exact same installed dependency tree and test harness.
  const filename = process.env.LEGACY_API_SOURCE_FILE || path.join(root, 'web', theme, helper);
  const module = new SourceTextModule(await readFile(filename, 'utf8'), { context, identifier: filename });
  await module.link(specifier => {
    assert.ok(Object.hasOwn(imports, specifier), `Unexpected helper import: ${specifier}`);
    const exports = imports[specifier];
    return new SyntheticModule(Object.keys(exports), function () {
      for (const [name, value] of Object.entries(exports)) this.setExport(name, value);
    }, { context });
  });
  await module.evaluate();
  const api = module.namespace.API;
  api.defaults.adapter = 'http';
  api.defaults.proxy = false;
  api.defaults.timeout = 10000;
  return { api, errors, redirects, actions, storage };
}

test(`${theme}: successful JSON requests preserve responses`, async t => {
  const { api, errors } = await fixture(t);
  const response = await api.get('/api/ok', { params: { page: 2 } });
  assert.equal(response.status, 200);
  assert.equal(response.data.ok, true);
  assert.equal(response.data.query.page, '2');
  assert.equal(errors.length, 0);
});

test(`${theme}: HTTP errors remain rejected`, async t => {
  const { api, errors } = await fixture(t);
  let caught;
  await assert.rejects(api.get('/api/error'), error => {
    caught = error;
    assert.ok(axios.isAxiosError(error));
    assert.equal(error.response.status, 422);
    assert.equal(error.response.data.message, 'validation failed');
    if (theme === 'berry') assert.equal(error.message, 'validation failed');
    return true;
  });
  assert.equal(errors.length, 1);
  assert.equal(errors[0], caught, 'The UI and caller receive the same Axios error');
});

test(`${theme}: cancellation remains detectable`, async t => {
  const { api } = await fixture(t);
  const controller = new AbortController();
  controller.abort();
  await assert.rejects(api.get('/api/ok', { signal: controller.signal }), error => axios.isCancel(error));
});

test(`${theme}: JSON writes retain payloads and caller headers`, async t => {
  const { api } = await fixture(t);
  const response = await api.post('/api/echo', { name: 'test', enabled: true }, {
    headers: { Authorization: 'Bearer test-only' },
  });
  assert.deepEqual(response.data.body, { name: 'test', enabled: true });
  assert.equal(response.data.method, 'POST');
  assert.equal(response.data.headers.authorization, 'Bearer test-only');
  assert.equal(response.data.query._, undefined);
  assert.equal(response.data.headers['cache-control'], undefined);
});

if (theme === 'berry') {
  test('berry: GET cache prevention preserves existing parameters and headers', async t => {
    const { api } = await fixture(t);
    const { data } = await api.get('/api/ok?q=a%2Bb', {
      params: { page: 2 }, headers: { Authorization: 'Bearer test-only' },
    });
    assert.equal(data.query.q, 'a+b');
    assert.equal(data.query.page, '2');
    assert.match(data.query._, /^\d+$/);
    assert.equal(data.headers.authorization, 'Bearer test-only');
    assert.equal(data.headers['cache-control'], 'no-cache, no-store, must-revalidate');
    assert.equal(data.headers.pragma, 'no-cache');
    assert.equal(data.headers.expires, '0');
  });

  test('berry: parallel 401 responses reject every caller but redirect only once', async t => {
    const { api, redirects, actions, storage } = await fixture(t);
    const results = await Promise.allSettled([api.get('/api/private'), api.get('/api/private')]);
    assert.ok(results.every(result => result.status === 'rejected'));
    assert.deepEqual(redirects, ['/login']);
    assert.equal(actions.length, 1);
    assert.equal(actions[0].type, 'LOGIN');
    assert.equal(actions[0].payload, null);
    assert.equal(storage.has('user'), false);
    assert.equal(storage.has('token'), false);
    assert.equal(storage.get('theme'), 'dark');
  });

  test('berry: 401 on the login page does not cause a redirect loop', async t => {
    const { api, redirects, actions } = await fixture(t, '/login');
    await assert.rejects(api.get('/api/private'));
    await assert.rejects(api.get('/api/private'));
    assert.equal(redirects.length, 0);
    assert.equal(actions.length, 2, 'The non-navigation guard is reset');
  });

  test('berry: router and DOM bindings share one core and route context', () => {
    const domRequire = createRequire(require.resolve('react-router-dom'));
    assert.equal(require.resolve('react-router'), domRequire.resolve('react-router'));
    const router = require('react-router');
    const dom = require('react-router-dom');
    assert.equal(router.MemoryRouter, dom.MemoryRouter);
    assert.equal(router.useLocation, dom.useLocation);
    const matches = dom.matchRoutes([{ path: '/channels/:id' }], '/channels/42');
    assert.equal(matches[0].params.id, '42');
  });
}
