import { act, renderHook } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

const mocks = vi.hoisted(() => ({
  copy: vi.fn<(text: string) => Promise<void>>(),
  notify: vi.fn(),
}));

vi.mock('@/lib/utils', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/utils')>();
  return { ...actual, copyToClipboard: mocks.copy };
});

vi.mock('@/components/ui/notifications', () => ({
  /** useNotifications exposes the notify spy so toast feedback can be asserted. */
  useNotifications: () => ({ notify: mocks.notify, dismiss: vi.fn() }),
}));

import { usePlaygroundActions } from '../usePlaygroundActions';

/** renderActions mounts the playground action hook with inert collaborators. */
function renderActions() {
  return renderHook(() =>
    usePlaygroundActions({
      messages: [],
      setMessages: vi.fn(),
      setCurrentMessage: vi.fn(),
      sendMessage: vi.fn().mockResolvedValue(undefined),
      regenerateMessage: vi.fn().mockResolvedValue(undefined),
      isStreaming: false,
      setExpandedReasonings: vi.fn(),
      setExportDialogOpen: vi.fn(),
    })
  );
}

describe('usePlaygroundActions copy', () => {
  beforeEach(() => {
    mocks.copy.mockReset();
    mocks.notify.mockReset();
  });

  it('copies through the shared clipboard helper and toasts success', async () => {
    mocks.copy.mockResolvedValue(undefined);
    const { result } = renderActions();

    await act(async () => {
      await result.current.handleCopyMessage(0, 'hello');
    });

    expect(mocks.copy).toHaveBeenCalledExactlyOnceWith('hello');
    expect(mocks.notify).toHaveBeenCalledExactlyOnceWith(expect.objectContaining({ type: 'success', title: 'Copied!' }));
  });

  it('toasts an error instead of success when the copy is rejected', async () => {
    mocks.copy.mockRejectedValue(new Error('denied'));
    const { result } = renderActions();

    await act(async () => {
      await result.current.handleCopyMessage(0, 'hello');
    });

    expect(mocks.notify).toHaveBeenCalledExactlyOnceWith(expect.objectContaining({ type: 'error', title: 'Copy failed' }));
  });
});
