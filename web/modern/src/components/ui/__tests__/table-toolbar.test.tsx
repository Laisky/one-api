import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import { EnhancedDataTable } from '../enhanced-data-table';
import { DataTable } from '../data-table';
import { TableToolbar, type TableBatchAction } from '../table-toolbar';
import { SearchableDropdown } from '../searchable-dropdown';
import { chooseTableSelection, chooseTableAction } from '@/test/table-toolbar';
import type { ModernColumnDef } from '@/lib/table';

const rows = [
  { uuid: 'alpha', name: 'Alpha' },
  { uuid: 'bravo', name: 'Bravo' },
];
const columns: ModernColumnDef<(typeof rows)[number]>[] = [{ accessorKey: 'name', header: 'Name' }];

for (const [label, Component] of [
  ['enhanced', EnhancedDataTable],
  ['basic', DataTable],
] as const) {
  describe(`${label} compact toolbar`, () => {
    it('keeps selection and view controls in one group and hides inapplicable batch commands', async () => {
      const run = vi.fn();
      render(
        <Component
          columns={columns}
          data={rows}
          total={25}
          onRefresh={vi.fn()}
          batchActions={[{ id: 'reset', label: 'Reset selected models', onSelect: run }]}
        />
      );
      const toolbar = screen.getByRole('group', { name: 'Table controls' });
      expect(within(toolbar).getByRole('button', { name: 'Row selection' })).toBeInTheDocument();
      expect(within(toolbar).getByRole('button', { name: 'Refresh' })).toBeInTheDocument();
      expect(screen.queryByRole('button', { name: 'Actions' })).not.toBeInTheDocument();
      expect(screen.queryByRole('button', { name: 'Select all pages' })).not.toBeInTheDocument();
      await userEvent.click(screen.getByRole('checkbox', { name: 'Select Alpha' }));
      expect(within(toolbar).getByRole('button', { name: 'Actions' })).toBeEnabled();
      expect(run).not.toHaveBeenCalled();
      await chooseTableAction('Reset selected models');
      expect(run).toHaveBeenCalledOnce();
    });

    it('exposes page and filtered all-page commands in one menu and clears without invoking an action', async () => {
      const run = vi.fn();
      render(
        <Component
          columns={columns}
          data={rows}
          total={25}
          batchActions={[{ id: 'reset', label: 'Reset selected models', onSelect: run }]}
        />
      );
      await userEvent.click(screen.getByRole('button', { name: 'Row selection' }));
      expect(screen.getByRole('menuitem', { name: 'Clear selection' })).toHaveAttribute('data-disabled');
      expect(screen.getByText('All pages includes only results matching the current filters.')).toBeInTheDocument();
      await userEvent.click(screen.getByRole('menuitem', { name: 'Select all pages' }));
      expect(screen.getByRole('button', { name: 'Row selection' })).toHaveTextContent('All 25');
      await userEvent.click(screen.getByRole('checkbox', { name: 'Select Alpha' }));
      expect(screen.getByRole('status')).toHaveTextContent('24 items (1 excluded)');
      await chooseTableSelection('Clear selection');
      expect(screen.queryByRole('button', { name: 'Actions' })).not.toBeInTheDocument();
      expect(run).not.toHaveBeenCalled();
    });

    it('supports keyboard selection and restores focus on Escape', async () => {
      render(<Component columns={columns} data={rows} total={25} />);
      const user = userEvent.setup();
      const trigger = screen.getByRole('button', { name: 'Row selection' });
      trigger.focus();
      await user.keyboard('{Enter}{Home}{Enter}');
      expect(screen.getByRole('checkbox', { name: 'Select Alpha' })).toBeChecked();
      await user.click(trigger);
      await user.keyboard('{Escape}');
      expect(trigger).toHaveFocus();
      expect(screen.queryByRole('menu')).not.toBeInTheDocument();
    });

    it('blocks pending commands and removes the contextual menu after a filter change', async () => {
      const props = {
        columns,
        data: rows,
        total: 25,
        selectionScope: 'A',
        batchActions: [{ id: 'reset', label: 'Reset', onSelect: vi.fn() }],
      };
      const { rerender } = render(<Component {...props} />);
      await userEvent.click(screen.getByRole('checkbox', { name: 'Select Alpha' }));
      rerender(<Component {...props} batchActionsBusy batchActionsDisabled />);
      expect(screen.getByRole('button', { name: 'Actions' })).toBeDisabled();
      expect(screen.getByRole('button', { name: 'Actions' })).toHaveAttribute('aria-busy', 'true');
      rerender(<Component {...props} selectionScope="B" />);
      expect(screen.queryByRole('button', { name: 'Actions' })).not.toBeInTheDocument();
    });
  });
}

describe('toolbar command semantics', () => {
  it('preserves accessible search and refresh, and labels destructive commands only inside the menu', async () => {
    const search = vi.fn(),
      refresh = vi.fn(),
      remove = vi.fn(),
      reset = vi.fn();
    const actions: TableBatchAction[] = [
      { id: 'reset', label: 'Reset', onSelect: reset },
      { id: 'delete', label: 'Delete', destructive: true, onSelect: remove },
    ];
    render(
      <TableToolbar
        searchControl={<input aria-label="Search records" />}
        onSearchSubmit={search}
        onRefresh={refresh}
        hasSelection
        batchActions={actions}
      />
    );
    await userEvent.click(screen.getByRole('button', { name: 'Search' }));
    await userEvent.click(screen.getByRole('button', { name: 'Refresh' }));
    expect(search).toHaveBeenCalledOnce();
    expect(refresh).toHaveBeenCalledOnce();
    expect(screen.queryByText('Delete')).not.toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: 'Actions' }));
    const menu = screen.getByRole('menu', { name: 'Actions for selected rows' });
    expect(within(menu).getByRole('separator')).toBeInTheDocument();
    await userEvent.keyboard('{Escape}');
    expect(screen.getByRole('button', { name: 'Actions' })).toHaveFocus();
    expect(remove).not.toHaveBeenCalled();
    expect(reset).not.toHaveBeenCalled();
  });

  it('disables an already-open command when its action becomes unavailable', async () => {
    const run = vi.fn();
    const { rerender } = render(<TableToolbar hasSelection batchActions={[{ id: 'reset', label: 'Reset', onSelect: run }]} />);
    await userEvent.click(screen.getByRole('button', { name: 'Actions' }));
    rerender(<TableToolbar hasSelection batchActions={[{ id: 'reset', label: 'Reset', onSelect: run, disabled: true }]} />);
    expect(screen.getByRole('menuitem', { name: 'Reset' })).toHaveAttribute('data-disabled');
    await userEvent.keyboard('{Home}{Enter}');
    expect(run).not.toHaveBeenCalled();
  });

  it('shows an applied free-text query instead of replacing it with the placeholder', () => {
    render(<SearchableDropdown value="provider-name-prefix" options={[]} placeholder="Search channels" />);
    expect(screen.getByRole('combobox')).toHaveTextContent('provider-name-prefix');
  });
});
