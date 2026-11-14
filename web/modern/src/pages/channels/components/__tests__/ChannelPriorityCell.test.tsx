import { fireEvent, render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

import { ChannelPriorityCell } from '../ChannelPriorityCell';

/** renderCell mounts a focused priority cell holding an edited draft and returns its input and commit spy. */
function renderCell() {
  const onCommit = vi.fn();
  render(<ChannelPriorityCell value={1} ariaLabel="Priority" onCommit={onCommit} />);
  const input = screen.getByRole('spinbutton', { name: 'Priority' });
  fireEvent.change(input, { target: { value: '5' } });
  input.focus();
  return { input, onCommit };
}

describe('ChannelPriorityCell Enter handling', () => {
  it('ignores an IME-confirming Enter reported through isComposing', () => {
    const { input, onCommit } = renderCell();

    fireEvent.keyDown(input, { key: 'Enter', isComposing: true });

    expect(input).toHaveFocus();
    expect(onCommit).not.toHaveBeenCalled();
  });

  it('ignores an IME-confirming Enter reported through keyCode 229', () => {
    const { input, onCommit } = renderCell();

    fireEvent.keyDown(input, { key: 'Enter', keyCode: 229 });

    expect(input).toHaveFocus();
    expect(onCommit).not.toHaveBeenCalled();
  });

  it('commits the edited priority on a plain Enter', () => {
    const { input, onCommit } = renderCell();

    fireEvent.keyDown(input, { key: 'Enter', keyCode: 13 });

    expect(input).not.toHaveFocus();
    expect(onCommit).toHaveBeenCalledExactlyOnceWith(5);
  });
});
