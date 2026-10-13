import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import {
  createBrowserReporter,
  readConfig,
  reactRootOptions,
  safeAssetURL,
  sanitizeEnvelope,
  sanitizeEvent,
} from '../../web/legacy/browser-sentry.mjs';

const env = {
  VITE_SENTRY_ENABLED: 'true',
  VITE_SENTRY_DSN: 'https://0123456789abcdef0123456789abcdef@o123.ingest.sentry.io/456',
  VITE_SENTRY_ENVIRONMENT: 'production',
  VITE_SENTRY_RELEASE: 'one-api@' + 'a'.repeat(40),
};
const location = { hostname: 'oneapi.laisky.com', protocol: 'https:' };
/** fixture returns a synthetic exception containing intentionally sensitive content. */
function fixture() {
  return {
    event_id: 'b'.repeat(32),
    timestamp: 1234,
    message: 'prompt secret',
    logentry: { message: 'completion' },
    user: { email: 'private@example.test', ip_address: '127.0.0.1' },
    request: {
      url: 'https://oneapi.laisky.com/chat?token=secret',
      headers: { Authorization: 'Bearer secret' },
      cookies: 'secret',
      data: 'prompt',
    },
    breadcrumbs: [{ data: { tool_arguments: 'secret', tool_result: 'secret' } }],
    extra: { localStorage: 'secret', history: 'secret' },
    contexts: { response: { body: 'secret' } },
    tags: { account: 'secret' },
    release: 'secret',
    environment: 'secret',
    fingerprint: ['secret'],
    exception: {
      values: [
        {
          type: 'TypeError',
          value: 'auth token=secret prompt completion',
          mechanism: { type: 'secret', handled: false, data: { response: 'secret' } },
          stacktrace: {
            frames: [
              {
                filename: 'https://oneapi.laisky.com/assets/index-Abcd1234.js?api_key=secret#secret',
                lineno: 12,
                colno: 3,
                function: 'prompt',
                vars: { token: 'secret' },
                context_line: 'secret',
                pre_context: ['secret'],
              },
              { filename: 'https://other.test/assets/index-Abcd1234.js', lineno: 8 },
              { filename: 'https://oneapi.laisky.com/assets/sk-secret-Abcd1234.js', lineno: 8 },
            ],
          },
        },
      ],
    },
  };
}
/** mockSDK captures projected events without transport or network calls. */
function mockSDK({ failInit = false, failCapture = false } = {}) {
  const events = [];
  let options;
  let calls = 0;
  return {
    events,
    get options() {
      return options;
    },
    get calls() {
      return calls;
    },
    globalHandlersIntegration(config) {
      return { name: 'GlobalHandlers', config };
    },
    init(value) {
      calls++;
      options = value;
      if (failInit) throw new Error('secret');
      return { isEnabled: () => true };
    },
    captureException(error) {
      if (failCapture) throw new Error('secret');
      const event = options.beforeSend(fixture(), { originalException: error });
      if (event) events.push(event);
    },
  };
}

test('valid errors retain bounded static diagnostic fields and deployment identity only', () => {
  const config = readConfig(env, location, 'modern', ['/assets/index-Abcd1234.js']);
  const clean = sanitizeEvent(fixture(), config);
  assert.deepEqual(clean, {
    platform: 'javascript',
    level: 'error',
    environment: 'production',
    release: env.VITE_SENTRY_RELEASE,
    tags: { site: 'one-api', theme: 'modern' },
    event_id: 'b'.repeat(32),
    timestamp: 1234,
    exception: {
      values: [
        {
          type: 'TypeError',
          value: '[redacted browser error]',
          mechanism: { type: 'generic', handled: false },
          stacktrace: { frames: [{ filename: 'https://oneapi.laisky.com/assets/index-Abcd1234.js', in_app: true, lineno: 12, colno: 3 }] },
        },
      ],
    },
  });
  assert.doesNotMatch(JSON.stringify(clean), /secret|prompt|completion|private@example|Authorization|tool_|localStorage|history|api_key/);
});

test('unsupported full hosts, protocol, themes and missing or malformed configuration stay disabled', () => {
  for (const hostname of [
    'chat.laisky.com',
    'mcp.laisky.com',
    'cv.laisky.com',
    'sso.laisky.com',
    'chat2.laisky.com',
    'oneapi.laisky.com.evil.test',
    'localhost',
  ]) {
    const sdk = mockSDK();
    assert.equal(createBrowserReporter(sdk, env, { location: { ...location, hostname } }, 'modern').enabled, false);
    assert.equal(sdk.calls, 0);
  }
  for (const change of [
    { VITE_SENTRY_ENABLED: undefined },
    { VITE_SENTRY_DSN: '' },
    { VITE_SENTRY_ENVIRONMENT: '' },
    { VITE_SENTRY_RELEASE: '' },
    { VITE_SENTRY_RELEASE: 'one-api@secret' },
    { VITE_SENTRY_ENVIRONMENT: 'production\nsecret' },
    { VITE_SENTRY_DSN: env.VITE_SENTRY_DSN.replace('@', ':secret@') },
    { VITE_SENTRY_DSN: env.VITE_SENTRY_DSN + '?token=secret' },
    { VITE_SENTRY_DSN: env.VITE_SENTRY_DSN.replace('sentry.io', 'evil.test') },
  ]) {
    assert.equal(readConfig({ ...env, ...change }, location, 'modern'), null);
  }
  assert.equal(readConfig(env, { ...location, protocol: 'http:' }, 'modern'), null);
  assert.equal(readConfig(env, { ...location, port: '8443' }, 'modern'), null);
  assert.equal(readConfig(env, location, 'default'), null);
});

test('safe asset URLs reject arbitrary valid-looking secret basenames, credentials, source paths and oversized input', () => {
  for (const value of [
    'javascript:secret',
    '//evil.test/assets/index-Abcd1234.js',
    'https://user:secret@oneapi.laisky.com/assets/index-Abcd1234.js',
    '/assets/prompt-secret-Abcd1234.js',
    '/assets/sk-abcdefabcdefabcdefabcdefabcdefab-Abcd1234.js',
    '/assets/index-Abcd123456789.js',
    '/assets/index-%73ecret12.js',
    '/src/main.tsx?token=secret',
    '/assets/' + 'x'.repeat(3000) + '.js',
  ]) {
    assert.equal(safeAssetURL(value), undefined);
  }
  assert.equal(safeAssetURL('/assets/index-Abcd1234.js?prompt=secret#token'), 'https://oneapi.laisky.com/assets/index-Abcd1234.js');
});

test('projection is bounded, drops other event types, and fails closed on hostile data', () => {
  const config = readConfig(env, location, 'air', ['/assets/index-Abcd1234.js']);
  const value = fixture();
  value.exception.values[0].type = 'SecretError-prompt';
  value.exception.values[0].stacktrace.frames = Array.from({ length: 5000 }, () => ({
    filename: '/assets/index-Abcd1234.js',
    lineno: Infinity,
    colno: -2,
  }));
  value.exception.values = Array(5000).fill(value.exception.values[0]);
  const clean = sanitizeEvent(value, config);
  assert.equal(clean.exception.values.length, 3);
  assert.equal(clean.exception.values[0].stacktrace.frames.length, 30);
  assert.equal(clean.exception.values[0].type, 'Error');
  assert.equal(clean.exception.values[0].stacktrace.frames[0].lineno, undefined);
  assert.equal(sanitizeEvent({ ...fixture(), type: 'transaction' }, config), null);
  assert.equal(sanitizeEvent({ message: 'secret' }, config), null);
  assert.equal(
    sanitizeEvent(
      {
        get exception() {
          throw new Error('secret');
        },
      },
      config
    ),
    null
  );
});

test('SDK options collect errors only, no default integrations or sensitive categories', () => {
  const sdk = mockSDK();
  const reporter = createBrowserReporter(sdk, env, { location }, 'modern');
  assert.equal(reporter.enabled, true);
  assert.equal(sdk.options.defaultIntegrations, false);
  assert.deepEqual(sdk.options.integrations, [{ name: 'GlobalHandlers', config: { onerror: true, onunhandledrejection: true } }]);
  assert.deepEqual(sdk.options.dataCollection, {
    userInfo: false,
    cookies: false,
    httpHeaders: false,
    httpBodies: [],
    urlQueryParams: false,
    graphQL: { document: false, variables: false },
    genAI: { inputs: false, outputs: false },
    databaseQueryData: false,
    queues: false,
    stackFrameVariables: false,
    frameContextLines: 0,
  });
  for (const name of ['sendClientReports', 'debug']) assert.equal(sdk.options[name], false);
  assert.equal(sdk.options.tracesSampleRate, undefined);
  assert.equal(sdk.options.tracesSampler, undefined);
  assert.equal(sdk.options.beforeSendLog({ body: 'secret' }), null);
  assert.equal(sdk.options.beforeSendMetric({ attributes: { prompt: 'secret' } }), null);
  assert.deepEqual(sdk.options.tracePropagationTargets, []);
  assert.equal(sdk.options.beforeSendTransaction(fixture()), null);
});

test('duplicate init and boundary/global reports of one object produce one projected event', () => {
  const sdk = mockSDK();
  const target = { location };
  const reporter = createBrowserReporter(sdk, env, target, 'modern');
  assert.equal(createBrowserReporter(sdk, env, target, 'modern'), reporter);
  assert.equal(sdk.calls, 1);
  const error = new Error('secret prompt');
  reporter.capture(error);
  reporter.capture(error);
  assert.equal(sdk.events.length, 1);
  reporter.capture(new Error('another secret'));
  assert.equal(sdk.events.length, 2);
});

test('initialization and capture failures cannot crash the application; disabled root hooks are empty', () => {
  const sdk = mockSDK({ failInit: true });
  const target = { location };
  const reporter = createBrowserReporter(sdk, env, target, 'modern');
  assert.equal(reporter.enabled, false);
  assert.doesNotThrow(() => reporter.capture(new Error('secret')));
  assert.equal(createBrowserReporter(sdk, env, target, 'modern'), reporter);
  assert.equal(sdk.calls, 1);
  assert.deepEqual(reactRootOptions(reporter), {});
  const failedCapture = createBrowserReporter(mockSDK({ failCapture: true }), env, { location }, 'berry');
  assert.doesNotThrow(() => failedCapture.capture(new Error('secret')));
});

test('configuration snapshot cannot acquire sensitive values after initialization', () => {
  const sdk = mockSDK();
  const mutable = { ...env };
  const reporter = createBrowserReporter(sdk, mutable, { location }, 'air');
  mutable.VITE_SENTRY_ENVIRONMENT = 'secret';
  mutable.VITE_SENTRY_RELEASE = 'prompt-secret';
  reporter.capture(new Error('secret'));
  assert.equal(sdk.events[0].environment, 'production');
  assert.equal(sdk.events[0].release, env.VITE_SENTRY_RELEASE);
});

test('all shipped themes use the shared policy before application imports', () => {
  for (const theme of ['modern', 'air', 'berry']) {
    const base = new URL('../../web/' + theme + '/src/', import.meta.url);
    const entry = readFileSync(new URL(theme === 'modern' ? 'main.tsx' : 'index.js', base), 'utf8');
    assert.ok(entry.indexOf("'./browser-sentry'") < entry.indexOf("'./App'") || theme === 'berry');
    assert.match(entry, /<SentryBoundary>/);
    const instrument = readFileSync(new URL(theme === 'modern' ? 'browser-sentry.ts' : 'browser-sentry.js', base), 'utf8');
    assert.match(instrument, /legacy\/browser-sentry\.mjs/);
    assert.match(instrument, new RegExp("window,\\s*'" + theme + "'"));
  }
  const modern = readFileSync(new URL('../../web/modern/src/main.tsx', import.meta.url), 'utf8');
  assert.match(modern, /createRoot\([\s\S]*reactRootOptions\(browserReporter\)/);
});

test('partial initialization or missing client cannot leak events through registered handlers', () => {
  for (const throwing of [false, true]) {
    let options;
    const sdk = {
      globalHandlersIntegration() {
        return { name: 'GlobalHandlers' };
      },
      init(value) {
        options = value;
        if (throwing) throw new Error('secret');
      },
    };
    const reporter = createBrowserReporter(sdk, env, { location }, 'modern');
    assert.equal(reporter.enabled, false);
    const hint = { attachments: [{ filename: 'prompt.txt', data: 'secret' }] };
    assert.equal(options.beforeSend(fixture(), hint), null);
    assert.deepEqual(hint.attachments, []);
  }
});

test('hint and scope attachments and breadcrumbs are removed before successful transport', () => {
  const sdk = mockSDK();
  createBrowserReporter(sdk, env, { location }, 'modern');
  const hint = { attachments: [{ filename: 'tool-results.txt', data: 'secret' }] };
  assert.ok(sdk.options.beforeSend(fixture(), hint));
  assert.deepEqual(hint.attachments, []);
  assert.equal(sdk.options.maxBreadcrumbs, 0);
  assert.equal(sdk.options.beforeBreadcrumb({ message: 'secret' }), null);
});

test('final envelope privacy gate strips internal SDK event data, attachments and trace metadata', () => {
  const config = readConfig(env, location, 'modern', ['/assets/index-Abcd1234.js']);
  const envelope = [
    { event_id: 'b'.repeat(32), trace: { baggage: 'secret', transaction: 'prompt' }, dsn: 'secret', sdk: { secret: 'secret' } },
    [
      [{ type: 'event', content_type: 'secret' }, fixture()],
      [{ type: 'attachment', filename: 'secret' }, 'prompt'],
    ],
  ];
  const clean = sanitizeEnvelope(envelope, config);
  assert.deepEqual(clean[0], { event_id: 'b'.repeat(32) });
  assert.equal(clean[1].length, 1);
  assert.deepEqual(clean[1][0][0], { type: 'event' });
  assert.doesNotMatch(JSON.stringify(clean), /secret|prompt|completion|baggage|attachment|dsn/);
  for (const type of [
    'transaction',
    'span',
    'log',
    'metric',
    'session',
    'replay_event',
    'replay_recording',
    'client_report',
    'attachment',
  ]) {
    assert.equal(sanitizeEnvelope([{}, [[{ type }, fixture()]]], config), null);
  }
  assert.equal(sanitizeEnvelope([{}, [[{ type: 'event' }, { message: 'secret' }]]], config), null);
});

test('SDK final transport blocks partial-init internal events and safely delegates only error envelopes', async () => {
  for (const failInit of [false, true]) {
    const sent = [];
    let transport;
    const sdk = {
      globalHandlersIntegration() {
        return { name: 'GlobalHandlers' };
      },
      makeFetchTransport() {
        return {
          send(envelope) {
            sent.push(envelope);
            return Promise.resolve({ statusCode: 200 });
          },
          flush() {
            return Promise.resolve(true);
          },
        };
      },
      init(options) {
        transport = options.transport({});
        transport.send([{}, [[{ type: 'event' }, fixture()]]]);
        if (failInit) throw new Error('secret');
        return { isEnabled: () => true };
      },
    };
    const reporter = createBrowserReporter(sdk, env, { location }, 'modern');
    assert.equal(sent.length, 0);
    await transport.send([
      { trace: { prompt: 'secret' } },
      [
        [{ type: 'event' }, fixture()],
        [{ type: 'attachment' }, 'secret'],
      ],
    ]);
    assert.equal(sent.length, failInit ? 0 : 1);
    if (!failInit) assert.doesNotMatch(JSON.stringify(sent), /secret|prompt|attachment/);
    assert.equal(reporter.enabled, !failInit);
  }
});

test('only the immutable snapshot of loaded script/preload/module assets can survive projection', () => {
  const assets = ['/assets/index-Abcd1234.js'];
  const config = readConfig(env, location, 'modern', assets);
  assets.push('/assets/index-secret12.js');
  const event = fixture();
  event.exception.values[0].stacktrace.frames.push({ filename: '/assets/index-secret12.js', lineno: 5 });
  const clean = sanitizeEvent(event, config);
  assert.equal(clean.exception.values[0].stacktrace.frames.length, 1);
  assert.doesNotMatch(JSON.stringify(clean), /secret12/);
  assert.ok(Object.isFrozen(config));
  assert.ok(Object.isFrozen(config.assetURLs));
});

test('loaded page asset snapshot is bounded and transport explicitly omits credentials and referrer', async () => {
  let options;
  let fetchOptions;
  const sdk = {
    globalHandlersIntegration() {
      return { name: 'GlobalHandlers' };
    },
    makeFetchTransport(value) {
      fetchOptions = value.fetchOptions;
      return {
        send() {
          return Promise.resolve({});
        },
        flush() {
          return Promise.resolve(true);
        },
      };
    },
    init(value) {
      options = value;
      value.transport({});
      return { isEnabled: () => true };
    },
  };
  const nodes = Array.from({ length: 5000 }, () => ({ src: 'https://oneapi.laisky.com/assets/index-Abcd1234.js' }));
  const reporter = createBrowserReporter(
    sdk,
    env,
    {
      location,
      document: {
        querySelectorAll() {
          return nodes;
        },
      },
    },
    'modern',
    '/assets/vendor-Abcd1234.js'
  );
  assert.equal(reporter.enabled, true);
  const clean = options.beforeSend(fixture(), {});
  assert.equal(clean.exception.values[0].stacktrace.frames.length, 1);
  assert.deepEqual(fetchOptions, { credentials: 'omit', referrerPolicy: 'no-referrer' });
});

test('theme shims project exactly the four public settings instead of inlining the entire Vite environment', () => {
  for (const theme of ['modern', 'air', 'berry']) {
    const file = new URL('../../web/' + theme + '/src/browser-sentry.' + (theme === 'modern' ? 'ts' : 'js'), import.meta.url);
    const source = readFileSync(file, 'utf8');
    assert.doesNotMatch(source, /import\.meta\.env\s*[,)]/);
    const fields = [...source.matchAll(/(VITE_[A-Z_]+):\s*import\.meta\.env\.(VITE_[A-Z_]+)/g)];
    assert.equal(fields.length, 4);
    assert.deepEqual(fields.map((match) => match[1]).sort(), [
      'VITE_SENTRY_DSN',
      'VITE_SENTRY_ENABLED',
      'VITE_SENTRY_ENVIRONMENT',
      'VITE_SENTRY_RELEASE',
    ]);
    assert.ok(fields.every((match) => match[1] === match[2]));
  }
});
