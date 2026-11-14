import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { copyToClipboard } from '@/lib/clipboard';

const originalClipboard = Object.getOwnPropertyDescriptor(navigator, 'clipboard');
const originalExecCommand = document.execCommand;

/** setClipboard replaces navigator.clipboard for one test case. */
function setClipboard(value: unknown) {
  Object.defineProperty(navigator, 'clipboard', { configurable: true, value });
}

/** setExecCommand replaces document.execCommand for one test case. */
function setExecCommand(impl: (command: string) => boolean) {
  Object.defineProperty(document, 'execCommand', { configurable: true, writable: true, value: vi.fn(impl) });
}

describe('copyToClipboard', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  afterEach(() => {
    if (originalClipboard) {
      Object.defineProperty(navigator, 'clipboard', originalClipboard);
    } else {
      Reflect.deleteProperty(navigator, 'clipboard');
    }
    Object.defineProperty(document, 'execCommand', { configurable: true, writable: true, value: originalExecCommand });
  });

  it('uses the async Clipboard API when it is available', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    setClipboard({ writeText });
    setExecCommand(() => true);

    await expect(copyToClipboard('hello')).resolves.toBeUndefined();

    expect(writeText).toHaveBeenCalledWith('hello');
    expect(document.execCommand).not.toHaveBeenCalled();
  });

  it('falls back to execCommand when the async write is rejected', async () => {
    setClipboard({ writeText: vi.fn().mockRejectedValue(new DOMException('denied', 'NotAllowedError')) });
    let copiedValue = '';
    setExecCommand(() => {
      copiedValue = (document.activeElement as HTMLTextAreaElement | null)?.value ?? '';
      return true;
    });

    await expect(copyToClipboard('fallback')).resolves.toBeUndefined();

    expect(document.execCommand).toHaveBeenCalledWith('copy');
    expect(copiedValue).toBe('fallback');
    expect(document.querySelector('textarea')).toBeNull();
  });

  it('falls back to execCommand on insecure origins without navigator.clipboard', async () => {
    setClipboard(undefined);
    setExecCommand(() => true);

    await expect(copyToClipboard('plain-http')).resolves.toBeUndefined();
    expect(document.execCommand).toHaveBeenCalledWith('copy');
  });

  it('rejects when both the async API and the fallback fail', async () => {
    setClipboard({ writeText: vi.fn().mockRejectedValue(new Error('denied')) });
    setExecCommand(() => false);

    await expect(copyToClipboard('nope')).rejects.toThrow('Copy to clipboard failed');
  });

  it('rejects when the clipboard API is missing and execCommand fails', async () => {
    setClipboard(undefined);
    setExecCommand(() => false);

    await expect(copyToClipboard('nope')).rejects.toThrow('clipboard API unavailable');
  });

  it('keeps the helper textarea inside an open dialog so focus traps keep the selection', async () => {
    setClipboard(undefined);
    const dialog = document.createElement('div');
    dialog.setAttribute('role', 'dialog');
    const trigger = document.createElement('button');
    dialog.append(trigger);
    document.body.append(dialog);
    trigger.focus();
    let parentRole: string | null = null;
    setExecCommand(() => {
      parentRole = (document.activeElement?.parentElement as HTMLElement | null)?.getAttribute('role') ?? null;
      return true;
    });

    await copyToClipboard('in-dialog');

    expect(parentRole).toBe('dialog');
    expect(document.activeElement).toBe(trigger);
    dialog.remove();
  });
});
