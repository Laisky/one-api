import { act, renderHook } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { useClipboardManager } from '../useClipboardManager';

describe('useClipboardManager', () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.runOnlyPendingTimers();
    vi.useRealTimers();
  });

  it('marks a token as copied and clears the flag after three seconds', () => {
    const { result } = renderHook(() => useClipboardManager());

    act(() => {
      result.current.handleCopySuccess('018f0000-0000-7000-8000-000000000042');
    });

    expect(result.current.copiedTokens['018f0000-0000-7000-8000-000000000042']).toBe(true);

    act(() => {
      vi.advanceTimersByTime(3000);
    });

    expect(result.current.copiedTokens['018f0000-0000-7000-8000-000000000042']).toBeUndefined();
  });

  it('records the token that needs manual copying and clears it on demand', () => {
    const { result } = renderHook(() => useClipboardManager());
    const token = { ref: '018f0000-0000-7000-8000-000000000007', key: 'manual-key' };

    act(() => {
      result.current.handleCopyFailure(token);
    });

    expect(result.current.manualCopyToken).toEqual(token);

    act(() => {
      result.current.clearManualCopyToken();
    });

    expect(result.current.manualCopyToken).toBeNull();
  });
});
