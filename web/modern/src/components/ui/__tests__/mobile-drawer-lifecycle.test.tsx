import { StrictMode } from 'react';
import { cleanup, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';
import { MobileDrawer } from '../mobile-drawer';

/** Drawers renders independently controlled modal lifetimes to cover arbitrary close order. */
function Drawers({ a, b }: { a: boolean; b: boolean }) {
  return (
    <>
      <MobileDrawer isOpen={a} onClose={() => {}} title="Drawer A">
        <button>A action</button>
      </MobileDrawer>
      <MobileDrawer isOpen={b} onClose={() => {}} title="Drawer B">
        <button>B action</button>
      </MobileDrawer>
    </>
  );
}

afterEach(() => {
  cleanup();
  document.getElementById('root')?.remove();
});

describe('MobileDrawer shared modality', () => {
  it.each([true, false])('keeps the app inert until both owners release, first drawer closes: %s', (firstCloses) => {
    const root = document.createElement('div');
    root.id = 'root';
    document.body.append(root);
    const { rerender, unmount } = render(<Drawers a b={false} />);
    rerender(<Drawers a b />);
    expect(root).toHaveAttribute('inert');
    rerender(<Drawers a={!firstCloses} b={firstCloses} />);
    expect(root).toHaveAttribute('inert');
    expect(screen.getByRole('dialog', { name: firstCloses ? 'Drawer B' : 'Drawer A' })).toBeVisible();
    unmount();
    expect(root).not.toHaveAttribute('inert');
    expect(document.body.style.position).toBe('');
  });

  it('exposes the top drawer when two owners open in the same commit', () => {
    const { unmount } = render(<Drawers a b />);
    const top = screen.getByRole('dialog', { name: 'Drawer B' });
    expect(top).not.toHaveAttribute('aria-hidden', 'true');
    expect(top).not.toHaveAttribute('inert');
    expect(top).toContainElement(document.activeElement as HTMLElement);
    const lower = screen.getByText('A action').closest('[role="dialog"]');
    expect(lower).toHaveAttribute('inert');
    unmount();
  });

  it('preserves pre-existing inert through StrictMode and interrupted/repeated lifetimes', () => {
    const root = document.createElement('div');
    root.id = 'root';
    root.setAttribute('inert', 'prior');
    document.body.append(root);
    const { rerender, unmount } = render(
      <StrictMode>
        <Drawers a b />
      </StrictMode>
    );
    rerender(
      <StrictMode>
        <Drawers a={false} b={false} />
      </StrictMode>
    );
    expect(root).toHaveAttribute('inert', 'prior');
    rerender(
      <StrictMode>
        <Drawers a b />
      </StrictMode>
    );
    unmount();
    expect(root).toHaveAttribute('inert', 'prior');
    expect(document.body.style.position).toBe('');
  });
});
