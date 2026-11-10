const assert = require('node:assert/strict');
const { test } = require('node:test');
const { React, createTransport, defaultExport, loadModule, mount, settle, theme } = require('./legacy-react-harness.cjs');

// This suite mounts the real Air and Berry log pages with their real UI
// libraries and records the requests they send. Transport, toast, authentication,
// selected formatting/copy helpers and unrelated modal/tooltip modules are
// replaced. The asserted URLs come from real event handlers and request builders;
// error counts qualify component notifications, not Axios interceptor behavior.

/** logRows returns a full page of rows so pagination offers a following page. */
function logRows(prefix) {
  return Array.from({ length: 10 }, (_, index) => ({
    id: index + 1, uuid: `${prefix}-${index}`, type: 2, created_at: 1700000000 + index, model_name: prefix,
    token_name: 'token', prompt_tokens: index, completion_tokens: index, quota: index, elapsed_time: index, content: 'detail',
  }));
}

/** mountPage renders the real theme log page and returns helpers for user interactions and request inspection. */
async function mountPage({ admin, respond, strict = false }) {
  localStorage.removeItem('page-size');
  const { API, requests } = createTransport(respond || ((request, count) => ({ success: true, message: '', data: logRows(`row-${count}`), total: 25 })));
  const failures = [];
  /** recordFailure keeps UI-reported failures for the absence assertion. */
  const recordFailure = error => { failures.push(error); };
  /** formatTimestamp renders the legacy pages' local timestamp text. */
  const formatTimestamp = value => new Date(value * 1000).toISOString().replace('T', ' ').slice(0, 19);
  const passthrough = defaultExport(({ children }) => children ?? null);
  const Page = theme === 'air'
    ? loadModule('web/air/src/components/LogsTable.js', {
      'web/air/src/helpers/index.js': { API, copy: async () => true, isAdmin: () => admin, showError: recordFailure, showSuccess: () => {}, timestamp2string: formatTimestamp },
      'web/air/src/helpers/utils.js': { showError: recordFailure },
      'web/air/src/helpers/render.js': { renderNumber: String, renderQuota: String, stringToColor: () => 'blue' },
      'web/air/src/components/TracingModal.js': defaultExport(() => null),
      'web/air/src/components/ResourceRefTooltip.js': passthrough,
    }).default
    : loadModule('web/berry/src/views/Log/index.js', {
      'web/berry/src/utils/api.js': { API },
      'web/berry/src/utils/common.js': { showError: recordFailure, renderQuota: String, isAdmin: () => admin, timestamp2string: formatTimestamp, copy: async () => true },
      'web/berry/src/views/Log/component/TracingModal.js': defaultExport(() => null),
      'web/berry/src/ui-component/ResourceRefTooltip.js': passthrough,
    }).default;
  const element = React.createElement(Page);
  const view = await mount(strict ? React.createElement(React.StrictMode, null, element) : element);
  const { container, textElement, click, type } = view;
  return {
    ...view, requests, failures,
    /** sortBy clicks the real sortable header that displays the supplied label. */
    sortBy: label => click(textElement(label)),
    /** headerText returns the visible header text for the supplied label. */
    headerText: label => textElement(label).closest('th').textContent,
    /** button returns the real button that displays the supplied label. */
    button: label => textElement(label).closest('button'),
    /** search submits the supplied keyword through the page's real keyword field and search button. */
    search: async keyword => {
      await type(container.querySelector(theme === 'air' ? 'input[name="search_keyword"]' : 'input[name="keyword"]'), keyword);
      await click(textElement('搜索').closest('button'));
    },
    /** choose opens the real Semi Select matching the supplied selector and clicks the option with the supplied text. */
    choose: async (selector, text) => {
      await click(container.querySelector(selector));
      const option = [...document.querySelectorAll('.semi-select-option')].find(item => item.textContent.trim() === text);
      assert.ok(option, `Find select option ${text}`);
      await click(option);
    },
    /** goToPage clicks the real Semi pagination item for the supplied page number. */
    goToPage: async number => {
      const item = [...container.querySelectorAll('.semi-page-item')].find(entry => entry.textContent.trim() === String(number));
      assert.ok(item, `Find pagination page ${number}`);
      await click(item);
    },
    /** last returns the most recent recorded request. */
    last: () => requests.at(-1),
  };
}

/** sortParams returns only the sort parameters of a recorded request so assertions show the complete sort contract. */
function sortParams(request) {
  const names = request.path.endsWith('/search') ? ['sort', 'order'] : ['sort_by', 'sort_order'];
  return Object.fromEntries(names.filter(name => name in request.params).map(name => [name, request.params[name]]));
}

const labels = theme === 'air'
  ? { prompt: '提示', quota: '花费', latency: 'Latency' }
  : { prompt: '提示', quota: '费用', latency: 'Latency' };

for (const admin of [false, true]) {
  const role = admin ? 'admin' : 'user';
  const listPath = admin ? '/api/log/' : '/api/log/self';
  const searchPath = admin ? '/api/log/search' : '/api/log/self/search';

  test(`${theme}: ${role} log sorting requests the clicked column and direction`, async () => {
    const page = await mountPage({ admin });
    try {
      assert.equal(page.requests.length, 1, 'The page loads once on mount');
      assert.equal(page.last().path, listPath);
      assert.deepEqual(sortParams(page.last()), {}, 'The initial listing has no explicit sort');

      await page.sortBy(labels.latency);
      assert.equal(page.last().path, listPath);
      assert.deepEqual(sortParams(page.last()), { sort_by: 'elapsed_time', sort_order: 'desc' }, 'The first sort requests the clicked column descending');
      assert.equal(page.last().params.p, '0', 'Sorting restarts from the first page');
      assert.match(page.headerText(labels.latency), /↓/);

      await page.sortBy(labels.latency);
      assert.deepEqual(sortParams(page.last()), { sort_by: 'elapsed_time', sort_order: 'asc' }, 'A second click requests the toggled direction');
      assert.match(page.headerText(labels.latency), /↑/);

      await page.sortBy(labels.quota);
      assert.deepEqual(sortParams(page.last()), { sort_by: 'quota', sort_order: 'desc' }, 'Switching columns requests the new column descending');
      assert.match(page.headerText(labels.quota), /↓/);
      assert.doesNotMatch(page.headerText(labels.latency), /[↑↓]/);

      assert.equal(page.requests.length, 4, 'Each click sends exactly one request');
      for (const request of page.requests) {
        for (const name of ['start_timestamp', 'end_timestamp']) {
          assert.match(request.params[name], /^\d+$/, `${name} must be whole Unix seconds the server can parse`);
        }
      }
      assert.deepEqual(page.failures, []);
    } finally {
      await page.unmount();
    }
  });

  test(`${theme}: ${role} keyword search sorting requests the clicked column and direction`, async () => {
    const page = await mountPage({ admin });
    try {
      await page.search('needle');
      assert.equal(page.last().path, searchPath);
      assert.equal(page.last().params.keyword, 'needle');
      assert.deepEqual(sortParams(page.last()), {}, 'An unsorted search sends no sort');

      await page.sortBy(labels.prompt);
      assert.equal(page.last().path, searchPath, 'Sorting keeps the keyword search endpoint');
      assert.equal(page.last().params.keyword, 'needle');
      assert.deepEqual(sortParams(page.last()), { sort: 'prompt_tokens', order: 'desc' });

      await page.sortBy(labels.prompt);
      assert.deepEqual(sortParams(page.last()), { sort: 'prompt_tokens', order: 'asc' });
      assert.match(page.headerText(labels.prompt), /↑/);

      await page.sortBy(labels.latency);
      assert.deepEqual(sortParams(page.last()), { sort: 'elapsed_time', order: 'desc' });
      assert.deepEqual(page.failures, []);
    } finally {
      await page.unmount();
    }
  });
}

if (theme === 'air') {
  test('air: the admin log filters render the username search field', async () => {
    const page = await mountPage({ admin: true });
    try {
      assert.ok(page.textElement('用户名称'), 'The username filter keeps its form label');
      assert.ok(page.container.querySelector('input[placeholder="搜索用户名称"]'), 'The username search field is rendered');
    } finally {
      await page.unmount();
    }
  });

  test('air: the selected log type is retained by paging, querying and page-size changes', async () => {
    const page = await mountPage({ admin: false });
    try {
      await page.choose('.logs-select .semi-select-selection', '消费');
      assert.equal(page.last().params.type, '2', 'Selecting a log type reloads that type');

      await page.goToPage(2);
      assert.equal(page.last().params.p, '1');
      assert.equal(page.last().params.type, '2', 'Loading the next page keeps the selected type');

      await page.click(page.button('查询'));
      assert.equal(page.last().params.p, '0');
      assert.equal(page.last().params.type, '2', 'Querying keeps the selected type');

      await page.sortBy(labels.latency);
      assert.equal(page.last().params.type, '2', 'Sorting keeps the selected type');

      await page.choose('.semi-page-switch .semi-select-selection', '每页条数：20');
      assert.equal(page.last().params.page_size, '20');
      assert.equal(page.last().params.type, '2', 'Changing the page size keeps the selected type');
      assert.deepEqual(sortParams(page.last()), { sort_by: 'elapsed_time', sort_order: 'desc' }, 'Changing the page size keeps the sort');
      assert.deepEqual(page.failures, []);
    } finally {
      await page.unmount();
    }
  });
}

if (theme === 'berry') {
  test('berry: refreshing clears the keyword with one request and keeps the visible sort', async () => {
    const page = await mountPage({ admin: false });
    try {
      await page.search('needle');
      await page.sortBy(labels.latency);
      assert.deepEqual(sortParams(page.last()), { sort: 'elapsed_time', order: 'desc' });

      const before = page.requests.length;
      await page.click(page.button('刷新/清除搜索条件'));
      const sent = page.requests.slice(before);
      assert.equal(sent.length, 1, 'Refreshing sends exactly one request');
      assert.equal(sent[0].path, '/api/log/self', 'Refreshing leaves keyword search');
      assert.equal(sent[0].params.keyword, undefined, 'The cleared keyword is not sent');
      assert.equal(sent[0].params.p, '0');
      assert.deepEqual(sortParams(sent[0]), { sort_by: 'elapsed_time', sort_order: 'desc' }, 'The visible sort still applies');
      assert.match(page.headerText(labels.latency), /↓/);
      assert.equal(page.container.querySelector('input[name="keyword"]').value, '', 'The keyword field is cleared');
      assert.deepEqual(page.failures, []);
    } finally {
      await page.unmount();
    }
  });
}


/** controlledPage mounts a log page whose post-mount requests settle only when the test chooses. */
async function controlledPage(admin) {
  const pending = [];
  const page = await mountPage({ admin, respond: (request, count) => count === 1
    ? { success: true, data: logRows('initial'), total: 25 }
    : new Promise((resolve, reject) => pending.push({ request, resolve, reject })) });
  return {
    ...page, pending,
    /** refresh submits the structured listing after clearing the search keyword. */
    refresh: async () => {
      if (theme === 'air') {
        await page.type(page.container.querySelector('input[name="search_keyword"]'), '');
        await page.click(page.button('查询'));
      } else await page.click(page.button('刷新/清除搜索条件'));
    },
    /** busy returns whether the currently requested operation still displays its loading indicator. */
    busy: () => Boolean(page.container.querySelector(theme === 'air' ? '.semi-button-loading' : '[role="progressbar"]')),
  };
}

/** finish resolves a controlled request with identifiable rows and flushes its UI updates. */
async function finish(request, label, extra = {}) {
  request.resolve({ success: true, data: logRows(label), total: 25, ...extra });
  await settle();
}

for (const admin of [false, true]) {
  for (const latest of ['search', 'listing']) {
    test(`${theme}: ${admin ? 'admin' : 'user'} superseded ${latest === 'search' ? 'listing' : 'search'} cannot replace newer ${latest} rows or mode`, async () => {
      const page = await controlledPage(admin);
      try {
        if (latest === 'search') {
          await page.sortBy(labels.latency);
          await page.search('current');
        } else {
          await page.search('obsolete');
          await page.refresh();
        }
        assert.equal(page.pending.length, 2, 'Two actual user actions issue two controlled requests');
        assert.equal(page.busy(), true, 'The controlled request shows loading');
        await finish(page.pending[1], 'current-result');
        page.assertRendered();
        assert.match(page.container.textContent, /current-result/);
        await finish(page.pending[0], 'obsolete-result', { total: 1 });
        assert.match(page.container.textContent, /current-result/, 'The latest result remains visible');
        assert.doesNotMatch(page.container.textContent, /obsolete-result/);
        if (theme === 'air') {
          const total = latest === 'search' ? 25 : 20;
          assert.match(page.container.textContent, new RegExp(`共\\s+${total}\\s+条`), 'The latest result owns the pager total');
        }
        assert.equal(page.busy(), false, 'The current request owns busy cleanup');
        await page.sortBy(labels.quota);
        assert.equal(page.pending.length, 3, 'An obsolete sort does not leave sorting blocked');
        assert.equal(page.last().path.endsWith('/search'), latest === 'search', 'The current endpoint mode is retained');
        await finish(page.pending[2], 'sorted-result');
        assert.deepEqual(page.failures, []);
      } finally { await page.unmount(); }
    });
  }
}

for (const outcome of ['success', 'server failure', 'rejection']) {
  test(`${theme}: superseded ${outcome} cannot clear newer loading or report a component error`, async () => {
    const page = await controlledPage(false);
    try {
      await page.search('obsolete');
      await page.refresh();
      assert.equal(page.pending.length, 2);
      if (outcome === 'rejection') {
        page.pending[0].reject(new Error('obsolete transport failure'));
        await settle();
      } else await finish(page.pending[0], 'obsolete-result', outcome === 'server failure' ? { success: false, message: 'obsolete server failure' } : {});
      assert.equal(page.busy(), true, 'An older completion must not clear current busy state');
      assert.deepEqual(page.failures, [], 'Obsolete component-level failures are ignored');
      await finish(page.pending[1], 'current-result');
      assert.equal(page.busy(), false);
      assert.match(page.container.textContent, /current-result/);
    } finally { await page.unmount(); }
  });
}

for (const outcome of ['server failure', 'rejection']) {
  test(`${theme}: current ${outcome} still reports a component error and clears loading`, async () => {
    const page = await controlledPage(false);
    try {
      await page.search('current');
      if (outcome === 'rejection') {
        page.pending[0].reject(new Error('current transport failure'));
        await settle();
      } else await finish(page.pending[0], '', { success: false, message: 'current server failure' });
      assert.equal(page.busy(), false);
      assert.equal(page.failures.length, 1, 'A current failure is still surfaced');
      assert.match(String(page.failures[0]), /current .* failure/);
      assert.match(page.container.textContent, /initial/, 'A failed request preserves previously loaded rows');
    } finally { await page.unmount(); }
  });
}

test(`${theme}: a page request superseded by refresh cannot append rows or move the pager`, async () => {
  const page = await controlledPage(false);
  try {
    if (theme === 'air') await page.goToPage(2);
    else await page.click(page.container.querySelector('button[aria-label="Go to next page"]'));
    await page.refresh();
    assert.equal(page.pending.length, 2);
    await finish(page.pending[1], 'current-result');
    await finish(page.pending[0], 'obsolete-page');
    assert.match(page.container.textContent, /current-result/, 'The refreshed first page remains selected');
    assert.doesNotMatch(page.container.textContent, /obsolete-page/);
    assert.deepEqual(page.failures, []);
  } finally { await page.unmount(); }
});

test(`${theme}: unmount invalidates pending component error notifications`, async () => {
  const page = await controlledPage(false);
  await page.search('pending');
  await page.unmount();
  await finish(page.pending[0], '', { success: false, message: 'unmounted failure' });
  assert.deepEqual(page.failures, []);
});


for (const outcome of ['success', 'server failure', 'rejection']) {
  test(`${theme}: superseded same-kind ${outcome} cannot clear current loading`, async () => {
    const page = await controlledPage(false);
    try {
      assert.equal(page.busy(), false, 'The initial request has settled');
      if (theme === 'air') {
        await page.sortBy(labels.latency);
        await page.choose('.logs-select .semi-select-selection', '消费');
      } else {
        await page.refresh();
        await page.refresh();
      }
      assert.equal(page.pending.length, 2);
      assert.equal(page.busy(), true);
      if (outcome === 'rejection') page.pending[0].reject(new Error('obsolete same-kind failure'));
      else page.pending[0].resolve(outcome === 'success'
        ? { success: true, data: logRows('obsolete-result'), total: 25 }
        : { success: false, message: 'obsolete same-kind failure' });
      await settle();
      page.assertRendered();
      assert.equal(page.busy(), true, 'Only the current request may clear loading');
      assert.deepEqual(page.failures, []);
      await finish(page.pending[1], 'current-result');
      page.assertRendered();
      assert.equal(page.busy(), false);
      assert.match(page.container.textContent, /current-result/);
    } finally { await page.unmount(); }
  });
}


test(`${theme}: effect replay still commits the current initial listing`, async () => {
  const page = await mountPage({ admin: false, strict: true });
  try {
    page.assertRendered();
    assert.match(page.container.textContent, /row-/);
    assert.deepEqual(page.failures, []);
  } finally { await page.unmount(); }
});

if (theme === 'berry') {
  test('berry: cached-page navigation supersedes an uncached page request', async () => {
    const page = await controlledPage(false);
    try {
      await page.click(page.container.querySelector('button[aria-label="Go to next page"]'));
      await finish(page.pending[0], 'second-page');
      assert.match(page.container.textContent, /second-page/);
      await page.click(page.container.querySelector('button[aria-label="Go to next page"]'));
      assert.equal(page.pending.length, 2);
      assert.equal(page.busy(), true);
      await page.click(page.container.querySelector('button[aria-label="Go to previous page"]'));
      assert.match(page.container.textContent, /initial/, 'Cached navigation displays the selected page immediately');
      await finish(page.pending[1], 'obsolete-third-page');
      page.assertRendered();
      assert.match(page.container.textContent, /initial/, 'An older request cannot move away from the cached selection');
      assert.doesNotMatch(page.container.textContent, /obsolete-third-page/);
      assert.equal(page.busy(), false);
      assert.deepEqual(page.failures, []);
    } finally { await page.unmount(); }
  });
}

{
  test(`${theme}: old cached pages cannot cancel a pending new sort`, async () => {
    const page = await controlledPage(false);
    try {
      if (theme === 'air') await page.goToPage(2);
      else await page.click(page.container.querySelector('button[aria-label="Go to next page"]'));
      await finish(page.pending[0], 'old-second-page');
      await page.sortBy(labels.quota);
      assert.equal(page.pending.length, 2);
      if (theme === 'air') await page.goToPage(2);
      else await page.click(page.container.querySelector('button[aria-label="Go to next page"]'));
      await finish(page.pending[1], 'new-sorted-result');
      page.assertRendered();
      assert.match(page.container.textContent, /new-sorted-result/, 'Old-query cache cannot supersede a new query');
      assert.doesNotMatch(page.container.textContent, /old-second-page/);
      assert.equal(page.busy(), false);
      assert.deepEqual(page.failures, []);
    } finally { await page.unmount(); }
  });
}
