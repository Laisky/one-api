import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import {
  createBrowserReporter as initializeBrowserReporter,
  readConfig as readPublicConfig,
  reactRootOptions,
  sanitizeEvent,
} from '../../web/legacy/browser-sentry.mjs';
import { markPrivateError } from '../../web/legacy/browser-sentry-privacy.mjs';

const env = {
  dsn: 'https://0123456789abcdef0123456789abcdef@o123.ingest.sentry.io/456',
  environment: 'production',
};
const location = { hostname: 'oneapi.laisky.com', protocol: 'https:', port: '' };
const debugID = '2055b92d-c7d2-43d3-be25-9fa700054800';
/** release supplies synthetic immutable build identity for privacy tests. */
const release = (theme) => 'one-api-' + theme + '@sha256:' + 'a'.repeat(64);
/** createBrowserReporter exercises the factory with independent immutable build identity. */
function createBrowserReporter(sdk, settings, target, theme, entryURL) {
  return initializeBrowserReporter(sdk, settings, target, theme, entryURL, release(theme));
}
/** readConfig exercises configuration validation with a synthetic built release. */
function readConfig(settings, targetLocation, theme, assets = []) {
  return readPublicConfig(settings, targetLocation, theme, assets, release(theme));
}
/** fixture supplies useful SDK diagnostics alongside specifically keyed private values. */
function fixture(error = new TypeError('Cannot read properties of undefined')) {
  return {
    event_id: 'b'.repeat(32),
    timestamp: 1234,
    message: error.message,
    user: { email: 'synthetic-private@example.test', ip_address: '203.0.113.10' },
    request: {
      url: 'https://user:synthetic-private@oneapi.laisky.com/channel?token=synthetic-private#synthetic-private',
      headers: { Authorization: 'Bearer synthetic-private' },
      cookies: 'synthetic-private',
      data: 'synthetic-private prompt',
      query_string: 'token=synthetic-private',
    },
    breadcrumbs: [{ category: 'fetch', data: { url: '/api/channel?api_key=synthetic-private', status_code: 200 } }],
    extra: {
      api_token: 'synthetic-private',
      rawResponse: 'synthetic-private completion',
      arguments: ['synthetic-private'],
      token_count: 42,
    },
    contexts: {
      browser: { name: 'Chrome', version: '123' },
      operation: { route: 'channels', body: 'synthetic-private' },
      react: { componentStack: '\n    at ChannelDashboard' },
    },
    tags: { operation: 'channel-list', user_id: 'synthetic-private' },
    release: 'untrusted release',
    environment: 'untrusted environment',
    debug_meta: { images: [{ type: 'sourcemap', code_file: 'https://oneapi.laisky.com/assets/settings-Abcd1234.js', debug_id: debugID }] },
    exception: {
      values: [
        {
          type: error.name,
          value: error.message,
          mechanism: { type: 'generic', handled: false },
          stacktrace: {
            frames: [
              {
                filename: 'https://oneapi.laisky.com/assets/index-Abcd1234.js?api_key=synthetic-private#synthetic-private',
                lineno: 12,
                colno: 3,
                function: 'renderDashboard',
                vars: { token: 'synthetic-private' },
                context_line: 'synthetic-private',
                pre_context: ['synthetic-private'],
              },
              { filename: 'https://oneapi.laisky.com/assets/settings-Abcd1234.js', lineno: 18, function: 'loadSettings' },
              { filename: 'https://vendor.test/vendor.js', lineno: 8, function: 'dispatch' },
            ],
          },
        },
      ],
    },
  };
}
/** mockSDK exercises application hooks without transport or real telemetry. */
function mockSDK({ failInit = false, failCapture = false } = {}) {
  const events = [];
  const reactCalls = [];
  let options;
  let calls = 0;
  /** capture applies the installed hook to a synthetic SDK-shaped diagnostic. */
  function capture(error, componentStack, context) {
    if (failCapture) throw new Error('synthetic-private SDK failure');
    const event = fixture(error);
    if (componentStack) event.contexts.react = { componentStack };
    if (context) Object.assign(event.contexts, context.contexts);
    const clean = options.beforeSend(event, { originalException: error });
    if (clean) events.push(clean);
  }
  return {
    events,
    reactCalls,
    get options() {
      return options;
    },
    get calls() {
      return calls;
    },
    init(value) {
      calls++;
      options = value;
      if (failInit) throw new Error('synthetic-private SDK failure');
      return { isEnabled: () => true };
    },
    captureException(error) {
      capture(error);
    },
    captureReactException(error, info, context) {
      reactCalls.push({ error, info, context });
      capture(error, info.componentStack, context);
    },
    reactErrorHandler() {
      return (error, info) => capture(error, info?.componentStack);
    },
  };
}

test('ordinary diagnostics retain useful messages, full lazy/vendor frames, context and Debug IDs with targeted privacy', () => {
  const config = readConfig(env, location, 'modern', ['/assets/index-Abcd1234.js']);
  const clean = sanitizeEvent(fixture(), config);
  assert.equal(clean.message, 'Cannot read properties of undefined');
  assert.equal(clean.exception.values[0].value, 'Cannot read properties of undefined');
  assert.deepEqual(
    clean.exception.values[0].stacktrace.frames.map((frame) => frame.function),
    ['renderDashboard', 'loadSettings', 'dispatch']
  );
  assert.deepEqual(
    clean.exception.values[0].stacktrace.frames.map((frame) => frame.filename),
    [
      'https://oneapi.laisky.com/assets/index-Abcd1234.js',
      'https://oneapi.laisky.com/assets/settings-Abcd1234.js',
      'https://vendor.test/vendor.js',
    ]
  );
  assert.equal(clean.contexts.react.componentStack, '\n    at ChannelDashboard');
  assert.deepEqual(clean.contexts.browser, { name: 'Chrome', version: '123' });
  assert.equal(clean.contexts.operation.route, 'channels');
  assert.equal(clean.debug_meta.images[0].debug_id, debugID);
  assert.equal(clean.tags.operation, 'channel-list');
  assert.equal(clean.extra.token_count, 42);
  assert.equal(clean.request.url, 'https://oneapi.laisky.com/channel');
  assert.equal(clean.breadcrumbs[0].data.url, '/api/channel');
  assert.equal(clean.user, undefined);
  assert.equal(clean.release, release('modern'));
  assert.equal(clean.environment, 'production');
  assert.equal(clean.tags.theme, 'modern');
  assert.doesNotMatch(JSON.stringify(clean), /synthetic-private|Authorization|rawResponse|api_token|context_line|pre_context/);
});

test('marked private response causes and SDK-truncated copies are sanitized while ordinary outer diagnostics remain useful', () => {
  const privateMessage = 'synthetic-private completion '.repeat(30);
  const cause = markPrivateError(new Error(privateMessage), 'Private response decoding failed');
  const error = new Error('Channel dashboard could not render', { cause });
  const event = fixture(error);
  event.exception.values.push({ type: 'Error', value: privateMessage.slice(0, 250) + '...' });
  event.contexts.react.componentStack = 'Error: ' + privateMessage;
  const hint = { originalException: error, attachments: [{ filename: 'private.txt', data: privateMessage }] };
  const clean = sanitizeEvent(event, readConfig(env, location, 'air'), hint);
  assert.equal(clean.exception.values[0].value, 'Channel dashboard could not render');
  assert.equal(clean.exception.values[1].value, 'Private response decoding failed');
  assert.equal(clean.contexts.react.componentStack, 'Error: Private response decoding failed');
  assert.equal(clean.exception.values[0].stacktrace.frames[0].function, 'renderDashboard');
  assert.deepEqual(hint.attachments, []);
  assert.equal(cause.message, privateMessage);
  assert.doesNotMatch(JSON.stringify(clean), /synthetic-private/);
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
    { enabled: true },
    { dsn: '' },
    { environment: '' },
    { environment: 'production\nsecret' },
    { dsn: env.dsn.replace('@', ':secret@') },
    { dsn: env.dsn + '?token=secret' },
    { dsn: env.dsn.replace('sentry.io', 'evil.test') },
  ]) {
    assert.equal(readConfig({ ...env, ...change }, location, 'modern'), null);
  }
  for (const settings of [undefined, null, [], {}]) assert.equal(readConfig(settings, location, 'modern'), null);
  assert.equal(readConfig(env, { ...location, protocol: 'http:' }, 'modern'), null);
  assert.equal(readConfig(env, { ...location, port: '8443' }, 'modern'), null);
  assert.equal(readConfig(env, location, 'default'), null);
});

test('metadata traversal is bounded and getter/cycle controls preserve ordinary diagnostic stacks', () => {
  const event = fixture();
  event.extra = { payload: {} };
  let getters = 0;
  Object.defineProperty(event.extra, 'hostile', {
    enumerable: true,
    get() {
      getters++;
      throw new Error('synthetic-private');
    },
  });
  event.extra.payload.self = event.extra.payload;
  event.extra.wide = Object.fromEntries(Array.from({ length: 1000 }, (_, index) => ['field' + index, { result: index }]));
  event.exception.values[0].stacktrace.frames = Array.from({ length: 80 }, (_, index) => ({
    filename: '/assets/lazy.js',
    lineno: index + 1,
    function: 'frame' + index,
  }));
  const clean = sanitizeEvent(event, readConfig(env, location, 'air'));
  assert.equal(getters, 0);
  assert.equal(clean.extra.hostile, undefined);
  assert.equal(clean.extra.payload.self, '[Truncated]');
  assert.ok(Object.keys(clean.extra.wide).length <= 200);
  assert.equal(clean.exception.values[0].stacktrace.frames.length, 80);
  assert.equal(clean.exception.values[0].stacktrace.frames[79].function, 'frame79');
  assert.equal(sanitizeEvent(null, readConfig(env, location, 'air')), null);
});

test('SDK uses standard transport and diagnostic defaults while disabling sessions and sensitive collection', () => {
  const sdk = mockSDK();
  assert.equal(createBrowserReporter(sdk, env, { location }, 'modern').enabled, true);
  assert.equal(sdk.options.defaultIntegrations, undefined);
  assert.equal(sdk.options.transport, undefined);
  assert.equal(sdk.options.maxBreadcrumbs, undefined);
  const defaults = ['GlobalHandlers', 'BrowserApiErrors', 'LinkedErrors', 'Dedupe', 'HttpContext', 'Breadcrumbs', 'BrowserSession'].map(
    (name) => ({ name })
  );
  assert.deepEqual(sdk.options.integrations(defaults), defaults.slice(0, -1));
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
  });
  assert.deepEqual(sdk.options.transportOptions.fetchOptions, { credentials: 'omit', referrerPolicy: 'no-referrer' });
  for (const name of ['sendClientReports', 'debug']) assert.equal(sdk.options[name], false);
  for (const name of ['tracesSampleRate', 'tracesSampler', 'beforeSendTransaction']) assert.equal(sdk.options[name], undefined);
  assert.equal(sdk.options.beforeSendLog({ body: 'synthetic-private' }), null);
  assert.equal(sdk.options.beforeSendMetric({ attributes: { prompt: 'synthetic-private' } }), null);
  assert.deepEqual(sdk.options.tracePropagationTargets, []);
});

test('duplicate init and boundary/global reports of one object emit once and distinct errors retain ordinary messages', () => {
  const sdk = mockSDK();
  const target = { location };
  const reporter = createBrowserReporter(sdk, env, target, 'modern');
  assert.equal(createBrowserReporter(sdk, env, target, 'modern'), reporter);
  assert.equal(sdk.calls, 1);
  const error = new Error('Channel dashboard failed');
  reporter.capture(error, { componentStack: '\n    at Dashboard' });
  reporter.capture(error);
  assert.equal(sdk.events.length, 1);
  assert.equal(sdk.events[0].exception.values[0].value, error.message);
  assert.deepEqual(sdk.reactCalls[0].info, { componentStack: '\n    at Dashboard' });
  assert.equal(sdk.events[0].contexts.react.componentStack, '\n    at Dashboard');
  reporter.capture(new Error('Settings dashboard failed'));
  assert.equal(sdk.events.length, 2);
});

test('SDK failures cannot crash application code, logging only static messages, and disabled root hooks remain empty', () => {
  const original = console.warn;
  const warnings = [];
  console.warn = (...args) => warnings.push(args);
  try {
    const sdk = mockSDK({ failInit: true });
    const target = { location };
    const reporter = createBrowserReporter(sdk, env, target, 'modern');
    assert.equal(reporter.enabled, false);
    assert.doesNotThrow(() => reporter.capture(new Error('synthetic-private')));
    assert.equal(createBrowserReporter(sdk, env, target, 'modern'), reporter);
    assert.equal(sdk.calls, 1);
    assert.deepEqual(reactRootOptions(reporter), {});
    const failedCapture = createBrowserReporter(mockSDK({ failCapture: true }), env, { location }, 'berry');
    assert.doesNotThrow(() => failedCapture.capture(new Error('synthetic-private')));
    assert.doesNotThrow(() => failedCapture.capture(new Error('synthetic-private'), { componentStack: '\n    at Dashboard' }));
    assert.ok(warnings.length > 0);
    assert.ok(warnings.every((args) => args.length === 1 && args[0] === 'One API error reporting unavailable.'));
  } finally {
    console.warn = original;
  }
});

test('configuration snapshot cannot acquire new environment or release values after initialization', () => {
  const sdk = mockSDK();
  const mutable = { ...env };
  const reporter = createBrowserReporter(sdk, mutable, { location }, 'air');
  mutable.environment = 'staging';
  mutable.release = 'untrusted-release';
  reporter.capture(new Error('Channel dashboard failed'));
  assert.equal(sdk.events[0].environment, 'production');
  assert.equal(sdk.events[0].release, release('air'));
});

test('all shipped themes settle readiness before lazy application loading and apply shared React capture', () => {
  for (const theme of ['modern', 'air', 'berry']) {
    const base = new URL('../../web/' + theme + '/src/', import.meta.url);
    const entry = readFileSync(new URL(theme === 'modern' ? 'main.tsx' : 'index.js', base), 'utf8');
    assert.match(entry, /mountBrowserApp\(\s*browserReporterReady/);
    assert.match(entry, /import\('\.\/browser-app'\)/);
    assert.ok(entry.indexOf('mountBrowserApp(') < entry.indexOf("import('./browser-app')"));
    assert.doesNotMatch(entry, /import\s+.*from\s+['"](?:react|react-dom\/client|\.\/App)['"]/);
    const app = readFileSync(new URL(theme === 'modern' ? 'browser-app.tsx' : 'browser-app.js', base), 'utf8');
    assert.match(app, /export function mountApplication\(browserReporter/);
    assert.match(app, /<SentryBoundary>/);
    assert.match(app, /createRoot\(/);
    if (theme === 'modern') assert.match(app, /createRoot\([\s\S]*reactRootOptions\(browserReporter\)/);
    const instrument = readFileSync(new URL(theme === 'modern' ? 'browser-sentry.ts' : 'browser-sentry.js', base), 'utf8');
    assert.match(instrument, /legacy\/browser-sentry-runtime\.mjs/);
    assert.match(instrument, new RegExp("window,\\s*'" + theme + "'"));
  }
});

test('partial initialization, missing or disabled client cannot leak events through registered hooks', () => {
  for (const scenario of ['missing', 'throw', 'disabled']) {
    let options;
    const sdk = {
      init(value) {
        options = value;
        if (scenario === 'throw') throw new Error('synthetic-private');
        if (scenario === 'disabled') return { isEnabled: () => false };
      },
    };
    const reporter = createBrowserReporter(sdk, env, { location }, 'modern');
    assert.equal(reporter.enabled, false);
    const hint = { attachments: [{ filename: 'private.txt', data: 'synthetic-private' }] };
    assert.equal(options.beforeSend(fixture(), hint), null);
    assert.deepEqual(hint.attachments, []);
  }
});

test('safe breadcrumbs remain useful while console objects, private DOM and attachments are excluded', () => {
  const sdk = mockSDK();
  createBrowserReporter(sdk, env, { location }, 'modern');
  const hint = { attachments: [{ filename: 'tool-results.txt', data: 'synthetic-private' }] };
  assert.ok(sdk.options.beforeSend(fixture(), hint));
  assert.deepEqual(hint.attachments, []);
  assert.equal(
    sdk.options.beforeBreadcrumb(
      { category: 'console', message: 'Dashboard ready' },
      { input: ['Dashboard ready', { token: 'synthetic-private' }] }
    ).message,
    'Dashboard ready'
  );
  const navigation = sdk.options.beforeBreadcrumb({
    category: 'navigation',
    data: { from: '/channel?token=synthetic-private', to: '/settings#synthetic-private' },
  });
  assert.deepEqual(navigation.data, { from: '/channel', to: '/settings' });
  const click = sdk.options.beforeBreadcrumb({
    category: 'ui.click',
    message: 'button#synthetic-private[title="synthetic-private"]',
    data: { value: 'synthetic-private', id: 'synthetic-private' },
  });
  assert.equal(click.message, 'button');
  assert.doesNotMatch(JSON.stringify(click), /synthetic-private/);
});

test('theme shims embed immutable release identity, never runtime settings or arbitrary environment values', () => {
  for (const theme of ['modern', 'air', 'berry']) {
    const file = new URL('../../web/' + theme + '/src/browser-sentry.' + (theme === 'modern' ? 'ts' : 'js'), import.meta.url);
    const source = readFileSync(file, 'utf8');
    assert.doesNotMatch(source, /import\.meta\.env|VITE_SENTRY|enabled:/);
    assert.doesNotMatch(source, /@sentry\/react/);
    assert.match(source, /await import\('\.\/browser-sentry-sdk'\)/);
    assert.match(source, /__ONEAPI_BROWSER_RELEASE__/);
    const bridge = readFileSync(
      new URL('../../web/' + theme + '/src/browser-sentry-sdk.' + (theme === 'modern' ? 'ts' : 'js'), import.meta.url),
      'utf8'
    );
    for (const name of ['init', 'captureException', 'captureReactException', 'reactErrorHandler'])
      assert.match(bridge, new RegExp('\\b' + name + '\\b'));
    assert.doesNotMatch(bridge, /makeFetchTransport|globalHandlersIntegration/);
  }
});

test('runtime settings cannot provide or replace immutable build release identity', () => {
  assert.equal(readPublicConfig(env, location, 'modern', [], 'one-api@' + 'a'.repeat(40)), null);
  assert.equal(readPublicConfig(env, location, 'modern', [], release('air')), null);
  assert.equal(readConfig({ ...env, release: release('modern') }, location, 'modern'), null);
});
