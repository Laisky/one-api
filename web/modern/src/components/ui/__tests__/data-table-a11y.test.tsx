import { cleanup, render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { ModernColumnDef } from '@/lib/table';
import { DataTable } from '../data-table';
import { EnhancedDataTable } from '../enhanced-data-table';

vi.mock('@/hooks/useResponsive', () => ({ useResponsive: () => ({ isMobile: false, isTablet: false }) }));

const rows = [
  { uuid: 'a', name: 'Alpha', status: 'Enabled' },
  { uuid: 'b', name: 'Bravo', status: 'Disabled' },
];
const columns: ModernColumnDef<(typeof rows)[number]>[] = [
  { accessorKey: 'name', header: 'Name' },
  { accessorKey: 'status', header: 'Status' },
];

afterEach(() => {
  cleanup();
});

for (const [label, Component] of [
  ['basic', DataTable],
  ['enhanced', EnhancedDataTable],
] as const) {
  describe(`${label} table scroll region`, () => {
    it('wraps the table in one keyboard-focusable, named scroll region', () => {
      render(<Component columns={columns} data={rows} total={rows.length} enableSelection={false} />);

      const region = screen.getByRole('region', { name: 'Data table' });
      expect(region).toHaveAttribute('tabindex', '0');
      expect(region).toHaveClass('table-scroll');
      expect(region).toContainElement(screen.getByRole('table'));
      // A single scroller: no nested overflow wrapper between the region and the table.
      expect(screen.getByRole('table').parentElement).toBe(region);
    });

    it('accepts a caller-provided accessible name for the region', () => {
      render(<Component columns={columns} data={rows} total={rows.length} enableSelection={false} ariaLabel="Usage logs" />);

      expect(screen.getByRole('region', { name: 'Usage logs' })).toBeInTheDocument();
    });

    it('scopes header cells to their columns', () => {
      render(<Component columns={columns} data={rows} total={rows.length} enableSelection={false} />);

      const headers = screen.getAllByRole('columnheader');
      expect(headers).toHaveLength(2);
      for (const header of headers) expect(header).toHaveAttribute('scope', 'col');
    });

    it('leaves the region out of the tab order while loading', () => {
      render(<Component columns={columns} data={rows} total={rows.length} enableSelection={false} loading />);

      expect(screen.getByRole('region', { name: 'Data table', hidden: true })).toHaveAttribute('inert');
    });
  });
}

describe('EnhancedDataTable row activation', () => {
  it('exposes a keyboard-reachable open control in the first data cell of clickable rows', async () => {
    const user = userEvent.setup();
    const onRowClick = vi.fn();
    render(
      <EnhancedDataTable columns={columns} data={rows} total={rows.length} enableSelection={false} onRowClick={onRowClick} />
    );

    const openButtons = screen.getAllByRole('button', { name: 'Open record' });
    expect(openButtons).toHaveLength(rows.length);
    expect(openButtons[0].closest('td')).toHaveTextContent('Alpha');

    // Tab from the scroll region reaches the first row's open control, and Enter opens it exactly once.
    screen.getByRole('region', { name: 'Data table' }).focus();
    await user.tab();
    expect(document.activeElement).toBe(openButtons[0]);
    await user.keyboard('{Enter}');
    expect(onRowClick).toHaveBeenCalledTimes(1);
    expect(onRowClick).toHaveBeenCalledWith(rows[0]);

    // Pointer clicks on the row body keep working and are not doubled by the button.
    await user.click(screen.getByText('Bravo'));
    expect(onRowClick).toHaveBeenCalledTimes(2);
    expect(onRowClick).toHaveBeenLastCalledWith(rows[1]);
  });

  it('does not render open controls when rows are not clickable', () => {
    render(<EnhancedDataTable columns={columns} data={rows} total={rows.length} enableSelection={false} />);

    const table = screen.getByRole('table');
    expect(within(table).queryByRole('button', { name: 'Open record' })).toBeNull();
  });
});
