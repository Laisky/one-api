import type { FormEvent } from 'react';
import { act, cleanup, fireEvent, render, renderHook, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { api } from '@/lib/api';
import { useDuplicateChannel } from '@/pages/channels/useDuplicateChannel';
import { TokenDuplicateAction, useDuplicateToken } from '@/pages/tokens/useDuplicateToken';
import { duplicateActionTranslations } from '@/i18n/locales/duplicate-action';
import { DuplicateAction } from '../DuplicateAction';

const { notify } = vi.hoisted(() => ({ notify: vi.fn() }));
vi.mock('@/components/ui/notifications', () => ({ useNotifications: () => ({ notify }) }));
vi.mock('@/lib/api', () => ({ api: { get: vi.fn(), post: vi.fn() } }));

type Kind = 'token' | 'channel';
const source = { name: 'source-9', expired_time: -1, remain_quota: 10, unlimited_quota: false };

/** deferred returns a promise whose completion is controlled by the test. */
function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((done) => { resolve = done; });
  return { promise, resolve };
}

/** Actions renders both entry points with the appropriate resource hook and shared mutation state. */
function Actions({ kind, refresh }: { kind: Kind; refresh: () => void | Promise<void> }) {
  const useAction = kind === 'token' ? useDuplicateToken : useDuplicateChannel;
  const action = useAction(refresh);
  return (
    <>
      {[false, true].map((compact) => kind === 'token' ? (
        <TokenDuplicateAction key={String(compact)} tokenRef="source-uuid" action={action} compact={compact} />
      ) : (
        <DuplicateAction
          key={String(compact)}
          onDuplicate={() => action.duplicate('source-uuid')}
          pending={action.pending.has('source-uuid')}
          compact={compact}
        />
      ))}
    </>
  );
}

afterEach(cleanup);
beforeEach(() => {
  vi.resetAllMocks();
  vi.mocked(api.get).mockResolvedValue({ data: { success: true, data: source } });
  vi.mocked(api.post).mockResolvedValue({ data: { success: true, data: { name: 'source-10' } } });
});

describe.each(['token', 'channel'] as const)('%s shared duplicate interaction', (kind) => {
  const useAction = kind === 'token' ? useDuplicateToken : useDuplicateChannel;

  it('suppresses synchronous re-entry, including equivalent numeric references', async () => {
    const response = deferred<{ data: { success: boolean } }>();
    vi.mocked(api.post).mockReturnValue(response.promise);
    const { result } = renderHook(() => useAction(vi.fn()));
    let request!: Promise<void>;
    act(() => {
      request = result.current.duplicate(7);
      void result.current.duplicate('7');
    });
    await waitFor(() => expect(api.post).toHaveBeenCalledTimes(1));
    expect(result.current.pending.has('7')).toBe(true);
    await act(async () => {
      response.resolve({ data: { success: true } });
      await request;
    });
    expect(result.current.pending.size).toBe(0);
  });

  it('disables and animates both layouts through creation and refresh, without submitting a parent form', async () => {
    const response = deferred<{ data: { success: boolean } }>();
    const refresh = deferred<void>();
    vi.mocked(api.post).mockReturnValue(response.promise);
    const submit = vi.fn((event: FormEvent) => event.preventDefault());
    render(<form onSubmit={submit}><Actions kind={kind} refresh={() => refresh.promise} /></form>);
    const initial = screen.getAllByRole('button', { name: 'Duplicate' });
    expect(initial[0]).toHaveTextContent('Duplicate');
    expect(initial[1].textContent).toBe('');
    for (const button of initial) {
      expect(button).toHaveAttribute('type', 'button');
      expect(button).toHaveAttribute('title', 'Duplicate');
      expect(button).toHaveClass('touch-target');
      expect(button.querySelector('svg')).toHaveClass('h-4', 'w-4');
    }
    fireEvent.click(initial[0]);
    await waitFor(() => expect(api.post).toHaveBeenCalledTimes(1));
    for (const button of screen.getAllByRole('button', { name: 'Duplicating...' })) {
      expect(button).toBeDisabled();
      expect(button).toHaveAttribute('aria-busy', 'true');
      expect(button.querySelector('svg')).toHaveClass('animate-spin');
      fireEvent.click(button);
    }
    expect(api.post).toHaveBeenCalledTimes(1);
    expect(submit).not.toHaveBeenCalled();
    await act(async () => response.resolve({ data: { success: true } }));
    expect(screen.getAllByRole('button', { name: 'Duplicating...' })).toHaveLength(2);
    await act(async () => refresh.resolve());
    for (const button of screen.getAllByRole('button', { name: 'Duplicate' })) expect(button).toBeEnabled();
  });

  it('lets different rows proceed independently', async () => {
    const { result } = renderHook(() => useAction(vi.fn()));
    await act(async () => {
      await Promise.all([result.current.duplicate('first'), result.current.duplicate('second')]);
    });
    expect(api.post).toHaveBeenCalledTimes(2);
    expect(result.current.pending.size).toBe(0);
  });

  it('refreshes the latest page context after an in-flight request completes', async () => {
    const response = deferred<{ data: { success: boolean } }>();
    vi.mocked(api.post).mockReturnValue(response.promise);
    const oldRefresh = vi.fn();
    const newRefresh = vi.fn();
    const { result, rerender } = renderHook(({ refresh }) => useAction(refresh), { initialProps: { refresh: oldRefresh } });
    let request!: Promise<void>;
    act(() => { request = result.current.duplicate('source-uuid'); });
    await waitFor(() => expect(api.post).toHaveBeenCalledTimes(1));
    rerender({ refresh: newRefresh });
    await act(async () => {
      response.resolve({ data: { success: true } });
      await request;
    });
    expect(oldRefresh).not.toHaveBeenCalled();
    expect(newRefresh).toHaveBeenCalledTimes(1);
  });

  it.each([
    ['rejected envelope', { data: { success: false, message: 'permission denied' } }],
    ['HTTP failure', { response: { data: { message: 'permission denied' } } }],
    ['network failure', new Error('permission denied')],
  ] as const)('reports a %s once, does not retry, and releases the lock', async (label, response) => {
    if (label === 'rejected envelope') vi.mocked(api.post).mockResolvedValueOnce(response);
    else vi.mocked(api.post).mockRejectedValueOnce(response);
    const refresh = vi.fn();
    const { result } = renderHook(() => useAction(refresh));
    await act(async () => result.current.duplicate('source-uuid'));
    expect(api.post).toHaveBeenCalledTimes(1);
    expect(notify).toHaveBeenCalledExactlyOnceWith({ type: 'error', message: 'permission denied' });
    expect(refresh).not.toHaveBeenCalled();
    expect(result.current.pending.size).toBe(0);
    await act(async () => result.current.duplicate('source-uuid'));
    expect(api.post).toHaveBeenCalledTimes(2);
    expect(refresh).toHaveBeenCalledTimes(1);
  });

  it('distinguishes a refresh failure from a failed mutation', async () => {
    const { result } = renderHook(() => useAction(vi.fn().mockRejectedValue(new Error('list unavailable'))));
    await act(async () => result.current.duplicate('source-uuid'));
    expect(api.post).toHaveBeenCalledTimes(1);
    expect(notify).toHaveBeenNthCalledWith(1, { type: 'success', message: `Created ${kind} "source-10".` });
    expect(notify).toHaveBeenNthCalledWith(2, {
      type: 'error',
      message: `The ${kind} was created, but the list could not be refreshed. Refresh the page.`,
    });
    expect(result.current.pending.size).toBe(0);
  });
});

describe('duplicate action translations', () => {
  it('provides the same complete key set and name placeholder in all supported languages', () => {
    const keys = Object.keys(duplicateActionTranslations.en).sort();
    expect(Object.keys(duplicateActionTranslations).sort()).toEqual(['en', 'es', 'fr', 'ja', 'zh']);
    for (const translations of Object.values(duplicateActionTranslations)) {
      expect(Object.keys(translations).sort()).toEqual(keys);
      expect(Object.values(translations).every((value) => value.length > 0)).toBe(true);
      expect(translations.channel_success).toContain('{{name}}');
    }
  });
});
