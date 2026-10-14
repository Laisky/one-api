import assert from 'node:assert/strict';
import test from 'node:test';
import { sanitizeEvent } from '../../web/legacy/browser-sentry.mjs';

const config = Object.freeze({
  environment: 'staging',
  release: 'one-api-modern@sha256:' + 'a'.repeat(64),
  theme: 'modern',
  assetURLs: ['https://oneapi.laisky.com/assets/entry.js'],
});

test('ordinary diagnostics preserve message, later/vendor frames, component context and Debug IDs', () => {
  const event = {
    exception: {
      values: [
        {
          type: 'TypeError',
          value: "Cannot read properties of undefined (reading 'map')",
          stacktrace: {
            frames: [
              {
                filename: 'https://oneapi.laisky.com/assets/lazy-chat.js?token=private-canary#private-canary',
                function: 'renderChat',
                lineno: 12,
                colno: 3,
              },
              { filename: 'https://cdn.example/vendor.js', function: 'libraryCallback', lineno: 5, colno: 8 },
            ],
          },
        },
      ],
    },
    contexts: {
      react: { componentStack: '\n    at ChatMessage (https://oneapi.laisky.com/assets/lazy-chat.js:12:3)' },
      browser: { name: 'Chrome', version: '1' },
    },
    debug_meta: {
      images: [
        {
          type: 'sourcemap',
          code_file: 'https://oneapi.laisky.com/assets/lazy-chat.js?token=private-canary',
          debug_id: '12345678-1234-4234-8234-123456789abc',
        },
      ],
    },
  };
  const clean = sanitizeEvent(event, config);
  assert.equal(clean.exception.values[0].value, event.exception.values[0].value);
  assert.equal(clean.exception.values[0].type, 'TypeError');
  assert.equal(clean.exception.values[0].stacktrace.frames.length, 2);
  assert.equal(clean.exception.values[0].stacktrace.frames[0].function, 'renderChat');
  assert.equal(clean.exception.values[0].stacktrace.frames[0].filename, 'https://oneapi.laisky.com/assets/lazy-chat.js');
  assert.equal(clean.contexts.browser.name, 'Chrome');
  assert.ok(clean.contexts.react.componentStack.includes('ChatMessage'));
  assert.equal(clean.debug_meta.images[0].debug_id, event.debug_meta.images[0].debug_id);
  assert.equal(JSON.stringify(clean).includes('private-canary'), false);
});

test('private-payload negative control excludes explicit user, request body, credentials and callback arguments', () => {
  const event = {
    exception: { values: [{ type: 'Error', value: 'Safe diagnostic' }] },
    user: { email: 'private-canary' },
    request: {
      url: 'https://oneapi.laisky.com/api?token=private-canary',
      headers: { Authorization: 'private-canary' },
      data: { prompt: 'private-canary' },
      cookies: 'private-canary',
    },
    extra: {
      api_key: 'private-canary',
      arguments: [{ detail: 'private-canary' }],
      toolArguments: 'private-canary',
      history: 'private-canary',
    },
  };
  const clean = sanitizeEvent(event, config);
  assert.ok(clean);
  assert.equal(JSON.stringify(clean).includes('private-canary'), false);
  assert.equal(clean.user, undefined);
  assert.equal(clean.request?.headers, undefined);
  assert.equal(clean.request?.data, undefined);
  assert.equal(clean.extra?.arguments, undefined);
  assert.equal(clean.release, config.release);
});
