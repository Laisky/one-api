import { StrictMode } from 'react';
import { act, cleanup, renderHook, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('@/lib/api', () => ({ api: { get: vi.fn() } }));

import { api } from '@/lib/api';
import { useAuthStore } from '@/lib/stores/auth';
import { invalidateSystemStatusCache } from '@/lib/utils';
import { useSystemStatus } from '../useSystemStatus';

const mockedGet = vi.mocked(api.get);

/** response builds a successful /api/status response with the given name. */
const response = (name: string) => ({ data: { success: true, data: { system_name: name } } });

/** deferred creates a controllable API response promise for race regressions. */
function deferred() {
  let resolve!: (value: ReturnType<typeof response>) => void;
  const promise = new Promise<ReturnType<typeof response>>((res) => { resolve = res; });
  return { promise, resolve };
}

describe('useSystemStatus lifecycle', () => {
  beforeEach(() => {
    invalidateSystemStatusCache();
    localStorage.clear();
    mockedGet.mockReset();
  });
  afterEach(cleanup);

  it('does not let an older load replace the latest forced refresh', async () => {
    const old = deferred();
    const latest = deferred();
    mockedGet.mockReturnValueOnce(old.promise).mockReturnValueOnce(latest.promise);
    const { result } = renderHook(useSystemStatus);
    let refresh!: ReturnType<typeof result.current.refreshSystemStatus>;
    act(() => { refresh = result.current.refreshSystemStatus(); });
    await act(async () => { old.resolve(response('Old')); });
    expect(result.current.isSystemStatusLoading).toBe(true);
    expect(result.current.systemStatus).toEqual({});
    await act(async () => { latest.resolve(response('Latest')); await refresh; });
    expect(result.current.systemStatus.system_name).toBe('Latest');
    expect(result.current.isSystemStatusLoading).toBe(false);
  });

  it('keeps the refreshed React state when the original response finishes last', async () => {
    const old = deferred();
    mockedGet.mockReturnValueOnce(old.promise).mockResolvedValueOnce(response('Newest'));
    const { result } = renderHook(useSystemStatus);
    await act(async () => { await result.current.refreshSystemStatus(); });
    expect(result.current.systemStatus.system_name).toBe('Newest');
    await act(async () => { old.resolve(response('Old')); });
    expect(result.current.systemStatus.system_name).toBe('Newest');
    expect(result.current.isSystemStatusLoading).toBe(false);
  });

  it('clears mounted consumers on logout and discards delayed responses', async () => {
    localStorage.setItem('status', JSON.stringify({ system_name: 'Private' }));
    const pending = deferred();
    mockedGet.mockReturnValueOnce(pending.promise);
    const { result } = renderHook(useSystemStatus);
    expect(result.current.systemStatus.system_name).toBe('Private');
    act(() => useAuthStore.getState().logout());
    expect(result.current.systemStatus).toEqual({});
    await act(async () => { pending.resolve(response('Stale')); });
    expect(result.current.systemStatus).toEqual({});
    expect(result.current.isSystemStatusLoading).toBe(false);
    expect(localStorage.getItem('status')).toBeNull();
  });

  it('survives StrictMode cleanup and rearming with a shared pending request', async () => {
    const pending = deferred();
    mockedGet.mockReturnValueOnce(pending.promise);
    const { result, unmount } = renderHook(useSystemStatus, { wrapper: StrictMode });
    await act(async () => { pending.resolve(response('Strict')); });
    await waitFor(() => expect(result.current.systemStatus.system_name).toBe('Strict'));
    expect(mockedGet).toHaveBeenCalledTimes(1);
    unmount();
    act(() => useAuthStore.getState().logout());
    expect(localStorage.getItem('status')).toBeNull();
  });
});
