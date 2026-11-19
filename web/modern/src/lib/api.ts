import { reportUnexpectedRequestError } from '../../../legacy/browser-request-errors.mjs';
import { markPrivateError } from '../../../legacy/browser-sentry-privacy.mjs';
import axios from 'axios';
import { useAuthStore } from '@/lib/stores/auth';

// Unified API client - callers must provide complete URLs including /api prefix
// This eliminates ambiguity and ensures consistency across all API calls
export const api = axios.create({
  timeout: 10000,
});

// Always send cookies for session-based auth
api.defaults.withCredentials = true;

// Request interceptor
api.interceptors.request.use(
  (config) => {
    // For session-based authentication, we rely on cookies (withCredentials: true)
    // Only add Authorization header for specific API endpoints that require token auth
    // Most dashboard/web endpoints use session-based auth via cookies
    const token = localStorage.getItem('token');
    if (token && config.url?.startsWith('/v1/')) {
      // Only add token for API endpoints that require token authentication
      config.headers.Authorization = `Bearer ${token}`;
    }

    // Disable caching for all GET requests to /api endpoints to always fetch fresh data
    if (config.method?.toLowerCase() === 'get' && config.url && config.url.startsWith('/api')) {
      // Set explicit no-cache headers
      config.headers['Cache-Control'] = 'no-cache, no-store, must-revalidate';
      config.headers['Pragma'] = 'no-cache';
      config.headers['Expires'] = '0';

      // Append a cache-busting timestamp query param while preserving existing params
      try {
        const urlObj = new URL(config.url, window.location.origin);
        urlObj.searchParams.set('_', Date.now().toString());
        config.url = urlObj.pathname + urlObj.search;
      } catch (_e) {
        // Fallback: if URL constructor fails (should not for relative paths), leave URL unchanged
      }
    }
    return config;
  },
  (error) => Promise.reject(error)
);

// Re-entrancy guard — multiple parallel requests can return 401 in the same tick.
// Without this, each one triggers a redirect, which on iOS WKWebView can lead to
// the address bar accumulating nested `redirect_to` parameters. In production
// `window.location.replace` discards the JS context so the flag never needs to
// clear; the test-only reset below restores it between cases.
let authFailureInFlight = false;
// Exported for unit tests only — production code must not call this.
export const __resetAuthFailureGateForTests = () => {
  authFailureInFlight = false;
};

// Validate a candidate redirect target — must be a same-origin internal path,
// rejecting protocol-relative (`//evil.com`) and absolute (`https://...`) values
// per OWASP "Unvalidated Redirects and Forwards".
export const isSafeInternalPath = (raw: string): boolean => {
  if (!raw || typeof raw !== 'string') return false;
  if (!raw.startsWith('/')) return false;
  // Reject protocol-relative URLs (//host) and Windows-style (/\host)
  if (raw.startsWith('//') || raw.startsWith('/\\')) return false;
  // Reject any embedded scheme
  if (raw.includes('://')) return false;
  try {
    const url = new URL(raw, window.location.origin);
    return url.origin === window.location.origin;
  } catch {
    return false;
  }
};

// Helper function to handle authentication failures
const handleAuthFailure = () => {
  if (authFailureInFlight) return;
  authFailureInFlight = true;

  // Clear zustand-persisted auth state. The store's `logout()` writes
  // `{user:null, token:null, isAuthenticated:false}` which the persist middleware
  // flushes to `localStorage['auth-storage']` — without this, a stale `isAuthenticated:true`
  // re-hydrates on the next page load and components keep firing authenticated
  // requests, producing an infinite 401 → redirect loop.
  try {
    useAuthStore.getState().logout();
  } catch {
    // Belt-and-suspenders if the store fails to load.
    localStorage.removeItem('auth-storage');
  }

  // Clear legacy keys kept for backward compatibility with older builds.
  localStorage.removeItem('token');
  localStorage.removeItem('user');

  // Loop guard: if we're already on the login page, do not redirect again —
  // that would re-encode the current URL (which already contains a
  // `redirect_to=...`) and produce the deeply nested URLs seen on iOS Chrome
  // when ITP has dropped the session cookie but localStorage survived.
  if (window.location.pathname === '/login') {
    authFailureInFlight = false;
    return;
  }

  const currentPath = window.location.pathname + window.location.search;
  const target = isSafeInternalPath(currentPath) ? `/login?redirect_to=${encodeURIComponent(currentPath)}` : '/login';

  // `replace` (vs. `href`) prevents the broken state from accumulating in
  // history — the user can't back-button into a 401 retry.
  window.location.replace(target);
};

/**
 * serverMessage returns the `message` string of a one-api JSON response body.
 * It takes the decoded response body and returns the message, or an empty string
 * when the body has no non-blank string `message`.
 */
const serverMessage = (data: unknown): string => {
  const message = (data as { message?: unknown } | null | undefined)?.message;
  return typeof message === 'string' && message.trim() !== '' ? message : '';
};

/**
 * surfaceServerRejectionMessage replaces the generic axios text ("Request failed
 * with status code 400") of a rejected 4xx response with the server-provided
 * `message`, so callers that display `error.message` show the real reason.
 * It takes the rejected value and mutates it in place, keeping the same object,
 * its `response`, `code` and stack; other errors (2xx, 5xx, network, cancellation)
 * are left untouched. The server text is annotated as private so browser telemetry
 * keeps reporting the original axios message instead of response content.
 */
const surfaceServerRejectionMessage = (error: unknown): void => {
  if (!axios.isAxiosError(error)) return;
  const status = error.response?.status;
  if (typeof status !== 'number' || status < 400 || status > 499) return;
  const message = serverMessage(error.response?.data);
  if (!message || message === error.message) return;
  markPrivateError(error, error.message, true);
  error.message = message;
};

// Response interceptor
api.interceptors.response.use(
  (response) => {
    // Handle legacy 200 OK with success: false for auth errors
    if (response.data && response.data.success === false) {
      const url = response.config?.url || '';
      const message = serverMessage(response.data).toLowerCase();
      const isAuthError = message.includes('access token is invalid') || message.includes('not logged in');

      // Do not redirect for known public endpoints
      const isPublicEndpoint = url.startsWith('/api/models/display') || url.startsWith('/api/tools/display');

      if (isAuthError && !isPublicEndpoint) {
        handleAuthFailure();
        return response;
      }
    }
    return response;
  },
  (error) => {
    // Handle proper HTTP status codes for authentication/authorization failures
    const status = error.response?.status;
    const message = serverMessage(error.response?.data).toLowerCase();
    if (status === 401 || (status === 403 && message.includes('not logged in'))) {
      handleAuthFailure();
    }
    if (axios.isAxiosError(error) && !error.response && error.request) reportUnexpectedRequestError(error);
    surfaceServerRejectionMessage(error);
    return Promise.reject(error);
  }
);

export default api;
