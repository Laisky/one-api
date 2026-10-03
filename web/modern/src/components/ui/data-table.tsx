import { TableToolbar, type TableToolbarProps } from '@/components/ui/table-toolbar';
import { useSelectableTable, type TableSelectionOptions } from '@/components/ui/table-selection';
import * as React from 'react';
import { flexRender, type RowData, type SortingState, useTable } from '@tanstack/react-table';
import { modernTableFeatures, type ModernColumnDef as ColumnDef } from '@/lib/table';
import { useResponsive } from '@/hooks/useResponsive';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table';
import { Button } from '@/components/ui/button';
import { AdvancedPagination } from '@/components/ui/advanced-pagination';
import { getMobileColumnLabel, getMobileField, MobileTable, MobileTableSort } from '@/components/ui/mobile-table';
import { getMobileRecordId } from '@/components/ui/mobile-table-identity';
import { ArrowUpDown, ArrowUp, ArrowDown } from 'lucide-react';
import { useTranslation } from 'react-i18next';

/** DataTableProps describes shared selection, server pagination, sorting, and toolbar controls. */
export interface DataTableProps<TData extends RowData, TValue = unknown>
  extends TableSelectionOptions<TData>, Omit<TableToolbarProps, 'selectionControl' | 'hasSelection'> {
  columns: ColumnDef<TData, TValue>[];
  data: TData[];
  pageIndex?: number;
  pageSize?: number;
  total?: number;
  onPageChange?: (pageIndex: number, pageSize: number) => void;
  onPageSizeChange?: (pageSize: number) => void;
  sortBy?: string;
  sortOrder?: 'asc' | 'desc';
  onSortChange?: (sortBy: string, sortOrder: 'asc' | 'desc') => void;
  loading?: boolean;
}

/** DataTable renders the same cell definitions as a desktop table or a progressively disclosed mobile list. */
export function DataTable<TData extends RowData, TValue = unknown>({
  columns: originalColumns,
  data,
  enableSelection,
  selection,
  selectionScope,
  selectionDisabled,
  selectionTotal,
  getSelectionId,
  getSelectionLabel,
  pageIndex = 0,
  pageSize = 20,
  total = 0,
  onPageChange,
  onPageSizeChange,
  sortBy = '',
  sortOrder = 'desc',
  onSortChange,
  loading = false,
  searchControl,
  onSearchSubmit,
  onRefresh,
  toolbarActions,
  batchActions,
  batchActionsDisabled,
  batchActionsBusy,
}: DataTableProps<TData, TValue>) {
  const { t } = useTranslation();
  const selectable = useSelectableTable({
    columns: originalColumns,
    data,
    total,
    loading,
    enableSelection,
    selection,
    selectionScope: selectionScope ?? '',
    selectionDisabled,
    selectionTotal,
    getSelectionId,
    getSelectionLabel,
  });
  const columns = selectable.columns;
  const { isMobile } = useResponsive();
  const [sorting, setSorting] = React.useState<SortingState>([]);

  /** handleSort changes server ordering while retaining the basic table's descending-first convention. */
  const handleSort = (accessorKey: string) => {
    if (!onSortChange || loading) return;
    onSortChange(accessorKey, sortBy === accessorKey && sortOrder === 'desc' ? 'asc' : 'desc');
  };

  /** getSortIcon returns the current server ordering indicator for a column. */
  const getSortIcon = (accessorKey: string) => {
    if (!onSortChange) return null;
    if (sortBy === accessorKey) {
      return sortOrder === 'asc' ? <ArrowUp className="ml-2 h-4 w-4" /> : <ArrowDown className="ml-2 h-4 w-4" />;
    }
    return <ArrowUpDown className="ml-2 h-4 w-4 opacity-50" />;
  };

  const sortOptions = originalColumns.flatMap((column) => {
    const key = 'accessorKey' in column && typeof column.accessorKey === 'string' ? column.accessorKey : '';
    return key && column.enableSorting !== false ? [{ value: key, label: getMobileColumnLabel(column, key) }] : [];
  });
  const enhancedColumns = columns.map((column) => {
    const accessorKey = 'accessorKey' in column && typeof column.accessorKey === 'string' ? column.accessorKey : '';
    if (!accessorKey || !onSortChange || column.enableSorting === false) return column;
    const label = getMobileColumnLabel(column, accessorKey);
    return {
      ...column,
      // The sortable header is a function. Keep its original localized label
      // instead of exposing accessor keys such as display_name on mobile.
      meta: { ...column.meta, mobileLabel: label },
      header: () => (
        <Button variant="ghost" onClick={() => handleSort(accessorKey)} disabled={loading} className="h-auto p-0 font-semibold hover:bg-transparent">
          <span>{label}</span>
          {getSortIcon(accessorKey)}
        </Button>
      ),
    } as ColumnDef<TData, TValue>;
  });

  const table = useTable(
    {
      features: modernTableFeatures,
      data,
      columns: enhancedColumns as ColumnDef<TData, unknown>[],
      state: { sorting, pagination: { pageIndex, pageSize } },
      onSortingChange: setSorting,
      manualSorting: !!onSortChange,
      manualPagination: true,
      pageCount: Math.ceil(total / pageSize),
    },
    (state) => state
  );

  return (
    <div className="data-table-shell space-y-2">
      <TableToolbar
        selectionControl={selectable.controls}
        hasSelection={selectable.hasSelection}
        searchControl={searchControl}
        onSearchSubmit={onSearchSubmit}
        onRefresh={onRefresh}
        toolbarActions={toolbarActions}
        batchActions={batchActions}
        batchActionsDisabled={selectionDisabled || batchActionsDisabled}
        batchActionsBusy={batchActionsBusy}
        loading={loading}
      />
      {isMobile && (
        <MobileTableSort options={sortOptions} sortBy={sortBy} sortOrder={sortOrder} onSortChange={onSortChange} loading={loading} defaultOrder="desc" />
      )}
      <div className="relative" aria-busy={loading}>
        {loading && (
          <div className="absolute inset-0 z-10 flex items-center justify-center rounded-xl bg-background/60 backdrop-blur-sm">
            <div className="text-sm text-muted-foreground">{t('common.loading', 'Loading...')}</div>
          </div>
        )}
        {isMobile ? (
          <MobileTable
            loading={loading}
            emptyMessage={loading ? t('common.loading', 'Loading...') : t('common.no_data', 'No results.')}
            records={table.getRowModel().rows.map((row) => {
              const cells = row.getVisibleCells();
              const multipleFields = cells.filter((cell) => cell.column.id !== '__selection__').length > 1;
              return {
                id: JSON.stringify([selectionScope ?? '', pageIndex, getMobileRecordId(row.original, row.id, getSelectionId)]),
                selected: selectable.isSelected(row.original),
                fields: cells.map((cell, index) => getMobileField(
                  cell.column.columnDef,
                  cell.column.id,
                  flexRender(cell.column.columnDef.cell, cell.getContext()),
                  multipleFields && index === cells.length - 1
                )),
              };
            })}
          />
        ) : (
          <div className="rounded-md border overflow-x-auto" inert={loading}>
            <Table className={loading ? 'pointer-events-none opacity-60' : ''}>
              <TableHeader>
                {table.getHeaderGroups().map((headerGroup) => (
                  <TableRow key={headerGroup.id}>
                    {headerGroup.headers.map((header) => (
                      <TableHead
                        key={header.id}
                        className="text-left mobile:whitespace-normal mobile:break-words"
                        aria-sort={onSortChange && header.column.id === sortBy ? (sortOrder === 'asc' ? 'ascending' : 'descending') : undefined}
                      >
                        {header.isPlaceholder ? null : flexRender(header.column.columnDef.header, header.getContext())}
                      </TableHead>
                    ))}
                  </TableRow>
                ))}
              </TableHeader>
              <TableBody>
                {table.getRowModel().rows?.length ? (
                  table.getRowModel().rows.map((row) => (
                    <TableRow key={row.id} data-state={selectable.isSelected(row.original) ? 'selected' : undefined}>
                      {row.getVisibleCells().map((cell) => (
                        <TableCell key={cell.id} data-label={getMobileColumnLabel(cell.column.columnDef, cell.column.id)} className="mobile-table-cell break-words break-all">
                          {flexRender(cell.column.columnDef.cell, cell.getContext())}
                        </TableCell>
                      ))}
                    </TableRow>
                  ))
                ) : (
                  <TableRow>
                    <TableCell colSpan={columns.length} className="h-24 text-center">
                      {loading ? t('common.loading', 'Loading...') : t('common.no_data', 'No results.')}
                    </TableCell>
                  </TableRow>
                )}
              </TableBody>
            </Table>
          </div>
        )}
      </div>
      <AdvancedPagination
        currentPage={pageIndex + 1}
        totalPages={Math.ceil(total / pageSize)}
        pageSize={pageSize}
        totalItems={total}
        onPageChange={(page) => onPageChange?.(page - 1, pageSize)}
        onPageSizeChange={(newPageSize) => {
          onPageSizeChange?.(newPageSize);
          onPageChange?.(0, newPageSize);
        }}
        loading={loading}
      />
    </div>
  );
}
