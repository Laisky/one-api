import { act, renderHook, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('@/lib/api', () => ({
  api: { get: vi.fn() },
  default: { get: vi.fn() },
}));

import { api } from '@/lib/api';
import { useNotice } from '../useNotice';

const mockedGet = api.get as unknown as ReturnType<typeof vi.fn>;

describe('useNotice', () => {
  beforeEach(() => {
    window.localStorage.clear();
    mockedGet.mockReset();
  });

  afterEach(() => {
    window.localStorage.clear();
  });

  it('returns notice content when server data differs from stored seen value', async () => {
    mockedGet.mockResolvedValueOnce({ data: { success: true, data: '# Hello' } });

    const { result } = renderHook(() => useNotice());

    await waitFor(() => {
      expect(result.current.notice).not.toBeNull();
    });

    expect(result.current.notice?.content).toBe('# Hello');
  });

  it('persists dismissed content and clears state on dismiss', async () => {
    mockedGet.mockResolvedValueOnce({ data: { success: true, data: '# Hello' } });

    const { result } = renderHook(() => useNotice());

    await waitFor(() => {
      expect(result.current.notice).not.toBeNull();
    });

    act(() => {
      result.current.notice?.dismiss();
    });

    expect(window.localStorage.getItem('notice_seen_content')).toBe('# Hello');
    expect(result.current.notice).toBeNull();
  });

  it('returns null when notice content matches previously dismissed value', async () => {
    window.localStorage.setItem('notice_seen_content', '# Hello');
    mockedGet.mockResolvedValueOnce({ data: { success: true, data: '# Hello' } });

    const { result } = renderHook(() => useNotice());

    await waitFor(() => {
      expect(mockedGet).toHaveBeenCalledWith('/api/notice');
    });

    expect(result.current.notice).toBeNull();
  });

  it('returns null when notice content is empty', async () => {
    mockedGet.mockResolvedValueOnce({ data: { success: true, data: '   ' } });

    const { result } = renderHook(() => useNotice());

    await waitFor(() => {
      expect(mockedGet).toHaveBeenCalled();
    });

    expect(result.current.notice).toBeNull();
  });

  it('returns null when api responds with success: false', async () => {
    mockedGet.mockResolvedValueOnce({ data: { success: false, message: 'forbidden' } });

    const { result } = renderHook(() => useNotice());

    await waitFor(() => {
      expect(mockedGet).toHaveBeenCalled();
    });

    expect(result.current.notice).toBeNull();
  });
});
