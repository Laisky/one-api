import { reportUnexpectedRequestError } from '../../../legacy/browser-request-errors.mjs';
import { markPrivateError } from '../../../legacy/browser-sentry-privacy.mjs';
import { showError } from './utils';
import axios from 'axios';

export const API = axios.create({
  baseURL: process.env.REACT_APP_SERVER ? process.env.REACT_APP_SERVER : '',
});

/**
 * surfaceServerRejectionMessage replaces the generic Axios text ("Request failed
 * with status code 400") of a rejected 4xx response with the server-provided
 * `message`, so the toast and callers show the real reason. It takes the
 * rejected value and mutates it in place, keeping the same object, its
 * `response`, `code` and stack; other errors are left untouched. The server text
 * is annotated as private so telemetry keeps the original Axios message.
 */
function surfaceServerRejectionMessage(error) {
  if (!axios.isAxiosError(error)) return;
  const status = error.response?.status;
  if (typeof status !== 'number' || status < 400 || status > 499) return;
  const message = error.response?.data?.message;
  if (typeof message !== 'string' || message.trim() === '') return;
  if (message === error.message) return;
  markPrivateError(error, error.message, true);
  error.message = message;
}

API.interceptors.response.use(
  (response) => response,
  (error) => {
    if (axios.isAxiosError(error) && !error.response && error.request)
      reportUnexpectedRequestError(error);
    surfaceServerRejectionMessage(error);
    showError(error);
    return Promise.reject(error);
  },
);
