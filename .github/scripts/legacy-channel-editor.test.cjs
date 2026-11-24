const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const { test } = require('node:test');
const { React, act, createTransport, defaultExport, loadModule, mount, root, settle, theme, themeRequire } = require('./legacy-react-harness.cjs');

// This suite mounts the real Air EditChannel and Berry EditModal with their real
// UI libraries, form state and submit handlers. Transport, toasts, the debug
// panel and (for Air) the helper barrel are replaced. Each case of the shared
// contract fixture loads a stored channel, optionally types into the field that
// edits the legacy `other` value, submits, and asserts what the field displayed
// and what the update request carried. The relay prefers a non-empty typed config
// override (api_version, plugin, library_id) over `other`, so the field must show
// and edit the effective value without inventing or dropping an override.
// Berry additionally fills "related models" from the zero-config defaults while
// the pricing suggestions keep using the full provider catalog.
// Standalone: (cd web/air && node --test ../../.github/scripts/legacy-channel-editor.test.cjs)

const fixture = JSON.parse(fs.readFileSync(path.join(root, 'test/testdata/channel_other_override_cases.json'), 'utf8'));

// RENDERED_FIELDS maps each channel type this theme renders an `other` field for to the selector of that input.
const RENDERED_FIELDS = theme === 'air'
  ? { 3: 'input[name="azure_other"]', 18: 'input[name="other"]', 21: 'input[name="other"]' }
  : { 3: '#channel-other-label', 17: '#channel-other-label', 18: '#channel-other-label', 24: '#channel-other-label' };

/** expectedEntries lists the override spellings a fixture state places in config, in order. */
function expectedEntries(entry, state) {
  const entries = state.shadowed ? [[state.shadowed.key, state.shadowed.value]] : [];
  if (state.override !== null) entries.push([entry.config_key ?? entry.override_key, state.override]);
  return entries;
}

/** storedChannel returns the GET /api/channel/:id body for the supplied fixture case. */
function storedChannel(entry) {
  const config = { api_format: 'chat_completion', future_backend_key: 'kept', ...Object.fromEntries(expectedEntries(entry, entry.stored)) };
  return {
    id: 7, uuid: 'channel-7', name: 'Stored channel', type: entry.type, key: '', status: 1,
    base_url: 'https://upstream.example.test', other: entry.stored.other, models: 'stored-model', group: 'default',
    model_mapping: '', system_prompt: '', model_configs: '', inference_profile_arn_map: '', ratelimit: 0, auto_ban: 1,
    config: JSON.stringify(config),
  };
}

/** acceptUpdate answers every save request as accepted. */
const acceptUpdate = () => ({ success: true, message: '' });

/** transport returns a recording API double that answers the editor's loading requests and records update bodies; answerUpdate decides each save response. */
function transport(channel, extra = () => undefined, answerUpdate = acceptUpdate) {
  const updates = [];
  const { API, requests } = createTransport(request => {
    const answered = extra(request);
    if (answered !== undefined) return answered;
    if (request.path === `/api/channel/${channel?.uuid}`) return { success: true, message: '', data: { ...channel } };
    if (request.path === '/api/group/') return { success: true, data: ['default'] };
    if (request.path === '/api/channel/models') return { success: true, data: [{ id: 'stored-model', owned_by: 'openai' }] };
    if (request.path === '/api/channel/default-pricing') return { success: true, data: {} };
    if (request.path === '/api/channel/pricing/channel-7') return { success: true, data: { model_ratio: {}, completion_ratio: {} } };
    throw new Error(`Unexpected request ${request.path}`);
  });
  /** put records one update body and answers it through answerUpdate. */
  API.put = async (url, body) => { updates.push({ url, body }); return { data: await answerUpdate(updates.length) }; };
  /** post records one create body and answers it through answerUpdate. */
  API.post = async (url, body) => { updates.push({ url, body }); return { data: await answerUpdate(updates.length) }; };
  return { API, requests, updates };
}

/** findButton returns the rendered button that displays the supplied label anywhere in the document, including portals. */
function findButton(label) {
  const button = [...document.querySelectorAll('button')].find(element => element.textContent.trim() === label);
  assert.ok(button, `Find button ${label}`);
  return button;
}

/** mountEditor mounts the theme's real channel editor and returns interaction helpers plus recorded traffic. */
async function mountEditor({ channel, extra, before, answerUpdate }) {
  const failures = [];
  const { API, requests, updates } = transport(channel, extra, answerUpdate);
  /** recordFailure keeps UI-reported failures for the absence assertion. */
  const recordFailure = error => { failures.push(error); };
  let element;
  let common;
  if (theme === 'air') {
    const EditChannel = loadModule('web/air/src/pages/Channel/EditChannel.js', {
      'web/air/src/helpers/index.js': {
        API, isMobile: () => false, showError: recordFailure, showInfo: recordFailure, showSuccess: () => {},
        verifyJSON: text => { try { JSON.parse(text); return true; } catch { return false; } },
      },
      'web/air/src/helpers/utils.js': { showError: recordFailure },
      'web/air/src/components/ChannelDebugPanel.js': defaultExport(() => null),
    }).default;
    const { MemoryRouter } = themeRequire('react-router-dom');
    element = React.createElement(MemoryRouter, null, React.createElement(EditChannel, {
      editingChannel: channel ? { uuid: channel.uuid } : {}, visible: true, handleClose: () => {}, refresh: () => {},
    }));
  } else {
    const cache = new Map();
    const overrides = {
      'web/berry/src/utils/api.js': { API },
      notistack: { enqueueSnackbar: message => { failures.push(message); } },
      'web/berry/src/components/ChannelDebugPanel.js': defaultExport(() => null),
    };
    common = loadModule('web/berry/src/utils/common.js', overrides, cache);
    overrides['web/berry/src/utils/common.js'] = { ...common, showError: recordFailure, showSuccess: () => {} };
    if (before) await before(common);
    const EditModal = loadModule('web/berry/src/views/Channel/component/EditModal.js', overrides, cache).default;
    element = React.createElement(EditModal, { open: true, channelId: channel?.uuid, onCancel: () => {}, onOk: () => {} });
  }
  const view = await mount(element);
  return {
    ...view, requests, updates, failures, common,
    /** field returns the rendered input that edits the legacy `other` value for the supplied type. */
    field: type => document.querySelector(RENDERED_FIELDS[type]),
    /** chooseType selects the supplied channel type through the real type selector. */
    chooseType: async type => {
      if (theme === 'air') {
        const label = { 3: 'Azure OpenAI', 17: '阿里通义千问' }[type];
        await view.click(document.querySelector('.semi-sidesheet .semi-select'));
        const option = [...document.querySelectorAll('.semi-select-option')].find(item => item.textContent.trim() === label);
        assert.ok(option, `Find type option ${label}`);
        await view.click(option);
        await finishCloseAnimations();
      } else {
        const trigger = document.querySelector('#channel-type-label[role="combobox"]');
        assert.ok(trigger, 'Find the type selector');
        await act(async () => trigger.dispatchEvent(new window.MouseEvent('mousedown', { bubbles: true, cancelable: true, button: 0 })));
        await settle();
        await view.click(document.querySelector(`li[role="option"][data-value="${type}"]`));
      }
    },
    /** submit clicks the real submit button and waits for validation and the update request. */
    submit: async () => {
      await view.click(findButton('提交'));
      await settle();
    },
  };
}

/** finishCloseAnimations delivers the animation-end events jsdom never fires so Semi popups finish closing. */
async function finishCloseAnimations() {
  for (const element of document.querySelectorAll('[class*="-animation-hide"]')) {
    for (const name of ['animationend', 'webkitAnimationEnd']) {
      await act(async () => element.dispatchEvent(new window.Event(name, { bubbles: true })));
    }
  }
  await settle();
}

/** savedOverrideEntries lists, in order, every key of the submitted config JSON that spells key in any letter case. */
function savedOverrideEntries(body, key) {
  const config = JSON.parse(body.config);
  assert.equal(config.future_backend_key, 'kept', 'Unrelated config keys round-trip');
  return Object.entries(config).filter(([name]) => name.toLowerCase() === key);
}

/** savedOverride returns the override value in the submitted config JSON, or null when the key is absent. */
function savedOverride(body, key) {
  const config = JSON.parse(body.config);
  assert.equal(config.future_backend_key, 'kept', 'Unrelated config keys round-trip');
  return Object.hasOwn(config, key) ? config[key] : null;
}

test(`${theme}: the shared fixture covers every type whose field this theme renders`, () => {
  const fixtureTypes = new Set(fixture.cases.map(entry => entry.type));
  for (const type of fixtureTypes) {
    if (!Object.hasOwn(RENDERED_FIELDS, type)) assert.ok([17, 21].includes(type), `Type ${type} must be rendered or documented as hidden`);
  }
  for (const type of Object.keys(RENDERED_FIELDS).map(Number).filter(type => type !== 24)) assert.ok(fixtureTypes.has(type), `Fixture covers type ${type}`);
});

for (const entry of fixture.cases) {
  const rendered = Object.hasOwn(RENDERED_FIELDS, entry.type);
  // A type without a rendered field can still be saved for unrelated reasons; it must then round-trip untouched.
  if (!rendered && entry.edit !== null) continue;
  test(`${theme}: ${entry.name} displays and saves the effective value`, async () => {
    const channel = storedChannel(entry);
    const editor = await mountEditor({ channel });
    try {
      if (rendered) {
        const input = editor.field(entry.type);
        assert.ok(input, `The editor renders the field for type ${entry.type}`);
        assert.equal(input.value, entry.displayed, 'The field shows the value the relay uses');
        if (entry.edit !== null) await editor.type(input, entry.edit);
      } else {
        assert.equal(document.querySelector('input[name="other"]'), null, `Type ${entry.type} renders no field in ${theme}`);
      }
      await editor.submit();
      assert.deepEqual(editor.failures, []);
      assert.equal(editor.updates.length, 1, 'The editor sends one update');
      const { body } = editor.updates[0];
      assert.equal(body.other, entry.payload.other, 'The update carries the contract `other` value');
      assert.deepEqual(savedOverrideEntries(body, entry.override_key), expectedEntries(entry, entry.payload), 'The update carries the contract override spellings');
    } finally {
      await editor.unmount();
    }
  });
}

// A failed save must leave the form, and so the untouched-field comparison, exactly as loaded:
// otherwise the retry reads the resolved legacy value as an edit and writes it into the override.
for (const failure of theme === 'berry' ? ['unsuccessful', 'rejected'] : ['unsuccessful']) {
  test(`${theme}: a failed (${failure}) save leaves the field and the next untouched save unchanged`, async () => {
    const entry = fixture.cases.find(item => item.name === 'azure_conflict_unrelated_save');
    /** answerUpdate fails the first save and accepts the retry. */
    const answerUpdate = attempt => {
      if (attempt > 1) return acceptUpdate();
      if (failure === 'rejected') throw new Error('synthetic network failure');
      return { success: false, message: 'synthetic save failure' };
    };
    const editor = await mountEditor({ channel: storedChannel(entry), answerUpdate });
    try {
      await editor.submit();
      assert.equal(editor.updates.length, 1, 'The first save is sent');
      assert.equal(editor.field(entry.type).value, entry.displayed, 'A failed save keeps the field on the effective value');
      await editor.submit();
      assert.equal(editor.updates.length, 2, 'The retry is sent');
      for (const { body } of editor.updates) {
        assert.equal(body.other, entry.payload.other, 'Each attempt resends the stored `other` value');
        assert.deepEqual(savedOverrideEntries(body, entry.override_key), expectedEntries(entry, entry.payload), 'Each attempt resends the stored override');
      }
    } finally {
      await editor.unmount();
    }
  });
}

for (const [type, fallback] of theme === 'air' ? [[3, '2024-03-01-preview'], [18, 'v2.1']] : [[3, '2023-09-01-preview'], [18, 'v2.1']]) {
  test(`${theme}: an empty type ${type} field without an override still saves the legacy default`, async () => {
    const channel = storedChannel({ type, override_key: 'api_version', stored: { other: '', override: null } });
    const editor = await mountEditor({ channel });
    try {
      await editor.submit();
      assert.deepEqual(editor.failures, []);
      assert.equal(editor.updates.length, 1);
      assert.equal(editor.updates[0].body.other, fallback);
      assert.equal(savedOverride(editor.updates[0].body, 'api_version'), null, 'No override is created');
    } finally {
      await editor.unmount();
    }
  });
}

test(`${theme}: changing the channel type forgets the loaded override`, async () => {
  const channel = storedChannel({ type: 3, override_key: 'api_version', stored: { other: '2024-02-01', override: '2024-10-21' } });
  const editor = await mountEditor({ channel });
  try {
    assert.equal(editor.field(3).value, '2024-10-21');
    await editor.chooseType(17);
    await editor.chooseType(3);
    assert.equal(editor.field(3).value, '', 'A new type starts with an empty field');
    // The new type starts from a fresh config, so retyping the old displayed value is an ordinary edit.
    await editor.type(editor.field(3), '2024-10-21');
    if (theme === 'berry') await editor.type(document.querySelector('#channel-base_url-label'), 'https://upstream.example.test');
    await editor.click(findButton('填入所有模型'));
    await editor.submit();
    assert.deepEqual(editor.failures, []);
    assert.equal(editor.updates.length, 1);
    assert.equal(editor.updates[0].body.other, '2024-10-21');
    assert.equal(Object.hasOwn(JSON.parse(editor.updates[0].body.config), 'api_version'), false, 'No override is created');
  } finally {
    await editor.unmount();
  }
});

if (theme === 'berry') {
  const catalog = { 1: ['full-a', 'full-b', 'needs-pricing'], 3: ['azure-full'] };
  const defaults = { 1: ['full-a'], 3: [] };

  /** catalogRequests answers the full catalog and, unless failing, the zero-config defaults. */
  const catalogRequests = (failDefaults = false) => request => {
    if (request.path === '/api/models') return { success: true, message: '', data: catalog };
    if (request.path === '/api/models/defaults') {
      if (failDefaults) throw new Error('defaults unavailable');
      return { success: true, message: '', data: defaults };
    }
    return undefined;
  };

  /** clearModelCaches removes every persisted model cache so each test starts cold. */
  const clearModelCaches = () => {
    for (const key of ['channel_models', 'channel_default_models']) localStorage.removeItem(key);
  };

  /** modelsAfterFill clicks the real fill-related button, submits and returns the saved models. */
  const modelsAfterFill = async editor => {
    await editor.click(findButton('填入相关模型'));
    await editor.submit();
    assert.equal(editor.updates.length, 1, 'The filled channel saves');
    return editor.updates[0].body.models;
  };

  for (const channelId of ['channel-7', undefined]) {
    test(`berry: fill-related fills zero-config defaults for ${channelId ? 'a stored' : 'a new'} channel`, async () => {
      clearModelCaches();
      const channel = { ...storedChannel({ type: 1, override_key: 'api_version', stored: { other: '', override: null } }), models: 'custom-model' };
      const editor = await mountEditor({
        channel: channelId ? channel : undefined,
        extra: catalogRequests(),
        before: common => common.loadChannelModels(),
      });
      try {
        if (!channelId) {
          await editor.type(document.querySelector('#channel-name-label'), 'New channel');
          await editor.type(document.querySelector('#channel-key-label'), 'sk-new');
        }
        assert.deepEqual(editor.requests.filter(request => request.path.startsWith('/api/models')).map(request => request.path).sort(), ['/api/models', '/api/models/defaults']);
        assert.equal(await modelsAfterFill(editor), 'full-a');
        assert.deepEqual(editor.failures, []);
      } finally {
        await editor.unmount();
      }
    });
  }

  test('berry: fill-related falls back to the full catalog when the defaults request fails', async () => {
    clearModelCaches();
    localStorage.setItem('channel_default_models', JSON.stringify({ 1: ['stale-default'] }));
    const channel = storedChannel({ type: 1, override_key: 'api_version', stored: { other: '', override: null } });
    const editor = await mountEditor({ channel, extra: catalogRequests(true), before: common => common.loadChannelModels() });
    try {
      assert.equal(localStorage.getItem('channel_default_models'), null, 'A failed refresh drops stale defaults');
      assert.equal(await modelsAfterFill(editor), 'full-a,full-b,needs-pricing');
    } finally {
      await editor.unmount();
    }
  });

  /** raceTimeout resolves to the outcome of promise, or to 'timed out' when it has not settled within 200 ms. */
  const raceTimeout = promise => Promise.race([
    promise.then(() => 'loaded', error => `rejected: ${error.message}`),
    new Promise(resolve => setTimeout(() => resolve('timed out'), 200)),
  ]);

  for (const outcome of ['loaded', 'rejected: catalog unavailable']) {
    test(`berry: the catalog request settles (${outcome}) without waiting for a slow defaults request`, async () => {
      clearModelCaches();
      let releaseDefaults = () => {};
      const pendingDefaults = new Promise(resolve => { releaseDefaults = resolve; });
      const { API } = transport(undefined, request => {
        if (request.path === '/api/models') {
          if (outcome !== 'loaded') throw new Error('catalog unavailable');
          return { success: true, message: '', data: catalog };
        }
        if (request.path === '/api/models/defaults') return pendingDefaults;
        return undefined;
      });
      const common = loadModule('web/berry/src/utils/common.js', { 'web/berry/src/utils/api.js': { API }, notistack: { enqueueSnackbar: () => {} } });
      try {
        assert.equal(await raceTimeout(common.loadChannelModels()), outcome, 'The catalog outcome must not wait for the defaults request');
        if (outcome === 'loaded') {
          assert.deepEqual(JSON.parse(localStorage.getItem('channel_models')), catalog, 'The catalog is cached as soon as it arrives');
          assert.deepEqual(common.getChannelDefaultModels(1), catalog[1], 'Until the defaults arrive, fill-related uses the full catalog');
        }
        releaseDefaults({ success: true, message: '', data: defaults });
        await settle();
        assert.deepEqual(JSON.parse(localStorage.getItem('channel_default_models')), defaults, 'The defaults are cached when they arrive');
        assert.deepEqual(common.getChannelDefaultModels(1), defaults[1]);
      } finally {
        releaseDefaults({ success: true, message: '', data: defaults });
        clearModelCaches();
      }
    });
  }

  test('berry: fill-related falls back to the persisted full catalog when no defaults are cached', async () => {
    clearModelCaches();
    localStorage.setItem('channel_models', JSON.stringify(catalog));
    const channel = storedChannel({ type: 1, override_key: 'api_version', stored: { other: '', override: null } });
    const editor = await mountEditor({ channel });
    try {
      assert.equal(await modelsAfterFill(editor), 'full-a,full-b,needs-pricing');
    } finally {
      await editor.unmount();
    }
  });

  test('berry: corrupt saved default models fail with a private annotation', () => {
    clearModelCaches();
    const marked = [];
    const common = loadModule('web/berry/src/utils/common.js', {
      'web/berry/src/utils/api.js': { API: {} },
      notistack: { enqueueSnackbar: () => {} },
      '../../../legacy/browser-sentry-privacy.mjs': { markPrivateError: (error, summary) => { marked.push(summary); return error; } },
    });
    localStorage.setItem('channel_default_models', 'synthetic-corrupt-model-json');
    try {
      assert.throws(() => common.getChannelDefaultModels(1), SyntaxError);
      assert.deepEqual(marked, ['Saved model data could not be parsed.']);
    } finally {
      clearModelCaches();
    }
  });

  test('berry: pricing suggestions keep using the full provider catalog', async () => {
    clearModelCaches();
    const failures = [];
    const { API, requests } = transport(undefined, catalogRequests());
    const overrides = {
      'web/berry/src/utils/api.js': { API },
      'web/berry/src/utils/common.js': { showError: error => { failures.push(error); }, showSuccess: () => {} },
    };
    const PricingModal = loadModule('web/berry/src/views/Channel/component/PricingModal.js', overrides).default;
    const view = await mount(React.createElement(PricingModal, { open: true, onClose: () => {}, channelId: 'channel-7', channelName: 'Stored channel', channelType: 1 }));
    try {
      assert.deepEqual(requests.filter(request => request.path.startsWith('/api/models')).map(request => request.path), ['/api/models']);
      const input = document.querySelector('input[placeholder="e.g., gpt-4"]');
      assert.ok(input, 'Find the model suggestion input');
      await view.type(input, 'full');
      const options = [...document.querySelectorAll('[role="option"]')].map(option => option.textContent.trim());
      assert.deepEqual(options, catalog[1], 'Suggestions include catalog models that need operator pricing');
      assert.deepEqual(failures, []);
    } finally {
      await view.unmount();
    }
  });
}
