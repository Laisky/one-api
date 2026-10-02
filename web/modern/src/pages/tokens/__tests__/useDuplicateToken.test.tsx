import { act, fireEvent, render, renderHook, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { api } from '@/lib/api';
import { TokenDuplicateAction, useDuplicateToken } from '../useDuplicateToken';

const { notify } = vi.hoisted(() => ({ notify: vi.fn() }));
vi.mock('@/components/ui/notifications', () => ({ useNotifications: () => ({ notify }) }));
vi.mock('@/lib/api', () => ({ api: { get: vi.fn(), post: vi.fn() } }));

const source = {
  uuid: 'source-uuid',
  key: 'source-secret',
  name: 'production-9',
  expired_time: -1,
  remain_quota: 1234,
  unlimited_quota: false,
  models: 'model-a',
  subnet: '192.0.2.0/24',
  used_quota: 5678,
};
const sourceResponse = { data: { success: true, data: source } };

/** deferred returns a controllable promise for observing in-flight user interactions. */
function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((done) => {
    resolve = done;
  });
  return { promise, resolve };
}

/** Actions renders both layouts with one shared hook to test cross-button duplicate suppression. */
function Actions({ onSuccess }: { onSuccess: () => void | Promise<void> }) {
  const action = useDuplicateToken(onSuccess);
  return (
    <>
      <TokenDuplicateAction tokenRef="source-uuid" action={action} />
      <TokenDuplicateAction tokenRef="source-uuid" action={action} compact />
    </>
  );
}

describe('useDuplicateToken', () => {
  beforeEach(() => {
    vi.resetAllMocks();
    vi.mocked(api.get).mockResolvedValue(sourceResponse);
    vi.mocked(api.post).mockResolvedValue({ data: { success: true, data: { uuid: 'new-uuid', key: 'new-secret' } } });
  });

  it.each(['source-uuid', 7])('loads fresh settings using reference %s and submits only the creation payload', async (ref) => {
    const refresh = vi.fn();
    const { result } = renderHook(() => useDuplicateToken(refresh));
    await act(async () => result.current.duplicate(ref));
    expect(api.get).toHaveBeenCalledExactlyOnceWith(`/api/token/${ref}`);
    expect(api.post).toHaveBeenCalledExactlyOnceWith('/api/token/', {
      name: 'production-10',
      expired_time: -1,
      remain_quota: 1234,
      unlimited_quota: false,
      models: 'model-a',
      subnet: '192.0.2.0/24',
    });
    expect(refresh).toHaveBeenCalledTimes(1);
    expect(notify).toHaveBeenCalledExactlyOnceWith({ type: 'success', message: 'Created token "production-10".' });
    expect(JSON.stringify(notify.mock.calls)).not.toContain('secret');
    expect(result.current.pending.size).toBe(0);
  });

  it('blocks re-entrant calls even before React renders the disabled state', async () => {
    const request = deferred<typeof sourceResponse>();
    vi.mocked(api.get).mockReturnValue(request.promise);
    const { result } = renderHook(() => useDuplicateToken(vi.fn()));
    let first!: Promise<void>;
    act(() => {
      first = result.current.duplicate('source-uuid');
      void result.current.duplicate('source-uuid');
    });
    expect(api.get).toHaveBeenCalledTimes(1);
    expect(result.current.pending.has('source-uuid')).toBe(true);
    await act(async () => {
      request.resolve(sourceResponse);
      await first;
    });
    expect(api.post).toHaveBeenCalledTimes(1);
    expect(result.current.pending.size).toBe(0);
  });

  it('disables both layouts until creation and refresh complete', async () => {
    const creation = deferred<{ data: { success: boolean } }>();
    const refresh = deferred<void>();
    vi.mocked(api.post).mockReturnValue(creation.promise);
    render(<Actions onSuccess={() => refresh.promise} />);
    fireEvent.click(screen.getAllByRole('button', { name: 'Duplicate' })[0]);
    await waitFor(() => expect(api.post).toHaveBeenCalledTimes(1));
    for (const button of screen.getAllByRole('button', { name: 'Duplicating...' })) {
      expect(button).toBeDisabled();
      expect(button).toHaveAttribute('aria-busy', 'true');
      fireEvent.click(button);
    }
    expect(api.post).toHaveBeenCalledTimes(1);
    await act(async () => creation.resolve({ data: { success: true } }));
    expect(screen.getAllByRole('button', { name: 'Duplicating...' })).toHaveLength(2);
    await act(async () => refresh.resolve());
    expect(screen.getAllByRole('button', { name: 'Duplicate' })).toHaveLength(2);
    expect(screen.getAllByRole('button', { name: 'Duplicate' })[0]).toBeEnabled();
  });

  it('allows independent tokens to be duplicated concurrently', async () => {
    const { result } = renderHook(() => useDuplicateToken(vi.fn()));
    await act(async () => {
      await Promise.all([result.current.duplicate('first'), result.current.duplicate('second')]);
    });
    expect(api.get).toHaveBeenCalledTimes(2);
    expect(api.post).toHaveBeenCalledTimes(2);
    expect(result.current.pending.size).toBe(0);
  });

  it.each([
    { data: { success: false, message: 'source not found' } },
    { data: { success: true } },
  ])('does not create or refresh when the source cannot be read: %j', async (response) => {
    vi.mocked(api.get).mockResolvedValue(response);
    const refresh = vi.fn();
    const { result } = renderHook(() => useDuplicateToken(refresh));
    await act(async () => result.current.duplicate('source-uuid'));
    expect(api.post).not.toHaveBeenCalled();
    expect(refresh).not.toHaveBeenCalled();
    expect(notify).toHaveBeenCalledWith(expect.objectContaining({ type: 'error' }));
    expect(result.current.pending.size).toBe(0);
  });

  it('surfaces HTTP failures and unlocks the action for retry', async () => {
    vi.mocked(api.get).mockRejectedValueOnce({ response: { data: { message: 'permission denied' } } });
    const refresh = vi.fn();
    const { result } = renderHook(() => useDuplicateToken(refresh));
    await act(async () => result.current.duplicate('source-uuid'));
    expect(notify).toHaveBeenCalledWith({ type: 'error', message: 'permission denied' });
    expect(api.post).not.toHaveBeenCalled();
    await act(async () => result.current.duplicate('source-uuid'));
    expect(api.post).toHaveBeenCalledTimes(1);
    expect(refresh).toHaveBeenCalledTimes(1);
  });

  it('does not show success or refresh on a rejected create response', async () => {
    vi.mocked(api.post).mockResolvedValue({ data: { success: false, message: 'creation rejected' } });
    const refresh = vi.fn();
    const { result } = renderHook(() => useDuplicateToken(refresh));
    await act(async () => result.current.duplicate('source-uuid'));
    expect(notify).toHaveBeenCalledExactlyOnceWith({ type: 'error', message: 'creation rejected' });
    expect(refresh).not.toHaveBeenCalled();
    expect(result.current.pending.size).toBe(0);
  });

  it('does not automatically retry a failed create request', async () => {
    vi.mocked(api.post).mockRejectedValue(new Error('Network Error'));
    const refresh = vi.fn();
    const { result } = renderHook(() => useDuplicateToken(refresh));
    await act(async () => result.current.duplicate('source-uuid'));
    expect(api.post).toHaveBeenCalledTimes(1);
    expect(refresh).not.toHaveBeenCalled();
    expect(notify).toHaveBeenCalledExactlyOnceWith({ type: 'error', message: 'Network Error' });
    expect(result.current.pending.size).toBe(0);
  });

  it('reports name length validation without submitting a create request', async () => {
    vi.mocked(api.get).mockResolvedValue({ data: { success: true, data: { ...source, name: '令'.repeat(10) } } });
    const { result } = renderHook(() => useDuplicateToken(vi.fn()));
    await act(async () => result.current.duplicate('source-uuid'));
    expect(api.post).not.toHaveBeenCalled();
    expect(notify).toHaveBeenCalledWith({
      type: 'error',
      message: 'The new name exceeds 30 bytes. Shorten the source token name before duplicating it.',
    });
  });

  it('refreshes the latest list context when navigation changes during creation', async () => {
    const creation = deferred<{ data: { success: boolean } }>();
    vi.mocked(api.post).mockReturnValue(creation.promise);
    const oldRefresh = vi.fn();
    const newRefresh = vi.fn();
    const { result, rerender } = renderHook(({ refresh }) => useDuplicateToken(refresh), {
      initialProps: { refresh: oldRefresh },
    });
    let request!: Promise<void>;
    act(() => {
      request = result.current.duplicate('source-uuid');
    });
    await waitFor(() => expect(api.post).toHaveBeenCalledTimes(1));
    rerender({ refresh: newRefresh });
    await act(async () => {
      creation.resolve({ data: { success: true } });
      await request;
    });
    expect(oldRefresh).not.toHaveBeenCalled();
    expect(newRefresh).toHaveBeenCalledTimes(1);
  });

  it('distinguishes a refresh failure from a creation failure', async () => {
    const { result } = renderHook(() => useDuplicateToken(vi.fn().mockRejectedValue(new Error('refresh failed'))));
    await act(async () => result.current.duplicate('source-uuid'));
    expect(api.post).toHaveBeenCalledTimes(1);
    expect(notify).toHaveBeenNthCalledWith(1, { type: 'success', message: 'Created token "production-10".' });
    expect(notify).toHaveBeenNthCalledWith(2, {
      type: 'error',
      message: 'The token was created, but the list could not be refreshed. Refresh the page.',
    });
    expect(result.current.pending.size).toBe(0);
  });
});
