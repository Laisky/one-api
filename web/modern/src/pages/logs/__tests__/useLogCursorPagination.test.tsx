import { act, renderHook } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

import type { LogCursorFilters } from '@/lib/logCursor';
import { useLogCursorPagination } from '../useLogCursorPagination';

const FILTERS: LogCursorFilters = {
  type: '0',
  model_name: '',
  token_name: '',
  username: '',
  channel: '',
  start_timestamp: '',
  end_timestamp: '',
};

/**
 * pageBody builds a successful cursor response.
 *
 * @param items - the rows on the page.
 * @param nextCursor - the cursor for the following page, empty when last.
 * @returns the response body.
 */
function pageBody(items: unknown[], nextCursor: string) {
  return {
    success: true,
    version: 1,
    data: items,
    has_more: Boolean(nextCursor),
    next_cursor: nextCursor,
    count: { value: items.length, quality: 'exact', as_of: 1767225540, cached: false },
  };
}

/**
 * setup renders the hook with an injected transport.
 *
 * @param get - the transport stub.
 * @param onRestart - optional restart observer.
 * @returns the rendered hook.
 */
function setup(get: (url: string) => Promise<{ data: unknown }>, onRestart?: (code: string) => void) {
  return renderHook(() =>
    useLogCursorPagination<{ id: number }>({
      isAdminOrRoot: false,
      filters: FILTERS,
      pageSize: 2,
      toUnixSeconds: Number,
      get,
      onRestart,
    })
  );
}

describe('useLogCursorPagination', () => {
  it('walks forward and back using the recorded anchors', async () => {
    const urls: string[] = [];
    const get = vi.fn(async (url: string) => {
      urls.push(url);
      if (url.includes('cursor=lc1.p2')) return { data: pageBody([{ id: 3 }, { id: 4 }], '') };
      return { data: pageBody([{ id: 1 }, { id: 2 }], 'lc1.p2') };
    });

    const { result } = setup(get);

    await act(async () => {
      expect(await result.current.fetchPage('first')).toEqual([{ id: 1 }, { id: 2 }]);
    });
    expect(result.current.pageIndex).toBe(0);
    expect(result.current.hasMore).toBe(true);
    expect(result.current.hasPrevious).toBe(false);

    await act(async () => {
      expect(await result.current.fetchPage('next')).toEqual([{ id: 3 }, { id: 4 }]);
    });
    expect(result.current.pageIndex).toBe(1);
    expect(result.current.hasMore).toBe(false);
    expect(result.current.hasPrevious).toBe(true);

    await act(async () => {
      expect(await result.current.fetchPage('previous')).toEqual([{ id: 1 }, { id: 2 }]);
    });
    expect(result.current.pageIndex).toBe(0);

    // Page 1 is requested without a cursor; page 2 always carries the anchor
    // page 1 issued. No request ever carries a page number.
    expect(urls[0]).not.toContain('cursor=');
    expect(urls[1]).toContain('cursor=lc1.p2');
    expect(urls[2]).not.toContain('cursor=');
    expect(urls.every((url) => !/[?&]p=/.test(url))).toBe(true);
  });

  it('falls back instead of showing an empty list when the capability is disabled', async () => {
    const get = vi.fn(async () => ({
      data: { success: false, message: 'log cursor pagination is disabled', code: 'capability_disabled' },
    }));
    const { result } = setup(get);

    await act(async () => {
      expect(await result.current.fetchPage('first')).toBeNull();
    });
    expect(result.current.supported).toBe(false);

    // Once the server has proven it cannot answer, no further requests are made.
    await act(async () => {
      expect(await result.current.fetchPage('first')).toBeNull();
    });
    expect(get).toHaveBeenCalledTimes(1);
  });

  it('falls back when the request itself fails', async () => {
    const get = vi.fn(async () => {
      throw new Error('404');
    });
    const { result } = setup(get);

    await act(async () => {
      expect(await result.current.fetchPage('first')).toBeNull();
    });
    expect(result.current.supported).toBe(false);
  });

  it('restarts once in place when a cursor expires', async () => {
    const onRestart = vi.fn();
    let served = 0;
    const get = vi.fn(async (url: string) => {
      served++;
      if (url.includes('cursor=lc1.p2')) {
        return { data: { success: false, code: 'cursor_expired', restart_required: true } };
      }
      return { data: pageBody([{ id: 1 }, { id: 2 }], 'lc1.p2') };
    });

    const { result } = setup(get, onRestart);
    await act(async () => {
      await result.current.fetchPage('first');
    });
    served = 0;

    await act(async () => {
      // The user asked for page 2; the anchor is gone, so they get page 1 back
      // rather than an error they cannot act on.
      expect(await result.current.fetchPage('next')).toEqual([{ id: 1 }, { id: 2 }]);
    });

    expect(onRestart).toHaveBeenCalledWith('cursor_expired');
    expect(served).toBe(2);
    expect(result.current.pageIndex).toBe(0);
    expect(result.current.supported).toBe(true);
  });

  it('gives up rather than looping when the first page itself is refused twice', async () => {
    const get = vi.fn(async () => ({ data: { success: false, code: 'cursor_invalid', restart_required: true } }));
    const { result } = setup(get);

    await act(async () => {
      expect(await result.current.fetchPage('first')).toBeNull();
    });
    expect(get).toHaveBeenCalledTimes(2);
    expect(result.current.supported).toBe(false);
  });

  it('publishes the count and the budget notice', async () => {
    const get = vi.fn(async () => ({
      data: {
        success: true,
        version: 1,
        data: [{ id: 1 }],
        has_more: true,
        next_cursor: 'lc1.p2',
        count: { value: 10000, quality: 'lower_bound', as_of: 1767225540, cached: true },
        bytes_capped: true,
      },
    }));
    const { result } = setup(get);

    await act(async () => {
      await result.current.fetchPage('first');
    });

    expect(result.current.count).toEqual({ value: 10000, quality: 'lower_bound', asOf: 1767225540, cached: true });
    expect(result.current.notice).toBe('bytes_capped');
  });

  it('returns to the first page on reset', async () => {
    const get = vi.fn(async () => ({ data: pageBody([{ id: 1 }], 'lc1.next') }));
    const { result } = setup(get);

    await act(async () => {
      await result.current.fetchPage('first');
    });
    await act(async () => {
      await result.current.fetchPage('next');
    });
    expect(result.current.pageIndex).toBe(1);

    act(() => result.current.reset());
    expect(result.current.pageIndex).toBe(0);
    expect(result.current.count).toBeNull();
    expect(result.current.notice).toBeNull();
    expect(result.current.hasMore).toBe(false);
  });
});
