import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { useState } from 'react';
import { describe, expect, it, vi } from 'vitest';

import { MultiSelectCombobox } from '../multi-select-combobox';
import { buildComboboxItems, mergeComboboxOptions } from '../multi-select-combobox-items';

/**
 * Harness renders a controlled MultiSelectCombobox inside a form so tests can
 * observe selection changes and detect accidental form submission.
 */
function Harness({
  options,
  initial = [],
  allowCustom = true,
  onSubmit = vi.fn(),
}: {
  options: string[];
  initial?: string[];
  allowCustom?: boolean;
  onSubmit?: () => void;
}) {
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
        allowCustom={allowCustom}
        placeholder="Pick a group"
        emptyLabel="Nothing matches"
        createLabel={(value) => `Add "${value}"`}
      />
      <output data-testid="selected">{selected.join(',')}</output>
    </form>
  );
}

/** optionLabels returns the visible text of every dropdown row in order. */
const optionLabels = () => screen.queryAllByRole('option').map((option) => option.textContent ?? '');

describe('mergeComboboxOptions', () => {
  it('appends selected custom values and drops blanks and duplicates', () => {
    expect(mergeComboboxOptions(['default', ' vip ', 'vip', ''], ['custom', 'default', ' '])).toEqual(['default', 'vip', 'custom']);
  });
});

describe('buildComboboxItems', () => {
  const values = ['default', 'vip', 'svip', 'VIP-2', 'team'];

  it('lists every value when the query is empty', () => {
    expect(buildComboboxItems(values, '   ', true)).toEqual(values.map((value) => ({ kind: 'option', value })));
  });

  it('ranks exact, prefix, then substring matches case-insensitively', () => {
    expect(buildComboboxItems(values, 'Vip', false).map((item) => item.value)).toEqual(['vip', 'VIP-2', 'svip']);
  });

  it('offers a create row only when no value equals the trimmed query', () => {
    expect(buildComboboxItems(values, ' vi ', true).at(-1)).toEqual({ kind: 'create', value: 'vi' });
    expect(buildComboboxItems(values, 'vip', true).some((item) => item.kind === 'create')).toBe(false);
    expect(buildComboboxItems(values, 'vi', false).some((item) => item.kind === 'create')).toBe(false);
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

    // Prefix match ranks before substring match; the create row comes last.
    expect(optionLabels()).toEqual(['vip', 'svip', 'Add "vi"']);
    expect(input).toHaveAttribute('aria-activedescendant', screen.getByRole('option', { name: 'vip' }).id);

    await user.keyboard('{Enter}');

    expect(screen.getByTestId('selected')).toHaveTextContent(/^vip$/);
    expect(input).toHaveValue('');
    expect(onSubmit).not.toHaveBeenCalled();
  });

  it('navigates with arrow keys and adds a custom value from the create row', async () => {
    const user = userEvent.setup();
    render(<Harness options={['default', 'vip']} />);

    const input = screen.getByRole('combobox', { name: 'Groups' });
    await user.click(input);
    await user.type(input, 'vi');
    await user.keyboard('{ArrowDown}{Enter}');

    expect(screen.getByTestId('selected')).toHaveTextContent(/^vi$/);
    // The new custom value is now listed as a selected option.
    await user.type(input, 'vi');
    expect(screen.getByRole('option', { name: 'vi' })).toHaveAttribute('aria-selected', 'true');
  });

  it('shows the empty label when nothing matches and custom values are disabled', async () => {
    const user = userEvent.setup();
    const onSubmit = vi.fn();
    render(<Harness options={['default', 'vip']} allowCustom={false} onSubmit={onSubmit} />);

    const input = screen.getByRole('combobox', { name: 'Groups' });
    await user.click(input);
    await user.type(input, 'zzz{Enter}');

    expect(screen.getByText('Nothing matches')).toBeInTheDocument();
    expect(screen.queryAllByRole('option')).toHaveLength(0);
    expect(screen.getByTestId('selected')).toHaveTextContent(/^$/);
    expect(onSubmit).not.toHaveBeenCalled();
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
