import { fireEvent, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';

import { NameWithId } from '../NameWithId';

describe('NameWithId', () => {
  it('shows the id when the name is clicked', async () => {
    const user = userEvent.setup();

    render(<NameWithId name="Primary Channel" refId="018f0000-0000-7000-8000-000000000123" idLabel="UUID" />);

    await user.click(screen.getByRole('button', { name: 'Primary Channel' }));

    expect(screen.getAllByText('UUID: 018f0000-0000-7000-8000-000000000123').length).toBeGreaterThan(0);
  });

  it('does not bubble name clicks into parent row actions', async () => {
    const user = userEvent.setup();
    const onParentClick = vi.fn();

    render(
      <div onClick={onParentClick}>
        <NameWithId name="Primary Channel" refId="018f0000-0000-7000-8000-000000000123" idLabel="UUID" />
      </div>,
    );

    await user.click(screen.getByRole('button', { name: 'Primary Channel' }));

    expect(onParentClick).not.toHaveBeenCalled();
  });

  it('shows the id when the name receives a touch tap', () => {
    render(<NameWithId name="Primary Channel" refId="018f0000-0000-7000-8000-000000000123" idLabel="UUID" />);

    fireEvent.touchEnd(screen.getByRole('button', { name: 'Primary Channel' }));

    expect(screen.getAllByText('UUID: 018f0000-0000-7000-8000-000000000123').length).toBeGreaterThan(0);
  });

  it('does not bubble touch taps into parent row actions', () => {
    const onParentTouchEnd = vi.fn();

    render(
      <div onTouchEnd={onParentTouchEnd}>
        <NameWithId name="Primary Channel" refId="018f0000-0000-7000-8000-000000000123" idLabel="UUID" />
      </div>,
    );

    fireEvent.touchEnd(screen.getByRole('button', { name: 'Primary Channel' }));

    expect(onParentTouchEnd).not.toHaveBeenCalled();
  });
});
