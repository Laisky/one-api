import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { useEffect } from 'react';
import { DataTable } from '../data-table';
import { EnhancedDataTable } from '../enhanced-data-table';
import { Button } from '../button';
import { ResponsiveActionGroup } from '../responsive-action-group';
import { getMobileColumnLabel, getMobileField } from '../mobile-table';
import { getMobileRecordId } from '../mobile-table-identity';
import { mobileTableTranslations } from '@/i18n/locales/mobile-table';
import type { ModernColumnDef } from '@/lib/table';

const responsive = vi.hoisted(() => ({ isMobile: true, isTablet: false }));
vi.mock('@/hooks/useResponsive', () => ({ useResponsive: () => responsive }));

/** UserRecord models the information density and trailing display-only action column in the reported screenshot. */
interface UserRecord {
  uuid: string;
  username: string;
  display_name: string;
  group: string;
  used_quota: number;
  quota: number;
  created_at: string;
}

const rows: UserRecord[] = [
  { uuid: 'a', username: 'alpha', display_name: 'Alpha User', group: 'default', used_quota: 12, quota: 345, created_at: '2026-09-29' },
  { uuid: 'b', username: 'bravo', display_name: 'Bravo User', group: 'default', used_quota: 34, quota: 567, created_at: '2026-09-28' },
];

/** makeColumns retains real cell callbacks, including the legacy localized action header without an accessor or ID. */
function makeColumns(onDelete = vi.fn(), actionLabel = 'Actions'): ModernColumnDef<UserRecord>[] {
  return [
    { accessorKey: 'username', header: 'Username' },
    { accessorKey: 'display_name', header: 'Display Name' },
    { header: 'Role', cell: () => 'User' },
    { header: 'Status', cell: () => 'Enabled' },
    { accessorKey: 'group', header: 'Group' },
    { accessorKey: 'used_quota', header: 'Used Quota' },
    { accessorKey: 'quota', header: 'Remaining Quota' },
    { accessorKey: 'created_at', header: 'Registered' },
    {
      header: actionLabel,
      cell: ({ row }) => (
        <ResponsiveActionGroup>
          <Button type="button">Edit</Button>
          <Button type="button" variant="destructive" onClick={() => onDelete(row.original.uuid)}>Delete</Button>
        </ResponsiveActionGroup>
      ),
    },
  ];
}

for (const [kind, Component, defaultOrder] of [
  ['basic', DataTable, 'desc'],
  ['enhanced', EnhancedDataTable, 'asc'],
] as const) {
  describe(`${kind} mobile list regressions`, () => {
    beforeEach(() => { responsive.isMobile = true; responsive.isTablet = false; });

    it('keeps secondary fields and dangerous actions out of the default browsing surface without dropping them', async () => {
      const onDelete = vi.fn();
      render(<Component columns={makeColumns(onDelete)} data={rows} total={2} />);
      const card = screen.getByRole('article', { name: 'alpha' });
      expect(within(card).getByText('Enabled')).toBeVisible();
      expect(within(card).getByText('Remaining Quota')).not.toBeVisible();
      expect(within(card).queryByRole('button', { name: 'Delete' })).not.toBeInTheDocument();
      await userEvent.click(within(card).getByRole('button', { name: 'Details' }));
      expect(within(card).getByText('Remaining Quota')).toBeVisible();
      expect(within(card).getByText('345')).toBeVisible();
      await userEvent.click(within(card).getByRole('button', { name: 'Actions' }));
      await userEvent.click(within(card).getByRole('button', { name: 'Delete' }));
      expect(onDelete).toHaveBeenCalledExactlyOnceWith('a');
    });

    it('preserves translated labels after sortable headers become render functions', () => {
      render(<Component columns={makeColumns()} data={rows} total={2} sortBy="username" onSortChange={vi.fn()} />);
      const card = screen.getByRole('article', { name: 'alpha' });
      expect(within(card).getByText('Display Name')).toBeVisible();
      expect(within(card).queryByText('display_name')).not.toBeInTheDocument();
      expect(screen.getByRole('combobox', { name: 'Sort by' })).toHaveValue('username');
    });

    it('provides mobile sorting with the same first-sort direction as desktop and excludes unsortable fields', async () => {
      const onSortChange = vi.fn();
      const columns = makeColumns();
      columns[1] = { ...columns[1], enableSorting: false };
      render(<Component columns={columns} data={rows} total={2} onSortChange={onSortChange} />);
      const sort = screen.getByRole('combobox', { name: 'Sort by' });
      expect(within(sort).queryByRole('option', { name: 'Display Name' })).not.toBeInTheDocument();
      expect(within(sort).queryByRole('option', { name: 'Actions' })).not.toBeInTheDocument();
      await userEvent.selectOptions(sort, 'username');
      expect(onSortChange).toHaveBeenCalledExactlyOnceWith('username', defaultOrder);
    });

    it('toggles sort direction without treating the control as a record click', async () => {
      const onSortChange = vi.fn();
      render(<Component columns={makeColumns()} data={rows} total={2} sortBy="username" sortOrder="asc" onSortChange={onSortChange} />);
      await userEvent.click(screen.getByRole('button', { name: 'Sort descending' }));
      expect(onSortChange).toHaveBeenCalledExactlyOnceWith('username', 'desc');
    });

    it('keeps disclosures bound to stable records after sorting and closes them when the page changes', async () => {
      const props = { columns: makeColumns(), data: rows, total: 4, pageSize: 2 };
      const { rerender } = render(<Component {...props} />);
      await userEvent.click(within(screen.getByRole('article', { name: 'alpha' })).getByRole('button', { name: 'Actions' }));
      rerender(<Component {...props} data={[rows[1], rows[0]]} />);
      expect(within(screen.getByRole('article', { name: 'alpha' })).getByRole('button', { name: 'Delete' })).toBeVisible();
      expect(within(screen.getByRole('article', { name: 'bravo' })).queryByRole('button', { name: 'Delete' })).not.toBeInTheDocument();
      rerender(<Component {...props} pageIndex={1} />);
      expect(within(screen.getByRole('article', { name: 'alpha' })).queryByRole('button', { name: 'Delete' })).not.toBeInTheDocument();
    });

    it('keeps selection and disclosure controls independent from record navigation', async () => {
      const onRowClick = vi.fn();
      render(<Component columns={makeColumns()} data={rows} total={2} onRowClick={onRowClick} />);
      const card = screen.getByRole('article', { name: 'alpha' });
      await userEvent.click(within(card).getByRole('checkbox', { name: 'Select alpha' }));
      expect(within(card).getByRole('checkbox', { name: 'Select alpha' })).toBeChecked();
      await userEvent.click(within(card).getByRole('button', { name: 'Details' }));
      await userEvent.click(within(card).getByRole('button', { name: 'Actions' }));
      expect(onRowClick).not.toHaveBeenCalled();
      if (kind === 'enhanced') {
        await userEvent.click(within(card).getByRole('button', { name: 'Open record' }));
        expect(onRowClick).toHaveBeenCalledExactlyOnceWith(rows[0]);
      }
    });

    it('closes the inline action panel with Escape and restores focus to its trigger', async () => {
      render(<Component columns={makeColumns()} data={rows} total={2} />);
      const card = screen.getByRole('article', { name: 'alpha' });
      const trigger = within(card).getByRole('button', { name: 'Actions' });
      await userEvent.click(trigger);
      within(card).getByRole('button', { name: 'Edit' }).focus();
      await userEvent.keyboard('{Escape}');
      expect(trigger).toHaveAttribute('aria-expanded', 'false');
      expect(trigger).toHaveFocus();
    });

    it('marks retained mobile content inert during loading and disables navigation, sorting, and disclosure controls', async () => {
      render(<Component columns={makeColumns()} data={rows} total={40} pageSize={10} sortBy="username" onSortChange={vi.fn()} loading />);
      expect(screen.getByRole('article', { name: 'alpha' }).closest('ul')).toHaveAttribute('inert');
      expect(screen.getAllByRole('button', { name: 'Details' })[0]).toBeDisabled();
      expect(screen.getByRole('combobox', { name: 'Sort by' })).toBeDisabled();
      expect(screen.getByRole('button', { name: 'Next page' })).toBeDisabled();
    });

    it('retains zero-based server pagination and resets to page zero on a page-size change', async () => {
      const onPageChange = vi.fn();
      const onPageSizeChange = vi.fn();
      render(<Component columns={makeColumns()} data={rows} total={40} pageSize={10} onPageChange={onPageChange} onPageSizeChange={onPageSizeChange} />);
      expect(screen.getByRole('button', { name: 'Previous page' })).toBeDisabled();
      await userEvent.click(screen.getByRole('button', { name: 'Next page' }));
      expect(onPageChange).toHaveBeenCalledWith(1, 10);
      await userEvent.click(screen.getByRole('combobox', { name: 'Rows per page:' }));
      await userEvent.click(screen.getByRole('option', { name: '20' }));
      expect(onPageSizeChange).toHaveBeenCalledWith(20);
      expect(onPageChange).toHaveBeenLastCalledWith(0, 20);
    });

    it('does not mount a second copy of interactive cells when details are disclosed', async () => {
      const mounted = vi.fn();
      /** MeasuredCell records mounts without changing the cell's own state or behavior. */
      function MeasuredCell() { useEffect(() => { mounted(); }, []); return <span>Measured quota</span>; }
      const columns = makeColumns();
      columns[6] = { accessorKey: 'quota', header: 'Remaining Quota', cell: () => <MeasuredCell /> };
      render(<Component columns={columns} data={rows} total={2} />);
      const before = mounted.mock.calls.length;
      expect(before).toBeGreaterThan(0);
      const card = screen.getByRole('article', { name: 'alpha' });
      await userEvent.click(within(card).getByRole('button', { name: 'Details' }));
      await userEvent.click(within(card).getByRole('button', { name: 'Less' }));
      expect(mounted).toHaveBeenCalledTimes(before);
    });

    it('keeps localized trailing display columns reachable without English label matching', async () => {
      render(<Component columns={makeColumns(vi.fn(), '操作')} data={rows} total={2} />);
      const card = screen.getByRole('article', { name: 'alpha' });
      await userEvent.click(within(card).getByRole('button', { name: '操作' }));
      expect(within(card).getByRole('button', { name: 'Edit' })).toBeVisible();
    });

    it('keeps the desktop table and exposes its server sort direction accessibly', () => {
      responsive.isMobile = false;
      render(<Component columns={makeColumns()} data={rows} total={2} sortBy="username" sortOrder="asc" onSortChange={vi.fn()} />);
      expect(screen.getByRole('table')).toBeInTheDocument();
      expect(screen.queryByRole('article')).not.toBeInTheDocument();
      expect(screen.getByRole('columnheader', { name: 'Username' })).toHaveAttribute('aria-sort', 'ascending');
      expect(screen.getAllByRole('button', { name: 'Delete' })).toHaveLength(2);
    });
  });
}

describe('mobile contracts', () => {
  beforeEach(() => { responsive.isMobile = true; responsive.isTablet = false; });

  it('preserves caller-hidden fields and custom keyset pagination', async () => {
    render(<EnhancedDataTable columns={makeColumns()} data={rows} total={2} hideColumnsOnMobile={['quota']} paginationSlot={<button type="button">Continue cursor</button>} />);
    const card = screen.getByRole('article', { name: 'alpha' });
    await userEvent.click(within(card).getByRole('button', { name: 'Details' }));
    expect(within(card).queryByText('Remaining Quota')).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Continue cursor' })).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Next page' })).not.toBeInTheDocument();
  });

  it('provides an explicit mobile entry for formerly hover-only actions', async () => {
    const copy = vi.fn();
    render(<EnhancedDataTable columns={[{ accessorKey: 'username', header: 'Username' }]} data={rows} total={2} floatingRowActions={(row) => <Button onClick={() => copy(row.uuid)}>Copy record</Button>} />);
    const card = screen.getByRole('article', { name: 'alpha' });
    await userEvent.click(within(card).getByRole('button', { name: 'Actions' }));
    await userEvent.click(within(card).getByRole('button', { name: 'Copy record' }));
    expect(copy).toHaveBeenCalledExactlyOnceWith('a');
  });

  it('supports explicit labels and roles rather than treating every computed field as an action', () => {
    const column = { header: () => null, meta: { mobileLabel: 'Computed total', mobileRole: 'summary' as const } };
    expect(getMobileColumnLabel(column, 'internal_name')).toBe('Computed total');
    expect(getMobileField(column, 'total', 5, true).role).toBe('summary');
  });

  it('uses object identity for anonymous records and never serializes their secrets into keys', () => {
    const first = { key: 'secret-never-in-a-key' };
    const second = { key: 'another-secret' };
    expect(getMobileRecordId(first, '0')).toBe(getMobileRecordId(first, '1'));
    expect(getMobileRecordId(first, '0')).not.toBe(getMobileRecordId(second, '0'));
    expect(getMobileRecordId(first, '0')).not.toContain(first.key);
    expect(getMobileRecordId({ uuid: 'public-id' }, '0')).toBe('id:public-id');
  });

  it('keeps all locale keys and page-counter interpolation variables aligned', () => {
    const keys = Object.keys(mobileTableTranslations.en).sort();
    for (const messages of Object.values(mobileTableTranslations)) {
      expect(Object.keys(messages).sort()).toEqual(keys);
      expect(messages.page_status).toContain('{{page}}');
      expect(messages.page_status).toContain('{{pages}}');
    }
  });

  it('renders an honest empty state with disabled mobile navigation', () => {
    render(<DataTable columns={makeColumns()} data={[]} total={0} />);
    expect(screen.getByText('No results.')).toBeVisible();
    expect(screen.getByText('Page 0 of 0')).toBeVisible();
    expect(screen.getByRole('button', { name: 'Previous page' })).toBeDisabled();
    expect(screen.getByRole('button', { name: 'Next page' })).toBeDisabled();
  });
});
