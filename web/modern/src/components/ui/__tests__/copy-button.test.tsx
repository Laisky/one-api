import { act, cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

const mocks = vi.hoisted(() => ({ copy: vi.fn<(text: string) => Promise<void>>() }));

vi.mock('@/lib/utils', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/utils')>();
  return { ...actual, copyToClipboard: mocks.copy };
});

import { CopyButton } from '../copy-button';

/** flushCopy lets the awaited clipboard promise settle and React commit the resulting state. */
async function flushCopy() {
  await act(async () => {
    await Promise.resolve();
  });
}

/** statusRegion returns the always-mounted polite live region that announces copy outcomes. */
function statusRegion() {
  return screen.getByRole('status');
}

describe('CopyButton', () => {
  beforeEach(() => {
    mocks.copy.mockReset();
  });

  afterEach(() => {
    cleanup();
    vi.useRealTimers();
    vi.restoreAllMocks();
  });

  it('keeps an empty live region mounted before any copy so later messages are announced', () => {
    render(<CopyButton text="value" label="Copy value" />);

    expect(statusRegion()).toBeEmptyDOMElement();
    expect(statusRegion().closest('button')).toBeNull();
  });

  it('announces success and calls onCopySuccess when the clipboard write resolves', async () => {
    mocks.copy.mockResolvedValue(undefined);
    const onCopySuccess = vi.fn();
    render(<CopyButton text="value" label="Copy value" onCopySuccess={onCopySuccess} />);

    fireEvent.click(screen.getByRole('button', { name: 'Copy value' }));
    await flushCopy();

    expect(mocks.copy).toHaveBeenCalledExactlyOnceWith('value');
    expect(onCopySuccess).toHaveBeenCalledOnce();
    expect(statusRegion()).toHaveTextContent('Copied!');
    expect(screen.getByRole('button', { name: 'Copy value' })).toHaveAttribute('data-copied');
  });

  it('reports a rejected copy as a failure instead of success', async () => {
    mocks.copy.mockRejectedValue(new Error('denied'));
    vi.spyOn(console, 'error').mockImplementation(() => undefined);
    const onCopySuccess = vi.fn();
    const onCopyError = vi.fn();
    render(<CopyButton text="value" label="Copy value" onCopySuccess={onCopySuccess} onCopyError={onCopyError} />);

    fireEvent.click(screen.getByRole('button', { name: 'Copy value' }));
    await flushCopy();

    expect(onCopySuccess).not.toHaveBeenCalled();
    expect(onCopyError).toHaveBeenCalledExactlyOnceWith(expect.any(Error));
    expect(statusRegion()).toHaveTextContent('Copy failed');
    expect(statusRegion()).not.toHaveTextContent('Copied');
    const button = screen.getByRole('button', { name: 'Copy value' });
    expect(button).toHaveAttribute('data-failed');
    expect(button).not.toHaveAttribute('data-copied');
  });

  it('stays enabled while feedback shows so the copy can be retried', async () => {
    mocks.copy.mockRejectedValueOnce(new Error('denied')).mockResolvedValueOnce(undefined);
    vi.spyOn(console, 'error').mockImplementation(() => undefined);
    render(<CopyButton text="value" label="Copy value" />);
    const button = screen.getByRole('button', { name: 'Copy value' });

    fireEvent.click(button);
    expect(button).toBeEnabled();
    await flushCopy();
    expect(button).toBeEnabled();
    expect(statusRegion()).toHaveTextContent('Copy failed');

    fireEvent.click(button);
    await flushCopy();

    expect(mocks.copy).toHaveBeenCalledTimes(2);
    expect(statusRegion()).toHaveTextContent('Copied!');
    expect(button).toHaveAttribute('data-copied');
    expect(button).not.toHaveAttribute('data-failed');
  });

  it('restarts a single reset timer on repeated clicks and clears the message afterwards', async () => {
    vi.useFakeTimers();
    mocks.copy.mockResolvedValue(undefined);
    render(<CopyButton text="value" label="Copy value" />);
    const button = screen.getByRole('button', { name: 'Copy value' });

    fireEvent.click(button);
    await flushCopy();
    expect(statusRegion()).toHaveTextContent('Copied!');

    await act(async () => {
      vi.advanceTimersByTime(1500);
    });
    fireEvent.click(button);
    await flushCopy();

    await act(async () => {
      vi.advanceTimersByTime(1500);
    });
    expect(statusRegion()).toHaveTextContent('Copied!');
    expect(button).toHaveAttribute('data-copied');

    await act(async () => {
      vi.advanceTimersByTime(600);
    });
    expect(statusRegion()).toBeEmptyDOMElement();
    expect(button).not.toHaveAttribute('data-copied');
  });

  it('changes the announced text on a repeated identical outcome so screen readers repeat it', async () => {
    mocks.copy.mockResolvedValue(undefined);
    render(<CopyButton text="value" label="Copy value" />);
    const button = screen.getByRole('button', { name: 'Copy value' });

    fireEvent.click(button);
    await flushCopy();
    const first = statusRegion().textContent;

    fireEvent.click(button);
    await flushCopy();
    const second = statusRegion().textContent;

    expect(first?.trim()).toBe('Copied!');
    expect(second?.trim()).toBe('Copied!');
    expect(second).not.toBe(first);
  });

  it('does not propagate the click to parent row handlers', async () => {
    mocks.copy.mockResolvedValue(undefined);
    const onRowClick = vi.fn();
    render(
      <div onClick={onRowClick}>
        <CopyButton text="value" label="Copy value" />
      </div>
    );

    fireEvent.click(screen.getByRole('button', { name: 'Copy value' }));
    await flushCopy();

    expect(onRowClick).not.toHaveBeenCalled();
  });
});
