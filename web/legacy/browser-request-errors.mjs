import { isExpectedRequestError } from './browser-sentry-privacy.mjs';

let registration;

/** ownValue reads an own data descriptor without invoking request or error getters. */
function ownValue(value, key) {
  if (!value || (typeof value !== 'object' && typeof value !== 'function')) return undefined;
  const descriptor = Object.getOwnPropertyDescriptor(value, key);
  return descriptor && 'value' in descriptor ? descriptor.value : undefined;
}

/** errorName reads data names or the trusted DOMException name getter with its native brand check. */
function errorName(error) {
  const original = error;
  const nativePrototype = typeof DOMException === 'function' ? ownValue(DOMException, 'prototype') : undefined;
  for (let depth = 0; error && depth < 4; depth++, error = Object.getPrototypeOf(error)) {
    const name = ownValue(error, 'name');
    if (typeof name === 'string') return name;
    if (nativePrototype && error === nativePrototype) {
      const getter = Object.getOwnPropertyDescriptor(nativePrototype, 'name')?.get;
      if (typeof getter === 'function') {
        try {
          // Call only the native accessor; its brand check rejects lookalikes and wrapped objects.
          const nativeName = Reflect.apply(getter, original, []);
          if (typeof nativeName === 'string') return nativeName;
        } catch {
          // Arbitrary application name accessors remain unread, including failed native lookalikes.
        }
      }
    }
  }
  return undefined;
}

/** isExpectedFailure excludes explicit private responses, cancellation and HTTP response errors. */
function isExpectedFailure(error) {
  if (isExpectedRequestError(error)) return true;
  const name = errorName(error);
  if (name === 'AbortError' || name === 'CanceledError' || ownValue(error, 'code') === 'ERR_CANCELED') return true;
  const status = ownValue(ownValue(error, 'response'), 'status');
  return typeof status === 'number' && Number.isInteger(status) && status >= 100 && status <= 599;
}

/** registerRequestErrorCapture enables caught-error reporting after SDK initialization and returns an owned cleanup. */
export function registerRequestErrorCapture(capture) {
  if (typeof capture !== 'function') return () => {};
  const current = { capture, seen: new WeakSet() };
  registration = current;
  return () => {
    if (registration === current) registration = undefined;
  };
}

/** reportUnexpectedRequestError reports one unexpected error while preserving the caller's original UI flow. */
export function reportUnexpectedRequestError(error) {
  const current = registration;
  if (!current || error == null) return false;
  const objectError = typeof error === 'object' || typeof error === 'function';
  try {
    if (isExpectedFailure(error) || (objectError && current.seen.has(error))) return false;
    // Register before invoking the callback so reentrant reporting cannot duplicate an event.
    if (objectError) current.seen.add(error);
    const captured = current.capture(error);
    if (captured === false) {
      if (objectError) current.seen.delete(error);
      return false;
    }
    return true;
  } catch {
    if (objectError) current.seen.delete(error);
    try {
      console.warn('Browser request error reporting unavailable.');
    } catch {
      /* Preserve the original request UI handler if console reporting fails. */
    }
    return false;
  }
}
