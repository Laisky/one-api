import { useAuthStore } from '@/lib/stores/auth';
import { useTranslation } from 'react-i18next';
import type { RowData } from '@tanstack/react-table';
import { Button } from '@/components/ui/button';
import { Checkbox } from '@/components/ui/checkbox';
import { useTableSelection, type TableSelection } from '@/hooks/useTableSelection';
import type { ModernColumnDef } from '@/lib/table';

/** TableSelectionOptions adds a shared, optionally controlled selection contract to both table renderers. */
export interface TableSelectionOptions<TData> {
  enableSelection?: boolean;
  selection?: TableSelection;
  selectionScope?: string;
  selectionDisabled?: boolean;
  /** selectionTotal is null when cursor pagination has no exact count. */
  selectionTotal?: number | null;
  getSelectionId?: (row: TData) => string;
  getSelectionLabel?: (row: TData) => string;
}

/** defaultSelectionId uses public identity, with a legacy ID fallback, and never guesses a row index. */
export function defaultSelectionId(row: unknown): string {
  if (!row || typeof row !== 'object') return '';
  const item = row as { uuid?: unknown; id?: unknown };
  if (typeof item.uuid === 'string' && item.uuid) return item.uuid;
  return typeof item.id === 'string' || typeof item.id === 'number' ? String(item.id) : '';
}

/** defaultSelectionLabel names a row without exposing any other fields such as keys or tokens. */
function defaultSelectionLabel(row: unknown): string {
  const name =
    row && typeof row === 'object'
      ? (row as { name?: unknown; username?: unknown }).name || (row as { username?: unknown }).username
      : undefined;
  return typeof name === 'string' && name ? name : defaultSelectionId(row);
}

/** useSelectableTable supplies checkboxes and the same page/all-matching controls to desktop and mobile tables. */
export function useSelectableTable<TData extends RowData, TValue>(
  options: TableSelectionOptions<TData> & { columns: ModernColumnDef<TData, TValue>[]; data: TData[]; total: number; loading: boolean }
) {
  const { t } = useTranslation();
  const user = useAuthStore((state) => state.user);
  const local = useTableSelection(JSON.stringify([user?.uuid || user?.username, user?.role, options.selectionScope ?? '']));
  const selection = options.selection ?? local;
  const enabled = options.enableSelection !== false;
  const disabled = options.loading || options.selectionDisabled;
  const rowId = options.getSelectionId ?? defaultSelectionId;
  const rowLabel = options.getSelectionLabel ?? defaultSelectionLabel;
  const ids = [...new Set(options.data.map(rowId).filter(Boolean))];
  const selectedOnPage = ids.filter(selection.isSelected).length;
  const pageChecked = ids.length > 0 && selectedOnPage === ids.length ? true : selectedOnPage > 0 ? 'indeterminate' : false;
  const total = options.selectionTotal === undefined ? options.total : options.selectionTotal;
  const count = selection.selectedCount(total);
  const excluded = selection.snapshot.mode === 'all_matching' ? selection.snapshot.excluded_ids.length : 0;

  const column: ModernColumnDef<TData, TValue> = {
    id: '__selection__',
    enableSorting: false,
    header: () => (
      <Checkbox
        aria-label={t('table_selection.page')}
        checked={pageChecked}
        disabled={disabled || !ids.length}
        onCheckedChange={(checked) => selection.setPage(ids, checked === true)}
      />
    ),
    cell: ({ row }) => {
      const id = rowId(row.original);
      return (
        <Checkbox
          aria-label={t('table_selection.row', { name: rowLabel(row.original) })}
          checked={selection.isSelected(id)}
          disabled={disabled || !id}
          onClick={(event) => event.stopPropagation()}
          onCheckedChange={(checked) => selection.setRow(id, checked === true)}
        />
      );
    },
  };
  const controls = enabled ? (
    <div className="flex flex-wrap items-center gap-2 rounded-md border p-2" aria-label={t('table_selection.controls')}>
      <Button type="button" variant="outline" size="sm" disabled={disabled || !ids.length} onClick={() => selection.setPage(ids, true)}>
        {t('table_selection.page')}
      </Button>
      <Button
        type="button"
        variant="outline"
        size="sm"
        disabled={disabled || total === 0 || (total == null && !ids.length)}
        onClick={selection.selectAllMatching}
      >
        {t('table_selection.all_pages')}
      </Button>
      <Button type="button" variant="ghost" size="sm" disabled={disabled || !selection.hasSelection} onClick={selection.clear}>
        {t('table_selection.clear')}
      </Button>
      <span role="status" className="text-sm text-muted-foreground">
        {selection.snapshot.mode === 'all_matching'
          ? t(count == null ? 'table_selection.all_unknown' : 'table_selection.all_count', { count, excluded })
          : t('table_selection.count', { count })}
      </span>
    </div>
  ) : null;
  return {
    columns: enabled ? [column, ...options.columns] : options.columns,
    controls,
    isSelected: (row: TData) => enabled && selection.isSelected(rowId(row)),
  };
}
