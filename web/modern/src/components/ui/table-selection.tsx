import { useId, useLayoutEffect, useRef } from 'react';
import { ChevronDown, ListChecks } from 'lucide-react';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { useAuthStore } from '@/lib/stores/auth';
import { useTranslation } from 'react-i18next';
import type { RowData } from '@tanstack/react-table';
import { Button } from '@/components/ui/button';
import { Checkbox } from '@/components/ui/checkbox';
import { useTableSelection, type TableSelection } from '@/hooks/useTableSelection';
import { getSelectionRange } from '@/lib/table-selection-range';
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
  options: TableSelectionOptions<TData> & { columns: ModernColumnDef<TData, TValue>[]; data: TData[]; total: number; loading: boolean; rangeContext?: string }
) {
  const { t } = useTranslation();
  const statusId = useId();
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
  const rangeScope = JSON.stringify([
    user?.uuid || user?.username, user?.role, options.selectionScope ?? '',
    selection.scopeVersion, selection.snapshot.mode, options.rangeContext, ids,
  ]);
  const rangeAnchor = useRef<{ id: string; scope: string; order: string[] } | null>(null);
  // Reset only after a view commits. An abandoned concurrent render must not
  // discard the still-visible table's anchor. The click handler also checks scope.
  useLayoutEffect(() => {
    if (disabled || !enabled || !selection.hasSelection || rangeAnchor.current?.scope !== rangeScope) {
      rangeAnchor.current = null;
    }
  }, [disabled, enabled, selection.hasSelection, rangeScope]);

  /** selectPage resets the range anchor and updates only the supplied visible IDs. */
  const selectPage = (checked: boolean) => {
    if (disabled) return;
    rangeAnchor.current = null;
    selection.setPage(ids, checked);
  };

  const column: ModernColumnDef<TData, TValue> = {
    id: '__selection__',
    enableSorting: false,
    header: () => (
      <Checkbox
        aria-label={t('table_selection.page')}
        checked={pageChecked}
        disabled={disabled || !ids.length}
        onCheckedChange={(checked) => selectPage(checked === true)}
      />
    ),
    cell: ({ row, table }) => {
      const id = rowId(row.original);
      return (
        <Checkbox
          aria-label={t('table_selection.row', { name: rowLabel(row.original) })}
          checked={selection.isSelected(id)}
          disabled={disabled || !id}
          onClick={(event) => {
            event.stopPropagation();
            // Own the toggle so a mobile label cannot activate the replacement
            // checkbox again after this cell rerenders. Keep its padded hit area.
            event.preventDefault();
            if (disabled || !id) return;
            // Read the rendered row model, not numeric IDs or the unsorted input.
            const order = table.getRowModel().rows.map((visibleRow) => rowId(visibleRow.original)).filter(Boolean);
            const previous = rangeAnchor.current;
            const sameOrder = previous?.scope === rangeScope
              && previous.order.length === order.length
              && previous.order.every((visibleId, index) => visibleId === order[index]);
            if (event.shiftKey && sameOrder && previous && order.includes(previous.id)) {
              // Apply the entire range atomically, including all-matching exclusions.
              selection.setPage(getSelectionRange(order, previous.id, id), !selection.isSelected(id));
            } else {
              rangeAnchor.current = { id, scope: rangeScope, order };
              selection.setRow(id, !selection.isSelected(id));
            }
          }}
          onCheckedChange={(checked) => {
            if (!disabled && id) selection.setRow(id, checked === true);
          }}
        />
      );
    },
  };
  const status =
    selection.snapshot.mode === 'all_matching'
      ? t(count == null ? 'table_selection.all_unknown' : 'table_selection.all_count', { count, excluded })
      : t('table_selection.count', { count });
  const label =
    selection.snapshot.mode === 'all_matching'
      ? t(count == null ? 'table_selection.all_pages_short' : 'table_selection.all_count_short', { count })
      : selection.hasSelection
        ? t('table_selection.count', { count })
        : t('table_selection.select');
  const controls = enabled ? (
    <>
      <DropdownMenu key={selection.scopeVersion}>
        <DropdownMenuTrigger asChild>
          <Button
            type="button"
            variant={selection.hasSelection ? 'secondary' : 'outline'}
            size="sm"
            className="table-toolbar-control max-w-full gap-2"
            disabled={disabled}
            aria-label={t('table_selection.controls')}
            aria-describedby={statusId}
            title={status}
          >
            <ListChecks className="h-4 w-4 shrink-0" aria-hidden="true" />
            <span className="truncate">{label}</span>
            <ChevronDown className="h-4 w-4 shrink-0" aria-hidden="true" />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="start" className="table-toolbar-menu" aria-label={t('table_selection.controls')}>
          <DropdownMenuLabel>{t('table_selection.controls')}</DropdownMenuLabel>
          <p className="max-w-xs px-2 pb-2 text-xs text-muted-foreground">{status}</p>
          <p className="max-w-xs px-2 pb-2 text-xs text-muted-foreground">{t('table_selection.all_pages_hint')}</p>
          <DropdownMenuSeparator />
          <DropdownMenuItem
            className="table-toolbar-menu__item"
            disabled={disabled || !ids.length}
            onSelect={() => selectPage(true)}
          >
            {t('table_selection.page')}
          </DropdownMenuItem>
          <DropdownMenuItem
            className="table-toolbar-menu__item"
            disabled={disabled || total === 0 || (total == null && !ids.length)}
            onSelect={() => {
              rangeAnchor.current = null;
              selection.selectAllMatching();
            }}
          >
            {t('table_selection.all_pages')}
          </DropdownMenuItem>
          <DropdownMenuSeparator />
          <DropdownMenuItem
            className="table-toolbar-menu__item"
            disabled={disabled || !selection.hasSelection}
            onSelect={() => {
              rangeAnchor.current = null;
              selection.clear();
            }}
          >
            {t('table_selection.clear')}
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
      <span id={statusId} role="status" className="sr-only">
        {status}
      </span>
    </>
  ) : null;
  return {
    columns: enabled ? [column, ...options.columns] : options.columns,
    controls,
    hasSelection: enabled && selection.hasSelection && count !== 0,
    isSelected: (row: TData) => enabled && selection.isSelected(rowId(row)),
  };
}
