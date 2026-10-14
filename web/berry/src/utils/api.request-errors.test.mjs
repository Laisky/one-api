import assert from 'node:assert/strict';
import { afterEach, mock, test } from 'node:test';
import { readFile } from 'node:fs/promises';
import { createRequire } from 'node:module';
import { createContext, SourceTextModule, SyntheticModule } from 'node:vm';
import { registerRequestErrorCapture, reportUnexpectedRequestError } from '../../../legacy/browser-request-errors.mjs';
import { markPrivateError, sanitizeEvent } from '../../../legacy/browser-sentry-privacy.mjs';

const require = createRequire(import.meta.url);
const axios = require('axios');
const theme = 'berry';
let unregister = () => {};

afterEach(() => {
  unregister();
  unregister = () => {};
  mock.restoreAll();
});

/** createHarness evaluates the actual API client with offline UI, Axios factory and browser imports. */
async function createHarness(pathname = '/app/channels') {
  const uiErrors = [];
  const removedKeys = [];
  const redirects = [];
  const actions = [];
  let rejectedResponse;
  let fulfilledResponse;
  let requestHandler;
  let createdOptions;
  const client = {
    interceptors: {
      response: {
        use(fulfilled, rejected) {
          fulfilledResponse = fulfilled;
          rejectedResponse = rejected;
        }
      },
      request: {
        use(handler) {
          requestHandler = handler;
        }
      }
    }
  };
  const context = createContext({
    console,
    process: { env: {} },
    URL,
    window: {
      location: {
        pathname,
        origin: 'https://oneapi.laisky.com',
        replace(path) {
          redirects.push(path);
        }
      }
    },
    localStorage: {
      removeItem(key) {
        removedKeys.push(key);
      }
    }
  });
  const imports = {
    axios: {
      default: {
        create(options) {
          createdOptions = options;
          return client;
        },
        isAxiosError: axios.isAxiosError
      }
    },
    './utils': {
      showError(error) {
        uiErrors.push(error);
      }
    },
    './common': {
      showError(error) {
        uiErrors.push(error);
      }
    },
    'store/index': {
      store: {
        dispatch(action) {
          actions.push(action);
        }
      }
    },
    'store/actions': { LOGIN: 'fixture-login' },
    config: { default: { basename: '/app/' } },
    '../../../legacy/browser-request-errors.mjs': { reportUnexpectedRequestError },
    '../../../legacy/browser-sentry-privacy.mjs': { markPrivateError }
  };
  const file = new URL('./api.js', import.meta.url);
  const module = new SourceTextModule(await readFile(file, 'utf8'), { context, identifier: file.href });
  await module.link((specifier) => {
    const exports = imports[specifier];
    assert.ok(exports, 'Unexpected fixture import: ' + specifier);
    return new SyntheticModule(
      Object.keys(exports),
      function () {
        for (const [name, value] of Object.entries(exports)) this.setExport(name, value);
      },
      { context }
    );
  });
  await module.evaluate();
  assert.equal(module.namespace.API, client);
  assert.equal(typeof rejectedResponse, 'function');
  return {
    reject: rejectedResponse,
    fulfill: fulfilledResponse,
    request: requestHandler,
    uiErrors,
    removedKeys,
    redirects,
    actions,
    createdOptions
  };
}

/** rejectSame checks the actual interceptor's preserved rejection identity. */
async function rejectSame(harness, error) {
  await assert.rejects(harness.reject(error), (received) => {
    assert.equal(received, error);
    return true;
  });
}

/** networkError creates a native Axios failure with an actual request and no HTTP response. */
function networkError() {
  return new axios.AxiosError('Network Error', 'ERR_NETWORK', undefined, {});
}

/** httpError creates a native Axios response failure without performing a real request. */
function httpError(status, data = {}) {
  return new axios.AxiosError(
    'Original HTTP failure',
    'ERR_BAD_RESPONSE',
    undefined,
    {},
    {
      status,
      statusText: 'fixture status',
      data,
      headers: {},
      config: {}
    }
  );
}

test('captures one actual transport error across repeated handling while preserving UI and rejection', async () => {
  const captured = [];
  unregister = registerRequestErrorCapture((error) => {
    captured.push(error);
  });
  const harness = await createHarness();
  const error = networkError();
  const stack = error.stack;
  await rejectSame(harness, error);
  await rejectSame(harness, error);
  assert.deepEqual(captured, [error]);
  assert.deepEqual(harness.uiErrors, [error, error]);
  assert.equal(error.name, 'AxiosError');
  assert.equal(error.message, 'Network Error');
  assert.equal(error.stack, stack);
});

for (const status of [400, 429, 500]) {
  test('keeps HTTP ' + status + ' in the existing UI flow without capture', async () => {
    const captured = [];
    unregister = registerRequestErrorCapture((error) => {
      captured.push(error);
    });
    const harness = await createHarness();
    const error = httpError(status);
    await rejectSame(harness, error);
    assert.deepEqual(captured, []);
    assert.deepEqual(harness.uiErrors, [error]);
    assert.equal(error.response.status, status);
  });
}

test('keeps Axios cancellation out of capture and preserves its UI handoff and rejection', async () => {
  const captured = [];
  unregister = registerRequestErrorCapture((error) => {
    captured.push(error);
  });
  const harness = await createHarness();
  const error = new axios.CanceledError('fixture cancellation', undefined, {});
  await rejectSame(harness, error);
  assert.deepEqual(captured, []);
  assert.deepEqual(harness.uiErrors, [error]);
});

test('does not misclassify ordinary setup errors as actual Axios transport failures', async () => {
  const captured = [];
  unregister = registerRequestErrorCapture((error) => {
    captured.push(error);
  });
  const harness = await createHarness();
  const errors = [
    new TypeError('ordinary setup implementation failure'),
    new axios.AxiosError('ordinary Axios option failure', 'ERR_BAD_OPTION_VALUE')
  ];
  for (const error of errors) await rejectSame(harness, error);
  assert.deepEqual(captured, []);
  assert.deepEqual(harness.uiErrors, errors);
});

test('keeps the same UI and rejection when SDK capture is inactive', async () => {
  const harness = await createHarness();
  const error = networkError();
  await rejectSame(harness, error);
  assert.deepEqual(harness.uiErrors, [error]);
});

test('preserves UI and rejection when the registered reporter fails, using a static diagnostic', async () => {
  let attempts = 0;
  const warnings = [];
  unregister = registerRequestErrorCapture(() => {
    attempts++;
    throw new Error('synthetic-private-reporter-failure');
  });
  mock.method(console, 'warn', (...args) => {
    warnings.push(args);
  });
  const harness = await createHarness();
  const error = networkError();
  await rejectSame(harness, error);
  assert.equal(attempts, 1);
  assert.deepEqual(warnings, [['Browser request error reporting unavailable.']]);
  assert.deepEqual(harness.uiErrors, [error]);
});

test('preserves successful response and configured base URL without capture', async () => {
  const captured = [];
  unregister = registerRequestErrorCapture((error) => {
    captured.push(error);
  });
  const harness = await createHarness();
  const response = { data: { success: true } };
  assert.equal(harness.fulfill(response), response);
  assert.equal(harness.createdOptions.baseURL, theme === 'berry' ? '/' : '');
  assert.deepEqual(captured, []);
});

test('keeps a copied private HTTP body in UI while redacting its escaped event value', async () => {
  const captured = [];
  unregister = registerRequestErrorCapture((error) => {
    captured.push(error);
  });
  const harness = await createHarness();
  const canary = 'synthetic-private-http-body-canary';
  const error = httpError(400, { message: canary });
  const stack = error.stack;
  await rejectSame(harness, error);
  assert.deepEqual(captured, []);
  assert.deepEqual(harness.uiErrors, [error]);
  assert.equal(error.message, canary);
  assert.equal(error.name, 'AxiosError');
  assert.ok(error instanceof axios.AxiosError);
  assert.equal(error.response.status, 400);
  assert.equal(error.stack, stack);
  const sanitized = sanitizeEvent(
    {
      exception: { values: [{ type: error.name, value: error.message }] }
    },
    { theme: 'berry', release: 'one-api@abcdef0', environment: 'test' },
    { originalException: error }
  );
  assert.ok(sanitized);
  assert.equal(sanitized.exception.values[0].value, 'Request failed (HTTP 400)');
  assert.equal(JSON.stringify(sanitized).includes(canary), false);
  assert.equal(error.message, canary);
});

test('uses a controlled unknown status rather than reflecting malformed private status metadata', async () => {
  const harness = await createHarness();
  const error = httpError('synthetic-private-status', { message: 'synthetic-private-status-body' });
  await rejectSame(harness, error);
  const sanitized = sanitizeEvent(
    {
      exception: { values: [{ type: error.name, value: error.message }] }
    },
    { theme: 'berry', release: 'one-api@abcdef0', environment: 'test' },
    { originalException: error }
  );
  assert.ok(sanitized);
  assert.equal(sanitized.exception.values[0].value, 'Request failed (HTTP unknown)');
  assert.equal(error.response.status, 'synthetic-private-status');
  assert.equal(error.message, 'synthetic-private-status-body');
});

test('preserves the existing 401 cleanup and redirect gate without capture', async () => {
  const captured = [];
  unregister = registerRequestErrorCapture((error) => {
    captured.push(error);
  });
  const harness = await createHarness();
  const error = httpError(401, { message: 'fixture authentication response' });
  await rejectSame(harness, error);
  await rejectSame(harness, error);
  assert.deepEqual(captured, []);
  assert.deepEqual(harness.removedKeys, ['user', 'token']);
  assert.equal(harness.actions.length, 1);
  assert.equal(harness.actions[0].type, 'fixture-login');
  assert.equal(harness.actions[0].payload, null);
  assert.deepEqual(harness.redirects, ['/app/login']);
  assert.deepEqual(harness.uiErrors, [error, error]);
  assert.equal(error.message, 'fixture authentication response');
});

test('preserves Berry request caching and leaves existing Authorization unchanged', async () => {
  const harness = await createHarness();
  const config = { method: 'GET', url: '/api/models?fixture=1', headers: { Authorization: 'Bearer fixture' } };
  const returned = harness.request(config);
  assert.equal(returned, config);
  assert.equal(config.headers.Authorization, 'Bearer fixture');
  assert.equal(config.headers['Cache-Control'], 'no-cache, no-store, must-revalidate');
  assert.equal(config.headers.Pragma, 'no-cache');
  assert.equal(config.headers.Expires, '0');
  const url = new URL(config.url, 'https://oneapi.laisky.com');
  assert.equal(url.pathname, '/api/models');
  assert.equal(url.searchParams.get('fixture'), '1');
  assert.match(url.searchParams.get('_'), /^[0-9]+$/);
});
