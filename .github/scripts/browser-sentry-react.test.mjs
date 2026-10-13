import assert from 'node:assert/strict';
import { createRequire } from 'node:module';
import path from 'node:path';
import test from 'node:test';
import { createBrowserReporter, createErrorBoundary, reactRootOptions } from '../../web/legacy/browser-sentry.mjs';

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
const theme = path.basename(process.cwd());
const env = {
  dsn: 'https://0123456789abcdef0123456789abcdef@o123.ingest.sentry.io/456',
  environment: 'production',
};
/** setup supplies a mock SDK that runs the actual final privacy policy without networking. */
function setup(config = env) {
  const events = [];
  let options;
  const sdk = {
    globalHandlersIntegration() {
      return { name: 'GlobalHandlers' };
    },
    init(value) {
      options = value;
      return { isEnabled: () => true };
    },
    captureException(error) {
      const clean = options.beforeSend(
        { exception: { values: [{ type: error.name, value: error.message }] } },
        { originalException: error }
      );
      if (clean) events.push(clean);
    },
  };
  const reporter = createBrowserReporter(
    sdk,
    config,
    { location: dom.window.location },
    theme,
    undefined,
    'one-api-' + theme + '@sha256:' + 'a'.repeat(64)
  );
  return { reporter, events, options };
}
/** Broken simulates a rendering error whose message contains user content. */
function Broken() {
  throw new TypeError('prompt token completion secret');
}

test('mounted React ' + React.version + ' boundary captures a declarative router failure once', async () => {
  const { reporter, events, options } = setup();
  const Boundary = createErrorBoundary(React, reporter);
  const container = document.createElement('div');
  const hooks = React.version.startsWith('19.') ? reactRootOptions(reporter) : {};
  const root = createRoot(container, hooks);
  const errorLog = console.error;
  const warningLog = console.warn;
  console.error = () => {};
  console.warn = () => {};
  try {
    const routes = React.createElement(Routes, null, React.createElement(Route, { path: '/', element: React.createElement(Broken) }));
    const app = React.createElement(Boundary, null, React.createElement(BrowserRouter, null, routes));
    await act(async () => root.render(app));
    assert.equal(events.length, 1);
    assert.equal(events[0].exception.values[0].type, 'TypeError');
    assert.equal(events[0].exception.values[0].value, '[redacted browser error]');
    assert.equal(container.textContent, '');
    assert.equal(options.integrations[0].name, 'GlobalHandlers');
    await act(async () => root.unmount());
  } finally {
    console.error = errorLog;
    console.warn = warningLog;
  }
});

test('disabled boundary keeps the existing rendering behavior and adds no root hooks', async () => {
  const { reporter, events } = setup({});
  const Boundary = createErrorBoundary(React, reporter);
  const container = document.createElement('div');
  const root = createRoot(container, reactRootOptions(reporter));
  await act(async () => root.render(React.createElement(Boundary, null, React.createElement('p', null, 'Existing UI'))));
  assert.equal(container.textContent, 'Existing UI');
  assert.equal(events.length, 0);
  assert.deepEqual(reactRootOptions(reporter), {});
  await act(async () => root.unmount());
});

test('React 19 root hooks capture uncaught, caught and recoverable errors with safe logs', () => {
  const { reporter, events } = setup();
  const hooks = reactRootOptions(reporter);
  const logs = [];
  const errorLog = console.error;
  console.error = (...args) => logs.push(args);
  try {
    for (const hook of Object.values(hooks)) hook(new Error('secret'));
    assert.equal(events.length, 3);
    assert.ok(logs.every((args) => args.length === 1 && args[0] === 'One API UI error.'));
  } finally {
    console.error = errorLog;
    globalThis.IS_REACT_ACT_ENVIRONMENT = true;
  }
});

test('mounted React 19 uncaught root errors reach the reporting hook', { skip: !React.version.startsWith('19.') }, async () => {
  const { reporter, events } = setup();
  const root = createRoot(document.createElement('div'), reactRootOptions(reporter));
  const errorLog = console.error;
  console.error = () => {};
  try {
    globalThis.IS_REACT_ACT_ENVIRONMENT = false;
    flushSync(() => root.render(React.createElement(Broken)));
    assert.equal(events.length, 1);
    assert.equal(events[0].exception.values[0].value, '[redacted browser error]');
    await act(async () => root.unmount());
  } finally {
    console.error = errorLog;
    globalThis.IS_REACT_ACT_ENVIRONMENT = true;
  }
});

/** BrokenPrimitive simulates a sensitive primitive throw caught by the reporting boundary. */
function BrokenPrimitive() {
  throw 'secret prompt primitive';
}

test('mounted reporting boundary and React19 caught hook emit one event for a primitive throw', async () => {
  const { reporter, events } = setup();
  const Boundary = createErrorBoundary(React, reporter);
  const root = createRoot(document.createElement('div'), React.version.startsWith('19.') ? reactRootOptions(reporter) : {});
  const errorLog = console.error;
  console.error = () => {};
  try {
    await act(async () => root.render(React.createElement(Boundary, null, React.createElement(BrokenPrimitive))));
    assert.equal(events.length, 1);
    assert.equal(events[0].exception.values[0].value, '[redacted browser error]');
    await act(async () => root.unmount());
  } finally {
    console.error = errorLog;
  }
});

test('React19 caught hook reports a failure caught by a different boundary', { skip: !React.version.startsWith('19.') }, async () => {
  const { reporter, events } = setup();
  class ExistingBoundary extends React.Component {
    state = { failed: false };
    static getDerivedStateFromError() {
      return { failed: true };
    }
    render() {
      return this.state.failed ? 'Existing fallback' : this.props.children;
    }
  }
  const container = document.createElement('div');
  const root = createRoot(container, reactRootOptions(reporter));
  const errorLog = console.error;
  console.error = () => {};
  try {
    await act(async () => root.render(React.createElement(ExistingBoundary, null, React.createElement(BrokenPrimitive))));
    assert.equal(events.length, 1);
    assert.equal(container.textContent, 'Existing fallback');
    await act(async () => root.unmount());
  } finally {
    console.error = errorLog;
  }
});
