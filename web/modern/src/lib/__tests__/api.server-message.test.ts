import { AxiosError, type AxiosResponse, type InternalAxiosRequestConfig } from 'axios';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { sanitizeBreadcrumb } from '../../../../legacy/browser-sentry-privacy.mjs';
import { api } from '../api';

afterEach(() => {
  vi.restoreAllMocks();
  localStorage.clear();
});

/** httpRejectionAdapter builds an axios adapter that rejects like axios's own settle() does for a non-2xx status. */
const httpRejectionAdapter = (status: number, data: unknown) =>
  vi.fn((config: InternalAxiosRequestConfig) =>
    Promise.reject(
      new AxiosError(
        `Request failed with status code ${status}`,
        status >= 500 ? AxiosError.ERR_BAD_RESPONSE : AxiosError.ERR_BAD_REQUEST,
        config,
        {},
        { status, statusText: 'fixture', data, headers: {}, config } as AxiosResponse
      )
    )
  );

/** captureRejection runs one GET through the shared client and returns the rejected error. */
const captureRejection = async (adapter: ReturnType<typeof httpRejectionAdapter>): Promise<AxiosError> => {
  try {
    await api.get('/api/token/', { adapter });
  } catch (error) {
    return error as AxiosError;
  }
  throw new Error('expected the request to reject');
};

describe('api client surfaces server-provided 4xx messages', () => {
  it('rejects a 400 with the server message while keeping the axios response, code and status', async () => {
    const data = { success: false, message: 'resource uuid is required' };
    const error = await captureRejection(httpRejectionAdapter(400, data));

    expect(error).toBeInstanceOf(AxiosError);
    expect(error.message).toBe('resource uuid is required');
    expect(error.code).toBe(AxiosError.ERR_BAD_REQUEST);
    expect(error.response?.status).toBe(400);
    expect(error.response?.data).toBe(data);
  });

  it('keeps the generic axios text out of Sentry console breadcrumbs for the promoted server message', async () => {
    const error = await captureRejection(httpRejectionAdapter(400, { success: false, message: 'invalid resource reference' }));

    expect(error.message).toBe('invalid resource reference');
    const breadcrumb = sanitizeBreadcrumb({ category: 'console', message: 'fixture' }, { input: [error] });
    expect(breadcrumb?.message).toBe('Request failed with status code 400');
  });

  it.each([
    [404, { success: false }],
    [422, { success: false, message: '' }],
    [422, { success: false, message: '   ' }],
    [409, { success: false, message: { detail: 'not a string' } }],
    [400, 'plain text body'],
    [400, null],
  ])('keeps the axios message for HTTP %i without a usable string message (%j)', async (status, data) => {
    const error = await captureRejection(httpRejectionAdapter(status, data));

    expect(error.message).toBe(`Request failed with status code ${status}`);
    expect(error.response?.status).toBe(status);
  });

  it.each([500, 502])('keeps the axios message for HTTP %i even when the body carries a message', async (status) => {
    const error = await captureRejection(httpRejectionAdapter(status, { success: false, message: 'internal detail' }));

    expect(error.message).toBe(`Request failed with status code ${status}`);
    expect(error.response?.status).toBe(status);
  });

  it('still resolves a 2xx success:false response unchanged', async () => {
    const data = { success: false, message: 'resource not found' };
    const adapter = vi.fn(async (config: InternalAxiosRequestConfig): Promise<AxiosResponse> => ({
      status: 200,
      statusText: 'OK',
      data,
      headers: {},
      config,
    }));

    const response = await api.get('/api/token/', { adapter });

    expect(response.status).toBe(200);
    expect(response.data).toBe(data);
  });

  it('still resolves a 2xx success:false response whose message is not a string', async () => {
    const data = { success: false, message: { detail: 'structured' } };
    const adapter = vi.fn(async (config: InternalAxiosRequestConfig): Promise<AxiosResponse> => ({
      status: 200,
      statusText: 'OK',
      data,
      headers: {},
      config,
    }));

    const response = await api.get('/api/token/', { adapter });

    expect(response.data).toBe(data);
  });
});
