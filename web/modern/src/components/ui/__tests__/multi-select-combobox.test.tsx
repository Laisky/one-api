import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { useState } from 'react';
import { describe, expect, it, vi } from 'vitest';

import { MultiSelectCombobox } from '../multi-select-combobox';
import { filterComboboxValues, mergeComboboxOptions } from '../multi-select-combobox-items';

/**
 * Harness renders a controlled MultiSelectCombobox inside a form so tests can
 * observe selection changes and detect accidental form submission.
 */
function Harness({ options, initial = [], onSubmit = vi.fn() }: { options: string[]; initial?: string[]; onSubmit?: () => void }) {
  const [selected, setSelected] = useState<string[]>(initial);
  return (
    <form
      onSubmit={(event) => {
        event.preventDefault();
        onSubmit();
      }}
    >
      <MultiSelectCombobox
        aria-label="Groups"
        options={options}
        selected={selected}
        onAdd={(value) => setSelected((prev) => [...prev, value])}
        onRemove={(value) => setSelected((prev) => prev.filter((item) => item !== value))}
        placeholder="Pick a group"
        emptyLabel="Nothing matches"
      />
      <output data-testid="selected">{selected.join(',')}</output>
    </form>
  );
}

/** optionLabels returns the visible text of every dropdown row in order. */
const optionLabels = () => screen.queryAllByRole('option').map((option) => option.textContent ?? '');

describe('mergeComboboxOptions', () => {
  it('appends selected values missing from options and drops blanks and duplicates', () => {
    expect(mergeComboboxOptions(['default', ' vip ', 'vip', ''], ['legacy', 'default', ' '])).toEqual(['default', 'vip', 'legacy']);
  });
});

describe('filterComboboxValues', () => {
  const values = ['default', 'vip', 'svip', 'VIP-2', 'team'];

  it('lists every value when the query is empty', () => {
    expect(filterComboboxValues(values, '   ')).toEqual(values);
  });

  it('ranks exact, prefix, then substring matches case-insensitively', () => {
    expect(filterComboboxValues(values, 'Vip')).toEqual(['vip', 'VIP-2', 'svip']);
  });

  it('never returns the typed text itself when it is not a known value', () => {
    expect(filterComboboxValues(values, 'vi')).toEqual(['vip', 'VIP-2', 'svip']);
    expect(filterComboboxValues(values, 'partners')).toEqual([]);
  });
});

describe('MultiSelectCombobox', () => {
  it('shows every option on click without typing anything', async () => {
    const user = userEvent.setup();
    render(<Harness options={['default', 'vip', 'svip']} initial={['default']} />);

    const input = screen.getByRole('combobox', { name: 'Groups' });
    expect(input).toHaveAttribute('aria-expanded', 'false');
    expect(screen.queryByRole('listbox')).not.toBeInTheDocument();

    await user.click(input);

    expect(input).toHaveAttribute('aria-expanded', 'true');
    expect(optionLabels()).toEqual(['default', 'vip', 'svip']);
    expect(screen.getByRole('option', { name: 'default' })).toHaveAttribute('aria-selected', 'true');
    expect(screen.getByRole('option', { name: 'vip' })).toHaveAttribute('aria-selected', 'false');
  });

  it('opens when focused with the keyboard', async () => {
    const user = userEvent.setup();
    render(<Harness options={['default', 'vip']} />);

    await user.tab();

    expect(screen.getByRole('combobox', { name: 'Groups' })).toHaveFocus();
    expect(optionLabels()).toEqual(['default', 'vip']);
  });

  it('toggles options by click and keeps the dropdown open for further picks', async () => {
    const user = userEvent.setup();
    render(<Harness options={['default', 'vip', 'svip']} initial={['default']} />);

    const input = screen.getByRole('combobox', { name: 'Groups' });
    await user.click(input);
    await user.click(screen.getByRole('option', { name: 'vip' }));

    expect(screen.getByTestId('selected')).toHaveTextContent('default,vip');
    expect(input).toHaveFocus();
    expect(screen.getByRole('option', { name: 'vip' })).toHaveAttribute('aria-selected', 'true');

    await user.click(screen.getByRole('option', { name: 'default' }));

    expect(screen.getByTestId('selected')).toHaveTextContent(/^vip$/);
  });

  it('filters while typing and Enter picks the best match without submitting the form', async () => {
    const user = userEvent.setup();
    const onSubmit = vi.fn();
    render(<Harness options={['default', 'svip', 'vip']} onSubmit={onSubmit} />);

    const input = screen.getByRole('combobox', { name: 'Groups' });
    await user.click(input);
    await user.type(input, 'vi');

    // Prefix match ranks before substring match, and the typed text is never offered as a value.
    expect(optionLabels()).toEqual(['vip', 'svip']);
    expect(input).toHaveAttribute('aria-activedescendant', screen.getByRole('option', { name: 'vip' }).id);

    await user.keyboard('{Enter}');

    expect(screen.getByTestId('selected')).toHaveTextContent(/^vip$/);
    expect(input).toHaveValue('');
    expect(onSubmit).not.toHaveBeenCalled();
  });

  it('navigates with arrow keys, wrapping at both ends', async () => {
    const user = userEvent.setup();
    render(<Harness options={['default', 'vip', 'svip']} />);

    const input = screen.getByRole('combobox', { name: 'Groups' });
    await user.click(input);
    await user.keyboard('{ArrowUp}');
    expect(input).toHaveAttribute('aria-activedescendant', screen.getByRole('option', { name: 'svip' }).id);

    await user.keyboard('{ArrowDown}{ArrowDown}{Enter}');

    expect(screen.getByTestId('selected')).toHaveTextContent(/^vip$/);
  });

  it('cannot add a value that is not one of the options', async () => {
    const user = userEvent.setup();
    const onSubmit = vi.fn();
    render(<Harness options={['default', 'vip']} initial={['default']} onSubmit={onSubmit} />);

    const input = screen.getByRole('combobox', { name: 'Groups' });
    await user.click(input);
    await user.type(input, 'partners{Enter}');

    expect(screen.getByText('Nothing matches')).toBeInTheDocument();
    expect(screen.queryAllByRole('option')).toHaveLength(0);
    // Escape clears the highlight; Enter on unmatched text must still add nothing.
    await user.keyboard('{Escape}{Enter}{ArrowDown}{Enter}');
    expect(screen.getByTestId('selected')).toHaveTextContent(/^default$/);
    expect(onSubmit).not.toHaveBeenCalled();
  });

  it('lists a selected value missing from options so it can be deselected, but not re-added', async () => {
    const user = userEvent.setup();
    render(<Harness options={['default', 'vip']} initial={['default', 'legacy']} />);

    const input = screen.getByRole('combobox', { name: 'Groups' });
    await user.click(input);
    expect(screen.getByRole('option', { name: 'legacy' })).toHaveAttribute('aria-selected', 'true');

    await user.click(screen.getByRole('option', { name: 'legacy' }));

    expect(screen.getByTestId('selected')).toHaveTextContent(/^default$/);
    expect(screen.queryByRole('option', { name: 'legacy' })).not.toBeInTheDocument();
    await user.type(input, 'legacy{Enter}');
    expect(screen.getByTestId('selected')).toHaveTextContent(/^default$/);
  });

  it('closes on Escape and reopens on the next click', async () => {
    const user = userEvent.setup();
    render(<Harness options={['default', 'vip']} />);

    const input = screen.getByRole('combobox', { name: 'Groups' });
    await user.click(input);
    expect(screen.getByRole('listbox')).toBeInTheDocument();

    await user.keyboard('{Escape}');
    expect(input).toHaveAttribute('aria-expanded', 'false');

    await user.click(input);
    expect(within(screen.getByRole('listbox')).getAllByRole('option')).toHaveLength(2);
  });
});
