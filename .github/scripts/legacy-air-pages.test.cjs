const assert = require('node:assert/strict');
const { test } = require('node:test');
const { React, createTransport, defaultExport, loadModule, mount, settle, theme } = require('./legacy-react-harness.cjs');

// These tests mount real Air pages with the real Semi UI library and assert
// what a user sees or what the page sends. Transport, toast, auth/copy/timestamp
// helpers, selected quota formatting, and unrelated child modules are replaced.
// Dashboard FakeChart instances qualify data delivery, not canvas rendering.

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


/** controlledDashboard mounts the real dashboard with deferred quota requests and records chart specifications. */
async function controlledDashboard(strict = false) {
  const failures = [];
  const pending = [];
  const charts = [];
  const { API } = createTransport((request, count) => count === 1
    ? { success: true, data: [] }
    : new Promise((resolve, reject) => pending.push({ resolve, reject })));
  /** FakeChart records data delivered to the renderer without qualifying real canvas rendering. */
  function FakeChart() {
    const chart = {
      specs: [],
      /** renderAsync replaces canvas rendering with an already completed operation. */
      renderAsync: async () => {},
      /** updateSpec snapshots the supplied chart specification for ownership assertions. */
      updateSpec: spec => chart.specs.push(JSON.parse(JSON.stringify(spec))),
      /** reLayout replaces layout, which jsdom does not implement. */
      reLayout: () => {},
    };
    charts.push(chart);
    return chart;
  }
  const Detail = loadModule('web/air/src/pages/Detail/index.js', {
    'web/air/src/helpers/index.js': helpers(API, failures),
    'web/air/src/helpers/utils.js': { showError: error => failures.push(error) },
    '@visactor/vchart': defaultExport(FakeChart),
  }).default;
  const element = React.createElement(Detail);
  const view = await mount(strict ? React.createElement(React.StrictMode, null, element) : element);
  assert.equal(charts.length, 2, 'The real dashboard initializes both charts');
  assert.equal(view.container.querySelector('.semi-spin:not(.semi-spin-hidden)'), null, 'The initial request has settled');
  return { ...view, pending, charts, failures };
}

/** quotaResult returns identifiable quota data for chart ownership assertions. */
function quotaResult(model) {
  return { success: true, data: [{ model_name: model, quota: 100, count: 1, created_at: 1700000000 }] };
}

test('air: superseded dashboard ranges cannot overwrite newer chart data', async () => {
  const page = await controlledDashboard();
  try {
    await page.click(page.textElement('最近7天').closest('button'));
    await page.click(page.textElement('今天').closest('button'));
    assert.equal(page.pending.length, 2);
    page.pending[1].resolve(quotaResult('current-model'));
    await settle();
    for (const chart of page.charts) assert.match(JSON.stringify(chart.specs.at(-1)), /current-model/);
    const counts = page.charts.map(chart => chart.specs.length);
    page.pending[0].resolve(quotaResult('obsolete-model'));
    await settle();
    assert.deepEqual(page.charts.map(chart => chart.specs.length), counts, 'An obsolete range must not redraw either chart');
    assert.deepEqual(page.failures, []);
  } finally { await page.unmount(); }
});

for (const outcome of ['success', 'server failure', 'rejection']) {
  test(`air: superseded dashboard ${outcome} preserves current loading and component errors`, async () => {
    const page = await controlledDashboard();
    try {
      await page.click(page.textElement('最近7天').closest('button'));
      await page.click(page.textElement('今天').closest('button'));
      assert.equal(page.pending.length, 2);
      assert.ok(page.container.querySelector('.semi-spin:not(.semi-spin-hidden)'), 'The controlled current request displays loading');
      if (outcome === 'rejection') page.pending[0].reject(new Error('obsolete dashboard failure'));
      else page.pending[0].resolve(outcome === 'success' ? quotaResult('obsolete-model') : { success: false, message: 'obsolete dashboard failure' });
      await settle();
      page.assertRendered();
      assert.ok(page.container.querySelector('.semi-spin:not(.semi-spin-hidden)'), 'The newer request remains loading');
      assert.deepEqual(page.failures, []);
      page.pending[1].resolve(quotaResult('current-model'));
      await settle();
      assert.equal(page.container.querySelector('.semi-spin:not(.semi-spin-hidden)'), null);
    } finally { await page.unmount(); }
  });
}

test('air: unmount invalidates pending dashboard chart updates', async () => {
  const page = await controlledDashboard();
  await page.click(page.textElement('今天').closest('button'));
  await page.unmount();
  const counts = page.charts.map(chart => chart.specs.length);
  page.pending[0].resolve(quotaResult('unmounted-model'));
  await settle();
  assert.deepEqual(page.charts.map(chart => chart.specs.length), counts);
  assert.deepEqual(page.failures, []);
});


test('air: effect replay preserves the initialized dashboard request', async () => {
  const page = await controlledDashboard(true);
  try {
    page.assertRendered();
    assert.ok(page.charts.every(chart => chart.specs.length === 1), 'Both initialized charts receive initial data exactly once');
    assert.deepEqual(page.failures, []);
  } finally { await page.unmount(); }
});
