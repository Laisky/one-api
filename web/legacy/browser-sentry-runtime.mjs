import { createBrowserReporter, readConfig } from './browser-sentry.mjs';

const ready = new WeakMap();
const disabled = Object.freeze({ enabled: false, capture() {} });
const MAX_BYTES = 2048;

/** readRuntimeResponse consumes at most 2048 bytes of a JSON public configuration response, rejecting redirects and unexpected shapes. */
async function readRuntimeResponse(response) {
  if (
    !response?.ok ||
    response.redirected ||
    !/^application\/json(?:;|$)/i.test(response.headers?.get('content-type') || '') ||
    Number(response.headers?.get('content-length') || 0) > MAX_BYTES ||
    !response.body?.getReader
  )
    return null;
  const reader = response.body.getReader();
  let bytes = 0;
  const chunks = [];
  try {
    for (;;) {
      const part = await reader.read();
      if (part.done) break;
      if (!(part.value instanceof Uint8Array) || !Number.isSafeInteger(part.value.byteLength) || chunks.length >= MAX_BYTES) return null;
      bytes += part.value.byteLength;
      if (bytes > MAX_BYTES) return null;
      chunks.push(part.value);
    }
    const joined = new Uint8Array(bytes);
    let offset = 0;
    for (const chunk of chunks) {
      joined.set(chunk, offset);
      offset += chunk.byteLength;
    }
    const data = JSON.parse(new TextDecoder('utf-8', { fatal: true }).decode(joined));
    if (!data || Array.isArray(data) || Object.keys(data).length !== 1 || !Object.hasOwn(data, 'sentry')) return null;
    return data.sentry;
  } finally {
    await reader.cancel().catch(() => {});
  }
}

/** loadBrowserReporter fetches public runtime configuration once per window before rendering, with separate bounded configuration and lazy SDK deadlines and no error buffering or automatic retries. */
export function loadBrowserReporter(loadSDK, release, target, theme, entryURL, timeoutMs = 1000) {
  if (!target || (typeof target !== 'object' && typeof target !== 'function')) return Promise.resolve(disabled);
  if (ready.has(target)) return ready.get(target);
  const promise = (async () => {
    if (
      target.location?.hostname !== 'oneapi.laisky.com' ||
      target.location?.protocol !== 'https:' ||
      target.location?.port ||
      !['modern', 'air', 'berry'].includes(theme) ||
      !new RegExp('^one-api-' + theme + '@sha256:[a-f0-9]{64}$').test(release) ||
      typeof target.fetch !== 'function'
    )
      return disabled;
    const controller = new AbortController();
    let timer;
    try {
      const deadline = new Promise((resolve) => {
        timer = setTimeout(
          () => {
            try {
              controller.abort();
            } catch {
              /* Cleanup failure leaves reporting disabled without rejecting readiness. */
            }
            resolve(null);
          },
          Math.min(Math.max(timeoutMs, 1), 1000)
        );
      });
      const request = Promise.resolve()
        .then(() =>
          target.fetch('/api/browser-config', {
            method: 'GET',
            credentials: 'omit',
            referrerPolicy: 'no-referrer',
            mode: 'same-origin',
            redirect: 'error',
            cache: 'no-store',
            signal: controller.signal,
          })
        )
        .then(readRuntimeResponse)
        .catch(() => null);
      const settings = await Promise.race([request, deadline]);
      clearTimeout(timer);
      const config = readConfig(settings, target.location, theme, [], release);
      if (!config || typeof loadSDK !== 'function') return createBrowserReporter({}, null, target, theme, entryURL, release);
      const publicSettings = Object.freeze({ dsn: config.dsn, environment: config.environment });
      const sdkDeadline = new Promise((resolve) => {
        timer = setTimeout(() => resolve(null), Math.min(Math.max(timeoutMs, 1), 1000));
      });
      const sdk = await Promise.race([
        Promise.resolve()
          .then(loadSDK)
          .catch(() => null),
        sdkDeadline,
      ]);
      if (
        !sdk ||
        !['init', 'captureException', 'globalHandlersIntegration', 'makeFetchTransport'].every((name) => typeof sdk[name] === 'function')
      )
        return disabled;
      return createBrowserReporter(sdk, publicSettings, target, theme, entryURL, release);
    } catch {
      return disabled;
    } finally {
      clearTimeout(timer);
      try {
        controller.abort();
      } catch {
        /* Cleanup failure leaves reporting disabled without rejecting readiness. */
      }
    }
  })().catch(() => disabled);
  ready.set(target, promise);
  return promise;
}

/** mountBrowserApp owns readiness/mount rejections and rethrows the original failure through the native browser error path without swallowing errors or retrying rendering. */
export function mountBrowserApp(readiness, mount, target) {
  return readiness.then(mount).catch((error) => {
    /** rethrow preserves the original mount error for the browser's native error dispatcher. */
    function rethrow() {
      throw error;
    }
    try {
      if (typeof target?.queueMicrotask === 'function') {
        target.queueMicrotask(rethrow);
        return;
      }
    } catch {
      /* Fall back to the native timer path if the optional microtask scheduler fails. */
    }
    setTimeout(rethrow, 0);
  });
}
