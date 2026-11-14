import { act, cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

const mocks = vi.hoisted(() => ({ copy: vi.fn<(text: string) => Promise<void>>() }));

vi.mock('@/lib/utils', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/utils')>();
  return { ...actual, copyToClipboard: mocks.copy };
});

import { NameWithId } from '../NameWithId';

const REF_ID = '018f0000-0000-7000-8000-000000000123';

/** revealCopyButton taps the name to show the inline id row and returns its copy button. */
function revealCopyButton() {
  render(<NameWithId name="Primary Channel" refId={REF_ID} idLabel="UUID" />);
  fireEvent.touchEnd(screen.getByRole('button', { name: 'Primary Channel' }));
  return screen.getByRole('button', { name: 'Copy ID' });
}

/** flushCopy lets the awaited clipboard promise settle and React commit the resulting state. */
async function flushCopy() {
  await act(async () => {
    await Promise.resolve();
  });
}

describe('NameWithId copy feedback', () => {
  beforeEach(() => {
    mocks.copy.mockReset();
  });

  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
  });

  it('announces a successful copy of the reference id', async () => {
    mocks.copy.mockResolvedValue(undefined);
    const button = revealCopyButton();

    fireEvent.click(button);
    await flushCopy();

    expect(mocks.copy).toHaveBeenCalledExactlyOnceWith(REF_ID);
    expect(screen.getByRole('status')).toHaveTextContent('Copied!');
    expect(button).toHaveAttribute('title', 'Copied!');
  });

  it('reports a rejected copy as a failure instead of success', async () => {
    mocks.copy.mockRejectedValue(new Error('denied'));
    vi.spyOn(console, 'error').mockImplementation(() => undefined);
    const button = revealCopyButton();

    fireEvent.click(button);
    await flushCopy();

    expect(screen.getByRole('status')).toHaveTextContent('Copy failed');
    expect(screen.getByRole('status')).not.toHaveTextContent('Copied');
    expect(button).toHaveAttribute('title', 'Copy failed');
    expect(button).toHaveAccessibleName('Copy ID');
  });
});
