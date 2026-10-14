import { cleanup, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { useTableSort } from '@/hooks/useTableSort';
import type { ModernColumnDef } from '@/lib/table';
import { DataTable } from '../data-table';
import { EnhancedDataTable } from '../enhanced-data-table';

vi.mock('@/hooks/useResponsive', () => ({ useResponsive: () => ({ isMobile: false, isTablet: false }) }));
const rows = [{ uuid: 'a', name: 'Alpha' }, { uuid: 'b', name: 'Bravo' }];
const columns: ModernColumnDef<(typeof rows)[number]>[] = [{ accessorKey: 'name', header: 'Name' }];

beforeEach(() => localStorage.clear());
afterEach(() => {
  cleanup();
  localStorage.clear();
});

for (const [label, Component, firstOrder] of [
  ['basic', DataTable, 'desc'],
  ['enhanced', EnhancedDataTable, 'asc'],
] as const) {
  describe(`${label} header sorting persistence`, () => {
    /** SortableTable connects the production persistence hook to the production table header. */
    function SortableTable() {
      const sort = useTableSort(`test-${label}`, 'id');
      return (
        <>
          <output data-testid="sort-pair">{sort.sortBy}:{sort.sortOrder}</output>
          <Component
            columns={columns}
            data={rows}
            total={rows.length}
            enableSelection={false}
            sortBy={sort.sortBy}
            sortOrder={sort.sortOrder}
            onSortChange={(column, order) => {
              sort.setSortBy(column);
              sort.setSortOrder(order);
            }}
          />
        </>
      );
    }

    it('remembers a header click and continues toggling from the restored direction', async () => {
      const user = userEvent.setup();
      const view = render(<SortableTable />);
      await user.click(screen.getByRole('button', { name: 'Name' }));
      expect(screen.getByTestId('sort-pair')).toHaveTextContent(`name:${firstOrder}`);
      view.unmount();

      render(<SortableTable />);
      expect(screen.getByTestId('sort-pair')).toHaveTextContent(`name:${firstOrder}`);
      await user.click(screen.getByRole('button', { name: 'Name' }));
      expect(screen.getByTestId('sort-pair')).toHaveTextContent(`name:${firstOrder === 'asc' ? 'desc' : 'asc'}`);
    });
  });
}
