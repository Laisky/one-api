const assert = require('node:assert/strict');
const { execFileSync } = require('node:child_process');
const fs = require('node:fs');
const { createRequire } = require('node:module');
const path = require('node:path');
const { test } = require('node:test');
const vm = require('node:vm');

const root = path.resolve(__dirname, '../..');
const theme = process.env.LEGACY_THEME || path.basename(process.cwd());
assert.ok(['air', 'berry'].includes(theme), 'Select a legacy frontend');
const ts = createRequire(path.join(root, 'web', theme, 'package.json'))('typescript');
const events = new Set(['onClick', 'onChange', 'onBlur', 'onInput', 'onSearch', 'onSubmit']);

/** readSource loads current code or an explicitly selected historical negative control. */
function readSource(relative) {
  if (process.env.LEGACY_UI_BASELINE) return execFileSync('git', ['show', `${process.env.LEGACY_UI_BASELINE}:${relative}`], { cwd: root, encoding: 'utf8' });
  return fs.readFileSync(path.join(root, relative), 'utf8');
}
/** parse constructs an actual source tree without substituting application behavior. */
function parse(relative) {
  return ts.createSourceFile(relative, readSource(relative), ts.ScriptTarget.Latest, true, ts.ScriptKind.JSX);
}
/** expressions retrieves matching nodes from the inspected source file. */
function expressions(source, predicate) {
  const nodes = [];
  function visit(node) { if (predicate(node)) nodes.push(node); ts.forEachChild(node, visit); }
  visit(source);
  return nodes;
}
/** eventExpression returns the real JSX callback for the selected event and action. */
function eventExpression(source, event, action, qualifier = '') {
  const nodes = expressions(source, node => ts.isJsxAttribute(node) && node.name.text === event && node.initializer && ts.isJsxExpression(node.initializer) && node.initializer.expression?.getText(source).includes(action) && node.initializer.expression?.getText(source).includes(qualifier));
  assert.equal(nodes.length, 1, `Find one ${event}/${action} callback`);
  return nodes[0].initializer.expression.getText(source);
}

const form = theme === 'air' ? 'web/air/src/pages/User/AddUser.js' : 'web/berry/src/views/Token/component/EditModal.js';
const event = theme === 'air' ? 'onClick' : 'onSubmit';
for (const fails of [false, true]) {
  test(`${theme}: UI event ${fails ? 'failure is handled at its boundary' : 'success preserves arguments and result'}`, async () => {
    const calls = [];
    const errors = [];
    const failure = new Error('controlled transport failure');
    const callback = vm.runInNewContext(`(${eventExpression(parse(form), event, 'submit')})`, {
      submit: async (...args) => { calls.push(args); if (fails) throw failure; return 42; },
      reportUIError: error => { errors.push(error); },
    });
    const result = await callback('first', 'second');
    assert.deepEqual(calls, [['first', 'second']]);
    assert.equal(result, fails ? undefined : 42);
    assert.deepEqual(errors, fails ? [failure] : []);
  });
}

test(`${theme}: detached async calls and UI event handlers have an error owner`, () => {
  const missing = [];
  let inspected = 0;
  function walk(directory) {
    for (const entry of fs.readdirSync(path.join(root, directory), { withFileTypes: true })) {
      const relative = `${directory}/${entry.name}`;
      if (entry.isDirectory()) { walk(relative); continue; }
      if (!/\.jsx?$/.test(entry.name)) continue;
      const source = parse(relative);
      const asyncFunctions = expressions(source, node => ts.isFunctionLike(node) && node.modifiers?.some(item => item.kind === ts.SyntaxKind.AsyncKeyword));
      const names = new Set(asyncFunctions.map(node => node.name || node.parent.name).filter(name => name && ts.isIdentifier(name)).map(name => name.text));
      inspected += asyncFunctions.length;
      function unhandled(expression) {
        if (!ts.isCallExpression(expression)) return false;
        if (ts.isPropertyAccessExpression(expression.expression) && expression.expression.name.text === 'catch') return false;
        let current = expression;
        let chained = false;
        while (ts.isCallExpression(current) && ts.isPropertyAccessExpression(current.expression) && ['then', 'catch', 'finally'].includes(current.expression.name.text)) {
          chained = true;
          current = current.expression.expression;
        }
        return chained || (ts.isCallExpression(current) && ts.isIdentifier(current.expression) && names.has(current.expression.text));
      }
      for (const node of expressions(source, node => ts.isExpressionStatement(node) && unhandled(node.expression))) missing.push(`${relative}:${source.getLineAndCharacterOfPosition(node.getStart(source)).line + 1}: detached promise`);
      for (const node of expressions(source, node => ts.isJsxAttribute(node) && events.has(node.name.text) && node.initializer && ts.isJsxExpression(node.initializer))) {
        const callback = node.initializer.expression;
        if ((callback && ts.isIdentifier(callback) && names.has(callback.text)) ||
          (callback && ts.isArrowFunction(callback) && callback.modifiers?.some(item => item.kind === ts.SyntaxKind.AsyncKeyword)) ||
          (callback && ts.isArrowFunction(callback) && !ts.isBlock(callback.body) && unhandled(callback.body))) {
          missing.push(`${relative}:${source.getLineAndCharacterOfPosition(node.getStart(source)).line + 1}: UI event`);
        }
      }
    }
  }
  walk(`web/${theme}/src`);
  assert.ok(inspected >= 50, 'Inspect the frontend rather than an empty selection');
  assert.deepEqual(missing, [], 'Internal rejections must reach an awaiting caller or a terminal UI error handler');
});

if (theme === 'air') {
  test('component-owned confirmation callbacks retain their promise contract', () => {
    const expression = eventExpression(parse('web/air/src/components/ChannelsTable.js'), 'onConfirm', 'deleteAllDisabledChannels');
    assert.equal(expression, 'deleteAllDisabledChannels', 'Do not consume rejection contracts owned by the UI component');
  });

  for (const [file, action] of [['ChannelsTable.js', 'manageChannel'], ['RedemptionsTable.js', 'manageRedemption'], ['UsersTable.js', 'manageUser']]) {
    for (const result of ['business failure', 'transport failure', 'success']) {
      test(`air: ${file} deletion ${result} updates visible rows only after success`, async () => {
        const source = parse(`web/air/src/components/${file}`);
        const declarations = expressions(source, node => ts.isVariableDeclaration(node) && node.name.getText(source) === action);
        assert.equal(declarations.length, 1);
        const removed = [];
        const notifications = [];
        const updates = [];
        const failure = new Error('controlled network failure');
        const row = { id: 42, uuid: 'row-42', key: 'row-42', username: 'row-42', status: 1 };
        const request = async () => {
          if (result === 'transport failure') throw failure;
          return { data: { success: result === 'success', message: 'Denied by server', data: {} } };
        };
        const mutate = vm.runInNewContext(`(${declarations[0].initializer.getText(source)})`, {
          API: { delete: request, put: request, post: request }, channels: [row], redemptions: [row], users: [row],
          setChannels: value => updates.push(value), setRedemptions: value => updates.push(value), setUsers: value => updates.push(value),
          showError: error => notifications.push(error), showSuccess: () => {},
        });
        const callback = vm.runInNewContext(`(${eventExpression(source, 'onConfirm', action, "'delete'")})`, {
          [action]: mutate, record: row, channelRef: value => value.uuid || value.id, userRef: value => value.uuid || value.id,
          removeRecord: value => removed.push(value), reportUIError: error => notifications.push(error),
        });
        callback();
        await new Promise(setImmediate);
        assert.equal(removed.length, result === 'success' ? 1 : 0, 'A failed deletion must preserve the visible row');
        assert.equal(updates.length, result === 'success' ? 1 : 0, 'A failed mutation cannot update visible state');
        if (result === 'success') assert.equal(notifications.length, 0);
        else {
          assert.equal(notifications.length, 1);
          if (result === 'transport failure') assert.equal(notifications[0], failure);
          else assert.equal(notifications[0].message, 'Denied by server');
        }
      });
    }
  }
}

const themeRequire = createRequire(path.join(root, 'web', theme, 'package.json'));
const { JSDOM } = themeRequire('jsdom');
const dom = new JSDOM('<!doctype html><html><body></body></html>', { url: 'https://example.test/' });
globalThis.window = dom.window;
globalThis.document = dom.window.document;
globalThis.HTMLElement = dom.window.HTMLElement;
globalThis.navigator = dom.window.navigator;
globalThis.IS_REACT_ACT_ENVIRONMENT = true;
const React = themeRequire('react');
const { createRoot } = themeRequire('react-dom/client');
const act = React.act || themeRequire('react-dom/test-utils').act;
const sortColumns = ['prompt_tokens', 'completion_tokens', 'quota', 'elapsed_time'];

/** compileJSX evaluates real application JSX with the supplied scope and module loader and returns its exported value. */
function compileJSX(source, scope = {}, loadModule = themeRequire) {
  const output = ts.transpileModule(source, {
    compilerOptions: { jsx: ts.JsxEmit.ReactJSX, module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 },
  }).outputText;
  const module = { exports: {} };
  vm.runInNewContext(output, { ...scope, module, exports: module.exports, require: loadModule });
  return module.exports;
}

const berryLogHead = theme === 'berry'
  ? compileJSX(readSource('web/berry/src/views/Log/component/TableHead.js')).default
  : null;
const airLogSource = theme === 'air' ? parse('web/air/src/components/LogsTable.js') : null;
const airSortIcon = airLogSource && expressions(airLogSource, node => ts.isVariableDeclaration(node) && node.name.getText(airLogSource) === 'getSortIcon')[0];
const airTitles = airLogSource && expressions(airLogSource, node =>
  ts.isPropertyAssignment(node) && node.name.getText(airLogSource) === 'title' &&
  ts.isJsxElement(node.initializer) && node.initializer.getText(airLogSource).includes('getSortIcon('));
if (theme === 'air') assert.equal(airTitles.length, 4, 'Exercise every actual sortable Air log header');

/** renderLogHead creates the actual Berry header or actual Air title expressions for the supplied sort props. */
function renderLogHead(props) {
  if (theme === 'berry') return React.createElement('table', null, React.createElement(berryLogHead, props));
  const getSortIcon = compileJSX(`module.exports = (${airSortIcon.initializer.getText(airLogSource)});`, props);
  const titles = airTitles.map((node, index) => React.createElement('div', { key: sortColumns[index], 'data-sort-column': sortColumns[index] },
    compileJSX(`module.exports = (${node.initializer.getText(airLogSource)});`, {
      ...props,
      getSortIcon,
      handleSort: props.onSort,
      reportUIError: error => { throw error; },
    })));
  return React.createElement('div', null, titles);
}

/** translatedText replaces text nodes under the supplied root with translator-style elements and returns the count. */
function translatedText(container) {
  const walker = document.createTreeWalker(container, window.NodeFilter.SHOW_TEXT);
  const nodes = [];
  while (walker.nextNode()) if (walker.currentNode.textContent.trim()) nodes.push(walker.currentNode);
  for (const node of nodes) {
    const outer = document.createElement('font');
    const inner = document.createElement('font');
    inner.textContent = `Translated ${node.textContent}`;
    outer.appendChild(inner);
    node.replaceWith(outer);
  }
  return nodes.length;
}

/** sortHeader returns the mounted actual header element for the supplied column. */
function sortHeader(container, column) {
  return theme === 'berry'
    ? container.querySelectorAll('th')[4 + sortColumns.indexOf(column)]
    : container.querySelector(`[data-sort-column="${column}"] > span`);
}

for (const translated of [false, true]) {
  for (const column of sortColumns) {
    test(`${theme}: ${translated ? 'translated' : 'ordinary'} log sorting can switch away from ${column}`, async () => {
      const errors = [];
      const warnings = [];
      const originalConsoleError = console.error;
      console.error = (...messages) => warnings.push(messages.map(String).join(' '));
      /** SortBoundary preserves captured render errors so the test can distinguish crashes from successful sorting. */
      class SortBoundary extends React.Component {
        state = { error: null };
        /** getDerivedStateFromError stores the supplied error and returns the fallback state. */
        static getDerivedStateFromError(error) { return { error }; }
        /** componentDidCatch records the supplied error without emitting private DOM contents. */
        componentDidCatch(error) { errors.push(error); }
        /** render returns the supplied children or an empty alert after a rendering failure. */
        render() { return this.state.error ? React.createElement('div', { role: 'alert' }) : this.props.children; }
      }
      const container = document.createElement('div');
      document.body.appendChild(container);
      const mounted = createRoot(container);
      const clicks = [];
      const props = { userIsAdmin: false, sortBy: column, sortOrder: 'asc', sortLoading: false, onSort: async value => { clicks.push(value); } };
      /** update renders the actual header with the supplied sort state and returns its commit promise. */
      const update = next => act(async () => mounted.render(React.createElement(SortBoundary, null, renderLogHead({ ...props, ...next }))));
      try {
        await update({});
        assert.match(sortHeader(container, column).textContent, /↑/, 'The initial ascending indicator must be visible');
        if (translated) assert.ok(translatedText(container) > 0, 'Mutate actual mounted text rather than an empty selection');
        const nextColumn = sortColumns[(sortColumns.indexOf(column) + 1) % sortColumns.length];
        await act(async () => sortHeader(container, nextColumn).dispatchEvent(new window.MouseEvent('click', { bubbles: true })));
        assert.deepEqual(clicks, [nextColumn], 'Translation must preserve the actual sort callback and column argument');
        await update({ sortBy: nextColumn, sortLoading: true });
        assert.equal(errors.length, 0, errors.map(error => `${error.name}: ${error.message}`).join('\n'));
        assert.doesNotMatch(sortHeader(container, column).textContent, /[↑↓]/, 'The previous indicator must be removed');
        assert.match(sortHeader(container, nextColumn).textContent, /↑/, 'The next indicator must be visible');
        assert.match(sortHeader(container, nextColumn).textContent, /⏳/, 'Loading feedback must remain visible');
        await update({ sortBy: nextColumn, sortOrder: 'desc', sortLoading: false });
        assert.match(sortHeader(container, nextColumn).textContent, /↓/, 'Descending sorting must remain visible');
        assert.doesNotMatch(sortHeader(container, nextColumn).textContent, /⏳/, 'Loading feedback must clear');
        assert.equal(warnings.length, 0, warnings.join('\n'));
      } finally {
        await act(async () => mounted.unmount());
        container.remove();
        console.error = originalConsoleError;
      }
    });
  }
}

if (theme === 'air') {
  test('air: the actual logs component initializes state and helpers before constructing sortable headers', () => {
    /** createFixtureElement preserves JSX prop evaluation without rendering substituted UI dependencies. */
    const createFixtureElement = (type, props) => ({ type, props });
    const helpers = {
      isAdmin: () => false,
      timestamp2string: String,
      renderQuota: String,
      ITEMS_PER_PAGE: 20,
      default: () => null,
    };
    const view = compileJSX(readSource('web/air/src/components/LogsTable.js'), {
      localStorage: { getItem: () => null },
    }, name => {
      if (name === 'react/jsx-runtime') return { jsx: createFixtureElement, jsxs: createFixtureElement };
      if (name === 'react') return {
        ...React,
        /** useState supplies each real state initializer without executing a mounted component. */
        useState: initial => [typeof initial === 'function' ? initial() : initial, () => {}],
        /** useEffect avoids network effects while testing real component initialization. */
        useEffect: () => {},
      };
      if (name === '@douyinfe/semi-ui') return new Proxy({ Layout: { Header: 'header' } }, { get: (value, key) => value[key] || 'div' });
      if (name.startsWith('@douyinfe/semi-ui/')) return { default: 'div' };
      if (name === '@douyinfe/semi-icons') return { IconRefresh: 'svg' };
      if (name.endsWith('/latest-request.mjs')) return compileJSX(readSource('web/legacy/latest-request.mjs'));
      if (name.startsWith('.')) return helpers;
      return themeRequire(name);
    }).default;
    assert.doesNotThrow(() => view(), 'Initialization must reach the real JSX return before a caller can sort logs');
  });

  const debugSource = parse('web/air/src/components/ChannelDebugPanel.js');
  for (const count of ['model_configs_count', 'model_ratio_count']) {
    const labels = expressions(debugSource, node => ts.isJsxElement(node) && node.openingElement.tagName.getText(debugSource) === 'span' &&
      node.children.some(child => ts.isJsxExpression(child) && child.expression && ts.isBinaryExpression(child.expression) &&
        child.expression.left.getText(debugSource) === `migrationStatus.${count}`));
    assert.equal(labels.length, 1, `Exercise the actual ${count} label`);
    for (const translated of [false, true]) {
      test(`air: ${translated ? 'translated' : 'ordinary'} migration ${count} can refresh zero and positive counts`, async () => {
        const errors = [];
        const originalConsoleError = console.error;
        const warnings = [];
        console.error = (...messages) => warnings.push(messages.map(String).join(' '));
        /** CountBoundary records a rendering failure and preserves the test process for assertions. */
        class CountBoundary extends React.Component {
          state = { error: null };
          /** getDerivedStateFromError returns the error state for the supplied rendering failure. */
          static getDerivedStateFromError(error) { return { error }; }
          /** componentDidCatch records the supplied error for the regression assertion. */
          componentDidCatch(error) { errors.push(error); }
          /** render returns the actual label or an empty alert after a rendering failure. */
          render() { return this.state.error ? React.createElement('div', { role: 'alert' }) : this.props.children; }
        }
        const container = document.createElement('div');
        document.body.appendChild(container);
        const mounted = createRoot(container);
        /** update renders the actual count label with the supplied migration count and returns its commit promise. */
        const update = value => act(async () => mounted.render(React.createElement(CountBoundary, null,
          compileJSX(`module.exports = (${labels[0].getText(debugSource)});`, { migrationStatus: { [count]: value } }))));
        try {
          await update(0);
          assert.match(container.textContent, /0/, 'Preserve the existing zero-count display');
          if (translated) assert.ok(translatedText(container) > 0, 'Replace actual mounted text before the status refresh');
          await update(2);
          assert.equal(errors.length, 0, errors.map(error => `${error.name}: ${error.message}`).join('\n'));
          assert.match(container.textContent, /2/, 'A refreshed positive count must be visible');
          if (translated) assert.ok(translatedText(container) > 0, 'Replace the positive count before its next refresh');
          await update(4);
          assert.match(container.textContent, /4/, 'A changed positive count must update the visible text');
          await update(0);
          assert.match(container.textContent, /0/, 'The zero-count display must remain backward compatible');
          assert.equal(warnings.length, 0, warnings.join('\n'));
        } finally {
          await act(async () => mounted.unmount());
          container.remove();
          console.error = originalConsoleError;
        }
      });
    }
  }
}
