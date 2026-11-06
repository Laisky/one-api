import { api } from '@/lib/api';
import { act, renderHook, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { appendAssignedGroups, normalizeGroupCatalog, useGroupCatalog } from '../useGroupCatalog';

vi.mock('@/lib/api', () => ({
  api: { get: vi.fn() },
}));

const mockApiGet = vi.mocked(api.get);

describe('normalizeGroupCatalog', () => {
  it('puts default first, sorts the rest and removes exact duplicates', () => {
    expect(normalizeGroupCatalog(['vip', 'default', 'svip', 'vip', 'gold'])).toEqual(['default', 'gold', 'svip', 'vip']);
  });

  it('adds default even when the catalog omits it', () => {
    expect(normalizeGroupCatalog(['vip'])).toEqual(['default', 'vip']);
    expect(normalizeGroupCatalog([])).toEqual(['default']);
  });

  it('keeps names exactly as returned and drops names that cannot be assigned', () => {
    expect(normalizeGroupCatalog([' vip', 'my group', '', 'team,a', 'a'.repeat(33), 7, null, 'team_a', 'Gold'])).toEqual([
      'default',
      'Gold',
      'team_a',
    ]);
  });

  it('keeps names that differ only by case as distinct identifiers', () => {
    expect(normalizeGroupCatalog(['gold', 'Gold'])).toEqual(['default', 'gold', 'Gold']);
  });
});

describe('appendAssignedGroups', () => {
  it('appends assigned groups missing from the catalog once, after the catalog', () => {
    expect(appendAssignedGroups(['default', 'vip'], ['legacy group', 'vip', undefined, '', 'legacy group', 'partners'])).toEqual([
      'default',
      'vip',
      'legacy group',
      'partners',
    ]);
  });

  it('lists assigned groups even when the catalog is empty', () => {
    expect(appendAssignedGroups([], ['default'])).toEqual(['default']);
  });
});

describe('useGroupCatalog', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.spyOn(console, 'error').mockImplementation(() => {});
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('starts loading and then exposes the normalized catalog', async () => {
    mockApiGet.mockResolvedValue({ data: { success: true, data: ['vip', 'default'] } });
    const { result } = renderHook(() => useGroupCatalog());

    expect(result.current).toEqual(expect.objectContaining({ groups: [], loading: true, error: false }));
    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.groups).toEqual(['default', 'vip']);
    expect(result.current.error).toBe(false);
    expect(mockApiGet).toHaveBeenCalledTimes(1);
    expect(mockApiGet).toHaveBeenCalledWith('/api/group/');
  });

  it('reports a network error with an empty list instead of a default fallback', async () => {
    mockApiGet.mockRejectedValue(new Error('network down'));
    const { result } = renderHook(() => useGroupCatalog());

    await waitFor(() => expect(result.current.error).toBe(true));
    expect(result.current.groups).toEqual([]);
    expect(result.current.loading).toBe(false);
  });

  it('reports success:false and malformed payloads as errors', async () => {
    mockApiGet.mockResolvedValueOnce({ data: { success: false, message: 'forbidden' } });
    const first = renderHook(() => useGroupCatalog());
    await waitFor(() => expect(first.result.current.error).toBe(true));
    expect(first.result.current.groups).toEqual([]);

    // success:false is an error even when the body also carries a list.
    mockApiGet.mockResolvedValueOnce({ data: { success: false, message: 'forbidden', data: ['vip'] } });
    const withData = renderHook(() => useGroupCatalog());
    await waitFor(() => expect(withData.result.current.error).toBe(true));
    expect(withData.result.current.groups).toEqual([]);

    mockApiGet.mockResolvedValueOnce({ data: { success: true, data: null } });
    const second = renderHook(() => useGroupCatalog());
    await waitFor(() => expect(second.result.current.error).toBe(true));
    expect(second.result.current.groups).toEqual([]);
  });

  it('logs failures as plain strings', async () => {
    mockApiGet.mockRejectedValue(Object.assign(new Error('Request failed'), { response: { status: 503 } }));
    renderHook(() => useGroupCatalog());

    await waitFor(() => expect(console.error).toHaveBeenCalledTimes(1));
    expect(vi.mocked(console.error).mock.calls[0]).toEqual(['Failed to load the group catalog: HTTP 503']);
  });

  it('retries after a failure and recovers', async () => {
    mockApiGet.mockRejectedValueOnce(new Error('network down'));
    mockApiGet.mockResolvedValueOnce({ data: { success: true, data: ['svip', 'vip'] } });
    const { result } = renderHook(() => useGroupCatalog());
    await waitFor(() => expect(result.current.error).toBe(true));

    act(() => result.current.retry());
    expect(result.current.loading).toBe(true);
    expect(result.current.error).toBe(false);

    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.error).toBe(false);
    expect(result.current.groups).toEqual(['default', 'svip', 'vip']);
    expect(mockApiGet).toHaveBeenCalledTimes(2);
  });

  it('ignores a response that arrives after unmount', async () => {
    let resolve: (value: unknown) => void = () => {};
    mockApiGet.mockReturnValue(new Promise((settle) => (resolve = settle)) as ReturnType<typeof api.get>);
    const { result, unmount } = renderHook(() => useGroupCatalog());
    unmount();

    await act(async () => resolve({ data: { success: true, data: ['vip'] } }));
    expect(result.current.groups).toEqual([]);
    expect(result.current.loading).toBe(true);
  });
});
