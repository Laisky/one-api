import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { AxiosRequestConfig, AxiosResponse } from 'axios';
import { __resetAuthFailureGateForTests, api, isSafeInternalPath } from '@/lib/api';
import { useAuthStore } from '@/lib/stores/auth';

// Custom axios adapter — lets us stage canned responses without spinning up
// a real network or pulling in axios-mock-adapter (which isn't a dep).
type Canned = { status: number; data: unknown };
const stub: { route: (url: string, resp: Canned) => void; reset: () => void; adapter: (cfg: AxiosRequestConfig) => Promise<AxiosResponse> } = (() => {
  const responses = new Map<string, Canned>();
  return {
    route: (url, resp) => responses.set(url, resp),
    reset: () => responses.clear(),
    adapter: async (cfg) => {
      const stripped = (cfg.url || '').split('?')[0];
      const r = responses.get(stripped);
      if (!r) {
        return Promise.reject(Object.assign(new Error(`no stub for ${cfg.url}`), { config: cfg, response: { status: 599, data: {} } }));
      }
      const response: AxiosResponse = {
        data: r.data,
        status: r.status,
        statusText: '',
        headers: {},
        config: cfg as never,
      };
      if (r.status >= 400) {
        return Promise.reject(Object.assign(new Error(`HTTP ${r.status}`), { config: cfg, response }));
      }
      return response;
    },
  };
})();
api.defaults.adapter = stub.adapter;

// JSDOM's `window.location.replace` is a no-op for navigation, but we still
// need to observe what value the handler tried to commit.
const mockLocation = (pathname: string, search = '') => {
  const url = `https://example.com${pathname}${search}`;
  const replace = vi.fn();
  Object.defineProperty(window, 'location', {
    configurable: true,
    writable: true,
    value: {
      origin: 'https://example.com',
      href: url,
      pathname,
      search,
      replace,
    },
  });
  return replace;
};

describe('isSafeInternalPath', () => {
  beforeEach(() => {
    Object.defineProperty(window, 'location', {
      configurable: true,
      writable: true,
      value: { origin: 'https://example.com', pathname: '/', search: '', replace: vi.fn() },
    });
  });

  it.each([
    ['/dashboard', true],
    ['/channels?tab=1', true],
    ['/', true],
    // Reject open-redirect surfaces (OWASP "Unvalidated Redirects and Forwards").
    ['//evil.com', false],
    ['/\\evil.com', false],
    ['https://evil.com/x', false],
    ['http://evil.com/x', false],
    ['javascript:alert(1)', false],
    ['', false],
    ['dashboard', false],
    [' /dashboard', false],
  ])('isSafeInternalPath(%p) === %p', (input, expected) => {
    expect(isSafeInternalPath(input as string)).toBe(expected);
  });
});

describe('axios 401 interceptor (handleAuthFailure)', () => {
  let logoutSpy: ReturnType<typeof vi.fn>;
  const realLogout = useAuthStore.getState().logout;

  beforeEach(() => {
    stub.reset();
    __resetAuthFailureGateForTests();
    // Seed stale auth state so we can verify the handler clears it.
    useAuthStore.setState({
      user: { id: 1, username: 'stale', role: 1, status: 1, quota: 0, used_quota: 0, group: 'default' },
      token: 'stale-token',
      isAuthenticated: true,
    });
    logoutSpy = vi.fn(realLogout);
    useAuthStore.setState({ logout: logoutSpy });
    localStorage.setItem('token', 'legacy');
    localStorage.setItem('user', 'legacy');
  });

  afterEach(() => {
    localStorage.clear();
    useAuthStore.setState({ logout: realLogout });
  });

  it('on /channels 401: clears zustand state AND replaces to /login with encoded redirect_to', async () => {
    const replace = mockLocation('/channels', '?tab=config');
    stub.route('/api/anything', { status: 401, data: { success: false, message: 'unauthorized' } });

    await expect(api.get('/api/anything')).rejects.toBeTruthy();

    // Zustand logout invoked — this clears `auth-storage` (the real bug fix).
    expect(logoutSpy).toHaveBeenCalledTimes(1);
    // Legacy keys cleared too.
    expect(localStorage.getItem('token')).toBeNull();
    expect(localStorage.getItem('user')).toBeNull();
    // Redirect uses `replace` (not `href`) and encodes the full target once.
    expect(replace).toHaveBeenCalledTimes(1);
    expect(replace).toHaveBeenCalledWith('/login?redirect_to=%2Fchannels%3Ftab%3Dconfig');
  });

  it('loop guard: when ALREADY on /login, never redirects again (prevents nested redirect_to)', async () => {
    const replace = mockLocation('/login', '?redirect_to=%2Fchannels');
    stub.route('/api/user/passkey', { status: 401, data: { success: false, message: 'unauthorized' } });

    await expect(api.get('/api/user/passkey')).rejects.toBeTruthy();

    // State still cleared (defense in depth).
    expect(logoutSpy).toHaveBeenCalledTimes(1);
    // CRITICAL: no navigation — otherwise we'd build /login?redirect_to=%2Flogin%3F...
    expect(replace).not.toHaveBeenCalled();
  });

  it('re-entrancy guard: parallel 401s produce a single replace call', async () => {
    const replace = mockLocation('/dashboard', '');
    stub.route('/api/a', { status: 401, data: {} });
    stub.route('/api/b', { status: 401, data: {} });
    stub.route('/api/c', { status: 401, data: {} });

    await Promise.allSettled([api.get('/api/a'), api.get('/api/b'), api.get('/api/c')]);

    expect(replace).toHaveBeenCalledTimes(1);
    expect(replace).toHaveBeenCalledWith('/login?redirect_to=%2Fdashboard');
  });

  it('drops unsafe pathname and falls back to bare /login', async () => {
    // Force-craft an unusual pathname to verify the safety branch.
    const replace = mockLocation('//evil.com/path', '');
    stub.route('/api/anything', { status: 401, data: {} });

    await expect(api.get('/api/anything')).rejects.toBeTruthy();
    // pathname doesn't start with a safe single-slash path → fallback.
    expect(replace).toHaveBeenCalledWith('/login');
  });

  it('triggers on legacy `success:false` with "not logged in" message', async () => {
    const replace = mockLocation('/tokens', '');
    stub.route('/api/legacy', { status: 200, data: { success: false, message: 'Not logged in.' } });

    await api.get('/api/legacy');

    expect(logoutSpy).toHaveBeenCalledTimes(1);
    expect(replace).toHaveBeenCalledWith('/login?redirect_to=%2Ftokens');
  });
});
