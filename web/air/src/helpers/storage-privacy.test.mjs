import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import { createRequire } from 'node:module';
import { test } from 'node:test';
import { createContext, SourceTextModule, SyntheticModule } from 'node:vm';
import { markPrivateError as annotatePrivateError } from '../../../legacy/browser-sentry-privacy.mjs';

const require = createRequire(new URL('../../package.json', import.meta.url));
const ts = require('typescript');
const SUMMARY = 'Saved user data could not be parsed.';

/** fixture loads an actual helper with controlled browser storage and captures only privacy annotations. */
async function fixture(filename, values = {}) {
  const marked = new Map();
  const parseFailures = [];
  const storage = new Map(Object.entries(values));
  const state = { readFailure: undefined };
  const context = createContext({
    window: { innerWidth: 1024 },
    localStorage: {
      getItem(key) {
        if (state.readFailure) throw state.readFailure;
        return storage.get(key) ?? null;
      },
    },
    JSON: {
      parse(value) {
        try {
          return JSON.parse(value);
        } catch (error) {
          parseFailures.push({
            error,
            message: error.message,
            name: error.name,
            stack: error.stack,
          });
          throw error;
        }
      },
    },
  });
  const imports = {
    '@douyinfe/semi-ui': { Toast: {} },
    '../constants': { toastConstants: {} },
    react: { default: {} },
    'react-toastify': { toast() {} },
    '../../../legacy/browser-sentry-privacy.mjs': {
      markPrivateError(error, summary, expected = false) {
        marked.set(error, { summary, expected });
        return annotatePrivateError(error, summary, expected);
      },
    },
  };
  const source = await readFile(new URL(filename, import.meta.url), 'utf8');
  const output = ts.transpileModule(source, {
    compilerOptions: {
      module: ts.ModuleKind.ESNext,
      target: ts.ScriptTarget.ES2022,
      jsx: ts.JsxEmit.React,
    },
    fileName: filename,
  }).outputText;
  const module = new SourceTextModule(output, {
    context,
    identifier: filename,
  });
  await module.link((specifier) => {
    assert.ok(
      Object.hasOwn(imports, specifier),
      'Only expected helper imports are evaluated',
    );
    const exports = imports[specifier];
    return new SyntheticModule(
      Object.keys(exports),
      function initialize() {
        for (const [name, value] of Object.entries(exports))
          this.setExport(name, value);
      },
      { context },
    );
  });
  await module.evaluate();
  return { helper: module.namespace, marked, parseFailures, storage, state };
}

/** assertPrivateParseFailure checks annotation without changing the original native parse error. */
function assertPrivateParseFailure(fixture, call) {
  assert.throws(call, (error) => {
    assert.ok(
      fixture.marked.has(error),
      'Stored-data parse failures must receive a private annotation',
    );
    const original = fixture.parseFailures.at(-1);
    assert.equal(error, original.error);
    assert.ok(error instanceof SyntaxError);
    assert.equal(error.message, original.message);
    assert.equal(error.name, original.name);
    assert.equal(error.stack, original.stack);
    assert.deepEqual(fixture.marked.get(error), {
      summary: SUMMARY,
      expected: false,
    });
    return true;
  });
}

test('authHeader annotates corrupt persisted user JSON while preserving its native error', async () => {
  const loaded = await fixture('./auth-header.js', {
    user: 'synthetic-private-user-json',
  });
  assertPrivateParseFailure(loaded, () => loaded.helper.authHeader());
});

for (const name of ['isAdmin', 'isRoot']) {
  test(`${name} annotates only corrupt persisted user JSON`, async () => {
    const loaded = await fixture('./utils.js', {
      user: 'synthetic-private-user-json',
    });
    assertPrivateParseFailure(loaded, () => loaded.helper[name]());
  });
}

test('authHeader preserves valid bearer output and missing-user behavior without annotation', async () => {
  const loaded = await fixture('./auth-header.js');
  assert.deepEqual({ ...loaded.helper.authHeader() }, {});
  loaded.storage.set(
    'user',
    JSON.stringify({ token: 'controlled-test-token' }),
  );
  assert.deepEqual(
    { ...loaded.helper.authHeader() },
    { Authorization: 'Bearer controlled-test-token' },
  );
  assert.equal(loaded.marked.size, 0);
});

test('role checks preserve missing users and valid role thresholds', async () => {
  const loaded = await fixture('./utils.js');
  assert.equal(loaded.helper.isAdmin(), false);
  assert.equal(loaded.helper.isRoot(), false);
  for (const [role, admin, root] of [
    [1, false, false],
    [10, true, false],
    [100, true, true],
  ]) {
    loaded.storage.set('user', JSON.stringify({ role }));
    assert.equal(loaded.helper.isAdmin(), admin);
    assert.equal(loaded.helper.isRoot(), root);
  }
  assert.equal(loaded.marked.size, 0);
});

test('localStorage access errors remain ordinary errors rather than private JSON errors', async () => {
  for (const [filename, names] of [
    ['./auth-header.js', ['authHeader']],
    ['./utils.js', ['isAdmin', 'isRoot']],
  ]) {
    const loaded = await fixture(filename);
    const error = new Error('controlled browser storage failure');
    loaded.state.readFailure = error;
    for (const name of names)
      assert.throws(
        () => loaded.helper[name](),
        (caught) => caught === error,
      );
    assert.equal(loaded.marked.size, 0);
    assert.equal(loaded.parseFailures.length, 0);
  }
});

test('ordinary role-access errors after successful JSON parsing are not annotated', async () => {
  const loaded = await fixture('./utils.js', { user: 'null' });
  for (const name of ['isAdmin', 'isRoot'])
    assert.throws(
      () => loaded.helper[name](),
      (error) => error.name === 'TypeError',
    );
  assert.equal(loaded.marked.size, 0);
  assert.equal(loaded.parseFailures.length, 0);
});

test('verifyJSON retains swallowed-validation behavior without privacy annotation', async () => {
  const loaded = await fixture('./utils.js');
  assert.equal(loaded.helper.verifyJSON('synthetic-invalid-validation'), false);
  assert.equal(loaded.helper.verifyJSON('{"valid":true}'), true);
  assert.equal(loaded.marked.size, 0);
});
