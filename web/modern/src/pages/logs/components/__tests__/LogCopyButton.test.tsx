import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { LogCopyButton } from '../LogCopyButton';

describe('LogCopyButton', () => {
  const writeText = vi.fn();
  const originalExecCommand = document.execCommand;

  beforeEach(() => {
    writeText.mockReset();
    Object.defineProperty(navigator, 'clipboard', {
      configurable: true,
      value: { writeText },
    });
    Object.defineProperty(document, 'execCommand', { configurable: true, writable: true, value: vi.fn(() => false) });
  });

  afterEach(() => {
    Object.defineProperty(document, 'execCommand', { configurable: true, writable: true, value: originalExecCommand });
    vi.restoreAllMocks();
  });

  it('copies without triggering the containing log row', () => {
    const onRowClick = vi.fn();
    writeText.mockResolvedValue(undefined);

    render(
      <div onClick={onRowClick}>
        <LogCopyButton text="request-123" label="Copy ID" />
      </div>
    );

    fireEvent.click(screen.getByRole('button'));

    expect(writeText).toHaveBeenCalledWith('request-123');
    expect(onRowClick).not.toHaveBeenCalled();
  });

  it('announces a successful copy', async () => {
    writeText.mockResolvedValue(undefined);

    render(<LogCopyButton text="request-123" label="Copy ID" />);
    fireEvent.click(screen.getByRole('button', { name: 'Copy ID' }));

    await waitFor(() => {
      expect(screen.getByRole('status')).toHaveTextContent('Copied!');
    });
  });

  it('handles a rejected clipboard write without leaving an unhandled promise', async () => {
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => undefined);
    writeText.mockRejectedValue(new Error('permission denied'));

    render(<LogCopyButton text="request-123" label="Copy ID" />);
    fireEvent.click(screen.getByRole('button'));

    await waitFor(() => {
      expect(consoleError).toHaveBeenCalledWith('Failed to copy log reference');
    });
  });

  it('shows and announces a failed copy instead of staying silent', async () => {
    vi.spyOn(console, 'error').mockImplementation(() => undefined);
    writeText.mockRejectedValue(new Error('permission denied'));

    render(<LogCopyButton text="request-123" label="Copy ID" />);
    fireEvent.click(screen.getByRole('button', { name: 'Copy ID' }));
    await act(async () => {
      await Promise.resolve();
    });

    await waitFor(() => {
      expect(screen.getByRole('status')).toHaveTextContent('Copy failed');
    });
    expect(screen.getByRole('button', { name: 'Copy ID' })).toHaveAttribute('data-failed');
  });

  it('falls back to the legacy copy path when the async clipboard API is unavailable', async () => {
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: undefined });
    Object.defineProperty(document, 'execCommand', { configurable: true, writable: true, value: vi.fn(() => true) });

    render(<LogCopyButton text="request-123" label="Copy ID" />);
    fireEvent.click(screen.getByRole('button', { name: 'Copy ID' }));

    await waitFor(() => {
      expect(screen.getByRole('status')).toHaveTextContent('Copied!');
    });
    expect(document.execCommand).toHaveBeenCalledWith('copy');
  });

  it('uses the caller-provided localized accessible label', () => {
    writeText.mockResolvedValue(undefined);

    render(<LogCopyButton text="request-123" label="Copier l’ID" />);

    expect(screen.getByRole('button', { name: 'Copier l’ID' })).toHaveAttribute('title', 'Copier l’ID');
  });
});
