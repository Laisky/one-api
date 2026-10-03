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
