import { act, cleanup, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { useAuthStore } from '@/lib/stores/auth';
import type { ModernColumnDef } from '@/lib/table';
import { chooseTableSelection } from '@/test/table-toolbar';
import { DataTable } from '../data-table';
import { EnhancedDataTable } from '../enhanced-data-table';

const responsive = vi.hoisted(() => ({ isMobile: false, isTablet: false }));
vi.mock('@/hooks/useResponsive', () => ({ useResponsive: () => responsive }));
const names = ['Alpha', 'Bravo', 'Charlie', 'Delta', 'Echo', 'Foxtrot'];
const rows = names.map((name) => ({ uuid: name.toLowerCase(), name }));
const columns: ModernColumnDef<(typeof rows)[number]>[] = [{ accessorKey: 'name', header: 'Name' }];
afterEach(() => cleanup());

/** checkbox returns the accessible selection control for the supplied record name. */
function checkbox(name: string) {
  return screen.getByRole('checkbox', { name: `Select ${name}` });
}

/** shiftClick activates a row checkbox with the actual Shift modifier held. */
async function shiftClick(user: ReturnType<typeof userEvent.setup>, name: string) {
  await user.keyboard('{Shift>}');
  await user.click(checkbox(name));
  await user.keyboard('{/Shift}');
}

/** expectSelected checks the selected state of every visible fixture record. */
function expectSelected(selected: string[]) {
  for (const name of names) {
    if (selected.includes(name)) expect(checkbox(name)).toBeChecked();
    else expect(checkbox(name)).not.toBeChecked();
  }
}

for (const [label, Component] of [['basic', DataTable], ['enhanced', EnhancedDataTable]] as const) {
  for (const mobile of [false, true]) {
    describe(`${label} ${mobile ? 'mobile' : 'desktop'} Shift selection`, () => {
      beforeEach(() => {
        responsive.isMobile = mobile;
        responsive.isTablet = false;
      });

      it('selects the inclusive range and preserves unrelated selected rows', async () => {
        const user = userEvent.setup();
        const onRowClick = vi.fn();
        render(<Component columns={columns} data={rows} total={6} onRowClick={onRowClick} />);
        await user.click(checkbox('Foxtrot'));
        await user.click(checkbox('Alpha'));
        await shiftClick(user, 'Echo');
        expectSelected(names);
        expect(onRowClick).not.toHaveBeenCalled();
      });

      it('supports reverse ranges and the current displayed ordering', async () => {
        const user = userEvent.setup();
        render(<Component columns={columns} data={[...rows].reverse()} total={6} />);
        await user.click(checkbox('Bravo'));
        await shiftClick(user, 'Foxtrot');
        expectSelected(names.slice(1));
      });

      it('retains the non-Shift anchor across consecutive range selections', async () => {
        const user = userEvent.setup();
        render(<Component columns={columns} data={rows} total={6} />);
        await user.click(checkbox('Charlie'));
        await shiftClick(user, 'Echo');
        await shiftClick(user, 'Alpha');
        expectSelected(names.slice(0, 5));
      });

      it('deselects a checked range without clearing unrelated selections', async () => {
        const user = userEvent.setup();
        render(<Component columns={columns} data={rows} total={6} />);
        await chooseTableSelection('Select this page');
        await user.click(checkbox('Alpha'));
        await shiftClick(user, 'Echo');
        expectSelected(['Foxtrot']);
      });

      it('updates exclusions in all-matching mode rather than converting to page selection', async () => {
        const user = userEvent.setup();
        render(<Component columns={columns} data={rows} total={100} />);
        await chooseTableSelection('Select all pages');
        await user.click(checkbox('Alpha'));
        await shiftClick(user, 'Echo');
        expectSelected(['Foxtrot']);
        expect(screen.getByRole('status')).toHaveTextContent('95 items (5 excluded)');
      });

      it('treats Shift-click without an anchor as an ordinary single selection', async () => {
        const user = userEvent.setup();
        render(<Component columns={columns} data={rows} total={6} />);
        await shiftClick(user, 'Echo');
        expectSelected(['Echo']);
      });

      it('invalidates the anchor when the displayed ordering changes', async () => {
        const user = userEvent.setup();
        const props = { columns, data: rows, total: 6 };
        const { rerender } = render(<Component {...props} />);
        await user.click(checkbox('Alpha'));
        rerender(<Component {...props} data={[...rows].reverse()} />);
        await shiftClick(user, 'Echo');
        expectSelected(['Alpha', 'Echo']);
      });

      it('does not resurrect an anchor after navigating away and back', async () => {
        const user = userEvent.setup();
        const props = { columns, data: rows, total: 7, pageSize: 6 };
        const { rerender } = render(<Component {...props} />);
        await user.click(checkbox('Alpha'));
        rerender(<Component {...props} data={[{ uuid: 'other', name: 'Other' }]} pageIndex={1} />);
        rerender(<Component {...props} />);
        await shiftClick(user, 'Echo');
        expectSelected(['Alpha', 'Echo']);
      });

      it('invalidates the anchor when selection is cleared or its filter scope changes', async () => {
        const user = userEvent.setup();
        const props = { columns, data: rows, total: 6, selectionScope: 'A' };
        const { rerender } = render(<Component {...props} />);
        await user.click(checkbox('Alpha'));
        await chooseTableSelection('Clear selection');
        await shiftClick(user, 'Echo');
        expectSelected(['Echo']);
        rerender(<Component {...props} selectionScope="B" />);
        await shiftClick(user, 'Alpha');
        expectSelected(['Alpha']);
      });

      it('blocks disabled changes and invalidates the anchor across loading', async () => {
        const user = userEvent.setup();
        const props = { columns, data: rows, total: 6 };
        const { rerender } = render(<Component {...props} />);
        await user.click(checkbox('Alpha'));
        rerender(<Component {...props} loading />);
        expect(checkbox('Echo')).toBeDisabled();
        rerender(<Component {...props} selectionDisabled />);
        expect(checkbox('Echo')).toBeDisabled();
        rerender(<Component {...props} />);
        await shiftClick(user, 'Echo');
        expectSelected(['Alpha', 'Echo']);
      });

      it('preserves ordinary keyboard checkbox activation', async () => {
        const user = userEvent.setup();
        render(<Component columns={columns} data={rows} total={6} />);
        act(() => checkbox('Alpha').focus());
        await user.keyboard('[Space]');
        expectSelected(['Alpha']);
        act(() => checkbox('Alpha').focus());
        await user.keyboard('[Space]');
        expectSelected([]);
      });

      it('does not reuse another authenticated user\'s anchor', async () => {
        const original = useAuthStore.getState().user;
        const user = userEvent.setup();
        try {
          render(<Component columns={columns} data={rows} total={6} />);
          await user.click(checkbox('Alpha'));
          act(() => useAuthStore.setState({ user: {
            id: 'range-other', uuid: 'range-other', username: 'range-other', role: 10,
            status: 1, quota: 0, used_quota: 0, group: 'default',
          } }));
          await shiftClick(user, 'Echo');
          expectSelected(['Echo']);
        } finally {
          act(() => useAuthStore.setState({ user: original }));
        }
      });
    });
  }
}
