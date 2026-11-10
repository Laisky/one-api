const assert = require('node:assert/strict');
const { execFileSync } = require('node:child_process');
const fs = require('node:fs');
const { createRequire } = require('node:module');
const path = require('node:path');

// legacy-react-harness mounts real Air or Berry application modules in jsdom
// with the theme's own React and UI library. Tests replace only the modules
// they name, including transport, toasts, auth/formatting helpers, unrelated
// modals/tooltips and dashboard charts. Assertions exercise real page event
// handlers, state transitions and request builders, not the replaced modules.

const root = path.resolve(__dirname, '../..');
const theme = process.env.LEGACY_THEME || path.basename(process.cwd());
assert.ok(['air', 'berry'].includes(theme), 'Select a legacy frontend');
const themeRoot = path.join(root, 'web', theme);
const themeRequire = createRequire(path.join(themeRoot, 'package.json'));
const ts = themeRequire('typescript');
const { JSDOM } = themeRequire('jsdom');

const dom = new JSDOM('<!doctype html><html><body></body></html>', { url: 'https://example.test/', pretendToBeVisual: true });
globalThis.window = dom.window;
for (const key of Object.getOwnPropertyNames(dom.window)) {
  // DOM event constructors must come from jsdom even where Node defines its own.
  if (!(key in globalThis) || /Event$/.test(key)) globalThis[key] = dom.window[key];
}
globalThis.addEventListener = dom.window.addEventListener.bind(dom.window);
globalThis.removeEventListener = dom.window.removeEventListener.bind(dom.window);
globalThis.IS_REACT_ACT_ENVIRONMENT = true;
/** matchMedia reports a desktop viewport because jsdom implements no media queries. */
window.matchMedia = () => ({ matches: false, addListener() {}, removeListener() {}, addEventListener() {}, removeEventListener() {} });
globalThis.matchMedia = window.matchMedia;
/** ResizeObserver is inert because jsdom performs no layout. */
globalThis.ResizeObserver = window.ResizeObserver = class { observe() {} unobserve() {} disconnect() {} };
/** getContext returns an inert 2D context for animation libraries probed at import time. */
window.HTMLCanvasElement.prototype.getContext = () => new Proxy({}, { get: (target, key) => target[key] ?? (() => ({})), set: (target, key, value) => { target[key] = value; return true; } });
/** getBoundingClientRect reports an empty box for text ranges because jsdom performs no layout. */
window.Range.prototype.getBoundingClientRect = () => ({ x: 0, y: 0, width: 0, height: 0, top: 0, left: 0, right: 0, bottom: 0 });
/** getClientRects reports no boxes for text ranges because jsdom performs no layout. */
window.Range.prototype.getClientRects = () => [];
require.extensions['.css'] = () => {};

const React = themeRequire('react');
const { createRoot } = themeRequire('react-dom/client');
const act = React.act || themeRequire('react-dom/test-utils').act;

/** readSource returns current source, or the LEGACY_UI_BASELINE revision's source for a historical negative control. */
function readSource(relative) {
  if (process.env.LEGACY_UI_BASELINE) return execFileSync('git', ['show', `${process.env.LEGACY_UI_BASELINE}:${relative}`], { cwd: root, encoding: 'utf8' });
  return fs.readFileSync(path.join(root, relative), 'utf8');
}

/** resolveProject maps a relative or Berry baseUrl import to a repository source path, or returns null for a package. */
function resolveProject(directory, specifier) {
  let base;
  if (specifier.startsWith('.')) base = path.posix.join(directory, specifier);
  else if (theme === 'berry' && fs.existsSync(path.join(themeRoot, 'src', specifier.split('/')[0]))) base = `web/berry/src/${specifier}`;
  else return null;
  for (const candidate of [base, `${base}.js`, `${base}.jsx`, `${base}/index.js`]) {
    const absolute = path.join(root, candidate);
    if (fs.existsSync(absolute) && fs.statSync(absolute).isFile()) return candidate;
  }
  throw new Error(`Cannot resolve ${specifier} from ${directory}`);
}

/** defaultExport returns an ES-module namespace whose default export is the supplied value. */
function defaultExport(value) {
  return { __esModule: true, default: value };
}

/** loadModule compiles the repository module at the supplied path and returns its exports, substituting only overridden repository paths. */
function loadModule(relative, overrides = {}, cache = new Map()) {
  if (Object.hasOwn(overrides, relative)) return overrides[relative];
  if (cache.has(relative)) return cache.get(relative).exports;
  const output = ts.transpileModule(readSource(relative), {
    compilerOptions: { jsx: ts.JsxEmit.ReactJSX, module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022, esModuleInterop: true },
  }).outputText;
  const module = { exports: {} };
  cache.set(relative, module);
  const directory = path.posix.dirname(relative);
  /** localRequire resolves application imports before delegating packages to the theme's dependencies. */
  const localRequire = specifier => {
    if (/\.s?css$/.test(specifier)) return {};
    if (Object.hasOwn(overrides, specifier)) return overrides[specifier];
    // The package exports only its ES build by subpath; the CommonJS barrel exposes the same Paragraph component.
    if (specifier === '@douyinfe/semi-ui/lib/es/typography/paragraph') return defaultExport(themeRequire('@douyinfe/semi-ui').Typography.Paragraph);
    const resolved = resolveProject(directory, specifier);
    return resolved ? loadModule(resolved, overrides, cache) : themeRequire(specifier);
  };
  new Function('require', 'module', 'exports', output)(localRequire, module, module.exports);
  return module.exports;
}

/** createTransport returns an API double that records each GET as a path plus decoded query parameters and answers with the supplied responder. */
function createTransport(respond) {
  const requests = [];
  const API = {
    /** get records one request and returns the responder's body for it. */
    get: async (url, config = {}) => {
      const parsed = new URL(url, 'https://example.test');
      const params = Object.fromEntries(parsed.searchParams);
      for (const [key, value] of Object.entries(config.params || {})) params[key] = String(value);
      const request = { path: parsed.pathname, params };
      requests.push(request);
      return { data: await respond(request, requests.length) };
    },
  };
  return { API, requests };
}

/** settle lets pending request promises resolve and their state updates commit. */
async function settle() {
  for (let index = 0; index < 3; index++) await act(async () => { await new Promise(resolve => setTimeout(resolve, 0)); });
}

/** CrashBoundary records rendering failures so a crashed page fails with its real error instead of rendering nothing. */
class CrashBoundary extends React.Component {
  /** constructor initializes the boundary without a captured failure. */
  constructor(props) {
    super(props);
    this.state = { error: null };
  }

  /** getDerivedStateFromError returns the fallback state for the supplied rendering failure. */
  static getDerivedStateFromError(error) {
    return { error };
  }

  /** componentDidCatch records the supplied rendering failure for the mounting test. */
  componentDidCatch(error) {
    this.props.onError(error);
  }

  /** render returns the mounted page, or nothing after a rendering failure. */
  render() {
    return this.state.error ? null : this.props.children;
  }
}

/** mount renders the supplied element into a fresh container and returns user-interaction helpers bound to it. */
async function mount(element) {
  const container = document.createElement('div');
  document.body.appendChild(container);
  const mounted = createRoot(container);
  const errors = [];
  /** assertRendered rethrows the first rendering failure so tests report the page's real crash. */
  const assertRendered = () => {
    if (errors.length) throw errors[0];
  };
  await act(async () => mounted.render(React.createElement(CrashBoundary, { onError: error => errors.push(error) }, element)));
  await settle();
  assertRendered();

  /** textElement returns the element that directly owns the supplied visible text. */
  const textElement = text => {
    const walker = document.createTreeWalker(container, window.NodeFilter.SHOW_TEXT);
    while (walker.nextNode()) if (walker.currentNode.textContent.trim() === text) return walker.currentNode.parentElement;
    throw new Error(`Find visible text ${text}`);
  };
  /** click dispatches a bubbling user click and waits for the resulting requests and renders. */
  const click = async target => {
    assert.ok(target, 'Click an existing element');
    await act(async () => target.dispatchEvent(new window.MouseEvent('click', { bubbles: true, cancelable: true })));
    await settle();
    assertRendered();
  };
  /** type replaces an input value the way a user edit reaches React's change handler. */
  const type = async (input, value) => {
    assert.ok(input, 'Type into an existing input');
    await act(async () => {
      Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype, 'value').set.call(input, value);
      input.dispatchEvent(new window.Event('input', { bubbles: true }));
    });
    await settle();
    assertRendered();
  };
  /** unmount removes the mounted tree and its container. */
  const unmount = async () => {
    await act(async () => mounted.unmount());
    container.remove();
  };
  return { assertRendered, container, textElement, click, type, unmount };
}

module.exports = { React, act, createTransport, defaultExport, loadModule, mount, readSource, root, settle, theme, themeRequire, ts };
