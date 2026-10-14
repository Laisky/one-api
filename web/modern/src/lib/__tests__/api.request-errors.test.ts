import { AxiosError, CanceledError, type AxiosResponse, type InternalAxiosRequestConfig } from 'axios';
import { afterEach, expect, it, vi } from 'vitest';
import { registerRequestErrorCapture } from '../../../../legacy/browser-request-errors.mjs';
import { api } from '../api';

let unregister = () => {};
afterEach(() => {
  unregister();
  vi.restoreAllMocks();
  localStorage.clear();
});

it('reports one actual Axios transport failure and preserves the rejected error', async () => {
  const capture = vi.fn();
  unregister = registerRequestErrorCapture(capture);
  let failure: AxiosError;
  const adapter = vi.fn((config: InternalAxiosRequestConfig) => {
    failure = new AxiosError('Network Error', 'ERR_NETWORK', config, {});
    return Promise.reject(failure);
  });
  let received: unknown;
  try {
    await api.get('/api/models/display', { adapter });
  } catch (error) {
    received = error;
  }
  expect(failure!).toBeInstanceOf(AxiosError);
  expect(received).toBe(failure!);
  expect(capture).toHaveBeenCalledTimes(1);
  expect(capture).toHaveBeenCalledWith(failure!);
  expect(failure!.message).toBe('Network Error');
});

it.each([403, 429, 500])('keeps HTTP %i in the existing response/UI flow without an event', async (status) => {
  const capture = vi.fn();
  unregister = registerRequestErrorCapture(capture);
  const adapter = vi.fn((config: InternalAxiosRequestConfig) =>
    Promise.reject(
      new AxiosError('HTTP rejection', 'ERR_BAD_RESPONSE', config, {}, {
        status,
        statusText: 'fixture',
        data: { message: 'fixture response' },
        headers: {},
        config,
      } as AxiosResponse)
    )
  );
  await expect(api.get('/api/models/display', { adapter })).rejects.toMatchObject({ response: { status } });
  expect(capture).not.toHaveBeenCalled();
});

it('does not report Axios cancellation or alter its rejection', async () => {
  const capture = vi.fn();
  unregister = registerRequestErrorCapture(capture);
  const error = new CanceledError('cancel fixture', undefined, {});
  await expect(api.get('/api/models/display', { adapter: () => Promise.reject(error) })).rejects.toBe(error);
  expect(capture).not.toHaveBeenCalled();
});

it('preserves programmer-error rejection without misclassifying it as an Axios network failure', async () => {
  const capture = vi.fn();
  unregister = registerRequestErrorCapture(capture);
  const error = new TypeError('ordinary adapter implementation failure');
  await expect(api.get('/api/models/display', { adapter: () => Promise.reject(error) })).rejects.toBe(error);
  expect(capture).not.toHaveBeenCalled();
});

it('keeps the original network rejection when SDK capture is absent', async () => {
  const error = new AxiosError('Network Error', 'ERR_NETWORK', undefined, {});
  await expect(api.get('/api/models/display', { adapter: () => Promise.reject(error) })).rejects.toBe(error);
});

it('keeps the original network rejection when the registered capture fails', async () => {
  unregister = registerRequestErrorCapture(() => {
    throw new Error('synthetic-private-reporter-failure');
  });
  const warn = vi.spyOn(console, 'warn').mockImplementation(() => {});
  const error = new AxiosError('Network Error', 'ERR_NETWORK', undefined, {});
  await expect(api.get('/api/models/display', { adapter: () => Promise.reject(error) })).rejects.toBe(error);
  expect(warn).toHaveBeenCalledWith('Browser request error reporting unavailable.');
});
