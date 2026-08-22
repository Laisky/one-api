import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { LogCopyButton } from '../LogCopyButton';

describe('LogCopyButton', () => {
  const writeText = vi.fn();

  beforeEach(() => {
    writeText.mockReset();
    Object.defineProperty(navigator, 'clipboard', {
      configurable: true,
      value: { writeText },
    });
  });

  afterEach(() => {
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

  it('handles a rejected clipboard write without leaving an unhandled promise', async () => {
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => undefined);
    writeText.mockRejectedValue(new Error('permission denied'));

    render(<LogCopyButton text="request-123" label="Copy ID" />);
    fireEvent.click(screen.getByRole('button'));

    await waitFor(() => {
      expect(consoleError).toHaveBeenCalledWith('Failed to copy log reference');
    });
  });

  it('uses the caller-provided localized accessible label', () => {
    writeText.mockResolvedValue(undefined);

    render(<LogCopyButton text="request-123" label="Copier l’ID" />);

    expect(screen.getByRole('button', { name: 'Copier l’ID' })).toHaveAttribute('title', 'Copier l’ID');
  });
});
