const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const { createRequire } = require('node:module');
const { test } = require('node:test');

const root = path.resolve(__dirname, '../..');
const ts = createRequire(path.join(root, 'web/air/package.json'))('typescript');
const text = fs.readFileSync(path.join(root, 'web/air/src/components/TokensTable.js'), 'utf8');
const source = ts.createSourceFile('TokensTable.js', text, ts.ScriptTarget.Latest, true, ts.ScriptKind.JSX);
let handler;
/** visit locates the actual mutation handler without replacing its logic in tests. */
function visit(node) {
  if (ts.isVariableDeclaration(node) && node.name.getText(source) === 'manageToken') handler = node.initializer.getText(source);
  ts.forEachChild(node, visit);
}
visit(source);
assert.ok(handler, 'The real token mutation handler must exist');

/** fixture supplies controlled API results and records visible state mutations. */
function fixture(response, error) {
  const rows = [{ id: 1, uuid: 'token-one', status: 1 }, { id: 2, status: 1 }];
  const updates = [];
  const busy = [];
  const failures = [];
  const successes = [];
  const calls = [];
  const request = async (...args) => { calls.push(args); if (error) throw error; return { data: response }; };
  const mutate = vm.runInNewContext(`(${handler})`, {
    API: { delete: request, put: request }, tokens: rows, tokenRef: token => token.uuid || token.id,
    setTokensFormat: update => updates.push(update), setLoading: value => busy.push(value),
    showError: value => failures.push(value), showSuccess: value => successes.push(value),
  });
  return { rows, updates, busy, failures, successes, calls, mutate };
}

test('a rejected business mutation never removes a token or reports success', async () => {
  const state = fixture({ success: false, message: 'not authorized' });
  assert.equal(await state.mutate('token-one', 'delete'), false);
  assert.equal(state.updates.length, 0);
  assert.equal(state.successes.length, 0);
  assert.deepEqual(state.failures, ['not authorized']);
  assert.equal(state.busy.at(-1), false);
  assert.ok(!text.includes('removeRecord(record.key)'), 'UI callbacks cannot delete independently of API success');
});

test('transport errors preserve their identity and leave visible records unchanged', async () => {
  const error = new Error('network unavailable');
  const state = fixture(null, error);
  await assert.rejects(state.mutate(2, 'delete'), caught => caught === error);
  assert.equal(state.updates.length, 0);
  assert.equal(state.busy.at(-1), false);
});

for (const id of ['token-one', 2]) {
  test(`confirmed deletion updates only the matching reference: ${id}`, async () => {
    const state = fixture({ success: true });
    assert.equal(await state.mutate(id, 'delete'), true);
    assert.equal(state.updates[0].length, 1);
    assert.ok(state.updates[0].every(token => (token.uuid || token.id) !== id));
    assert.equal(state.rows.length, 2, 'Do not mutate the previous state array');
    assert.equal(state.successes.length, 1);
    assert.equal(state.busy.at(-1), false);
  });
}

test('status updates are immutable and retain unrelated tokens', async () => {
  const state = fixture({ success: true, data: { status: 2 } });
  await state.mutate('token-one', 'disable');
  assert.equal(state.updates[0][0].status, 2);
  assert.equal(state.rows[0].status, 1);
  assert.equal(state.updates[0][1], state.rows[1]);
  assert.equal(state.calls[0][1].uuid, 'token-one');
  assert.equal(state.calls[0][1].status, 2);
});

test('unsupported actions make no request and always release busy state', async () => {
  const state = fixture({ success: true });
  await assert.rejects(state.mutate(2, 'unknown'), /Unsupported token action/);
  assert.equal(state.calls.length, 0);
  assert.equal(state.busy.at(-1), false);
});
