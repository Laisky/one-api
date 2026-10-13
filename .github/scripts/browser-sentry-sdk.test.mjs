import assert from 'node:assert/strict';
import { createRequire } from 'node:module';
import path from 'node:path';
import test from 'node:test';
import { createBrowserReporter } from '../../web/legacy/browser-sentry.mjs';
import { mountBrowserApp } from '../../web/legacy/browser-sentry-runtime.mjs';

const require = createRequire(path.join(process.cwd(), 'package.json'));
const { JSDOM } = require('jsdom');
const dom = new JSDOM('<!doctype html><div></div>', { url: 'https://oneapi.laisky.com/?token=secret#prompt' });
globalThis.window = dom.window;
globalThis.document = dom.window.document;
const Sentry = require('@sentry/react');
const theme = path.basename(process.cwd());

test('actual SDK11 catches globals/rejections and sanitizes internal-event bypass using mocked final transport', async () => {
  const envelopes = [];
  const sdk = {
    ...Sentry,
    makeFetchTransport() {
      return {
        send(envelope) {
          envelopes.push(envelope);
          return Promise.resolve({ statusCode: 200 });
        },
        flush() {
          return Promise.resolve(true);
        },
      };
    },
  };
  const disabledReporter = createBrowserReporter(
    sdk,
    null,
    { location: dom.window.location },
    theme,
    undefined,
    'one-api-' + theme + '@sha256:' + 'a'.repeat(64)
  );
  const disabledFailure = new Error('synthetic-private-disabled-mount-secret');
  const disabledNative = [];
  await mountBrowserApp(
    Promise.resolve(disabledReporter),
    () => {
      throw disabledFailure;
    },
    {
      queueMicrotask(callback) {
        queueMicrotask(() => {
          try {
            callback();
          } catch (error) {
            disabledNative.push(error);
          }
        });
      },
    }
  );
  await new Promise((resolve) => setImmediate(resolve));
  assert.deepEqual(disabledNative, [disabledFailure]);
  assert.equal(envelopes.length, 0);
  const reporter = createBrowserReporter(
    sdk,
    {
      dsn: 'https://0123456789abcdef0123456789abcdef@o123.ingest.sentry.io/456',
      environment: 'production',
    },
    { location: dom.window.location },
    theme,
    undefined,
    'one-api-' + theme + '@sha256:' + 'a'.repeat(64)
  );
  assert.equal(reporter.enabled, true);
  assert.equal(envelopes.length, 0);
  const mountFailure = new Error('synthetic-private-mount-secret');
  const nativeErrors = [];
  const target = {
    queueMicrotask(callback) {
      queueMicrotask(() => {
        try {
          callback();
        } catch (error) {
          nativeErrors.push(error);
          globalThis.onerror(error.message, 'https://oneapi.laisky.com/assets/index-Abcd1234.js', 1, 1, error);
        }
      });
    },
  };
  await mountBrowserApp(
    Promise.resolve(reporter),
    () => {
      throw mountFailure;
    },
    target
  );
  await new Promise((resolve) => setImmediate(resolve));
  await Sentry.flush(1000);
  assert.deepEqual(nativeErrors, [mountFailure]);
  assert.equal(envelopes.length, 1);
  Sentry.setUser({ id: 'secret', email: 'private@example.test', ip_address: '127.0.0.1' });
  Sentry.setExtra('prompt', 'secret completion history tool arguments results');
  Sentry.setTag('api_key', 'secret');
  Sentry.getCurrentScope().addAttachment({ filename: 'secret.txt', data: 'prompt' });
  reporter.capture(new TypeError('secret token prompt completion'));
  const internal = new Error('secret internal pipeline');
  Sentry.captureException(internal, { data: { __sentry__: true }, attachments: [{ filename: 'secret.txt', data: 'prompt' }] });
  assert.equal(typeof globalThis.onerror, 'function');
  globalThis.onerror('secret prompt', 'https://oneapi.laisky.com/assets/index-Abcd1234.js?token=secret', 5, 2, new RangeError('secret'));
  assert.equal(typeof globalThis.onunhandledrejection, 'function');
  globalThis.onunhandledrejection({ reason: new Error('secret rejected tool result') });
  await Sentry.flush(1000);
  assert.equal(envelopes.length, 5);
  for (const envelope of envelopes) {
    assert.equal(envelope[1].length, 1);
    assert.deepEqual(envelope[1][0][0], { type: 'event' });
    assert.equal(envelope[1][0][1].tags.theme, theme);
    assert.equal(envelope[0].trace, undefined);
    assert.doesNotMatch(JSON.stringify(envelope), /secret|prompt|completion|private@example|attachment|api_key|tool/);
  }
  await Sentry.close(1000);
});
