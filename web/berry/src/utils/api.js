import { showError } from './common';
import axios from 'axios';
import { store } from 'store/index';
import { LOGIN } from 'store/actions';
import config from 'config';

export const API = axios.create({
  baseURL: process.env.REACT_APP_SERVER ? process.env.REACT_APP_SERVER : '/'
});

// Disable caching for all GET /api requests to ensure fresh data
API.interceptors.request.use((config) => {
  if (config.method && config.method.toLowerCase() === 'get' && config.url && config.url.startsWith('/api')) {
    config.headers['Cache-Control'] = 'no-cache, no-store, must-revalidate';
    config.headers['Pragma'] = 'no-cache';
    config.headers['Expires'] = '0';
    try {
      const urlObj = new URL(config.url, window.location.origin);
      urlObj.searchParams.set('_', Date.now().toString());
      config.url = urlObj.pathname + urlObj.search;
    } catch (e) {
      // ignore
    }
  }
  return config;
});

// Re-entrancy guard — parallel 401s should produce only one redirect.
let authFailureInFlight = false;

API.interceptors.response.use(
  (response) => response,
  (error) => {
    if (error.response?.status === 401) {
      if (!authFailureInFlight) {
        authFailureInFlight = true;
        try {
          localStorage.removeItem('user');
          localStorage.removeItem('token');
        } catch (_e) {
          // ignore — localStorage may be unavailable in some embedded contexts
        }
        store.dispatch({ type: LOGIN, payload: null });
        // Loop guard: if we're already on the login page, don't navigate again.
        // Without this, repeated 401s on the login page itself would hard-reload
        // forever (and on iOS Chrome, that's exactly what stale-cookie state
        // produces).
        const loginPath = (config.basename || '') + 'login';
        if (window.location.pathname !== loginPath && !window.location.pathname.endsWith('/login')) {
          window.location.replace(loginPath);
        } else {
          // Restore the gate — we did not navigate, so future failures should
          // be allowed to act if they fire on a different page later.
          authFailureInFlight = false;
        }
      }
    }

    if (error.response?.data?.message) {
      error.message = error.response.data.message;
    }

    showError(error);
  }
);
