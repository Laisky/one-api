import { useEffect } from 'react';
import { act, cleanup, renderHook } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { getTableSortStorageKey } from '@/lib/table-sort-preference';
import { useTableSort } from '../useTableSort';

const auth = vi.hoisted(() => ({ user: { uuid: 'account-a', role: 100 } }));
vi.mock('@/lib/stores/auth', () => ({
  useAuthStore: (selector: (state: typeof auth) => unknown) => selector(auth),
}));

/** storageKey returns the same table/principal key used by the hook under test. */
function storageKey(table = 'channels', uuid = auth.user.uuid, role = auth.user.role) {
  return getTableSortStorageKey(table, JSON.stringify([uuid, role]));
}

beforeEach(() => {
  auth.user = { uuid: 'account-a', role: 100 };
  localStorage.clear();
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  localStorage.clear();
});

describe('useTableSort', () => {
  it('restores the pair before the caller\'s first request effect', () => {
    localStorage.setItem(storageKey(), JSON.stringify({ sortBy: 'name', sortOrder: 'asc' }));
    const requests = vi.fn();
    const { result } = renderHook(() => {
      const sort = useTableSort('channels', 'id');
      useEffect(() => { requests(sort.sortBy, sort.sortOrder); }, [sort.sortBy, sort.sortOrder]);
      return sort;
    });
    expect(result.current).toMatchObject({ sortBy: 'name', sortOrder: 'asc' });
    expect(requests.mock.calls).toEqual([['name', 'asc']]);
  });

  it('does not write defaults or rewrite a restored preference on mount', () => {
    const saved = JSON.stringify({ sortBy: 'name', sortOrder: 'asc' });
    localStorage.setItem(storageKey(), saved);
    const write = vi.spyOn(Storage.prototype, 'setItem');
    renderHook(() => useTableSort('channels', 'id'));
    renderHook(() => useTableSort('users'));
    expect(write).not.toHaveBeenCalled();
  });

  it.each([false, true])('composes batched column/direction changes (direction first: %s)', (directionFirst) => {
    const { result, unmount } = renderHook(() => useTableSort('channels', 'id'));
    act(() => {
      if (directionFirst) {
        result.current.setSortOrder('asc');
        result.current.setSortBy('name');
      } else {
        result.current.setSortBy('name');
        result.current.setSortOrder('asc');
      }
    });
    expect(JSON.parse(localStorage.getItem(storageKey())!)).toEqual({ sortBy: 'name', sortOrder: 'asc' });
    unmount();
    const restored = renderHook(() => useTableSort('channels', 'id'));
    expect(restored.result.current).toMatchObject({ sortBy: 'name', sortOrder: 'asc' });
  });

  it('preserves functional setter behavior used by direction toggle buttons', () => {
    const { result } = renderHook(() => useTableSort('channels', 'id'));
    act(() => {
      result.current.setSortBy((previous) => previous === 'id' ? 'name' : 'id');
      result.current.setSortOrder((previous) => previous === 'desc' ? 'asc' : 'desc');
    });
    expect(result.current).toMatchObject({ sortBy: 'name', sortOrder: 'asc' });
  });

  it('restores each table independently and rejects callbacks from the old table', () => {
    const { result, rerender } = renderHook(({ table }) => useTableSort(table, 'id'), {
      initialProps: { table: 'channels' },
    });
    act(() => { result.current.setSortBy('name'); });
    const staleSetter = result.current.setSortBy;
    rerender({ table: 'tokens' });
    expect(result.current.sortBy).toBe('id');
    act(() => { staleSetter('priority'); });
    expect(result.current.sortBy).toBe('id');
    expect(localStorage.getItem(storageKey('tokens'))).toBeNull();
    rerender({ table: 'channels' });
    expect(result.current.sortBy).toBe('name');
  });

  it('restores account and role changes without overwriting another principal', () => {
    const { result, rerender } = renderHook(() => useTableSort('channels', 'id'));
    act(() => { result.current.setSortBy('name'); });
    auth.user = { uuid: 'account-b', role: 100 };
    localStorage.setItem(storageKey(), JSON.stringify({ sortBy: 'priority', sortOrder: 'asc' }));
    rerender();
    expect(result.current).toMatchObject({ sortBy: 'priority', sortOrder: 'asc' });
    auth.user = { uuid: 'account-b', role: 10 };
    rerender();
    expect(result.current).toMatchObject({ sortBy: 'id', sortOrder: 'desc' });
    expect(localStorage.getItem(storageKey())).toBeNull();
    auth.user = { uuid: 'account-a', role: 100 };
    rerender();
    expect(result.current).toMatchObject({ sortBy: 'name', sortOrder: 'desc' });
  });

  it('persists returning to default ordering', () => {
    const { result, unmount } = renderHook(() => useTableSort('users'));
    act(() => { result.current.setSortBy('username'); });
    act(() => { result.current.setSortBy(''); });
    unmount();
    const restored = renderHook(() => useTableSort('users'));
    expect(restored.result.current.sortBy).toBe('');
  });

  it('keeps sorting usable when reads and writes are blocked', () => {
    vi.spyOn(console, 'warn').mockImplementation(() => {});
    vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => { throw new Error('blocked'); });
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => { throw new Error('blocked'); });
    const { result } = renderHook(() => useTableSort('channels', 'id'));
    act(() => {
      result.current.setSortBy('name');
      result.current.setSortOrder('asc');
    });
    expect(result.current).toMatchObject({ sortBy: 'name', sortOrder: 'asc' });
  });

  it('rejects invalid column and direction updates', () => {
    const { result } = renderHook(() => useTableSort('channels', 'id'));
    act(() => {
      result.current.setSortBy('name&size=999');
      result.current.setSortOrder('invalid' as 'asc');
    });
    expect(result.current).toMatchObject({ sortBy: 'id', sortOrder: 'desc' });
    expect(localStorage.getItem(storageKey())).toBeNull();
  });

  it.each([
    ['channels', 'id'], ['users', ''], ['tokens', 'id'],
    ['redemptions', ''], ['mcp', 'id'], ['logs', 'created_at'],
  ])('%s ignores a removed saved column before the initial request', (table, defaultBy) => {
    localStorage.setItem(storageKey(table), JSON.stringify({ sortBy: 'removed_column', sortOrder: 'asc' }));
    const requests = vi.fn();
    const { result } = renderHook(() => {
      const sort = useTableSort(table, defaultBy);
      useEffect(() => { requests(sort.sortBy, sort.sortOrder); }, [sort.sortBy, sort.sortOrder]);
      return sort;
    });
    expect(result.current).toMatchObject({ sortBy: defaultBy, sortOrder: 'desc' });
    expect(requests.mock.calls).toEqual([[defaultBy, 'desc']]);
  });

});
