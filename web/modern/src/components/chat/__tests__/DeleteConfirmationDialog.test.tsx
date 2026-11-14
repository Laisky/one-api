import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';

import { DeleteConfirmationDialog } from '../DeleteConfirmationDialog';

/** renderDialog mounts an open delete confirmation with spy callbacks and returns them. */
function renderDialog() {
  const onClose = vi.fn();
  const onConfirm = vi.fn();
  render(<DeleteConfirmationDialog isOpen onClose={onClose} onConfirm={onConfirm} messageRole="user" messagePreview="Hello there" />);
  return { onClose, onConfirm };
}

describe('DeleteConfirmationDialog keyboard handling', () => {
  it('focuses Cancel first and does not delete when Enter is pressed on it', async () => {
    const user = userEvent.setup();
    const { onClose, onConfirm } = renderDialog();

    const cancel = screen.getByRole('button', { name: 'Cancel' });
    expect(cancel).toHaveFocus();

    await user.keyboard('{Enter}');

    expect(onConfirm).not.toHaveBeenCalled();
    expect(onClose).toHaveBeenCalledOnce();
  });

  it('deletes only when the destructive button itself is activated with Enter', async () => {
    const user = userEvent.setup();
    const { onClose, onConfirm } = renderDialog();

    screen.getByRole('button', { name: 'Delete Message' }).focus();
    await user.keyboard('{Enter}');

    expect(onConfirm).toHaveBeenCalledOnce();
    expect(onClose).toHaveBeenCalledOnce();
  });

  it('closes without deleting on Escape', async () => {
    const user = userEvent.setup();
    const { onClose, onConfirm } = renderDialog();

    await user.keyboard('{Escape}');

    expect(onConfirm).not.toHaveBeenCalled();
    expect(onClose).toHaveBeenCalled();
  });
});
