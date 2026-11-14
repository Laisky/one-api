import { cleanup, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';
import { Dialog, DialogContent, DialogDescription, DialogTitle } from '../dialog';

describe('DialogContent', () => {
  afterEach(() => {
    cleanup();
  });

  it('fits small viewports by scrolling inside the panel without chaining scroll to the page', () => {
    render(
      <Dialog open>
        <DialogContent>
          <DialogTitle>Edit</DialogTitle>
          <DialogDescription>Details</DialogDescription>
        </DialogContent>
      </Dialog>
    );

    const dialog = screen.getByRole('dialog', { name: 'Edit' });
    expect(dialog).toHaveClass('max-h-[calc(100dvh-2rem)]', 'overflow-y-auto', 'overscroll-contain');
  });

  it('lets callers override the default height and overflow', () => {
    render(
      <Dialog open>
        <DialogContent className="max-h-[90vh] overflow-hidden">
          <DialogTitle>Custom</DialogTitle>
          <DialogDescription>Details</DialogDescription>
        </DialogContent>
      </Dialog>
    );

    const dialog = screen.getByRole('dialog', { name: 'Custom' });
    expect(dialog).toHaveClass('max-h-[90vh]', 'overflow-hidden');
    expect(dialog).not.toHaveClass('max-h-[calc(100dvh-2rem)]');
    expect(dialog).not.toHaveClass('overflow-y-auto');
  });

  it('keeps a compact close button with a 24px box and a pseudo-element extended hit area', () => {
    render(
      <Dialog open>
        <DialogContent>
          <DialogTitle>Close me</DialogTitle>
          <DialogDescription>Details</DialogDescription>
        </DialogContent>
      </Dialog>
    );

    const close = screen.getByRole('button', { name: 'Close' });
    expect(close).toHaveClass('size-6', 'inline-flex', 'after:absolute', 'after:-inset-2.5', 'focus-visible:ring-2');
    expect(close.querySelector('svg')).toHaveAttribute('aria-hidden', 'true');
  });
});
