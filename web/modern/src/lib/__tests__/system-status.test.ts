import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('@/lib/api', () => ({
  api: { get: vi.fn() },
  default: { get: vi.fn() },
}));

import { api } from '@/lib/api';
import { useAuthStore } from '@/lib/stores/auth';
import { initializeSystem, invalidateSystemStatusCache, loadSystemStatus, SYSTEM_STATUS_CACHE_TTL_MS } from '@/lib/utils';

const mockedGet = api.get as unknown as ReturnType<typeof vi.fn>;

/** statusResponse builds the axios-shaped success envelope /api/status returns. */
const statusResponse = (systemName: string) => ({ data: { success: true, data: { system_name: systemName, quota_per_unit: '1000' } } });

/** deferred returns a promise together with the functions that settle it. */
function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (error: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

describe('loadSystemStatus request sharing', () => {
  let now = 1_000_000;

  beforeEach(() => {
    invalidateSystemStatusCache();
    window.localStorage.clear();
    mockedGet.mockReset();
    now = 1_000_000;
    vi.spyOn(Date, 'now').mockImplementation(() => now);
    vi.spyOn(console, 'error').mockImplementation(() => {});
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('shares one request between concurrent callers', async () => {
    const pending = deferred<ReturnType<typeof statusResponse>>();
    mockedGet.mockReturnValue(pending.promise);

    const calls = [loadSystemStatus(), loadSystemStatus(), loadSystemStatus()];
    pending.resolve(statusResponse('Shared'));
    const results = await Promise.all(calls);

    expect(mockedGet).toHaveBeenCalledTimes(1);
    expect(mockedGet).toHaveBeenCalledWith('/api/status');
    expect(results.map((status) => status?.system_name)).toEqual(['Shared', 'Shared', 'Shared']);
    expect(window.localStorage.getItem('system_name')).toBe('Shared');
  });

  it('reuses a response within the TTL and asks the server again once it expires', async () => {
    mockedGet.mockResolvedValueOnce(statusResponse('First')).mockResolvedValueOnce(statusResponse('Second'));

    await expect(loadSystemStatus()).resolves.toMatchObject({ system_name: 'First' });
    now += SYSTEM_STATUS_CACHE_TTL_MS - 1;
    await expect(loadSystemStatus()).resolves.toMatchObject({ system_name: 'First' });
    expect(mockedGet).toHaveBeenCalledTimes(1);

    now += 1;
    await expect(loadSystemStatus()).resolves.toMatchObject({ system_name: 'Second' });
    expect(mockedGet).toHaveBeenCalledTimes(2);
  });

  it('does not cache failures and falls back to the stored status meanwhile', async () => {
    window.localStorage.setItem('status', JSON.stringify({ system_name: 'Stored' }));
    mockedGet.mockRejectedValueOnce(new Error('network down')).mockResolvedValueOnce(statusResponse('Recovered'));

    await expect(loadSystemStatus()).resolves.toMatchObject({ system_name: 'Stored' });
    expect(console.error).toHaveBeenCalledWith('Error fetching system status: network down');

    await expect(loadSystemStatus()).resolves.toMatchObject({ system_name: 'Recovered' });
    expect(mockedGet).toHaveBeenCalledTimes(2);
  });

  it('does not cache an unsuccessful server answer', async () => {
    mockedGet.mockResolvedValueOnce({ data: { success: false, message: 'busy' } }).mockResolvedValueOnce(statusResponse('Fresh'));

    await expect(loadSystemStatus()).resolves.toBeNull();
    await expect(loadSystemStatus()).resolves.toMatchObject({ system_name: 'Fresh' });
    expect(mockedGet).toHaveBeenCalledTimes(2);
  });

  it('lets an explicit refresh bypass the TTL', async () => {
    mockedGet.mockResolvedValueOnce(statusResponse('Cached')).mockResolvedValueOnce(statusResponse('Forced'));

    await loadSystemStatus();
    await expect(loadSystemStatus({ force: true })).resolves.toMatchObject({ system_name: 'Forced' });
    // The forced response replaces the cached one for later non-forced callers.
    await expect(loadSystemStatus()).resolves.toMatchObject({ system_name: 'Forced' });
    expect(mockedGet).toHaveBeenCalledTimes(2);
  });

  it('keeps the newest response when an older request finishes last', async () => {
    const older = deferred<ReturnType<typeof statusResponse>>();
    mockedGet.mockReturnValueOnce(older.promise).mockResolvedValueOnce(statusResponse('Newer'));

    const olderCall = loadSystemStatus();
    await expect(loadSystemStatus({ force: true })).resolves.toMatchObject({ system_name: 'Newer' });
    older.resolve(statusResponse('Older'));
    await expect(olderCall).resolves.toBeNull();

    await expect(loadSystemStatus()).resolves.toMatchObject({ system_name: 'Newer' });
    expect(window.localStorage.getItem('system_name')).toBe('Newer');
    expect(mockedGet).toHaveBeenCalledTimes(2);
  });

  it('discards a successful status response that completes after actual logout', async () => {
    const pending = deferred<ReturnType<typeof statusResponse>>();
    mockedGet.mockReturnValueOnce(pending.promise).mockResolvedValueOnce(statusResponse('Public'));
    const call = loadSystemStatus();
    useAuthStore.getState().logout();
    pending.resolve(statusResponse('Before logout'));
    await expect(call).resolves.toBeNull();
    for (const key of ['status', 'system_name', 'logo', 'footer_html', 'quota_per_unit', 'display_in_currency', 'chat_link']) {
      expect(localStorage.getItem(key)).toBeNull();
    }
    await expect(loadSystemStatus()).resolves.toMatchObject({ system_name: 'Public' });
    expect(mockedGet).toHaveBeenCalledTimes(2);
  });

  it('does not fall back to a newer session status for an invalidated request', async () => {
    const pending = deferred<ReturnType<typeof statusResponse>>();
    mockedGet.mockReturnValueOnce(pending.promise).mockResolvedValueOnce(statusResponse('New session'));
    const call = loadSystemStatus();
    useAuthStore.getState().logout();
    await loadSystemStatus();
    pending.reject(new Error('old network failure'));
    await expect(call).resolves.toBeNull();
    expect(localStorage.getItem('system_name')).toBe('New session');
  });

  it('refetches once the persisted copy was cleared, e.g. by logout', async () => {
    mockedGet.mockResolvedValueOnce(statusResponse('Before logout')).mockResolvedValueOnce(statusResponse('After logout'));

    await loadSystemStatus();
    window.localStorage.removeItem('status');
    window.localStorage.removeItem('system_name');

    await expect(loadSystemStatus()).resolves.toMatchObject({ system_name: 'After logout' });
    expect(mockedGet).toHaveBeenCalledTimes(2);
    expect(JSON.parse(window.localStorage.getItem('status') ?? '{}')).toMatchObject({ system_name: 'After logout' });
    expect(window.localStorage.getItem('system_name')).toBe('After logout');
  });

  it('refetches when another writer replaced the persisted copy', async () => {
    mockedGet.mockResolvedValueOnce(statusResponse('Mine')).mockResolvedValueOnce(statusResponse('Latest'));

    await loadSystemStatus();
    window.localStorage.setItem('status', JSON.stringify({ system_name: 'Other tab' }));

    await expect(loadSystemStatus()).resolves.toMatchObject({ system_name: 'Latest' });
    expect(mockedGet).toHaveBeenCalledTimes(2);
  });

  it('refetches after invalidateSystemStatusCache', async () => {
    mockedGet.mockResolvedValueOnce(statusResponse('Before')).mockResolvedValueOnce(statusResponse('After'));

    await loadSystemStatus();
    invalidateSystemStatusCache();
    await expect(loadSystemStatus()).resolves.toMatchObject({ system_name: 'After' });
    expect(mockedGet).toHaveBeenCalledTimes(2);
  });
});

describe('initializeSystem', () => {
  beforeEach(() => {
    invalidateSystemStatusCache();
    window.localStorage.clear();
    mockedGet.mockReset();
    vi.spyOn(console, 'error').mockImplementation(() => {});
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('joins the request other status consumers already started', async () => {
    const pending = deferred<ReturnType<typeof statusResponse>>();
    mockedGet.mockReturnValue(pending.promise);

    const headerLoad = loadSystemStatus();
    const boot = initializeSystem();
    pending.resolve(statusResponse('Booted'));
    await Promise.all([headerLoad, boot]);

    expect(mockedGet).toHaveBeenCalledTimes(1);
    expect(window.localStorage.getItem('system_name')).toBe('Booted');
  });

  it('does not restore boot defaults when a pre-logout request fails late', async () => {
    const pending = deferred<ReturnType<typeof statusResponse>>();
    mockedGet.mockReturnValueOnce(pending.promise);
    const boot = initializeSystem();
    useAuthStore.getState().logout();
    pending.reject(new Error('late offline'));
    await boot;
    expect(localStorage.getItem('quota_per_unit')).toBeNull();
    expect(localStorage.getItem('display_in_currency')).toBeNull();
    expect(localStorage.getItem('system_name')).toBeNull();
  });

  it('stores default settings when the request fails', async () => {
    mockedGet.mockRejectedValue(new Error('offline'));

    await initializeSystem();

    expect(window.localStorage.getItem('quota_per_unit')).toBe('500000');
    expect(window.localStorage.getItem('display_in_currency')).toBe('true');
    expect(window.localStorage.getItem('system_name')).toBe('One API');
    expect(console.error).toHaveBeenCalledWith('Failed to initialize system settings: offline');
  });
});
