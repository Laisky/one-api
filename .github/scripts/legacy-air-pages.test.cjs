const assert = require('node:assert/strict');
const { test } = require('node:test');
const { React, createTransport, defaultExport, loadModule, mount, theme } = require('./legacy-react-harness.cjs');

// These tests mount real Air pages with the real Semi UI library and assert
// what a user sees or what the page sends. Only transport, toast and unrelated
// child modules are replaced.

assert.equal(theme, 'air', 'This suite covers Air pages');

/** formatTimestamp mirrors Air's local "YYYY-MM-DD HH:MM:SS" timestamp text for the supplied Unix seconds. */
function formatTimestamp(value) {
  const date = new Date(value * 1000);
  const pad = number => String(number).padStart(2, '0');
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${pad(date.getHours())}:${pad(date.getMinutes())}:${pad(date.getSeconds())}`;
}

/** helpers returns the Air helper module double backed by the supplied transport and failure log. */
function helpers(API, failures, admin = false) {
  return {
    API, isAdmin: () => admin, copy: async () => true, showSuccess: () => {},
    showError: error => { failures.push(error); },
    timestamp2string: formatTimestamp, timestamp2string1: value => formatTimestamp(value).slice(5, 13),
  };
}

test('air: the models page renders each channel panel and its model table', async () => {
  const failures = [];
  const { API } = createTransport(() => ({
    success: true,
    data: {
      OpenAI: { models: { 'gpt-4o': { input_price: 2.5, output_price: 10, max_tokens: 128000 } } },
      Anthropic: { models: { 'claude-opus': { input_price: 0, output_price: 0, max_tokens: 0 } } },
    },
  }));
  const Models = loadModule('web/air/src/pages/Models/index.js', {
    'web/air/src/helpers/index.js': helpers(API, failures),
    'web/air/src/helpers/utils.js': { showError: error => { failures.push(error); } },
  }).default;
  const view = await mount(React.createElement(Models));
  try {
    assert.ok([...view.container.querySelectorAll('h6')].some(heading => heading.textContent === 'Found 2 models in 2 channels'));
    assert.ok(view.textElement('gpt-4o'), 'An expanded channel shows its models');
    assert.ok(view.textElement('claude-opus'));
    assert.ok(view.textElement('Free'), 'Pricing remains formatted');

    const header = view.textElement('OpenAI').closest('.semi-collapse-header');
    assert.equal(header.getAttribute('aria-expanded'), 'true', 'Each channel panel starts expanded');
    await view.click(header);
    assert.equal(header.getAttribute('aria-expanded'), 'false', 'A user can collapse a channel panel');
    assert.deepEqual(failures, []);
  } finally {
    await view.unmount();
  }
});

test('air: the LobeChat token link carries the selected token key', async () => {
  const failures = [];
  const opened = [];
  const { API } = createTransport(request => request.path === '/api/token/'
    ? { success: true, data: [{ id: 7, uuid: 'token-7', name: 'prod', key: 'secretkey7', status: 1, remain_quota: 1, used_quota: 0, unlimited_quota: true, created_time: 1700000000, accessed_time: 1700000000, expired_time: -1 }] }
    : { success: true, data: [] });
  localStorage.setItem('chat_link', 'https://chat.example.test');
  const originalOpen = window.open;
  window.open = url => { opened.push(url); };
  const Tokens = loadModule('web/air/src/components/TokensTable.js', {
    'web/air/src/helpers/index.js': helpers(API, failures),
    'web/air/src/helpers/utils.js': { showError: error => { failures.push(error); } },
    'web/air/src/helpers/render.js': { renderQuota: String },
    'web/air/src/pages/Token/EditToken.js': defaultExport(() => null),
    'web/air/src/components/ResourceRefTooltip.js': defaultExport(({ children }) => children ?? null),
  }).default;
  const view = await mount(React.createElement(Tokens));
  try {
    await view.click(view.container.querySelector('.semi-button-split button.semi-button-last'));
    const item = [...document.querySelectorAll('.semi-dropdown-item')].find(entry => entry.textContent.trim() === 'LobeChat');
    assert.ok(item, 'Open the token chat menu');
    await view.click(item);
    assert.equal(opened.length, 1);
    const settings = JSON.parse(decodeURIComponent(opened[0].split('?settings=')[1]));
    assert.equal(settings.keyVaults.openai.apiKey, 'sk-secretkey7', 'LobeChat receives the row token, never sk-undefined');
    assert.deepEqual(failures, []);
  } finally {
    window.open = originalOpen;
    localStorage.removeItem('chat_link');
    await view.unmount();
  }
});

for (const [label, days] of [['今天', 0], ['最近7天', 6], ['最近30天', 29]]) {
  test(`air: the ${label} dashboard preset requests the range it displays`, async () => {
    const failures = [];
    const { API, requests } = createTransport(() => ({ success: true, data: [] }));
    /** FakeChart accepts every chart call because chart rendering is outside this request contract. */
    function FakeChart() { return new Proxy({}, { get: () => () => {} }); }
    const Detail = loadModule('web/air/src/pages/Detail/index.js', {
      'web/air/src/helpers/index.js': helpers(API, failures),
      'web/air/src/helpers/utils.js': { showError: error => { failures.push(error); } },
      '@visactor/vchart': defaultExport(FakeChart),
    }).default;
    /** presetRange returns the Unix-second range the preset represents at the supplied moment. */
    const presetRange = moment => {
      const start = new Date(moment);
      start.setDate(start.getDate() - days);
      start.setHours(0, 0, 0, 0);
      const end = new Date(moment);
      end.setHours(23, 59, 59, 999);
      return [start, end].map(date => String(Date.parse(formatTimestamp(date.getTime() / 1000)) / 1000));
    };
    const view = await mount(React.createElement(Detail));
    try {
      assert.equal(requests.length, 1, 'The dashboard loads once on mount');
      const before = presetRange(Date.now());
      await view.click(view.textElement(label).closest('button'));
      const after = presetRange(Date.now());
      assert.equal(requests.length, 2, 'A preset loads once');
      const sent = [requests[1].params.start_timestamp, requests[1].params.end_timestamp];
      assert.ok([before.join(), after.join()].includes(sent.join()), `The preset request must use the selected range, got ${sent.join()} for ${before.join()}`);
      assert.deepEqual(failures, []);
    } finally {
      await view.unmount();
    }
  });
}
