import { act, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { EnhancedDataTable } from '../enhanced-data-table';
import { DataTable } from '../data-table';
import { useAuthStore } from '@/lib/stores/auth';
import type { ModernColumnDef } from '@/lib/table';

const responsive = vi.hoisted(() => ({ isMobile: false, isTablet: false }));
vi.mock('@/hooks/useResponsive', () => ({ useResponsive: () => responsive }));
const columns: ModernColumnDef<{ uuid: string; name: string }>[] = [{ accessorKey: 'name', header: 'Name' }];
const rows = [
  { uuid: 'a', name: 'Alpha' },
  { uuid: 'b', name: 'Bravo' },
];

for (const [label, Component] of [
  ['enhanced', EnhancedDataTable],
  ['basic', DataTable],
] as const) {
  describe(`${label} table row selection`, () => {
    beforeEach(() => {
      responsive.isMobile = false;
    });

    it('selects this page, keeps stable IDs across pages/sorts, and supports mixed state', async () => {
      const user = userEvent.setup();
      const props = { columns, data: rows, total: 4, pageSize: 2, selectionScope: 'A' };
      const { rerender } = render(<Component {...props} />);
      await user.click(screen.getByRole('checkbox', { name: 'Select Alpha' }));
      expect(screen.getByRole('checkbox', { name: 'Select this page' })).toBePartiallyChecked();
      rerender(
        <Component
          {...props}
          data={[
            { uuid: 'c', name: 'Charlie' },
            { uuid: 'd', name: 'Delta' },
          ]}
          pageIndex={1}
        />
      );
      expect(screen.getByRole('checkbox', { name: 'Select Charlie' })).not.toBeChecked();
      await user.click(screen.getByRole('button', { name: 'Select this page' }));
      expect(screen.getByText('3 selected')).toBeInTheDocument();
      rerender(<Component {...props} data={[rows[1], rows[0]]} />);
      expect(screen.getByRole('checkbox', { name: 'Select Alpha' })).toBeChecked();
      expect(screen.getByRole('checkbox', { name: 'Select Bravo' })).not.toBeChecked();
      rerender(<Component {...props} selectionScope="B" />);
      expect(screen.getByText('0 selected')).toBeInTheDocument();
    });

    it('selects all matching pages and keeps excluded records unselected after paging', async () => {
      const user = userEvent.setup();
      const props = { columns, data: rows, total: 25, pageSize: 2 };
      const { rerender } = render(<Component {...props} />);
      await user.click(screen.getByRole('button', { name: 'Select all pages' }));
      await user.click(screen.getByRole('checkbox', { name: 'Select Alpha' }));
      rerender(<Component {...props} data={[{ uuid: 'c', name: 'Charlie' }]} pageIndex={1} />);
      expect(screen.getByRole('checkbox', { name: 'Select Charlie' })).toBeChecked();
      expect(screen.getByText('All matching pages selected: 24 items (1 excluded).')).toBeInTheDocument();
      rerender(<Component {...props} />);
      expect(screen.getByRole('checkbox', { name: 'Select Alpha' })).not.toBeChecked();
      await user.click(screen.getByRole('button', { name: 'Clear selection' }));
      expect(screen.getByRole('checkbox', { name: 'Select Bravo' })).not.toBeChecked();
    });

    it('supports mobile selection without triggering the row detail action', async () => {
      responsive.isMobile = true;
      const onRowClick = vi.fn();
      render(<Component columns={columns} data={rows} total={2} onRowClick={onRowClick} />);
      await userEvent.click(screen.getByRole('checkbox', { name: 'Select Alpha' }));
      expect(screen.getByRole('checkbox', { name: 'Select Alpha' })).toBeChecked();
      expect(onRowClick).not.toHaveBeenCalled();
      await userEvent.click(screen.getByText('Alpha'));
      if (label === 'enhanced') expect(onRowClick).toHaveBeenCalledWith(rows[0]);
    });

    it('clears selection when the authenticated principal changes', async () => {
      const original = useAuthStore.getState().user;
      try {
        render(<Component columns={columns} data={rows} total={2} />);
        await userEvent.click(screen.getByRole('checkbox', { name: 'Select Alpha' }));
        expect(screen.getByRole('checkbox', { name: 'Select Alpha' })).toBeChecked();
        act(() =>
          useAuthStore.setState({
            user: { id: 'other', uuid: 'other', username: 'other', role: 10, status: 1, quota: 0, used_quota: 0, group: 'default' },
          })
        );
        expect(screen.getByRole('checkbox', { name: 'Select Alpha' })).not.toBeChecked();
      } finally {
        act(() => useAuthStore.setState({ user: original }));
      }
    });

    it('blocks selection while loading and honestly describes an unknown cross-page count', async () => {
      const props = { columns, data: rows, total: 0, selectionTotal: null };
      const { rerender } = render(<Component {...props} loading />);
      expect(screen.getByRole('checkbox', { name: 'Select Alpha' })).toBeDisabled();
      rerender(<Component {...props} />);
      await userEvent.click(screen.getByRole('button', { name: 'Select all pages' }));
      expect(screen.getByText(/The exact count is resolved before execution/)).toBeInTheDocument();
    });
  });
}
