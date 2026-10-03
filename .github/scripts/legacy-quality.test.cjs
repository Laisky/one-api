const assert = require('node:assert/strict');
const { execFileSync } = require('node:child_process');
const fs = require('node:fs');
const { createRequire } = require('node:module');
const path = require('node:path');
const { test } = require('node:test');
const vm = require('node:vm');

const root = path.resolve(__dirname, '../..');
const theme = process.env.LEGACY_THEME || path.basename(process.cwd());
assert.ok(['air', 'berry'].includes(theme), 'Select the legacy theme');
const themeRequire = createRequire(path.join(root, 'web', theme, 'package.json'));
const ts = themeRequire('typescript');

/** readSource reads current code or an explicitly selected negative-control revision. */
function readSource(relative) {
  return process.env.LEGACY_QUALITY_BASELINE
    ? execFileSync('git', ['show', `${process.env.LEGACY_QUALITY_BASELINE}:${relative}`], { cwd: root, encoding: 'utf8' })
    : fs.readFileSync(path.join(root, relative), 'utf8');
}

/** loadModule compiles the real JSX module while replacing only its external UI bindings. */
function loadModule(relative, imports = {}, globals = {}) {
  const module = { exports: {} };
  const output = ts.transpileModule(readSource(relative), {
    fileName: relative,
    compilerOptions: { module: ts.ModuleKind.CommonJS, jsx: ts.JsxEmit.ReactJSX, target: ts.ScriptTarget.ES2022 },
  }).outputText;
  vm.runInNewContext(output, {
    module, exports: module.exports, URL, WeakSet,
    require: name => {
      assert.ok(Object.hasOwn(imports, name), `Unexpected import ${name} in ${relative}`);
      return imports[name];
    },
    ...globals,
  }, { filename: relative, timeout: 5000 });
  return module.exports;
}

/** extractFunction evaluates a real component's request handler with controlled state bindings. */
function extractFunction(relative, name, globals) {
  const text = readSource(relative);
  const source = ts.createSourceFile(relative, text, ts.ScriptTarget.Latest, true, ts.ScriptKind.JSX);
  const matches = [];
  function visit(node) {
    if (ts.isFunctionLike(node) && (node.name?.getText(source) === name || node.parent.name?.getText(source) === name)) matches.push(node);
    ts.forEachChild(node, visit);
  }
  visit(source);
  assert.equal(matches.length, 1, `Find exactly one ${name}`);
  return vm.runInNewContext(`(${matches[0].getText(source)})`, globals, { timeout: 5000 });
}

/** notifierFixture loads the actual notification helper and records notifications and logs. */
function notifierFixture() {
  const notifications = [];
  const logs = [];
  const notify = message => notifications.push(message);
  const imports = {
    react: {}, 'react/jsx-runtime': {},
    '@douyinfe/semi-ui': { Toast: { error: notify, info: notify } },
    '../constants': { toastConstants: {} }, 'react-toastify': { toast: notify },
    notistack: { enqueueSnackbar: notify },
    'constants/SnackbarConstants': { snackbarConstants: { Common: { ERROR: {}, INFO: {} }, Mobile: {} } },
    './api': { API: {} },
  };
  const relative = `web/${theme}/src/${theme === 'air' ? 'helpers/utils.js' : 'utils/common.js'}`;
  const helpers = loadModule(relative, imports, {
    window: { innerWidth: 1200, location: { pathname: '/login', replace: notify } },
    localStorage: { removeItem() {} }, console: { error: (...args) => logs.push(args) },
  });
  return { showError: helpers.showError, notifications, logs };
}

test(`${theme}: network failures retain their original error and never leak request credentials`, () => {
  const { showError, notifications, logs } = notifierFixture();
  const error = Object.assign(new Error('Network Error'), { name: 'AxiosError', config: { headers: { Authorization: 'secret-test-sentinel' } } });
  assert.doesNotThrow(() => showError(error), 'Network failures must not dereference a missing response');
  showError(error);
  assert.equal(notifications.length, 1, 'Interceptor and caller must notify exactly once');
  assert.match(notifications[0], /Network Error/);
  assert.equal(logs.length, 0, 'Never log the raw Axios error/config');
});

test(`${theme}: ordinary errors, strings, and null are safe notification inputs`, () => {
  const { showError, notifications } = notifierFixture();
  for (const error of [new Error('ordinary failure'), 'server failure', null]) assert.doesNotThrow(() => showError(error));
  assert.equal(notifications.length, 3);
});

test(`${theme}: cancellation is not displayed as a request failure`, () => {
  const { showError, notifications } = notifierFixture();
  showError({ name: 'CanceledError', code: 'ERR_CANCELED', message: 'cancelled' });
  assert.equal(notifications.length, 0);
});

test(`${theme}: token loading always clears request state and supports a successful retry`, async () => {
  const states = [];
  const data = [];
  const failure = new Error('network unavailable');
  let fail = true;
  const rows = [{ id: 42, name: 'retained' }];
  const handler = extractFunction(`web/${theme}/src/${theme === 'air' ? 'components/TokensTable.js' : 'views/Token/index.js'}`, 'loadTokens', {
    API: { get: async () => { if (fail) throw failure; return { data: { success: true, data: rows } }; } },
    setLoading: value => states.push(value), setSearching: value => states.push(value),
    setTokens: value => data.push(value), setTokensFormat: value => data.push(value),
    pageSize: 10, ITEMS_PER_PAGE: 10, orderBy: '', tokens: [{ id: 1 }],
    showError: () => assert.fail('No extra notification from an internal loader'),
  });
  await assert.rejects(handler(0), error => error === failure);
  assert.equal(states.at(-1), false, 'The loading state is released on rejection');
  assert.equal(data.length, 0, 'A failed request cannot erase existing records');
  fail = false;
  await handler(0);
  assert.equal(states.at(-1), false);
  assert.equal(data[0], rows);
});

test(`${theme}: password reset releases state and cooldown after a failed request`, async () => {
  const states = [];
  const disabled = [];
  const errors = [];
  const failure = new Error('network unavailable');
  let calls = 0;
  const handler = extractFunction(`web/${theme}/src/${theme === 'air' ? 'components/PasswordResetForm.js' : 'views/Authentication/AuthForms/ForgetPasswordForm.js'}`, theme === 'air' ? 'handleSubmit' : 'submit', {
    API: { get: async (url, config) => { calls++; assert.equal(url, '/api/reset_password'); assert.equal(config.params.email, 'a+b@example.test'); throw failure; } },
    email: 'a+b@example.test', loading: false, disableButton: false, inputs: {},
    turnstileEnabled: false, turnstileToken: '', setLoading: value => states.push(value),
    setDisableButton: value => disabled.push(value), setCountdown: () => {},
    showError: error => errors.push(error), showInfo: () => {}, showSuccess: () => assert.fail('No false success'),
    setInputs: () => assert.fail('A failed reset must preserve the email'), setSendEmail: () => assert.fail('No false success'),
  });
  if (theme === 'air') await handler({ preventDefault() {} });
  else await handler({ email: 'a+b@example.test' }, { setSubmitting: value => states.push(value) });
  assert.equal(states.at(-1), false, 'The loading state is released on rejection');
  assert.equal(disabled.at(-1), false, 'The failed request can be retried');
  assert.equal(calls, 1);
  assert.equal(errors[0], failure);
});

test(`${theme}: asynchronous request state setters are paired with finally cleanup`, () => {
  let checked = 0;
  const missing = [];
  function walk(directory) {
    for (const entry of fs.readdirSync(path.join(root, directory), { withFileTypes: true })) {
      const relative = `${directory}/${entry.name}`;
      if (entry.isDirectory()) { walk(relative); continue; }
      if (!/\.[jt]sx?$/.test(entry.name)) continue;
      const text = readSource(relative);
      const source = ts.createSourceFile(relative, text, ts.ScriptTarget.Latest, true, ts.ScriptKind.JSX);
      function visit(node) {
        if (ts.isFunctionLike(node) && node.body && ts.isBlock(node.body) && node.modifiers?.some(item => item.kind === ts.SyntaxKind.AsyncKeyword)) {
          const states = new Set();
          const finalStates = new Set();
          let awaits = false;
          function gather(child, final = false) {
            if (child !== node.body && ts.isFunctionLike(child)) return;
            if (ts.isCallExpression(child) && ts.isIdentifier(child.expression) && /^set(Loading|Searching|Submitting)$/.test(child.expression.text)) {
              if (child.arguments[0]?.kind === ts.SyntaxKind.TrueKeyword) states.add(child.expression.text);
              if (final && child.arguments[0]?.kind === ts.SyntaxKind.FalseKeyword) finalStates.add(child.expression.text);
            }
            if (ts.isAwaitExpression(child)) awaits = true;
            if (ts.isTryStatement(child) && child.finallyBlock) {
              gather(child.tryBlock, final);
              if (child.catchClause) gather(child.catchClause, final);
              gather(child.finallyBlock, true);
            } else ts.forEachChild(child, next => gather(next, final));
          }
          gather(node.body);
          if (awaits && states.size) {
            checked++;
            if ([...states].some(state => !finalStates.has(state))) missing.push(`${relative}:${source.getLineAndCharacterOfPosition(node.getStart(source)).line + 1}`);
          }
        }
        ts.forEachChild(node, visit);
      }
      visit(source);
    }
  }
  walk(`web/${theme}/src`);
  assert.ok(checked >= 20, 'Do not silently stop scanning request handlers');
  assert.deepEqual(missing, [], 'Every asynchronous busy state needs finally cleanup');
});

if (theme === 'air') {
  test('air: pagination keeps hook order when results transition from empty to populated', () => {
    const hooks = [];
    const component = loadModule('web/air/src/components/FixedPagination.js', {
      react: { useEffect: () => hooks.push('effect'), useMemo: callback => { hooks.push('memo'); return callback(); } },
      'react/jsx-runtime': { jsx: () => ({}), jsxs: () => ({}) },
      '@douyinfe/semi-ui': { Pagination: () => ({}) }, './FixedPagination.css': {},
    }, { console: { log() {} } }).default;
    component({ currentPage: 1, pageSize: 10, total: 0 });
    const empty = [...hooks];
    hooks.length = 0;
    component({ currentPage: 1, pageSize: 10, total: 30 });
    assert.deepEqual(hooks, empty, 'Visibility cannot change hook order');
  });
} else {
  test('berry: useAuth obtains its router inside the hook and redirects after render', () => {
    const calls = [];
    const effects = [];
    const { default: useAuth } = loadModule('web/berry/src/hooks/useAuth.js', {
      react: { useEffect: effect => effects.push(effect) },
      'utils/common': { isAdmin: () => false },
      'react-router-dom': { useNavigate: () => { calls.push('hook'); return route => calls.push(route); } },
    });
    assert.deepEqual(calls, [], 'No router hook may run while importing a module');
    useAuth();
    assert.deepEqual(calls, ['hook']);
    effects.forEach(effect => effect());
    assert.deepEqual(calls, ['hook', '/panel/404']);
  });

  test('berry: worker retirement neither waits for ready nor unregisters other applications', async () => {
    const removed = [];
    const registration = (name, url) => ({ active: { scriptURL: url }, unregister: async () => { removed.push(name); return true; } });
    const serviceWorker = { getRegistrations: async () => [
      registration('owned', 'https://one.test/service-worker.js'),
      registration('other', 'https://one.test/other/service-worker.js'),
    ] };
    Object.defineProperty(serviceWorker, 'ready', { get: () => assert.fail('An absent worker must never block readiness') });
    const { unregister } = loadModule('web/berry/src/serviceWorker.js', {}, {
      navigator: { serviceWorker }, process: { env: {} },
      window: { location: { origin: 'https://one.test' } },
    });
    await unregister();
    assert.deepEqual(removed, ['owned']);
  });
}
