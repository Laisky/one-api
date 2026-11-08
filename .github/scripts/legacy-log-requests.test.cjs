const assert = require('node:assert/strict');
const { test } = require('node:test');
const { React, createTransport, defaultExport, loadModule, mount, theme } = require('./legacy-react-harness.cjs');

// This suite mounts the real Air and Berry log pages with their real UI
// libraries and records the requests they send. Only the transport, toast and
// unrelated modal/tooltip modules are replaced, so the asserted URLs come from
// the pages' actual event handlers and request builders.

/** logRows returns a full page of rows so pagination offers a following page. */
function logRows(prefix) {
  return Array.from({ length: 10 }, (_, index) => ({
    id: index + 1, uuid: `${prefix}-${index}`, type: 2, created_at: 1700000000 + index, model_name: 'model',
    token_name: 'token', prompt_tokens: index, completion_tokens: index, quota: index, elapsed_time: index, content: 'detail',
  }));
}

/** mountPage renders the real theme log page and returns helpers for user interactions and request inspection. */
async function mountPage({ admin }) {
  const { API, requests } = createTransport((request, count) => ({ success: true, message: '', data: logRows(`row-${count}`), total: 25 }));
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
  const view = await mount(React.createElement(Page));
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
