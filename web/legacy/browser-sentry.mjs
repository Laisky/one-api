import { sanitizeEvent, sanitizeBreadcrumb } from './browser-sentry-privacy.mjs';
import { registerRequestErrorCapture } from './browser-request-errors.mjs';
export { sanitizeEvent } from './browser-sentry-privacy.mjs';

const HOST = 'oneapi.laisky.com';
const reporters = new WeakMap();
const boundaryMarker = Symbol('OneAPI reporting boundary');
const THEMES = new Set(['modern', 'air', 'berry']);

/** readConfig validates public runtime settings and immutable built release identity, returning a frozen snapshot or null. */
export function readConfig(settings, location, theme, _assets = [], release) {
  if (location?.hostname !== HOST || location?.protocol !== 'https:' || Boolean(location?.port) || !THEMES.has(theme)) return null;
  if (
    !settings ||
    typeof settings !== 'object' ||
    Array.isArray(settings) ||
    Object.keys(settings).length !== 2 ||
    !Object.hasOwn(settings, 'dsn') ||
    !Object.hasOwn(settings, 'environment') ||
    !['production', 'staging', 'development'].includes(settings.environment)
  )
    return null;
  if (typeof release !== 'string' || !new RegExp('^one-api-' + theme + '@sha256:[a-f0-9]{64}$').test(release)) return null;
  if (typeof settings.dsn !== 'string' || settings.dsn.length > 256) return null;
  try {
    const dsn = new URL(settings.dsn);
    if (
      dsn.protocol !== 'https:' ||
      dsn.password ||
      dsn.port ||
      dsn.search ||
      dsn.hash ||
      !/^[a-f0-9]{32}$/.test(dsn.username) ||
      !/^o[0-9]+\.ingest(?:\.[a-z]+)?\.sentry\.io$/.test(dsn.hostname) ||
      !/^\/[0-9]{1,20}$/.test(dsn.pathname) ||
      dsn.href !== settings.dsn
    )
      return null;
    return Object.freeze({ dsn: dsn.href, environment: settings.environment, release, theme });
  } catch {
    return null;
  }
}

/** createBrowserReporter initializes one errors-only SDK per window; SDK failures return a disabled reporter. */
export function createBrowserReporter(sdk, settings, target, theme, entryURL, release) {
  const disabled = Object.freeze({ enabled: false, capture() {} });
  if (!target || (typeof target !== 'object' && typeof target !== 'function')) return disabled;
  if (reporters.has(target)) return reporters.get(target);
  reporters.set(target, disabled);
  try {
    const config = readConfig(settings, target.location, theme, [], release);
    if (!config) return disabled;
    const seen = new WeakSet();
    let initialized = false;
    const client = sdk.init({
      dsn: config.dsn,
      environment: config.environment,
      release: config.release,
      debug: false,
      sendClientReports: false,
      // Retain the SDK's diagnostic defaults, excluding automatic session payloads.
      integrations(defaults) {
        return defaults.filter((integration) => integration.name !== 'BrowserSession');
      },
      transportOptions: { fetchOptions: { credentials: 'omit', referrerPolicy: 'no-referrer' } },
      beforeBreadcrumb: sanitizeBreadcrumb,
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
      },
      beforeSend(event, hint) {
        try {
          if (hint) hint.attachments = [];
          if (!initialized) return null;
          const clean = sanitizeEvent(event, config, hint);
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
    });
    if (!client || client.isEnabled?.() === false) return disabled;
    initialized = true;
    const reporter = {
      enabled: true,
      /** capture uses the official React helper for component diagnostics and never throws into application code. */
      capture(error, info) {
        try {
          const componentStack = typeof info?.componentStack === 'string' ? info.componentStack : undefined;
          if (componentStack) {
            sdk.captureReactException(error, { componentStack }, { contexts: { react: { componentStack } } });
          } else sdk.captureException(error);
          return true;
        } catch {
          console.warn('One API error reporting unavailable.');
          return false;
        }
      },
    };
    registerRequestErrorCapture((error) => reporter.capture(error));
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
  function report(error, info) {
    reporter.capture(error, info);
    console.error('One API UI error.');
  }
  /** reportCaught leaves this reporting boundary to componentDidCatch, including primitive throws. */
  function reportCaught(error, info) {
    if (info?.errorBoundary?.[boundaryMarker]) {
      console.error('One API UI error.');
      return;
    }
    report(error, info);
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
    /** getDerivedStateFromError preserves the previous blank-root failure UI. */
    static getDerivedStateFromError() {
      return { failed: true };
    }
    /** componentDidCatch reports rendering errors with the SDK's component-stack support. */
    componentDidCatch(error, info) {
      reporter.capture(error, info);
    }
    /** render returns application children until a root rendering failure. */
    render() {
      return this.state.failed ? null : this.props.children;
    }
  };
}
