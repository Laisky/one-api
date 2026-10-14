import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import { createRequire } from 'node:module';
import { test } from 'node:test';
import { createContext, SourceTextModule, SyntheticModule } from 'node:vm';
import { markPrivateError as annotatePrivateError } from '../../../legacy/browser-sentry-privacy.mjs';

const require = createRequire(new URL('../../package.json', import.meta.url));
const ts = require('typescript');

/** fixture loads actual common helpers with synthetic storage and isolated privacy/UI dependencies. */
async function fixture(values = {}) {
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
      }
    },
    JSON: {
      parse(value) {
        try {
          return JSON.parse(value);
        } catch (error) {
          parseFailures.push({ error, message: error.message, name: error.name, stack: error.stack });
          throw error;
        }
      }
    }
  });
  const imports = {
    react: { default: {} },
    notistack: { enqueueSnackbar() {} },
    'constants/SnackbarConstants': { snackbarConstants: { Common: {}, Mobile: {} } },
    './api': { API: {} },
    '../../../legacy/browser-sentry-privacy.mjs': {
      markPrivateError(error, summary, expected = false) {
        marked.set(error, { summary, expected });
        return annotatePrivateError(error, summary, expected);
      }
    }
  };
  const source = await readFile(new URL('./common.js', import.meta.url), 'utf8');
  const output = ts.transpileModule(source, {
    compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022, jsx: ts.JsxEmit.React },
    fileName: 'common.js'
  }).outputText;
  const module = new SourceTextModule(output, { context, identifier: 'common.js' });
  await module.link((specifier) => {
    assert.ok(Object.hasOwn(imports, specifier), 'Only expected helper imports are evaluated');
    const exports = imports[specifier];
    return new SyntheticModule(
      Object.keys(exports),
      function initialize() {
        for (const [name, value] of Object.entries(exports)) this.setExport(name, value);
      },
      { context }
    );
  });
  await module.evaluate();
  return { helper: module.namespace, marked, parseFailures, storage, state };
}

/** assertPrivateParseFailure checks the controlled summary and unchanged native parser error. */
function assertPrivateParseFailure(fixture, call, summary) {
  assert.throws(call, (error) => {
    assert.ok(fixture.marked.has(error), 'Stored-data parse failures must receive a private annotation');
    const original = fixture.parseFailures.at(-1);
    assert.equal(error, original.error);
    assert.ok(error instanceof SyntaxError);
    assert.equal(error.message, original.message);
    assert.equal(error.name, original.name);
    assert.equal(error.stack, original.stack);
    assert.deepEqual(fixture.marked.get(error), { summary, expected: false });
    return true;
  });
}

for (const name of ['isAdmin', 'isRoot']) {
  test(`${name} annotates corrupt persisted user JSON with unchanged error identity`, async () => {
    const loaded = await fixture({ user: 'synthetic-private-user-json' });
    assertPrivateParseFailure(loaded, () => loaded.helper[name](), 'Saved user data could not be parsed.');
  });
}

test('getChannelModels annotates corrupt persisted model JSON with unchanged error identity', async () => {
  const loaded = await fixture({ channel_models: 'synthetic-private-model-json' });
  assertPrivateParseFailure(loaded, () => loaded.helper.getChannelModels(1), 'Saved model data could not be parsed.');
});

test('role checks preserve missing users and valid thresholds without annotation', async () => {
  const loaded = await fixture();
  assert.equal(loaded.helper.isAdmin(), false);
  assert.equal(loaded.helper.isRoot(), false);
  for (const [role, admin, root] of [
    [1, false, false],
    [10, true, false],
    [100, true, true]
  ]) {
    loaded.storage.set('user', JSON.stringify({ role }));
    assert.equal(loaded.helper.isAdmin(), admin);
    assert.equal(loaded.helper.isRoot(), root);
  }
  assert.equal(loaded.marked.size, 0);
});

test('getChannelModels preserves missing, unknown and cached model outputs', async () => {
  const loaded = await fixture();
  assert.deepEqual([...loaded.helper.getChannelModels(1)], []);
  loaded.storage.set('channel_models', JSON.stringify({ 1: ['controlled-model'] }));
  assert.deepEqual([...loaded.helper.getChannelModels(1)], ['controlled-model']);
  assert.deepEqual([...loaded.helper.getChannelModels(2)], []);
  loaded.storage.set('channel_models', 'synthetic-corrupted-after-cache');
  assert.deepEqual([...loaded.helper.getChannelModels(1)], ['controlled-model']);
  assert.equal(loaded.marked.size, 0);
});

test('localStorage access errors keep ordinary identity and are not marked as parser errors', async () => {
  const loaded = await fixture();
  const error = new Error('controlled browser storage failure');
  loaded.state.readFailure = error;
  for (const name of ['isAdmin', 'isRoot', 'getChannelModels'])
    assert.throws(
      () => loaded.helper[name](1),
      (caught) => caught === error
    );
  assert.equal(loaded.marked.size, 0);
  assert.equal(loaded.parseFailures.length, 0);
});

test('ordinary role-access errors after successful parsing remain unannotated', async () => {
  const loaded = await fixture({ user: 'null' });
  for (const name of ['isAdmin', 'isRoot'])
    assert.throws(
      () => loaded.helper[name](),
      (error) => error.name === 'TypeError'
    );
  assert.equal(loaded.marked.size, 0);
  assert.equal(loaded.parseFailures.length, 0);
});

test('ordinary model-access errors after successful parsing remain unannotated', async () => {
  const loaded = await fixture({ channel_models: 'null' });
  assert.throws(
    () => loaded.helper.getChannelModels(1),
    (error) => error.name === 'TypeError'
  );
  assert.equal(loaded.marked.size, 0);
  assert.equal(loaded.parseFailures.length, 0);
});

test('verifyJSON retains swallowed invalid-input behavior without private annotation', async () => {
  const loaded = await fixture();
  assert.equal(loaded.helper.verifyJSON('synthetic-invalid-validation'), false);
  assert.equal(loaded.helper.verifyJSON('{"valid":true}'), true);
  assert.equal(loaded.marked.size, 0);
});
