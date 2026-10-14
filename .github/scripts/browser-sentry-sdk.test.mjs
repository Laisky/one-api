import assert from 'node:assert/strict';
import { createRequire } from 'node:module';
import path from 'node:path';
import test from 'node:test';
import { createBrowserReporter } from '../../web/legacy/browser-sentry.mjs';
import { mountBrowserApp } from '../../web/legacy/browser-sentry-runtime.mjs';
import { markPrivateError, privateResponseError } from '../../web/legacy/browser-sentry-privacy.mjs';

const require = createRequire(path.join(process.cwd(), 'package.json'));
const { JSDOM } = require('jsdom');
const dom = new JSDOM('<!doctype html><div></div>', { url: 'https://oneapi.laisky.com/?token=synthetic-private#prompt' });
globalThis.window = dom.window;
globalThis.document = dom.window.document;
for (const name of ['Element', 'HTMLElement', 'Event', 'CustomEvent', 'ErrorEvent', 'EventTarget']) globalThis[name] = dom.window[name];
const Sentry = require('@sentry/react');
const theme = path.basename(process.cwd());
const settings = { dsn: 'https://0123456789abcdef0123456789abcdef@o123.ingest.sentry.io/456', environment: 'production' };
const release = 'one-api-' + theme + '@sha256:' + 'a'.repeat(64);
const debugID = 'aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee';

/** clearScope removes fixture data with supported SDK methods and returns nothing. */
function clearScope(scope) {
  const data = scope.getScopeData();
  for (const key of Object.keys(data.extra)) scope.setExtra(key, undefined);
  for (const key of Object.keys(data.contexts)) scope.setContext(key, null);
  for (const key of Object.keys(data.tags)) scope.setTag(key, undefined);
  scope.setUser(null);
  scope.clearBreadcrumbs();
  scope.clearAttachments();
}

/** offline creates a real SDK client with memory transport for a test callback and closes its scope afterward. */
function offline(run) {
  return () =>
    Sentry.withScope(new Sentry.Scope(), async () => {
      const envelopes = [];
      let options;
      let initCalls = 0;
      const sdk = {
        ...Sentry,
        /** init intercepts test delivery only, returning the actual pinned SDK client. */
        init(value) {
          options = value;
          initCalls++;
          return Sentry.init({
            ...value,
            transport: () => ({
              /** send records a synthetic envelope in memory and returns a successful delivery result. */
              async send(envelope) {
                envelopes.push(envelope);
                return { statusCode: 200 };
              },
              /** flush confirms that the in-memory transport has no pending delivery. */
              async flush() {
                return true;
              },
            }),
          });
        },
      };
      const target = { location: dom.window.location, document: dom.window.document };
      const reporter = createBrowserReporter(sdk, settings, target, theme, 'https://oneapi.laisky.com/assets/index-Abcd1234.js', release);
      /** events selects error event payloads from recorded envelopes without assuming a custom envelope format. */
      function events() {
        return envelopes.flatMap((envelope) => envelope[1].filter((item) => item[0].type === 'event').map((item) => item[1]));
      }
      try {
        await run({
          sdk,
          target,
          reporter,
          envelopes,
          events,
          get options() {
            return options;
          },
          get initCalls() {
            return initCalls;
          },
        });
      } finally {
        await Sentry.close(1000);
        clearScope(Sentry.getCurrentScope());
        clearScope(Sentry.getIsolationScope());
      }
    });
}

test(
  'actual SDK11 retains ordinary messages, full lazy/vendor stacks, component stacks, context and Debug IDs',
  offline(async ({ reporter, events, options }) => {
    assert.equal(Sentry.SDK_VERSION, '11.7.0');
    assert.equal(reporter.enabled, true);
    const client = Sentry.getClient();
    client.addEventProcessor((event) => ({
      ...event,
      contexts: {
        ...event.contexts,
        browser: { name: 'Firefox', version: '140' },
        operation: { name: 'render-dashboard', retry_count: 2 },
      },
      debug_meta: {
        images: [{ type: 'sourcemap', debug_id: debugID, code_file: 'https://oneapi.laisky.com/assets/settings-Abcd1234.js' }],
      },
    }));
    const failure = new TypeError('Cannot render the channel dashboard');
    failure.stack =
      'TypeError: Cannot render the channel dashboard\n    at renderDashboard (https://oneapi.laisky.com/assets/settings-Abcd1234.js:42:9)\n    at dispatch (https://cdn.example.test/vendor.js:90:3)\n    at mount (https://oneapi.laisky.com/assets/index-Abcd1234.js:10:2)';
    reporter.capture(failure, { componentStack: '\n at ChannelDashboard\n at Application' });
    await Sentry.flush(1000);
    assert.equal(events().length, 1);
    const event = events()[0];
    const wire = JSON.stringify(event);
    assert.match(wire, /Cannot render the channel dashboard/);
    assert.match(wire, /renderDashboard/);
    assert.match(wire, /dispatch/);
    assert.match(wire, /settings-Abcd1234\.js/);
    assert.match(wire, /cdn\.example\.test\/vendor\.js/);
    assert.match(wire, /ChannelDashboard/);
    assert.equal(event.contexts.browser.name, 'Firefox');
    assert.equal(event.contexts.operation.name, 'render-dashboard');
    assert.equal(event.debug_meta.images[0].debug_id, debugID);
    assert.equal(event.release, release);
    assert.equal(event.tags.theme, theme);
    assert.equal(options.transport, undefined, 'application uses the standard SDK transport; only this fixture overrides it');
    assert.notEqual(options.defaultIntegrations, false);
    assert.equal(options.dataCollection.frameContextLines, undefined);
  })
);

test(
  'actual SDK11 filters marked private linked/React/truncated copies while retaining the ordinary outer error',
  offline(async ({ reporter, events }) => {
    const payload = 'synthetic-private-response-'.repeat(40);
    const inner = markPrivateError(new Error(payload), 'Private response decoding failed');
    inner.stack = `Error: ${payload}\n    at decodeResponse (https://oneapi.laisky.com/assets/settings-Abcd1234.js:9:3)`;
    const outer = new Error('Cannot initialize channel details', { cause: inner });
    Sentry.getClient().addEventProcessor((event) => ({
      ...event,
      exception: { values: [...(event.exception?.values ?? []), { type: 'Error', value: ('Error: ' + payload).slice(0, 250) + '...' }] },
    }));
    reporter.capture(outer, { componentStack: '\n at ChannelDetails' });
    await Sentry.flush(1000);
    assert.equal(events().length, 1);
    const wire = JSON.stringify(events());
    assert.doesNotMatch(wire, /synthetic-private-response/);
    assert.match(wire, /Private response decoding failed/);
    assert.match(wire, /Cannot initialize channel details/);
    assert.match(wire, /ChannelDetails/);
    assert.match(wire, /decodeResponse/);
    assert.equal(inner.message, payload, 'telemetry filtering does not rewrite the UI error');
  })
);

test(
  'actual SDK11 removes explicit private scope/request/callback data and attachments without losing technical metadata',
  offline(async ({ reporter, events, envelopes }) => {
    Sentry.setUser({ id: 'synthetic-person', email: 'synthetic-person@example.test', ip_address: '203.0.113.10' });
    Sentry.setExtras({
      api_token: 'synthetic-private-key',
      apiToken: 'synthetic-private-key',
      apiKeys: ['synthetic-private-key'],
      sso_token: 'synthetic-private-key',
      session_token: 'synthetic-private-key',
      rawResponse: 'synthetic-private-completion',
      arguments: ['synthetic-private-callback'],
      nested: { prompt: 'synthetic-private-prompt', toolArguments: { token: 'synthetic-private-token' }, user_id: 'synthetic-person' },
      token_count: 42,
    });
    Sentry.getCurrentScope().addAttachment({ filename: 'synthetic-private.txt', data: 'synthetic-private-attachment' });
    Sentry.getClient().addEventProcessor((event) => ({
      ...event,
      request: {
        url: 'https://user:synthetic-private@oneapi.laisky.com/api/channels?token=synthetic-private#synthetic-private',
        headers: { Authorization: 'synthetic-private', Cookie: 'synthetic-private' },
        cookies: { key: 'synthetic-private' },
        data: 'synthetic-private-body',
        query_string: 'token=synthetic-private',
      },
    }));
    const failure = new Error(
      'Request https://user:synthetic-private@oneapi.laisky.com/api/channels?token=synthetic-private#synthetic-private failed'
    );
    failure.stack =
      'Error: Request failed\n    at handleResponse (https://user:synthetic-private@oneapi.laisky.com/assets/settings-Abcd1234.js?token=synthetic-private#synthetic-private:14:2)';
    reporter.capture(failure);
    await Sentry.flush(1000);
    assert.equal(events().length, 1);
    const wire = JSON.stringify(envelopes);
    assert.doesNotMatch(wire, /synthetic-private|synthetic-person|203\.0\.113\.10/);
    assert.equal(events()[0].extra.token_count, 42);
    assert.match(wire, /handleResponse/);
    assert.ok(envelopes.every((envelope) => envelope[1].every((item) => item[0].type !== 'attachment')));
  })
);

test(
  'actual SDK11 keeps useful breadcrumbs while filtering private console arguments, URLs and input DOM',
  offline(async ({ reporter, events }) => {
    Sentry.addBreadcrumb({
      category: 'navigation',
      data: { from: 'https://oneapi.laisky.com/?token=synthetic-private', to: 'https://oneapi.laisky.com/channels#synthetic-private' },
    });
    Sentry.addBreadcrumb({
      category: 'fetch',
      data: {
        method: 'GET',
        status_code: 503,
        url: 'https://user:synthetic-private@oneapi.laisky.com/api/channels?token=synthetic-private',
        body: 'synthetic-private-body',
      },
    });
    console.info('Channel dashboard mounted');
    console.warn('Unexpected request status', {
      status: 503,
      rawResponse: 'synthetic-private-response',
      api_token: 'synthetic-private-key',
    });
    const input = document.createElement('textarea');
    input.id = 'synthetic-private-input';
    const event = new dom.window.Event('click');
    Object.defineProperty(event, 'target', { value: input });
    Sentry.addBreadcrumb({ category: 'ui.click', message: input.id }, { event });
    reporter.capture(new Error('Dashboard refresh failed'));
    await Sentry.flush(1000);
    const wire = JSON.stringify(events());
    assert.doesNotMatch(wire, /synthetic-private/);
    assert.match(wire, /Channel dashboard mounted/);
    assert.match(wire, /Unexpected request status/);
    assert.match(wire, /status_code/);
    assert.match(wire, /\/channels/);
  })
);

test(
  'actual SDK11 captures global errors and escaped expected response errors with safe diagnostics',
  offline(async ({ events, envelopes }) => {
    const ordinary = new RangeError('Unexpected dashboard range');
    assert.equal(typeof globalThis.onerror, 'function');
    globalThis.onerror(ordinary.message, 'https://oneapi.laisky.com/assets/settings-Abcd1234.js', 5, 2, ordinary);
    const expected = privateResponseError('synthetic-private-server-body', 'Channel request failed (HTTP 403)');
    assert.equal(typeof globalThis.onunhandledrejection, 'function');
    globalThis.onunhandledrejection({ reason: expected });
    await Sentry.flush(1000);
    assert.equal(events().length, 2);
    const wire = JSON.stringify(events());
    assert.match(wire, /Unexpected dashboard range/);
    assert.match(wire, /Channel request failed \(HTTP 403\)/);
    assert.doesNotMatch(wire, /synthetic-private/);
    assert.ok(
      envelopes.every((envelope) =>
        envelope[1].every(
          (item) => !['session', 'transaction', 'replay_event', 'replay_recording', 'profile', 'log'].includes(item[0].type)
        )
      )
    );
  })
);

test(
  'actual BrowserApiErrors excludes CustomEvent callback arguments while retaining the ordinary exception',
  offline(async ({ events }) => {
    const target = document.createElement('button');
    /** preventNativeConsole preserves the expected DOM error path without printing its synthetic fixture. */
    function preventNativeConsole(event) {
      event.preventDefault();
    }
    /** failHandler throws an ordinary callback error without reading the event's private tool payload. */
    function failHandler() {
      throw new Error('Dashboard event handler failed');
    }
    dom.window.addEventListener('error', preventNativeConsole);
    target.addEventListener('synthetic-test', failHandler);
    try {
      target.dispatchEvent(new dom.window.CustomEvent('synthetic-test', { detail: { toolArguments: 'synthetic-private-callback' } }));
      await Sentry.flush(1000);
      assert.equal(events().length, 1);
      assert.match(JSON.stringify(events()), /Dashboard event handler failed/);
      assert.doesNotMatch(JSON.stringify(events()), /synthetic-private-callback/);
      assert.equal(events()[0].extra?.arguments, undefined);
    } finally {
      target.removeEventListener('synthetic-test', failHandler);
      dom.window.removeEventListener('error', preventNativeConsole);
    }
  })
);

test(
  'actual SDK11 idempotence and native mount/global capture preserve disabled behavior and emit once',
  offline(async ({ sdk, target, reporter, events, initCalls }) => {
    assert.equal(createBrowserReporter(sdk, settings, target, theme, undefined, release), reporter);
    assert.equal(initCalls, 1);
    const disabled = createBrowserReporter(sdk, null, { location: dom.window.location }, theme, undefined, release);
    const disabledFailure = new Error('Disabled mount failed');
    const nativeDisabled = [];
    await mountBrowserApp(
      Promise.resolve(disabled),
      () => {
        throw disabledFailure;
      },
      {
        /** queueMicrotask records the native disabled mount exception without routing it to the enabled SDK. */
        queueMicrotask(callback) {
          queueMicrotask(() => {
            try {
              callback();
            } catch (error) {
              nativeDisabled.push(error);
            }
          });
        },
      }
    );
    await new Promise((resolve) => setImmediate(resolve));
    assert.deepEqual(nativeDisabled, [disabledFailure]);
    assert.equal(events().length, 0);
    const failure = new Error('Application mount failed');
    const nativeEnabled = [];
    await mountBrowserApp(
      Promise.resolve(reporter),
      () => {
        throw failure;
      },
      {
        /** queueMicrotask routes the native mount failure to the SDK global handler after application capture. */
        queueMicrotask(callback) {
          queueMicrotask(() => {
            try {
              callback();
            } catch (error) {
              nativeEnabled.push(error);
              globalThis.onerror(error.message, 'https://oneapi.laisky.com/assets/index-Abcd1234.js', 1, 1, error);
            }
          });
        },
      }
    );
    await new Promise((resolve) => setImmediate(resolve));
    reporter.capture(failure);
    await Sentry.flush(1000);
    assert.deepEqual(nativeEnabled, [failure]);
    assert.equal(events().length, 1);
    assert.match(JSON.stringify(events()), /Application mount failed/);
  })
);
