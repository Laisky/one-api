import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import path from 'node:path';
import { test } from 'node:test';
import { fileURLToPath } from 'node:url';

// The legacy edit-channel forms load the whole channel from GET /api/channel/:id and
// PUT it back. They neither display nor edit testing_model, so echoing the loaded value
// would overwrite a selection (such as SKIP) saved elsewhere while the form was open:
// the server treats an explicit null as "clear to auto-select" (issue #609).
// Standalone: node --test .github/scripts/legacy-channel-update.test.mjs
const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '../..');
const forms = {
  air: 'web/air/src/pages/Channel/EditChannel.js',
  berry: 'web/berry/src/views/Channel/component/EditModal.js',
};

/**
 * loadHelper imports the shared payload helper the legacy forms use.
 * @returns {Promise<Function>} The omitUnownedChannelFields export.
 */
async function loadHelper() {
  const module = await import(path.join(root, 'web/legacy/channel-update-payload.mjs'));
  assert.equal(typeof module.omitUnownedChannelFields, 'function', 'the shared helper must exist');
  return module.omitUnownedChannelFields;
}

test('the helper drops fields the legacy forms do not own and keeps everything else', async () => {
  const omitUnownedChannelFields = await loadHelper();
  const loaded = { uuid: 'c-1', name: 'Channel', models: 'gpt-4o', model_mapping: null, testing_model: null };
  const payload = omitUnownedChannelFields(loaded);
  assert.equal(Object.hasOwn(payload, 'testing_model'), false, 'an explicit null would clear the stored selection');
  assert.deepEqual(payload, { uuid: 'c-1', name: 'Channel', models: 'gpt-4o', model_mapping: null });
  assert.equal(loaded.testing_model, null, 'the form state is not mutated');
  assert.equal(Object.hasOwn(omitUnownedChannelFields({ ...loaded, testing_model: '__skip__' }), 'testing_model'), false);
});

for (const [theme, file] of Object.entries(forms)) {
  test(`${theme}: the channel update PUT is built through omitUnownedChannelFields`, async () => {
    const source = await readFile(path.join(root, file), 'utf8');
    const updates = [...source.matchAll(/API\.put\(\s*`\/api\/channel\/`\s*,\s*([^\n]+)/g)];
    assert.ok(updates.length > 0, `${file} must still update channels with PUT /api/channel/`);
    for (const [, argument] of updates) {
      assert.match(argument, /^omitUnownedChannelFields\(/, `${file} must not echo testing_model it never edits`);
    }
    assert.match(source, /from '\.\.\/(\.\.\/)+legacy\/channel-update-payload\.mjs'/, `${file} must import the shared helper`);
  });
}
