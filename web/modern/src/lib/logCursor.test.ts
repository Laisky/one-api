import { describe, expect, it } from 'vitest';

import {
  advanceLogCursorTrail,
  buildLogCursorQuery,
  currentLogCursor,
  currentRowsBefore,
  describeLogCount,
  logCursorPath,
  parseLogCursorResponse,
  rewindLogCursorTrail,
  startLogCursorTrail,
  type LogCursorFilters,
} from './logCursor';

const FILTERS: LogCursorFilters = {
  type: '0',
  model_name: '',
  token_name: '',
  username: '',
  channel: '',
  start_timestamp: '',
  end_timestamp: '',
};

const toUnixSeconds = (value: string) => Number(value);

describe('buildLogCursorQuery', () => {
  it('negotiates the capability version and never sends a page number', () => {
    const params = buildLogCursorQuery({
      filters: FILTERS,
      pageSize: 20,
      cursor: '',
      isAdminOrRoot: false,
      toUnixSeconds,
    });

    expect(params.get('v')).toBe('1');
    expect(params.get('size')).toBe('20');
    expect(params.has('p')).toBe(false);
    expect(params.has('cursor')).toBe(false);
  });

  it('withholds the site-wide filters from a non-admin request', () => {
    const filters = { ...FILTERS, username: 'someone-else', channel: '7', model_name: 'gpt-4.1' };

    const asUser = buildLogCursorQuery({ filters, pageSize: 20, cursor: '', isAdminOrRoot: false, toUnixSeconds });
    expect(asUser.has('username')).toBe(false);
    expect(asUser.has('channel')).toBe(false);
    expect(asUser.get('model_name')).toBe('gpt-4.1');

    const asAdmin = buildLogCursorQuery({ filters, pageSize: 20, cursor: '', isAdminOrRoot: true, toUnixSeconds });
    expect(asAdmin.get('username')).toBe('someone-else');
    expect(asAdmin.get('channel')).toBe('7');
  });

  it('omits the default type filter so it matches the legacy selection', () => {
    const params = buildLogCursorQuery({ filters: FILTERS, pageSize: 10, cursor: '', isAdminOrRoot: true, toUnixSeconds });
    expect(params.has('type')).toBe(false);

    const filtered = buildLogCursorQuery({
      filters: { ...FILTERS, type: '2' },
      pageSize: 10,
      cursor: '',
      isAdminOrRoot: true,
      toUnixSeconds,
    });
    expect(filtered.get('type')).toBe('2');
  });
});

describe('logCursorPath', () => {
  it('routes each scope to its own endpoint', () => {
    expect(logCursorPath(true)).toBe('/api/log/cursor');
    expect(logCursorPath(false)).toBe('/api/log/self/cursor');
  });
});

describe('parseLogCursorResponse', () => {
  const page = {
    success: true,
    version: 1,
    data: [{ id: 1 }],
    has_more: true,
    next_cursor: 'lc1.abc',
    count: { value: 42, quality: 'exact', as_of: 1767225540, cached: false },
  };

  it('reads a page', () => {
    const outcome = parseLogCursorResponse<{ id: number }>(page);
    expect(outcome).toEqual({
      kind: 'page',
      page: {
        items: [{ id: 1 }],
        hasMore: true,
        nextCursor: 'lc1.abc',
        count: { value: 42, quality: 'exact', asOf: 1767225540, cached: false },
        bytesCapped: false,
        oversizedRecord: false,
      },
    });
  });

  it('reports a restart rather than an empty page when the cursor is refused', () => {
    for (const code of ['cursor_invalid', 'cursor_expired', 'cursor_query_changed']) {
      expect(parseLogCursorResponse({ success: false, code, restart_required: true })).toEqual({ kind: 'restart', code });
    }
  });

  it('treats a server without the capability as unsupported, not as no logs', () => {
    // An old server answering /api/log/cursor with anything at all, a disabled
    // capability, and a body of the wrong shape must all lead to the fallback.
    expect(parseLogCursorResponse({ success: false, message: 'log cursor pagination is disabled', code: 'capability_disabled' })).toEqual({
      kind: 'unsupported',
    });
    expect(parseLogCursorResponse({ success: true, data: [], total: 0 })).toEqual({ kind: 'unsupported' });
    expect(parseLogCursorResponse({ success: true, version: 2, data: [] })).toEqual({ kind: 'unsupported' });
    expect(parseLogCursorResponse(null)).toEqual({ kind: 'unsupported' });
    expect(parseLogCursorResponse('<!DOCTYPE html>')).toEqual({ kind: 'unsupported' });
  });

  it('never carries a number on an unavailable count', () => {
    const outcome = parseLogCursorResponse({
      ...page,
      count: { value: 999, quality: 'unavailable', as_of: 1, cached: false },
    });
    expect(outcome.kind).toBe('page');
    if (outcome.kind !== 'page') return;
    expect(outcome.page.count.value).toBeNull();
  });

  it('falls back to an unavailable count when the field is missing or malformed', () => {
    for (const count of [undefined, null, 'many', { quality: 'wishful', value: 3 }]) {
      const outcome = parseLogCursorResponse({ ...page, count });
      expect(outcome.kind).toBe('page');
      if (outcome.kind !== 'page') return;
      expect(outcome.page.count).toEqual({ value: null, quality: 'unavailable', asOf: 0, cached: false });
    }
  });

  it('surfaces the budget flags', () => {
    const outcome = parseLogCursorResponse({ ...page, bytes_capped: true, oversized_record: true });
    expect(outcome.kind).toBe('page');
    if (outcome.kind !== 'page') return;
    expect(outcome.page.bytesCapped).toBe(true);
    expect(outcome.page.oversizedRecord).toBe(true);
  });
});

describe('the cursor trail', () => {
  it('starts at the first page with no cursor', () => {
    const trail = startLogCursorTrail();
    expect(currentLogCursor(trail)).toBe('');
    expect(currentRowsBefore(trail)).toBe(0);
    expect(trail.index).toBe(0);
  });

  it('places each page from the rows actually delivered, not from the page size', () => {
    // The middle page was cut short by the response budget. If the position
    // were derived from index * pageSize, every page after it would claim a
    // range it does not occupy.
    let trail = startLogCursorTrail();
    trail = advanceLogCursorTrail(trail, 'lc1.page2', 20);
    expect(currentRowsBefore(trail)).toBe(20);

    trail = advanceLogCursorTrail(trail, 'lc1.page3', 7);
    expect(currentRowsBefore(trail)).toBe(27);

    trail = rewindLogCursorTrail(trail);
    expect(currentRowsBefore(trail)).toBe(20);
  });

  it('returns to the same anchor when moving back and forward again', () => {
    let trail = startLogCursorTrail();
    trail = advanceLogCursorTrail(trail, 'lc1.page2', 20);
    trail = advanceLogCursorTrail(trail, 'lc1.page3', 20);
    expect(currentLogCursor(trail)).toBe('lc1.page3');

    trail = rewindLogCursorTrail(trail);
    expect(currentLogCursor(trail)).toBe('lc1.page2');

    // Forward again reuses the recorded cursor rather than a newly issued one,
    // so the user sees the page they just left.
    trail = advanceLogCursorTrail(trail, 'lc1.something-else', 20);
    expect(currentLogCursor(trail)).toBe('lc1.page3');
  });

  it('refuses to move past the last page or before the first', () => {
    let trail = startLogCursorTrail();
    expect(advanceLogCursorTrail(trail, '', 20)).toBe(trail);
    expect(rewindLogCursorTrail(trail)).toBe(trail);

    trail = advanceLogCursorTrail(trail, 'lc1.page2', 20);
    trail = rewindLogCursorTrail(trail);
    expect(trail.index).toBe(0);
    expect(rewindLogCursorTrail(trail).index).toBe(0);
  });

  it('keeps a visited trail stable and only forgets it on a restart', () => {
    let trail = startLogCursorTrail();
    trail = advanceLogCursorTrail(trail, 'lc1.page2', 20);
    trail = advanceLogCursorTrail(trail, 'lc1.page3', 20);
    trail = rewindLogCursorTrail(trail);
    trail = rewindLogCursorTrail(trail);

    // Re-walking an unchanged query reuses the recorded anchors, so the pages
    // do not shuffle under a user who is only navigating back and forth.
    trail = advanceLogCursorTrail(trail, 'lc1.a-newly-issued-anchor', 20);
    expect(currentLogCursor(trail)).toBe('lc1.page2');
    expect(trail.cursors).toEqual(['', 'lc1.page2', 'lc1.page3']);

    // A changed query or an expired cursor restarts the traversal, and the old
    // anchors — which no longer describe this query — go with it.
    const restarted = startLogCursorTrail();
    expect(restarted.cursors).toEqual(['']);
    expect(currentLogCursor(restarted)).toBe('');
  });
});

describe('describeLogCount', () => {
  it('states a total only when the count established one', () => {
    expect(describeLogCount({ value: 42, quality: 'exact', asOf: 1, cached: false }, 1, 20).key).toBe('logs.pagination.range_exact_total');
    expect(describeLogCount({ value: 10000, quality: 'lower_bound', asOf: 1, cached: false }, 1, 20).key).toBe(
      'logs.pagination.range_lower_bound_total'
    );
    expect(describeLogCount({ value: null, quality: 'unavailable', asOf: 1, cached: false }, 1, 20).key).toBe(
      'logs.pagination.range_unknown_total'
    );
  });

  it('carries the displayed range regardless of the count quality', () => {
    const label = describeLogCount({ value: null, quality: 'unavailable', asOf: 1, cached: false }, 21, 40);
    expect(label.params.from).toBe(21);
    expect(label.params.to).toBe(40);
  });
});
