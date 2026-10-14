import assert from 'node:assert/strict';
import { afterEach, mock, test } from 'node:test';
import { readFile } from 'node:fs/promises';
import { createRequire } from 'node:module';
import { createContext, SourceTextModule, SyntheticModule } from 'node:vm';
import {
  registerRequestErrorCapture,
  reportUnexpectedRequestError,
} from '../../../legacy/browser-request-errors.mjs';
import { markPrivateError } from '../../../legacy/browser-sentry-privacy.mjs';

const require = createRequire(import.meta.url);
const axios = require('axios');
const theme = 'air';
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
        },
      },
      request: {
        use(handler) {
          requestHandler = handler;
        },
      },
    },
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
        },
      },
    },
    localStorage: {
      removeItem(key) {
        removedKeys.push(key);
      },
    },
  });
  const imports = {
    axios: {
      default: {
        create(options) {
          createdOptions = options;
          return client;
        },
        isAxiosError: axios.isAxiosError,
      },
    },
    './utils': {
      showError(error) {
        uiErrors.push(error);
      },
    },
    './common': {
      showError(error) {
        uiErrors.push(error);
      },
    },
    'store/index': {
      store: {
        dispatch(action) {
          actions.push(action);
        },
      },
    },
    'store/actions': { LOGIN: 'fixture-login' },
    config: { default: { basename: '/app/' } },
    '../../../legacy/browser-request-errors.mjs': {
      reportUnexpectedRequestError,
    },
    '../../../legacy/browser-sentry-privacy.mjs': { markPrivateError },
  };
  const file = new URL('./api.js', import.meta.url);
  const module = new SourceTextModule(await readFile(file, 'utf8'), {
    context,
    identifier: file.href,
  });
  await module.link((specifier) => {
    const exports = imports[specifier];
    assert.ok(exports, 'Unexpected fixture import: ' + specifier);
    return new SyntheticModule(
      Object.keys(exports),
      function () {
        for (const [name, value] of Object.entries(exports))
          this.setExport(name, value);
      },
      { context },
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
    createdOptions,
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
      config: {},
    },
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
  test(
    'keeps HTTP ' + status + ' in the existing UI flow without capture',
    async () => {
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
    },
  );
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
    new axios.AxiosError(
      'ordinary Axios option failure',
      'ERR_BAD_OPTION_VALUE',
    ),
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
  assert.deepEqual(warnings, [
    ['Browser request error reporting unavailable.'],
  ]);
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
