import assert from 'node:assert/strict';
import test from 'node:test';
import { copyFileSync, mkdtempSync, mkdirSync, writeFileSync, symlinkSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import path from 'node:path';
import { pathToFileURL } from 'node:url';
import { loadBrowserReporter, mountBrowserApp } from '../../web/legacy/browser-sentry-runtime.mjs';
import { createBrowserReporter } from '../../web/legacy/browser-sentry.mjs';
import { browserRelease, readReleaseInput, releaseInputGuard, browserBuildContract } from '../../web/legacy/browser-release.mjs';

const settings = { dsn: 'https://0123456789abcdef0123456789abcdef@o123.ingest.sentry.io/456', environment: 'production' };
const release = (theme) => 'one-api-' + theme + '@sha256:' + 'a'.repeat(64);
/** setup supplies a synthetic target and SDK with only mocked delivery and capture methods. */
function setup(fetcher) {
  let calls = 0;
  let options;
  const events = [];
  const fixture = {
    target: { location: { hostname: 'oneapi.laisky.com', protocol: 'https:', port: '' }, fetch: fetcher },
    sdk: {
      globalHandlersIntegration() {
        return { name: 'GlobalHandlers' };
      },
      makeFetchTransport() {
        return {};
      },
      init(value) {
        calls++;
        options = value;
        return { isEnabled: () => true };
      },
      captureException(error) {
        const event = options.beforeSend(
          { exception: { values: [{ type: 'Error', value: error.message }] } },
          { originalException: error }
        );
        if (event) events.push(event);
      },
    },
    get calls() {
      return calls;
    },
    events,
  };
  fixture.loadSDK = () => fixture.sdk;
  return fixture;
}

/** response produces an isolated standard streaming JSON Response without any network operation. */
function response(value) {
  return new Response(JSON.stringify(value), { headers: { 'content-type': 'application/json' } });
}

for (const theme of ['modern', 'air', 'berry']) {
  test(theme + ': valid atomic runtime setting activates automatically and concurrent duplicate readiness initializes once', async () => {
    const requests = [];
    const fixture = setup((url, options) => {
      requests.push({ url, options });
      return response({ sentry: settings });
    });
    const first = loadBrowserReporter(fixture.loadSDK, release(theme), fixture.target, theme, '/assets/index-Abcd1234.js');
    const duplicate = loadBrowserReporter(fixture.loadSDK, release(theme), fixture.target, theme, '/assets/index-Abcd1234.js');
    assert.equal(first, duplicate);
    const reporter = await first;
    assert.equal(reporter.enabled, true);
    assert.equal(fixture.calls, 1);
    assert.equal(requests.length, 1);
    assert.equal(requests[0].url, '/api/browser-config');
    assert.equal(requests[0].options.credentials, 'omit');
    assert.equal(requests[0].options.referrerPolicy, 'no-referrer');
    assert.equal(requests[0].options.redirect, 'error');
    assert.equal(requests[0].options.cache, 'no-store');
    assert.equal(requests[0].options.mode, 'same-origin');
    reporter.capture(new Error('synthetic-private-secret'));
    assert.equal(fixture.events[0].release, release(theme));
    assert.doesNotMatch(JSON.stringify(fixture.events), /synthetic-private-secret/);
  });
}

test('absent, invalid, unavailable, redirect, non-JSON, oversized and hostile configuration responses stay disabled', async () => {
  const responses = [
    () => response({ sentry: null }),
    () => response({ sentry: { ...settings, enabled: true } }),
    () => response({ sentry: { ...settings, release: 'secret' } }),
    () => response({ sentry: { ...settings, environment: 'secret' } }),
    () => response({ sentry: settings, token: 'secret' }),
    () => Promise.reject(new Error('secret')),
    () => new Response('secret', { status: 503 }),
    () => new Response('not-json', { headers: { 'content-type': 'application/json' } }),
    () => new Response('secret'),
    () => new Response('x'.repeat(2049), { headers: { 'content-type': 'application/json' } }),
    () => ({ ok: true, redirected: true }),
    () => response({ sentry: { ...settings, dsn: settings.dsn + '?token=secret' } }),
  ];
  for (const fetcher of responses) {
    const fixture = setup(fetcher);
    const reporter = await loadBrowserReporter(fixture.loadSDK, release('modern'), fixture.target, 'modern');
    assert.equal(reporter.enabled, false);
    assert.equal(fixture.calls, 0);
    assert.doesNotThrow(() => reporter.capture(new Error('secret')));
  }
});

test('load deadline includes stalled response body and cannot initialize from a late successful response', async () => {
  for (const bodyStall of [false, true]) {
    let deliver;
    let signal;
    const fixture = setup((_url, options) => {
      signal = options.signal;
      if (bodyStall) return new Response(new ReadableStream({ start() {} }), { headers: { 'content-type': 'application/json' } });
      return new Promise((resolve) => {
        deliver = resolve;
      });
    });
    const reporter = await loadBrowserReporter(fixture.loadSDK, release('modern'), fixture.target, 'modern', undefined, 5);
    assert.equal(reporter.enabled, false);
    assert.equal(signal.aborted, true);
    if (deliver) deliver(response({ sentry: settings }));
    await new Promise((resolve) => setImmediate(resolve));
    assert.equal(fixture.calls, 0);
  }
});

test('unsupported hosts and invalid built identity do not request public configuration', async () => {
  for (const host of ['cv.laisky.com', 'sso.laisky.com', 'chat2.laisky.com', 'mcp.laisky.com', 'oneapi.laisky.com.evil.test']) {
    const fixture = setup(() => assert.fail('must not fetch'));
    fixture.target.location.hostname = host;
    assert.equal((await loadBrowserReporter(fixture.loadSDK, release('modern'), fixture.target, 'modern')).enabled, false);
  }
  const fixture = setup(() => assert.fail('must not fetch'));
  assert.equal((await loadBrowserReporter(fixture.loadSDK, 'secret', fixture.target, 'modern')).enabled, false);
});

test('runtime updates are immutable until reload; blank update disables the next page with no reinitialization or buffering', async () => {
  let value = { ...settings };
  let fetchCalls = 0;
  const fixture = setup(() => {
    fetchCalls++;
    return response({ sentry: value });
  });
  const original = await loadBrowserReporter(fixture.loadSDK, release('modern'), fixture.target, 'modern');
  value.environment = 'staging';
  value = null;
  assert.equal(await loadBrowserReporter(fixture.loadSDK, release('modern'), fixture.target, 'modern'), original);
  original.capture(new Error('secret'));
  assert.equal(fixture.events[0].environment, 'production');
  assert.equal(fetchCalls, 1);
  const nextPage = { ...fixture.target };
  const next = await loadBrowserReporter(fixture.loadSDK, release('modern'), nextPage, 'modern');
  assert.equal(next.enabled, false);
  assert.equal(fixture.calls, 1);
  next.capture(new Error('secret'));
  assert.equal(fixture.events.length, 1);
  assert.equal(createBrowserReporter(fixture.sdk, settings, nextPage, 'modern', undefined, release('modern')), next);
});

test('immutable release identity is independent of runtime settings, sensitive environment and Git, but changes with relevant source/build values', () => {
  const input = (name) => Buffer.from(name + ':synthetic-source');
  const identity = browserRelease('modern', { mode: 'production' }, input);
  assert.match(identity, /^one-api-modern@sha256:[a-f0-9]{64}$/);
  assert.equal(browserRelease('modern', { mode: 'production' }, input), identity);
  process.env.VITE_SENTRY_DSN = 'synthetic-unrelated-private-secret';
  try {
    assert.equal(browserRelease('modern', { mode: 'production' }, input), identity);
  } finally {
    delete process.env.VITE_SENTRY_DSN;
  }
  assert.notEqual(browserRelease('modern', { mode: 'development' }, input), identity);
  assert.notEqual(
    browserRelease('modern', { mode: 'production' }, (name) => Buffer.from(name + ':changed-source')),
    identity
  );
  assert.notEqual(browserRelease('air', { mode: 'production' }, input), identity);
});

test('every setup failure resolves duplicate readiness disabled rather than preventing application mount', async () => {
  for (const scenario of ['location', 'fetch', 'abort']) {
    const fixture = setup(() => assert.fail('must not fetch'));
    const original = globalThis.AbortController;
    try {
      if (scenario === 'location')
        Object.defineProperty(fixture.target, 'location', {
          get() {
            throw new Error('secret');
          },
        });
      if (scenario === 'fetch')
        Object.defineProperty(fixture.target, 'fetch', {
          get() {
            throw new Error('secret');
          },
        });
      if (scenario === 'abort') globalThis.AbortController = undefined;
      const pending = loadBrowserReporter(fixture.loadSDK, release('modern'), fixture.target, 'modern');
      assert.equal(loadBrowserReporter(fixture.loadSDK, release('modern'), fixture.target, 'modern'), pending);
      const reporter = await pending;
      assert.equal(reporter.enabled, false);
      assert.equal(fixture.calls, 0);
    } finally {
      globalThis.AbortController = original;
    }
  }
});

test('runtime SDK initialization failure remains disabled and cannot report or retry automatically', async () => {
  const fixture = setup(() => response({ sentry: settings }));
  let calls = 0;
  fixture.sdk.init = () => {
    calls++;
    throw new Error('secret');
  };
  const reporter = await loadBrowserReporter(fixture.loadSDK, release('modern'), fixture.target, 'modern');
  assert.equal(reporter.enabled, false);
  assert.equal(fixture.calls, 1);
  reporter.capture(new Error('secret'));
  assert.equal(fixture.events.length, 0);
  assert.equal(await loadBrowserReporter(fixture.loadSDK, release('modern'), fixture.target, 'modern'), reporter);
  assert.equal(calls, 1);
});

test('build-input guard accepts all current public assets and blocks future unlisted bundled sources or private paths', () => {
  for (const theme of ['modern', 'air', 'berry']) {
    const guard = releaseInputGuard(theme);
    assert.equal(guard.apply, 'build');
    assert.doesNotThrow(() => guard.buildStart());
    assert.equal(
      guard.transform(
        '',
        new URL('../../web/' + theme + '/src/browser-sentry.' + (theme === 'modern' ? 'ts' : 'js'), import.meta.url).pathname
      ),
      null
    );
    assert.throws(
      () => guard.transform('', new URL('../../web/' + theme + '/src/unlisted-new-source.ts', import.meta.url).pathname),
      /Unlisted browser build input/
    );
    assert.match(browserRelease(theme, { mode: 'production' }), new RegExp('^one-api-' + theme + '@sha256:[a-f0-9]{64}$'));
  }
  for (const name of ['web/modern/.env', 'web/modern/src/../../.env', '/etc/private', 'web/modern/node_modules/private']) {
    assert.throws(() => readReleaseInput(name), /Invalid browser release input/);
  }
});

test('release input reader refuses file and ancestor symlinks before reading synthetic private content', () => {
  const directory = mkdtempSync(path.join(tmpdir(), 'oneapi-release-inputs-'));
  try {
    mkdirSync(path.join(directory, 'web/modern/src'), { recursive: true });
    const privateFile = path.join(directory, 'synthetic-private.txt');
    writeFileSync(privateFile, 'synthetic-private-secret');
    symlinkSync(privateFile, path.join(directory, 'web/modern/src/file.ts'));
    assert.throws(() => readReleaseInput('web/modern/src/file.ts', directory), /Symbolic browser release input/);
    symlinkSync(path.join(directory, 'web/modern/src'), path.join(directory, 'web/modern/alias'));
    assert.throws(() => readReleaseInput('web/modern/alias/file.ts', directory), /Symbolic browser release input/);
  } finally {
    rmSync(directory, { recursive: true, force: true });
  }
});

test('one relevant source change changes identity and virtual or external modules do not bypass local input verification', async () => {
  const input = (name) => Buffer.from(name + ':synthetic-source');
  const original = browserRelease('modern', { mode: 'production' }, input);
  const changed = browserRelease('modern', { mode: 'production' }, (name) =>
    name === 'web/modern/src/main.tsx' ? Buffer.from('changed relevant source') : input(name)
  );
  assert.notEqual(changed, original);
  const guard = releaseInputGuard('modern');
  assert.equal(guard.transform('', '\0vite/modulepreload-polyfill.js'), null);
  // Each CI theme installs only its own dependencies; never borrow another theme's node_modules.
  for (const theme of ['modern', 'air', 'berry']) {
    const directory = mkdtempSync(path.join(tmpdir(), 'oneapi-release-dependencies-'));
    try {
      const legacy = path.join(directory, 'web/legacy');
      const source = 'web/' + theme + '/src/main.js';
      const dependencies = path.join(directory, 'web', theme, 'node_modules');
      mkdirSync(legacy, { recursive: true });
      mkdirSync(path.dirname(path.join(directory, source)), { recursive: true });
      mkdirSync(path.join(directory, 'web', theme, 'public'), { recursive: true });
      mkdirSync(path.join(dependencies, 'react'), { recursive: true });
      writeFileSync(path.join(directory, source), 'export const synthetic = true;\n');
      writeFileSync(path.join(legacy, 'browser-release-inputs.json'), JSON.stringify({ [theme]: [source] }));
      copyFileSync(new URL('../../web/legacy/browser-release.mjs', import.meta.url), path.join(legacy, 'browser-release.mjs'));
      const { releaseInputGuard: isolatedGuard } = await import(pathToFileURL(path.join(legacy, 'browser-release.mjs')).href);
      const guard = isolatedGuard(theme);
      const dependency = path.join(dependencies, 'react/index.js');

      assert.equal(guard.transform('', '\0vite/modulepreload-polyfill.js'), null);
      assert.throws(() => guard.transform('', dependency), /Unreviewed browser dependency input/);
      writeFileSync(dependency, 'export const syntheticDependency = true;\n');
      assert.equal(guard.load(dependency), null);
      assert.equal(guard.transform('', dependency), null);
      assert.equal((await guard.resolveId.call({ resolve: async () => ({ id: dependency }) }, 'react', source)).id, dependency);
      assert.throws(() => guard.load(path.join(dependencies, 'missing.js')), /Unreviewed browser dependency input/);
      await assert.rejects(
        guard.resolveId.call({ resolve: async () => ({ id: dependency, external: true }) }, 'react', source),
        /External browser module input is not approved/
      );

      const privateFile = path.join(directory, 'synthetic-private.js');
      const escape = path.join(dependencies, 'escape.js');
      writeFileSync(privateFile, 'synthetic-private-secret');
      symlinkSync(privateFile, escape);
      assert.throws(() => guard.load(escape), /Unreviewed browser dependency input/);
      assert.throws(() => guard.transform('', escape), /Unreviewed browser dependency input/);
      await assert.rejects(
        guard.resolveId.call({ resolve: async () => ({ id: escape }) }, './escape', source),
        /Unreviewed browser dependency input/
      );
      assert.throws(() => guard.load(privateFile), /Unlisted browser build input/);
      assert.doesNotThrow(() => guard.buildStart());
      assert.doesNotThrow(() => guard.generateBundle());
    } finally {
      rmSync(directory, { recursive: true, force: true });
    }
  }
});

test('resolved aliases, outside-web imports, out-of-repository source and canonical symlink targets fail before module loading', async () => {
  const guard = releaseInputGuard('modern');
  for (const id of [
    new URL('../../common/private.js', import.meta.url).pathname,
    '/synthetic-private-outside-repository.js',
    '/synthetic/node_modules/forged-private.js',
  ]) {
    assert.throws(() => guard.load(id), /Unlisted browser build input/);
    assert.throws(() => guard.transform('', id), /Unlisted browser build input/);
    await assert.rejects(
      guard.resolveId.call({ resolve: async () => ({ id }) }, './local-alias', '/synthetic-importer.js'),
      /Unlisted browser build input/
    );
  }
  const legitimate = new URL('../../web/modern/src/main.tsx', import.meta.url).pathname;
  assert.equal(
    (await guard.resolveId.call({ resolve: async () => ({ id: legitimate }) }, './main', '/synthetic-importer.js')).id,
    legitimate
  );
});

test('absent or invalid runtime settings never request or evaluate the SDK bridge', async () => {
  for (const value of [null, { ...settings, enabled: true }, { ...settings, release: 'secret' }, { ...settings, dsn: '' }]) {
    const fixture = setup(() => response({ sentry: value }));
    fixture.loadSDK = () => assert.fail('SDK must not load');
    assert.equal((await loadBrowserReporter(fixture.loadSDK, release('modern'), fixture.target, 'modern')).enabled, false);
  }
});

test('SDK bridge throw, rejection, missing export and stalled late module never prevent disabled readiness or initialize after timeout', async () => {
  for (const scenario of ['throw', 'reject', 'missing', 'stall']) {
    const fixture = setup(() => response({ sentry: settings }));
    let deliver;
    let factoryCalls = 0;
    fixture.loadSDK = () => {
      factoryCalls++;
      if (scenario === 'throw') throw new Error('synthetic-private-secret');
      if (scenario === 'reject') return Promise.reject(new Error('synthetic-private-secret'));
      if (scenario === 'missing') return { init: fixture.sdk.init };
      return new Promise((resolve) => {
        deliver = resolve;
      });
    };
    const pending = loadBrowserReporter(fixture.loadSDK, release('modern'), fixture.target, 'modern', undefined, 10);
    assert.equal(loadBrowserReporter(fixture.loadSDK, release('modern'), fixture.target, 'modern', undefined, 10), pending);
    const reporter = await pending;
    assert.equal(reporter.enabled, false);
    assert.equal(factoryCalls, 1);
    assert.equal(fixture.calls, 0);
    if (deliver) deliver(fixture.sdk);
    await new Promise((resolve) => setImmediate(resolve));
    assert.equal(fixture.calls, 0);
    reporter.capture(new Error('secret'));
    assert.equal(fixture.events.length, 0);
  }
});

test('non-imported source input additions and concurrent source changes cannot share a previously emitted release identity', () => {
  let changed = false;
  const input = (name) => Buffer.from(name + (changed && name === 'web/modern/src/main.tsx' ? ':changed' : ':original'));
  const contract = browserBuildContract('modern', { mode: 'production' }, input);
  assert.doesNotThrow(() => contract.plugin.buildStart());
  changed = true;
  assert.throws(() => contract.plugin.generateBundle(), /changed after release snapshot/);
  const guard = releaseInputGuard('modern');
  for (const file of ['new-tailwind-class.tsx', 'new-sass-partial.scss']) {
    assert.throws(() => guard.load(new URL('../../web/modern/src/' + file, import.meta.url).pathname), /Unlisted browser build input/);
  }
});

test('zero-byte or non-byte response streams are bounded without starving readiness deadlines', async () => {
  for (const invalid of [false, true]) {
    let chunks = 0;
    const stream = new ReadableStream({
      pull(controller) {
        chunks++;
        controller.enqueue(invalid ? { byteLength: Infinity } : new Uint8Array(0));
      },
    });
    const fixture = setup(() => new Response(stream, { headers: { 'content-type': 'application/json' } }));
    const reporter = await loadBrowserReporter(fixture.loadSDK, release('modern'), fixture.target, 'modern');
    assert.equal(reporter.enabled, false);
    assert.ok(chunks <= 2050);
    assert.equal(fixture.calls, 0);
  }
});

test('abort and cleanup exceptions cannot reject or leave timed-out readiness pending', async () => {
  const original = globalThis.AbortController;
  try {
    globalThis.AbortController = class {
      signal = {};
      abort() {
        throw new Error('synthetic-private-secret');
      }
    };
    const fixture = setup(() => new Promise(() => {}));
    const reporter = await loadBrowserReporter(fixture.loadSDK, release('modern'), fixture.target, 'modern', undefined, 5);
    assert.equal(reporter.enabled, false);
    assert.equal(fixture.calls, 0);
  } finally {
    globalThis.AbortController = original;
  }
});

test('mount rejection owner redispatches the original error once and never retries mounting for active or disabled reporting', async () => {
  for (const enabled of [false, true]) {
    const error = new Error('synthetic-private-mount-error');
    const nativeErrors = [];
    let mounts = 0;
    const readiness = Promise.resolve({ enabled, capture() {} });
    const target = {
      queueMicrotask(callback) {
        queueMicrotask(() => {
          try {
            callback();
          } catch (failure) {
            nativeErrors.push(failure);
          }
        });
      },
    };
    await mountBrowserApp(
      readiness,
      () => {
        mounts++;
        throw error;
      },
      target
    );
    await new Promise((resolve) => setImmediate(resolve));
    assert.equal(mounts, 1);
    assert.deepEqual(nativeErrors, [error]);
  }
});

test('native timer fallback owns mount errors when microtask scheduling is absent or throws', async () => {
  const original = globalThis.setTimeout;
  try {
    for (const target of [
      {},
      {
        queueMicrotask() {
          throw new Error('synthetic-scheduler-error');
        },
      },
    ]) {
      const error = new Error('synthetic-private-mount-error');
      const nativeErrors = [];
      globalThis.setTimeout = (callback) => {
        try {
          callback();
        } catch (failure) {
          nativeErrors.push(failure);
        }
      };
      await mountBrowserApp(
        Promise.resolve({ enabled: false, capture() {} }),
        () => {
          throw error;
        },
        target
      );
      assert.deepEqual(nativeErrors, [error]);
    }
  } finally {
    globalThis.setTimeout = original;
  }
});
