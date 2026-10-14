import assert from 'node:assert/strict';
import test from 'node:test';
import { registerRequestErrorCapture, reportUnexpectedRequestError } from '../../web/legacy/browser-request-errors.mjs';
import { markPrivateError, privateResponseError } from '../../web/legacy/browser-sentry-privacy.mjs';

const privateText = 'synthetic-private-request-fixture';

/** recordErrors registers a synthetic callback and returns its captured objects and cleanup function. */
function recordErrors() {
  const captured = [];
  const unregister = registerRequestErrorCapture(function recordOriginalError(error) {
    // recordOriginalError records the original synthetic error object without accessing its contents.
    captured.push(error);
  });
  return { captured, unregister };
}

// leavesUnconfiguredErrorsUnseen verifies disabled reporting does not consume an error before SDK registration.
test('unconfigured reporting is a no-op and the same object can be captured after registration', function leavesUnconfiguredErrorsUnseen() {
  const error = new TypeError('Failed to fetch');
  assert.equal(reportUnexpectedRequestError(error), false);
  const registry = recordErrors();
  try {
    assert.equal(reportUnexpectedRequestError(error), true);
    assert.deepEqual(registry.captured, [error]);
  } finally {
    registry.unregister();
  }
  assert.equal(reportUnexpectedRequestError(new Error('Offline')), false);
});

// capturesEachOriginalObjectOnce verifies object identity deduplication across repeated handler calls.
test('each original unexpected error object is captured once and distinct objects remain independent', function capturesEachOriginalObjectOnce() {
  const registry = recordErrors();
  const first = new Error('Network connection failed');
  const second = new Error('Network connection failed');
  try {
    assert.equal(reportUnexpectedRequestError(first), true);
    assert.equal(reportUnexpectedRequestError(first), false);
    assert.equal(reportUnexpectedRequestError(second), true);
    assert.equal(reportUnexpectedRequestError(second), false);
    assert.equal(registry.captured.length, 2);
    assert.equal(registry.captured[0], first);
    assert.equal(registry.captured[1], second);
  } finally {
    registry.unregister();
  }
  let reentrantCalls = 0;
  let nestedResult;
  const unregisterReentrant = registerRequestErrorCapture(function captureWithReentrantHandler(error) {
    // captureWithReentrantHandler records whether reporting the same object recursively is suppressed.
    reentrantCalls++;
    nestedResult = reportUnexpectedRequestError(error);
  });
  try {
    assert.equal(reportUnexpectedRequestError(first), true);
    assert.equal(nestedResult, false);
    assert.equal(reentrantCalls, 1);
  } finally {
    unregisterReentrant();
  }
});

// preservesNewerRegistration verifies old cleanup cannot remove a newer owner and registration resets deduplication.
test('new registration resets deduplication and old unregister only releases its own registration', function preservesNewerRegistration() {
  const original = recordErrors();
  const error = new Error('Browser request failed');
  assert.equal(reportUnexpectedRequestError(error), true);
  const current = recordErrors();
  try {
    original.unregister();
    assert.equal(reportUnexpectedRequestError(error), true);
    assert.equal(reportUnexpectedRequestError(error), false);
    const next = new Error('A second browser request failed');
    assert.equal(reportUnexpectedRequestError(next), true);
    assert.deepEqual(original.captured, [error]);
    assert.deepEqual(current.captured, [error, next]);
  } finally {
    current.unregister();
    original.unregister();
  }
  assert.equal(reportUnexpectedRequestError(new Error('No SDK registration')), false);
});

// skipsMarkedExpectedRequests verifies source-marked private responses and expected linked causes never reach capture.
test('private response errors and explicitly expected marked causes are skipped', function skipsMarkedExpectedRequests() {
  const registry = recordErrors();
  const privateResponse = privateResponseError(privateText, 'Provider request rejected');
  const expected = markPrivateError(new Error(privateText), 'Authentication required', true);
  const wrapper = new Error('Request wrapper failed', { cause: expected });
  const unexpected = markPrivateError(new Error(privateText), 'Unexpected provider parsing failure', false);
  try {
    assert.equal(reportUnexpectedRequestError(privateResponse), false);
    assert.equal(reportUnexpectedRequestError(expected), false);
    assert.equal(reportUnexpectedRequestError(wrapper), false);
    assert.equal(reportUnexpectedRequestError(unexpected), true);
    assert.deepEqual(registry.captured, [unexpected]);
    assert.equal(privateResponse.message, privateText);
    assert.equal(expected.message, privateText);
  } finally {
    registry.unregister();
  }
});

// skipsCancellationErrors verifies browser and Axios cancellation classifications suppress capture without changing errors.
test('native AbortError and Axios cancellation errors are skipped', function skipsCancellationErrors() {
  const registry = recordErrors();
  const errors = [
    new DOMException('The request was aborted', 'AbortError'),
    Object.assign(new Error('The request was canceled'), { code: 'ERR_CANCELED' }),
    Object.assign(new Error('The request was canceled'), { name: 'CanceledError' }),
  ];
  try {
    for (const error of errors) assert.equal(reportUnexpectedRequestError(error), false);
    assert.deepEqual(registry.captured, []);
    assert.equal(errors[1].code, 'ERR_CANCELED');
    assert.equal(errors[2].name, 'CanceledError');
  } finally {
    registry.unregister();
  }
});

// distinguishesHTTPResponses verifies only actual integer HTTP status codes suppress request-error capture.
test('actual HTTP response status codes are skipped while malformed or unrelated status values are captured', function distinguishesHTTPResponses() {
  const registry = recordErrors();
  try {
    for (const status of [100, 200, 400, 500, 599]) {
      const error = Object.assign(new Error('HTTP response rejected'), { response: { status } });
      assert.equal(reportUnexpectedRequestError(error), false);
    }
    const unexpected = [];
    for (const status of [0, 99, 600, 999, 200.5, '401', undefined]) {
      const error = Object.assign(new Error('No valid HTTP response'), { response: { status } });
      unexpected.push(error);
      assert.equal(reportUnexpectedRequestError(error), true);
    }
    const unrelated = Object.assign(new Error('Unexpected application failure'), { status: 401 });
    unexpected.push(unrelated);
    assert.equal(reportUnexpectedRequestError(unrelated), true);
    assert.deepEqual(registry.captured, unexpected);
  } finally {
    registry.unregister();
  }
});

// preservesUnexpectedFetchError verifies normal failed-fetch diagnostics preserve original message, stack and identity.
test('normal failed-fetch TypeError reaches capture unchanged and caller UI handling remains available', function preservesUnexpectedFetchError() {
  const registry = recordErrors();
  const error = new TypeError('Failed to fetch');
  const stack = error.stack;
  let handled;
  try {
    assert.equal(reportUnexpectedRequestError(error), true);
    handled = error.message;
    assert.equal(registry.captured[0], error);
    assert.equal(error.message, 'Failed to fetch');
    assert.equal(error.stack, stack);
    assert.equal(handled, 'Failed to fetch');
  } finally {
    registry.unregister();
  }
});

// containsCaptureFailure verifies a failing SDK callback cannot break the caller or expose private error contents in warnings.
test('capture failure returns false and emits only a static string warning while caller UI handling continues', function containsCaptureFailure() {
  const originalWarn = console.warn;
  const warnings = [];
  console.warn = function recordWarning(...values) {
    // recordWarning records synthetic warnings for privacy assertions and returns no value.
    warnings.push(values);
  };
  let attempts = 0;
  const unregister = registerRequestErrorCapture(function failThenAcceptSyntheticCapture() {
    // failThenAcceptSyntheticCapture first throws, then declines capture, and finally accepts the same synthetic object.
    attempts++;
    if (attempts === 1) throw new Error(privateText);
    return attempts > 2;
  });
  const error = new Error(privateText);
  let handled = false;
  try {
    assert.doesNotThrow(function reportAndHandleError() {
      // reportAndHandleError verifies failed reporting still permits the existing caller handler to run.
      assert.equal(reportUnexpectedRequestError(error), false);
      handled = true;
    });
    assert.equal(handled, true);
    assert.equal(reportUnexpectedRequestError(error), false);
    assert.equal(reportUnexpectedRequestError(error), true);
    assert.equal(reportUnexpectedRequestError(error), false);
    assert.equal(attempts, 3);
    assert.ok(warnings.length > 0);
    for (const values of warnings) {
      assert.equal(values.length, 1);
      assert.equal(typeof values[0], 'string');
      assert.doesNotMatch(values[0], /synthetic-private-request-fixture/);
    }
    assert.equal(error.message, privateText);
    const recordedWarn = console.warn;
    const unregisterBrokenLogger = registerRequestErrorCapture(function failCaptureWithBrokenLogger() {
      // failCaptureWithBrokenLogger throws a synthetic private failure while the warning implementation is unavailable.
      throw new Error(privateText);
    });
    console.warn = function throwsPrivateWarningFailure() {
      // throwsPrivateWarningFailure simulates a failing warning implementation and returns no value.
      throw new Error(privateText);
    };
    let handledWithBrokenLogger = false;
    try {
      assert.doesNotThrow(function reportWithBrokenLoggerAndHandle() {
        // reportWithBrokenLoggerAndHandle verifies both capture and logging failures leave caller UI handling available.
        assert.equal(reportUnexpectedRequestError(new Error(privateText)), false);
        handledWithBrokenLogger = true;
      });
      assert.equal(handledWithBrokenLogger, true);
    } finally {
      unregisterBrokenLogger();
      console.warn = recordedWarn;
    }
  } finally {
    unregister();
    console.warn = originalWarn;
  }
});

// isolatesHostileClassification verifies getters are skipped and hostile proxy traps fail without exposing private contents.
test('classification skips getters and contains hostile proxies without exposing private contents', function isolatesHostileClassification() {
  const registry = recordErrors();
  const originalWarn = console.warn;
  const warnings = [];
  console.warn = function recordClassificationWarning(...values) {
    // recordClassificationWarning records safe classification warnings without accessing error values.
    warnings.push(values);
  };
  const getterError = new Error('Unexpected transport failure');
  let getterCalls = 0;
  Object.defineProperty(getterError, 'response', {
    get: function throwsPrivateResponseGetter() {
      // throwsPrivateResponseGetter simulates hostile response metadata and returns no value.
      getterCalls++;
      throw new Error(privateText);
    },
  });
  const proxyError = new Proxy(new Error('Unexpected transport failure'), {
    getOwnPropertyDescriptor: function throwsPrivateProxyDescriptor() {
      // throwsPrivateProxyDescriptor simulates hostile descriptor access and returns no value.
      throw new Error(privateText);
    },
    getPrototypeOf: function throwsPrivateProxyPrototype() {
      // throwsPrivateProxyPrototype simulates hostile prototype access and returns no value.
      throw new Error(privateText);
    },
  });
  try {
    assert.equal(reportUnexpectedRequestError(getterError), true);
    assert.equal(getterCalls, 0);
    assert.doesNotThrow(function safelyClassifyProxy() {
      // safelyClassifyProxy checks that hostile classification returns false without throwing.
      assert.equal(reportUnexpectedRequestError(proxyError), false);
    });
    assert.deepEqual(registry.captured, [getterError]);
    for (const values of warnings) {
      assert.ok(
        values.every(function isStaticWarning(value) {
          // isStaticWarning returns whether the warning argument is a string without private synthetic content.
          return typeof value === 'string' && !value.includes(privateText);
        })
      );
    }
  } finally {
    registry.unregister();
    console.warn = originalWarn;
  }
});
