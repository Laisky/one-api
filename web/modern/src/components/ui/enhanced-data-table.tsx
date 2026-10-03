import { TableToolbar, type TableBatchAction } from '@/components/ui/table-toolbar';
import { useSelectableTable, type TableSelectionOptions } from '@/components/ui/table-selection';
import { AdvancedPagination } from '@/components/ui/advanced-pagination';
import { Button } from '@/components/ui/button';
import { SearchableDropdown, type SearchOption } from '@/components/ui/searchable-dropdown';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table';
import { getMobileColumnLabel, getMobileField, MobileTable, MobileTableSort } from '@/components/ui/mobile-table';
import { getMobileRecordId } from '@/components/ui/mobile-table-identity';
import { useResponsive } from '@/hooks/useResponsive';
import { cn } from '@/lib/utils';
import { flexRender, type RowData, type SortingState, useTable } from '@tanstack/react-table';
import { modernTableFeatures, type ModernColumnDef as ColumnDef } from '@/lib/table';
import { ArrowDown, ArrowUp, ArrowUpDown } from 'lucide-react';
import * as React from 'react';
import { createPortal } from 'react-dom';
import { useTranslation } from 'react-i18next';

const INTERACTIVE_ROW_TARGET_SELECTOR = [
  'button', 'a[href]', 'input', 'select', 'textarea', 'label', 'summary',
  '[role="button"]', '[role="link"]', '[role="checkbox"]', '[data-mobile-controls]',
].join(', ');

/** EnhancedDataTableProps adds search, row interactions, and responsive presentation to the shared selection contract. */
export interface EnhancedDataTableProps<TData extends RowData, TValue = unknown> extends TableSelectionOptions<TData> {
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
  searchValue?: string;
  searchOptions?: SearchOption[];
  searchLoading?: boolean;
  onSearchChange?: (query: string) => void;
  onSearchValueChange?: (value: string) => void;
  onSearchSubmit?: () => void;
  onSearchSelect?: (key: string) => void;
  searchPlaceholder?: string;
  allowSearchAdditions?: boolean;
  toolbarActions?: React.ReactNode;
  batchActions?: TableBatchAction[];
  batchActionsDisabled?: boolean;
  batchActionsBusy?: boolean;
  onRefresh?: () => void;
  onRowClick?: (row: TData) => void;
  floatingRowActions?: (row: TData) => React.ReactNode;
  mobileCardLayout?: boolean;
  hideColumnsOnMobile?: string[];
  compactMode?: boolean;
  loading?: boolean;
  className?: string;
  emptyMessage?: string;
  /** paginationSlot preserves custom controls such as keyset traversal instead of assuming arbitrary page access. */
  paginationSlot?: React.ReactNode;
}

/** EnhancedDataTable renders searchable desktop tables and the same compact mobile records as DataTable. */
export function EnhancedDataTable<TData extends RowData, TValue = unknown>({
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
  searchValue = '',
  searchOptions = [],
  searchLoading = false,
  onSearchChange,
  onSearchValueChange,
  onSearchSubmit,
  onSearchSelect,
  searchPlaceholder,
  allowSearchAdditions = true,
  toolbarActions,
  batchActions,
  batchActionsDisabled,
  batchActionsBusy,
  onRefresh,
  onRowClick,
  floatingRowActions,
  mobileCardLayout = true,
  hideColumnsOnMobile = [],
  compactMode = false,
  loading = false,
  className,
  paginationSlot,
  emptyMessage,
}: EnhancedDataTableProps<TData, TValue>) {
  const { t } = useTranslation();
  const selectable = useSelectableTable({
    columns: originalColumns,
    data,
    total,
    loading,
    enableSelection,
    selection,
    selectionScope: selectionScope ?? searchValue,
    selectionDisabled,
    selectionTotal,
    getSelectionId,
    getSelectionLabel,
  });
  const columns = selectable.columns;
  const { isMobile, isTablet } = useResponsive();
  const [sorting, setSorting] = React.useState<SortingState>([]);
  const effectiveSearchPlaceholder = searchPlaceholder || t('common.search_placeholder', 'Search...');
  const effectiveEmptyMessage = emptyMessage || t('common.no_data', 'No results found.');
  const [hoveredRowData, setHoveredRowData] = React.useState<TData | null>(null);
  const [floatingPos, setFloatingPos] = React.useState<{ top: number; left: number } | null>(null);
  const hoverTimeoutRef = React.useRef<NodeJS.Timeout | undefined>(undefined);

  React.useEffect(() => () => {
    if (hoverTimeoutRef.current) clearTimeout(hoverTimeoutRef.current);
  }, []);

  /** clearFloatingActions removes the current desktop hover action surface. */
  const clearFloatingActions = () => {
    setHoveredRowData(null);
    setFloatingPos(null);
  };

  /** shouldIgnoreFloatingActions excludes nested controls from record-level pointer handling. */
  const shouldIgnoreFloatingActions = (target: EventTarget | null) =>
    target instanceof Element && Boolean(target.closest(INTERACTIVE_ROW_TARGET_SELECTOR));

  /** handleRowMouseEnter anchors desktop actions without intercepting interactive cells. */
  const handleRowMouseEnter = (event: React.MouseEvent<HTMLTableRowElement>, row: TData) => {
    if (!floatingRowActions || loading || isMobile) return;
    if (hoverTimeoutRef.current) clearTimeout(hoverTimeoutRef.current);
    if (shouldIgnoreFloatingActions(event.target)) {
      clearFloatingActions();
      return;
    }
    setFloatingPos({ top: event.clientY, left: event.clientX + 16 });
    setHoveredRowData(row);
  };

  /** handleRowMouseMove keeps an existing action surface stationary while the pointer remains on its record. */
  const handleRowMouseMove = (event: React.MouseEvent<HTMLTableRowElement>, row: TData) => {
    if (!floatingRowActions || loading || isMobile) return;
    if (hoverTimeoutRef.current) clearTimeout(hoverTimeoutRef.current);
    if (shouldIgnoreFloatingActions(event.target)) {
      if (hoveredRowData !== null || floatingPos !== null) clearFloatingActions();
      return;
    }
    if (hoveredRowData === row && floatingPos !== null) return;
    setFloatingPos({ top: event.clientY, left: event.clientX + 16 });
    setHoveredRowData(row);
  };

  /** handleRowMouseLeave gives the pointer time to enter the existing floating action surface. */
  const handleRowMouseLeave = () => {
    if (!floatingRowActions) return;
    hoverTimeoutRef.current = setTimeout(clearFloatingActions, 100);
  };

  /** handleFloatingMouseEnter cancels pending dismissal while interacting with desktop actions. */
  const handleFloatingMouseEnter = () => {
    if (hoverTimeoutRef.current) clearTimeout(hoverTimeoutRef.current);
  };

  /** handleFloatingMouseLeave dismisses desktop actions after a short pointer grace period. */
  const handleFloatingMouseLeave = () => {
    hoverTimeoutRef.current = setTimeout(clearFloatingActions, 100);
  };

  /** handleSort preserves ascending-first server sorting and ignores requests while loading. */
  const handleSort = (accessorKey: string) => {
    if (!onSortChange || loading) return;
    onSortChange(accessorKey, sortBy === accessorKey && sortOrder === 'asc' ? 'desc' : 'asc');
  };

  /** getSortIcon returns the existing desktop ordering indicator. */
  const getSortIcon = (accessorKey: string) => {
    if (onSortChange && sortBy === accessorKey) {
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

  /** handleSearchAddition delegates a free-form search value to the caller's existing search state. */
  const handleSearchAddition = (value: string) => {
    onSearchValueChange?.(value);
  };

  /** isHiddenColumn applies the caller's explicit column policy on small screens only. */
  const isHiddenColumn = (column: ColumnDef<TData, unknown>) => {
    const accessorKey = 'accessorKey' in column ? (column.accessorKey as string) : '';
    return (isMobile || isTablet) && hideColumnsOnMobile.includes(accessorKey);
  };
  const visibleColumns = columns.filter((column) => !isHiddenColumn(column as ColumnDef<TData, unknown>));

  return (
    <div className={cn('data-table-shell space-y-4', className)}>
      <TableToolbar
        selectionControl={selectable.controls}
        hasSelection={selectable.hasSelection}
        loading={loading}
        onRefresh={onRefresh}
        onSearchSubmit={onSearchSubmit}
        toolbarActions={toolbarActions}
        batchActions={batchActions}
        batchActionsDisabled={selectionDisabled || batchActionsDisabled}
        batchActionsBusy={batchActionsBusy}
        searchControl={
          onSearchChange ? (
            <SearchableDropdown
              value={searchValue}
              placeholder={effectiveSearchPlaceholder}
              searchPlaceholder={effectiveSearchPlaceholder}
              options={searchOptions}
              remoteFiltered
              onSearchChange={onSearchChange}
              onChange={onSearchValueChange}
              onSelect={onSearchSelect}
              onAddItem={allowSearchAdditions ? handleSearchAddition : undefined}
              loading={searchLoading}
              noResultsMessage={t('common.no_results', 'No results found')}
              additionLabel={t('common.search_for', 'Search for: ')}
              allowAdditions={allowSearchAdditions}
              clearable
              className="table-toolbar-control"
            />
          ) : undefined
        }
      />
      {isMobile && mobileCardLayout && (
        <MobileTableSort options={sortOptions} sortBy={sortBy} sortOrder={sortOrder} onSortChange={onSortChange} loading={loading} />
      )}
      <div className="relative" aria-busy={loading}>
        {loading && (
          <div className="absolute inset-0 z-10 flex items-center justify-center bg-background/60 backdrop-blur-sm rounded-md">
            <div className="text-sm text-muted-foreground">{t('common.loading', 'Loading...')}</div>
          </div>
        )}
        {isMobile && mobileCardLayout ? (
          <MobileTable
            loading={loading}
            emptyMessage={loading ? t('common.loading', 'Loading...') : effectiveEmptyMessage}
            records={table.getRowModel().rows.map((row) => {
              const cells = row.getVisibleCells().filter((cell) => !isHiddenColumn(cell.column.columnDef));
              const multipleFields = cells.filter((cell) => cell.column.id !== '__selection__').length > 1;
              return {
                id: JSON.stringify([selectionScope ?? searchValue, pageIndex, getMobileRecordId(row.original, row.id, getSelectionId)]),
                selected: selectable.isSelected(row.original),
                onOpen: onRowClick ? () => onRowClick(row.original) : undefined,
                extraActions: floatingRowActions?.(row.original),
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
            <div className="overflow-x-auto w-full">
              <Table className={cn('min-w-max', loading && 'pointer-events-none opacity-60')}>
                <TableHeader>
                  {table.getHeaderGroups().map((headerGroup) => (
                    <TableRow key={headerGroup.id}>
                      {headerGroup.headers.map((header) => {
                        if (isHiddenColumn(header.column.columnDef)) return null;
                        return (
                          <TableHead
                            key={header.id}
                            className={cn(compactMode ? 'px-2 py-2' : 'px-4 py-3')}
                            aria-sort={onSortChange && header.column.id === sortBy ? (sortOrder === 'asc' ? 'ascending' : 'descending') : undefined}
                          >
                            {header.isPlaceholder ? null : flexRender(header.column.columnDef.header, header.getContext())}
                          </TableHead>
                        );
                      })}
                    </TableRow>
                  ))}
                </TableHeader>
                <TableBody>
                  {table.getRowModel().rows?.length ? (
                    table.getRowModel().rows.map((row) => (
                      <TableRow
                        key={row.id}
                        data-state={selectable.isSelected(row.original) ? 'selected' : undefined}
                        className={cn('hover:bg-muted/50 transition-colors', onRowClick && 'cursor-pointer')}
                        onClick={(event) => {
                          if (!loading && !shouldIgnoreFloatingActions(event.target)) onRowClick?.(row.original);
                        }}
                        onMouseEnter={(event) => handleRowMouseEnter(event, row.original)}
                        onMouseMove={(event) => handleRowMouseMove(event, row.original)}
                        onMouseLeave={handleRowMouseLeave}
                      >
                        {row.getVisibleCells().map((cell) => {
                          if (isHiddenColumn(cell.column.columnDef)) return null;
                          return (
                            <TableCell key={cell.id} className={cn(compactMode ? 'px-2 py-2' : 'px-4 py-3')}>
                              {flexRender(cell.column.columnDef.cell, cell.getContext())}
                            </TableCell>
                          );
                        })}
                      </TableRow>
                    ))
                  ) : (
                    <TableRow>
                      <TableCell colSpan={visibleColumns.length} className="h-24 text-center">
                        {loading ? t('common.loading', 'Loading...') : effectiveEmptyMessage}
                      </TableCell>
                    </TableRow>
                  )}
                </TableBody>
              </Table>
            </div>
          </div>
        )}
      </div>
      {paginationSlot ?? (
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
      )}
      {floatingRowActions && !loading && !isMobile && hoveredRowData && data.includes(hoveredRowData) && floatingPos && createPortal(
        <div
          className="fixed z-50 transform -translate-y-1/2 bg-background border rounded-md shadow-lg p-1 animate-in fade-in zoom-in-95 duration-100"
          style={{ top: floatingPos.top, left: floatingPos.left }}
          onMouseEnter={handleFloatingMouseEnter}
          onMouseLeave={handleFloatingMouseLeave}
        >
          {floatingRowActions(hoveredRowData)}
        </div>,
        document.body
      )}
    </div>
  );
}
