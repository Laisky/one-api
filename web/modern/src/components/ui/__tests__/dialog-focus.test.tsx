import { createRef } from 'react';
import { cleanup, render, screen, waitFor } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { Dialog, DialogContent, DialogTitle, DialogDescription, DialogTrigger } from '../dialog';

afterEach(cleanup);

/** Modal renders a controlled modal and its native trigger to protect focus and forwarded refs. */
function Modal({
  open,
  contentRef,
  onCloseAutoFocus,
}: {
  open: boolean;
  contentRef?: React.Ref<HTMLDivElement>;
  onCloseAutoFocus?: (event: Event) => void;
}) {
  return (
    <Dialog open={open}>
      <DialogTrigger>Launch modal</DialogTrigger>
      <DialogContent ref={contentRef} onCloseAutoFocus={onCloseAutoFocus}>
        <DialogTitle>Reference modal</DialogTitle>
        <DialogDescription>Focus compatibility</DialogDescription>
        <button>Modal action</button>
      </DialogContent>
    </Dialog>
  );
}

describe('DialogContent compatibility', () => {
  it('preserves the native trigger return for an initially open controlled dialog', async () => {
    const previous = document.createElement('button');
    document.body.append(previous);
    previous.focus();
    const { rerender } = render(<Modal open />);
    expect(screen.getByRole('dialog')).toBeVisible();
    rerender(<Modal open={false} />);
    await waitFor(() => expect(screen.getByRole('button', { name: 'Launch modal' })).toHaveFocus());
    previous.remove();
  });

  it('preserves forwarded object refs and caller autofocus prevention', async () => {
    const ref = createRef<HTMLDivElement>();
    const custom = vi.fn((event: Event) => event.preventDefault());
    const { rerender } = render(<Modal open contentRef={ref} onCloseAutoFocus={custom} />);
    expect(ref.current).toBe(screen.getByRole('dialog'));
    rerender(<Modal open={false} contentRef={ref} onCloseAutoFocus={custom} />);
    await waitFor(() => expect(custom).toHaveBeenCalledTimes(1));
    expect(ref.current).toBeNull();
    expect(screen.getByRole('button', { name: 'Launch modal' })).not.toHaveFocus();
  });

  it('preserves React callback-ref cleanup', () => {
    const release = vi.fn();
    const ref = vi.fn(() => release);
    const { unmount } = render(<Modal open contentRef={ref} />);
    expect(ref).toHaveBeenCalledWith(screen.getByRole('dialog'));
    unmount();
    expect(release).toHaveBeenCalledTimes(1);
  });
});
