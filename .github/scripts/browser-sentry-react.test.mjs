import assert from 'node:assert/strict';
import { createRequire } from 'node:module';
import path from 'node:path';
import test from 'node:test';
import { createBrowserReporter, createErrorBoundary, reactRootOptions } from '../../web/legacy/browser-sentry.mjs';
import { markPrivateError } from '../../web/legacy/browser-sentry-privacy.mjs';

const require = createRequire(path.join(process.cwd(), 'package.json'));
const { JSDOM } = require('jsdom');
const dom = new JSDOM('<!doctype html><div id="root"></div>', { url: 'https://oneapi.laisky.com/' });
globalThis.window = dom.window;
globalThis.document = dom.window.document;
globalThis.HTMLElement = dom.window.HTMLElement;
globalThis.IS_REACT_ACT_ENVIRONMENT = true;
const React = require('react');
const { createRoot } = require('react-dom/client');
const { flushSync } = require('react-dom');
const { BrowserRouter, Routes, Route } = require('react-router-dom');
const act = React.act || require('react-dom/test-utils').act;
const Sentry = require('@sentry/react');
const theme = path.basename(process.cwd());
const env = { dsn: 'https://0123456789abcdef0123456789abcdef@o123.ingest.sentry.io/456', environment: 'production' };
const release = 'one-api-' + theme + '@sha256:' + 'a'.repeat(64);

/** clearScope removes this fixture's SDK scope data with supported methods and returns nothing. */
function clearScope(scope) {
  const data = scope.getScopeData();
  for (const key of Object.keys(data.extra)) scope.setExtra(key, undefined);
  for (const key of Object.keys(data.contexts)) scope.setContext(key, null);
  for (const key of Object.keys(data.tags)) scope.setTag(key, undefined);
  scope.setUser(null);
  scope.clearBreadcrumbs();
  scope.clearAttachments();
}

/** offline supplies actual SDK React capture through memory transport, returning a scope-isolated test callback. */
function offline(run, config = env) {
  return () =>
    Sentry.withScope(new Sentry.Scope(), async () => {
      const events = [];
      const sdk = {
        ...Sentry,
        /** init returns an actual SDK client whose synthetic event envelopes stay in memory. */
        init(options) {
          return Sentry.init({
            ...options,
            transport: () => ({
              /** send records only error-event payloads and returns a successful delivery result. */
              async send(envelope) {
                for (const item of envelope[1]) if (item[0].type === 'event') events.push(item[1]);
                return { statusCode: 200 };
              },
              /** flush reports that the memory transport has no queued work. */
              async flush() {
                return true;
              },
            }),
          });
        },
      };
      const reporter = createBrowserReporter(sdk, config, { location: dom.window.location }, theme, undefined, release);
      const errorLog = console.error;
      const warningLog = console.warn;
      console.error = () => {};
      console.warn = () => {};
      try {
        await run({ reporter, events });
      } finally {
        await Sentry.close(1000);
        clearScope(Sentry.getCurrentScope());
        clearScope(Sentry.getIsolationScope());
        console.error = errorLog;
        console.warn = warningLog;
        globalThis.IS_REACT_ACT_ENVIRONMENT = true;
      }
    });
}

/** Broken throws an ordinary render error and never returns a rendered node. */
function Broken() {
  throw new TypeError('Cannot render the channel dashboard');
}

/** BrokenPrimitive throws an ordinary primitive render failure and never returns a node. */
function BrokenPrimitive() {
  throw 'Dashboard render failed';
}

test(
  'mounted React ' + React.version + ' boundary captures a router error once with useful component stack',
  offline(async ({ reporter, events }) => {
    const Boundary = createErrorBoundary(React, reporter);
    const container = document.createElement('div');
    const root = createRoot(container, React.version.startsWith('19.') ? reactRootOptions(reporter) : {});
    try {
      const routes = React.createElement(Routes, null, React.createElement(Route, { path: '/', element: React.createElement(Broken) }));
      await act(async () =>
        root.render(
          React.createElement(React.StrictMode, null, React.createElement(Boundary, null, React.createElement(BrowserRouter, null, routes)))
        )
      );
      await Sentry.flush(1000);
      assert.equal(events.length, 1);
      assert.ok(events[0].exception.values.some((exception) => exception.type === 'TypeError'));
      assert.match(JSON.stringify(events), /Cannot render the channel dashboard/);
      assert.match(JSON.stringify(events), /Broken/);
      assert.equal(container.textContent, '');
    } finally {
      await act(async () => root.unmount());
    }
  })
);

test(
  'disabled reporting is transparent and adds no React root hooks',
  offline(async ({ reporter, events }) => {
    const Boundary = createErrorBoundary(React, reporter);
    const container = document.createElement('div');
    const root = createRoot(container, reactRootOptions(reporter));
    try {
      await act(async () => root.render(React.createElement(Boundary, null, React.createElement('p', null, 'Existing UI'))));
      assert.equal(container.textContent, 'Existing UI');
      assert.equal(events.length, 0);
      assert.deepEqual(reactRootOptions(reporter), {});
    } finally {
      await act(async () => root.unmount());
    }
  }, {})
);

test(
  'React19 root callbacks retain component information and string-only application logs',
  offline(async ({ reporter, events }) => {
    const hooks = reactRootOptions(reporter);
    const logs = [];
    const original = console.error;
    console.error = (...args) => logs.push(args);
    try {
      for (const [source, hook] of Object.entries(hooks))
        hook(new Error('Dashboard ' + source + ' failed'), { componentStack: '\n at ChannelDashboard' });
      await Sentry.flush(1000);
      assert.equal(events.length, 3);
      assert.ok(events.every((event) => JSON.stringify(event).includes('ChannelDashboard')));
      assert.ok(logs.every((args) => args.length === 1 && typeof args[0] === 'string'));
    } finally {
      console.error = original;
    }
  })
);

test(
  'mounted React19 uncaught root failures reach the actual SDK with useful component stack',
  { skip: !React.version.startsWith('19.') },
  offline(async ({ reporter, events }) => {
    const root = createRoot(document.createElement('div'), reactRootOptions(reporter));
    try {
      globalThis.IS_REACT_ACT_ENVIRONMENT = false;
      flushSync(() => root.render(React.createElement(Broken)));
      await Sentry.flush(1000);
      assert.equal(events.length, 1);
      assert.match(JSON.stringify(events), /Cannot render the channel dashboard/);
      assert.match(JSON.stringify(events), /Broken/);
    } finally {
      await act(async () => root.unmount());
    }
  })
);

test(
  'mounted own reporting boundary and React19 caught hook emit one event for a primitive throw',
  offline(async ({ reporter, events }) => {
    const Boundary = createErrorBoundary(React, reporter);
    const root = createRoot(document.createElement('div'), React.version.startsWith('19.') ? reactRootOptions(reporter) : {});
    try {
      await act(async () =>
        root.render(React.createElement(React.StrictMode, null, React.createElement(Boundary, null, React.createElement(BrokenPrimitive))))
      );
      await Sentry.flush(1000);
      assert.equal(events.length, 1);
      assert.match(JSON.stringify(events), /Dashboard render failed/);
    } finally {
      await act(async () => root.unmount());
    }
  })
);

test(
  'React19 caught hook reports a different boundary failure while preserving its fallback',
  { skip: !React.version.startsWith('19.') },
  offline(async ({ reporter, events }) => {
    class ExistingBoundary extends React.Component {
      state = { failed: false };
      /** getDerivedStateFromError returns the existing UI boundary's failure state. */
      static getDerivedStateFromError() {
        return { failed: true };
      }
      /** render returns the existing fallback or the provided children. */
      render() {
        return this.state.failed ? 'Existing fallback' : this.props.children;
      }
    }
    const container = document.createElement('div');
    const root = createRoot(container, reactRootOptions(reporter));
    try {
      await act(async () => root.render(React.createElement(ExistingBoundary, null, React.createElement(BrokenPrimitive))));
      await Sentry.flush(1000);
      assert.equal(events.length, 1);
      assert.equal(container.textContent, 'Existing fallback');
    } finally {
      await act(async () => root.unmount());
    }
  })
);

test(
  'mounted React boundary preserves ordinary outer messages/stacks while filtering private inner/React copies',
  offline(async ({ reporter, events }) => {
    const payload = 'synthetic-private-response-'.repeat(40);
    const inner = markPrivateError(new Error(payload), 'Private response decoding failed');
    const outer = new Error('Cannot initialize channel details', { cause: inner });
    /** BrokenPrivate throws a marked-response error chain without returning a UI node. */
    function BrokenPrivate() {
      throw outer;
    }
    const Boundary = createErrorBoundary(React, reporter);
    const root = createRoot(document.createElement('div'), React.version.startsWith('19.') ? reactRootOptions(reporter) : {});
    try {
      await act(async () => root.render(React.createElement(Boundary, null, React.createElement(BrokenPrivate))));
      await Sentry.flush(1000);
      assert.equal(events.length, 1);
      const wire = JSON.stringify(events);
      assert.doesNotMatch(wire, /synthetic-private-response/);
      assert.match(wire, /Private response decoding failed/);
      assert.match(wire, /Cannot initialize channel details/);
      assert.match(wire, /BrokenPrivate/);
      assert.equal(inner.message, payload);
    } finally {
      await act(async () => root.unmount());
    }
  })
);
