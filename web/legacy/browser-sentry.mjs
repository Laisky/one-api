const HOST = 'oneapi.laisky.com';
const reporters = new WeakMap();
const boundaryMarker = Symbol('OneAPI reporting boundary');
const TYPES = new Set(['Error', 'TypeError', 'RangeError', 'ReferenceError', 'SyntaxError', 'URIError', 'EvalError', 'AggregateError']);
const THEMES = new Set(['modern', 'air', 'berry']);

/** readConfig validates explicit public build configuration and returns it, or null. */
export function readConfig(env, location, theme, assets = []) {
  if (location?.hostname !== HOST || location?.protocol !== 'https:' || Boolean(location?.port) || !THEMES.has(theme)) return null;
  if (env?.VITE_SENTRY_ENABLED !== 'true' || !['production', 'staging', 'development'].includes(env.VITE_SENTRY_ENVIRONMENT)) return null;
  if (typeof env.VITE_SENTRY_RELEASE !== 'string' || !/^one-api@[a-f0-9]{40}$/.test(env.VITE_SENTRY_RELEASE)) return null;
  try {
    const dsn = new URL(env.VITE_SENTRY_DSN);
    if (
      dsn.protocol !== 'https:' ||
      dsn.password ||
      dsn.port ||
      dsn.search ||
      dsn.hash ||
      !/^[a-f0-9]{32}$/.test(dsn.username) ||
      !/^o[0-9]+\.ingest(?:\.[a-z]+)?\.sentry\.io$/.test(dsn.hostname) ||
      !/^\/[0-9]+$/.test(dsn.pathname)
    )
      return null;
    const assetURLs = Object.freeze(assets.slice(0, 40).map(safeAssetURL).filter(Boolean));
    return Object.freeze({ dsn: dsn.href, environment: env.VITE_SENTRY_ENVIRONMENT, release: env.VITE_SENTRY_RELEASE, theme, assetURLs });
  } catch {
    return null;
  }
}

/** safeAssetURL retains bounded first-party static JS asset paths, excluding credentials, query and fragment. */
export function safeAssetURL(value) {
  if (typeof value !== 'string' || value.length > 2048) return undefined;
  try {
    const url = new URL(value, `https://${HOST}/`);
    if (
      url.protocol !== 'https:' ||
      url.hostname !== HOST ||
      url.port ||
      url.username ||
      url.password ||
      !/^\/assets\/(?:index|vendor|router|tanstack-query|tanstack-table|charts|forms|network|misc-utils)-[A-Za-z0-9_-]{8}\.js$/.test(
        url.pathname
      )
    )
      return undefined;
    return url.origin + url.pathname;
  } catch {
    return undefined;
  }
}

/** sanitizeEvent projects errors into bounded diagnostic fields and discards all arbitrary content. */
export function sanitizeEvent(event, config) {
  try {
    if (event?.type || !Array.isArray(event?.exception?.values) || !event.exception.values.length) return null;
    const values = event.exception.values.slice(-3).map((value) => {
      const frames = Array.isArray(value?.stacktrace?.frames)
        ? value.stacktrace.frames.slice(-30).flatMap((frame) => {
            const filename = safeAssetURL(frame?.filename);
            if (!filename || !config.assetURLs.includes(filename)) return [];
            const clean = { filename, in_app: true };
            for (const field of ['lineno', 'colno']) {
              if (Number.isSafeInteger(frame[field]) && frame[field] > 0 && frame[field] <= 10000000) clean[field] = frame[field];
            }
            return [clean];
          })
        : [];
      const clean = { type: TYPES.has(value?.type) ? value.type : 'Error', value: '[redacted browser error]' };
      if (frames.length) clean.stacktrace = { frames };
      if (typeof value?.mechanism?.handled === 'boolean') {
        clean.mechanism = { type: 'generic', handled: value.mechanism.handled };
      }
      return clean;
    });
    const clean = {
      platform: 'javascript',
      level: 'error',
      environment: config.environment,
      release: config.release,
      tags: { site: 'one-api', theme: config.theme },
      exception: { values },
    };
    if (typeof event.event_id === 'string' && /^[a-f0-9]{32}$/.test(event.event_id)) clean.event_id = event.event_id;
    if (typeof event.timestamp === 'number' && Number.isFinite(event.timestamp) && event.timestamp > 0) clean.timestamp = event.timestamp;
    return clean;
  } catch {
    return null;
  }
}

/** sanitizeEnvelope enforces the privacy policy at transport even for SDK internal errors that bypass beforeSend. */
export function sanitizeEnvelope(envelope, config) {
  try {
    if (!Array.isArray(envelope) || !Array.isArray(envelope[1])) return null;
    const item = envelope[1][0];
    if (!Array.isArray(item) || item[0]?.type !== 'event') return null;
    const event = sanitizeEvent(item[1], config);
    if (!event) return null;
    const headers = {};
    if (event.event_id) headers.event_id = event.event_id;
    return [headers, [[{ type: 'event' }, event]]];
  } catch {
    return null;
  }
}

/** createBrowserReporter initializes one errors-only SDK per window; SDK failures return a disabled reporter. */
export function createBrowserReporter(sdk, env, target, theme, entryURL) {
  const disabled = { enabled: false, capture() {} };
  if (!target || (typeof target !== 'object' && typeof target !== 'function')) return disabled;
  if (reporters.has(target)) return reporters.get(target);
  reporters.set(target, disabled);
  try {
    const assets = [entryURL];
    const nodes = target.document?.querySelectorAll('script[src],link[rel="modulepreload"][href]');
    for (let index = 0; nodes && index < Math.min(nodes.length, 39); index++) {
      assets.push(nodes[index].src || nodes[index].href);
    }
    const config = readConfig(env, target.location, theme, assets);
    if (!config) return disabled;
    const seen = new WeakSet();
    let initialized = false;
    const client = sdk.init({
      dsn: config.dsn,
      environment: config.environment,
      release: config.release,
      transport(options) {
        const delegate = sdk.makeFetchTransport({ ...options, fetchOptions: { credentials: 'omit', referrerPolicy: 'no-referrer' } });
        return {
          /** send projects the final envelope before the SDK network transport can see it. */
          send(envelope) {
            const clean = initialized ? sanitizeEnvelope(envelope, config) : null;
            if (!clean) return Promise.resolve({});
            try {
              return Promise.resolve(delegate.send(clean)).catch(() => {
                console.warn('One API error reporting unavailable.');
                return {};
              });
            } catch {
              console.warn('One API error reporting unavailable.');
              return Promise.resolve({});
            }
          },
          /** flush drains only the already sanitized transport queue, returning false on SDK failure. */
          flush(timeout) {
            try {
              return Promise.resolve(delegate.flush(timeout)).catch(() => false);
            } catch {
              return Promise.resolve(false);
            }
          },
        };
      },
      debug: false,
      defaultIntegrations: false,
      integrations: [sdk.globalHandlersIntegration({ onerror: true, onunhandledrejection: true })],
      sendClientReports: false,
      maxBreadcrumbs: 0,
      beforeBreadcrumb() {
        return null;
      },
      tracePropagationTargets: [],
      beforeSendLog() {
        return null;
      },
      beforeSendMetric() {
        return null;
      },
      dataCollection: {
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
      },
      beforeSend(event, hint) {
        try {
          if (hint) hint.attachments = [];
          if (!initialized) return null;
          const clean = sanitizeEvent(event, config);
          if (!clean) return null;
          const error = hint?.originalException;
          if (error && (typeof error === 'object' || typeof error === 'function')) {
            if (seen.has(error)) return null;
            seen.add(error);
          }
          return clean;
        } catch {
          return null;
        }
      },
      beforeSendTransaction() {
        return null;
      },
    });
    if (!client || client.isEnabled?.() === false) return disabled;
    initialized = true;
    const reporter = {
      enabled: true,
      /** capture reports an exception while keeping SDK failures out of the application. */
      capture(error) {
        try {
          sdk.captureException(error);
        } catch {
          console.warn('One API error reporting unavailable.');
        }
      },
    };
    reporters.set(target, reporter);
    return reporter;
  } catch {
    console.warn('One API error reporting unavailable.');
    return disabled;
  }
}

/** reactRootOptions returns React 19 error hooks when capture is enabled, preserving defaults otherwise. */
export function reactRootOptions(reporter) {
  if (!reporter.enabled) return {};
  /** report captures root errors and logs only fixed, non-sensitive text. */
  function report(error) {
    reporter.capture(error);
    console.error('One API UI error.');
  }
  /** reportCaught leaves this reporting boundary to componentDidCatch, including primitive throws. */
  function reportCaught(error, info) {
    if (info?.errorBoundary?.[boundaryMarker]) {
      console.error('One API UI error.');
      return;
    }
    report(error);
  }
  return { onUncaughtError: report, onCaughtError: reportCaught, onRecoverableError: report };
}

/** createErrorBoundary returns a reporting boundary, or a transparent component when capture is disabled. */
export function createErrorBoundary(React, reporter) {
  if (!reporter.enabled) {
    /** DisabledBoundary returns the existing application children unchanged. */
    return function DisabledBoundary({ children }) {
      return children;
    };
  }
  return class BrowserErrorBoundary extends React.Component {
    [boundaryMarker] = true;
    state = { failed: false };
    /** getDerivedStateFromError removes the failed root, matching the previous blank-root failure UI. */
    static getDerivedStateFromError() {
      return { failed: true };
    }
    /** componentDidCatch reports rendering errors without component props or source context. */
    componentDidCatch(error) {
      reporter.capture(error);
    }
    /** render returns the application children until a root rendering failure. */
    render() {
      return this.state.failed ? null : this.props.children;
    }
  };
}
