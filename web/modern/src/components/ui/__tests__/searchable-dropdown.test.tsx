import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it } from 'vitest';

import { SearchableDropdown, type SearchOption } from '../searchable-dropdown';

const UUID = '018f0000-0000-7000-8000-000000000123';

const options: SearchOption[] = [
  { key: UUID, value: 'alice', text: 'alice' },
  { key: 'other-uuid', value: 'bob', text: 'bob' },
];

/**
 * openAndType opens the dropdown and types a query into its search input.
 */
async function openAndType(query: string) {
  const user = userEvent.setup();
  await user.click(screen.getByRole('combobox'));
  await user.type(screen.getByPlaceholderText('Find'), query);
  return user;
}

describe('SearchableDropdown', () => {
  it('keeps server-provided results when the parent owns the search', async () => {
    render(<SearchableDropdown options={options} remoteFiltered searchPlaceholder="Find" onSearchChange={() => {}} />);

    await openAndType(UUID);

    // The backend matched "alice" by UUID; the dropdown must not filter it out
    // just because the UUID is not part of the visible value.
    expect(screen.getByText('alice')).toBeInTheDocument();
  });

  it('filters locally when the caller owns the options', async () => {
    render(<SearchableDropdown options={options} searchPlaceholder="Find" />);

    await openAndType('ali');

    expect(screen.getByText('alice')).toBeInTheDocument();
    expect(screen.queryByText('bob')).not.toBeInTheDocument();
  });

  it('matches local options on their extra keywords', async () => {
    const withKeywords: SearchOption[] = [{ key: UUID, value: 'alice', text: 'alice', keywords: [UUID] }, options[1]];

    render(<SearchableDropdown options={withKeywords} searchPlaceholder="Find" />);

    await openAndType(UUID);

    expect(screen.getByText('alice')).toBeInTheDocument();
    expect(screen.queryByText('bob')).not.toBeInTheDocument();
  });
});
