import { AdvancedPagination } from '@/components/ui/advanced-pagination';
import { Button } from '@/components/ui/button';
import { SearchableDropdown, type SearchOption } from '@/components/ui/searchable-dropdown';
import {
    Table,
    TableBody,
    TableCell,
    TableHead,
    TableHeader,
    TableRow,
} from '@/components/ui/table';
import { useResponsive } from '@/hooks/useResponsive';
import { cn } from '@/lib/utils';
import type { ColumnDef, SortingState } from '@tanstack/react-table';
import { flexRender, getCoreRowModel, useReactTable } from '@tanstack/react-table';
import { ArrowDown, ArrowUp, ArrowUpDown, RotateCcw, Search } from 'lucide-react';
import * as React from 'react';
import { createPortal } from 'react-dom';
import { useTranslation } from 'react-i18next';

export interface EnhancedDataTableProps<TData, TValue> {
    columns: ColumnDef<TData, TValue>[];
    data: TData[];
    pageIndex?: number;
    pageSize?: number;
    total?: number;
    onPageChange?: (pageIndex: number, pageSize: number) => void;
    onPageSizeChange?: (pageSize: number) => void;

    // Server-side sorting support
    sortBy?: string;
    sortOrder?: 'asc' | 'desc';
    onSortChange?: (sortBy: string, sortOrder: 'asc' | 'desc') => void;

    // Search functionality
    searchValue?: string;
    searchOptions?: SearchOption[];
    searchLoading?: boolean;
    onSearchChange?: (query: string) => void;
    onSearchValueChange?: (value: string) => void;
    onSearchSubmit?: () => void;
    searchPlaceholder?: string;
    allowSearchAdditions?: boolean;

    // Toolbar actions
    toolbarActions?: React.ReactNode;
    onRefresh?: () => void;

    // Row interaction
    onRowClick?: (row: TData) => void;
    floatingRowActions?: (row: TData) => React.ReactNode;

    // Responsive options
    mobileCardLayout?: boolean;
    hideColumnsOnMobile?: string[];
    compactMode?: boolean;

    loading?: boolean;
    className?: string;
    emptyMessage?: string;
}

export function EnhancedDataTable<TData, TValue>({
    columns,
    data,
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
    searchPlaceholder,
    allowSearchAdditions = true,
    toolbarActions,
    onRefresh,
    onRowClick,
    floatingRowActions,
    mobileCardLayout = true,
    hideColumnsOnMobile = [],
    compactMode = false,
    loading = false,
    className,
    emptyMessage,
}: EnhancedDataTableProps<TData, TValue>) {
    const { t } = useTranslation();
    const { isMobile, isTablet } = useResponsive();
    // Client-side sorting state (for display only when no server-side sorting)
    const [sorting, setSorting] = React.useState<SortingState>([]);

    // Default values with translation
    const effectiveSearchPlaceholder =
        searchPlaceholder || t('common.search_placeholder', 'Search...');
    const effectiveEmptyMessage = emptyMessage || t('common.no_data', 'No results found.');

    // Floating actions state
    const [hoveredRowData, setHoveredRowData] = React.useState<TData | null>(null);
    const [floatingPos, setFloatingPos] = React.useState<{ top: number; left: number } | null>(
        null
    );
    const hoverTimeoutRef = React.useRef<NodeJS.Timeout>();

    const handleRowMouseEnter = (event: React.MouseEvent<HTMLTableRowElement>, row: TData) => {
        if (!floatingRowActions) return;
        if (hoverTimeoutRef.current) clearTimeout(hoverTimeoutRef.current);

        setFloatingPos({
            top: event.clientY,
            left: event.clientX + 16,
        });
        setHoveredRowData(row);
    };

    const handleRowMouseLeave = () => {
        if (!floatingRowActions) return;
        hoverTimeoutRef.current = setTimeout(() => {
            setHoveredRowData(null);
            setFloatingPos(null);
        }, 100);
    };

    const handleFloatingMouseEnter = () => {
        if (hoverTimeoutRef.current) clearTimeout(hoverTimeoutRef.current);
    };

    const handleFloatingMouseLeave = () => {
        hoverTimeoutRef.current = setTimeout(() => {
            setHoveredRowData(null);
            setFloatingPos(null);
        }, 100);
    };

    // Handle column header click for server-side sorting
    const handleSort = (accessorKey: string) => {
        if (!onSortChange) return;
        if (loading) return; // Prevent repeated actions while loading

        // If clicking the same column, toggle order
        if (sortBy === accessorKey) {
            const newOrder = sortOrder === 'desc' ? 'asc' : 'desc';
            onSortChange(accessorKey, newOrder);
        } else {
            // New column, default to asc (tests expect ascending on first click)
            onSortChange(accessorKey, 'asc');
        }
    };

    // Get sort icon based on current sort state
    const getSortIcon = (accessorKey: string) => {
        if (!onSortChange) return <ArrowUpDown className="ml-2 h-4 w-4 opacity-50" />;

        if (sortBy === accessorKey) {
            return sortOrder === 'asc' ? (
                <ArrowUp className="ml-2 h-4 w-4" />
            ) : (
                <ArrowDown className="ml-2 h-4 w-4" />
            );
        }
        return <ArrowUpDown className="ml-2 h-4 w-4 opacity-50" />;
    };

    // Enhanced columns with sorting support
    const enhancedColumns = columns.map((column) => {
        // Check if column has accessorKey for sorting
        const hasAccessorKey = 'accessorKey' in column && typeof column.accessorKey === 'string';
        const accessorKey = hasAccessorKey ? (column.accessorKey as string) : '';

        if (!accessorKey || !onSortChange) return column;

        return {
            ...column,
            header: () => {
                const headerContent =
                    typeof column.header === 'string' ? column.header : accessorKey;

                return (
                    <Button
                        variant="ghost"
                        onClick={() => handleSort(accessorKey)}
                        className="h-auto p-0 font-semibold hover:bg-transparent"
                    >
                        <span>{headerContent}</span>
                        {getSortIcon(accessorKey)}
                    </Button>
                );
            },
        } as ColumnDef<TData, TValue>;
    });

    const table = useReactTable({
        data,
        columns: enhancedColumns,
        state: {
            sorting,
            pagination: {
                pageIndex,
                pageSize,
            },
        },
        onSortingChange: setSorting,
        getCoreRowModel: getCoreRowModel(),
        manualSorting: !!onSortChange, // Use manual sorting if server-side sorting is available
        manualPagination: true,
        pageCount: Math.ceil(total / pageSize),
    });

    const handleSearchAddition = (value: string) => {
        if (onSearchValueChange) {
            onSearchValueChange(value);
        }
    };

    // Filter columns for mobile display
    const getVisibleColumns = () => {
        if (!isMobile || hideColumnsOnMobile.length === 0) return columns;

        return columns.filter((column) => {
            const accessorKey = 'accessorKey' in column ? (column.accessorKey as string) : '';
            return !hideColumnsOnMobile.includes(accessorKey);
        });
    };

    const visibleColumns = getVisibleColumns();

    return (
        <div className={cn('space-y-4', className)}>
            {/* Search and Actions Toolbar */}
            {(onSearchChange || toolbarActions || onRefresh) && (
                <div
                    className={cn(
                        'flex gap-4 flex-wrap',
                        isMobile ? 'flex-col space-y-4' : 'items-center justify-between'
                    )}
                >
                    <div
                        className={cn(
                            'flex gap-2 flex-wrap',
                            isMobile ? 'flex-col space-y-2' : 'flex-1 items-center'
                        )}
                    >
                        {onSearchChange && (
                            <>
                                <div className={cn(isMobile ? 'w-full' : 'flex-1 max-w-md')}>
                                    <SearchableDropdown
                                        value={searchValue}
                                        placeholder={effectiveSearchPlaceholder}
                                        searchPlaceholder={effectiveSearchPlaceholder}
                                        options={searchOptions}
                                        onSearchChange={onSearchChange}
                                        onChange={onSearchValueChange}
                                        onAddItem={
                                            allowSearchAdditions ? handleSearchAddition : undefined
                                        }
                                        loading={searchLoading}
                                        noResultsMessage={t(
                                            'common.no_results',
                                            'No results found'
                                        )}
                                        additionLabel={t('common.search_for', 'Search for: ')}
                                        allowAdditions={allowSearchAdditions}
                                        clearable={true}
                                    />
                                </div>
                                {onSearchSubmit && (
                                    <Button
                                        onClick={onSearchSubmit}
                                        disabled={loading}
                                        variant="outline"
                                        className={cn(
                                            isMobile ? 'w-full touch-target' : '',
                                            'gap-2'
                                        )}
                                    >
                                        <Search className="h-4 w-4" />
                                        {!isMobile && t('common.search', 'Search')}
                                    </Button>
                                )}
                            </>
                        )}
                    </div>

                    <div
                        className={cn('flex gap-2 flex-wrap', isMobile ? 'w-full' : 'items-center')}
                    >
                        {onRefresh && (
                            <Button
                                onClick={onRefresh}
                                disabled={loading}
                                variant="outline"
                                size={compactMode || isMobile ? 'sm' : 'sm'}
                                className={cn(isMobile ? 'flex-1 touch-target' : '', 'gap-2')}
                            >
                                <RotateCcw className="h-4 w-4" />
                                {!compactMode && !isMobile && t('common.refresh', 'Refresh')}
                            </Button>
                        )}
                        <div className={cn(isMobile ? 'flex gap-2 flex-1' : 'flex gap-2')}>
                            {toolbarActions}
                        </div>
                    </div>
                </div>
            )}

            {/* Data Table */}
            <div className="relative">
                {/* Loading overlay */}
                {loading && (
                    <div className="absolute inset-0 z-10 flex items-center justify-center bg-background/60 backdrop-blur-sm rounded-md">
                        <div className="text-sm text-muted-foreground">
                            {t('common.loading', 'Loading...')}
                        </div>
                    </div>
                )}

                {/* Mobile Card Layout */}
                {isMobile && mobileCardLayout ? (
                    <div className="space-y-4">
                        {table.getRowModel().rows?.length ? (
                            table.getRowModel().rows.map((row) => (
                                <div
                                    key={row.id}
                                    className="bg-card border rounded-lg p-3 space-y-2 shadow-sm"
                                >
                                    {row.getVisibleCells().map((cell) => {
                                        const headerDef = cell.column.columnDef.header;
                                        const label =
                                            typeof headerDef === 'string'
                                                ? headerDef
                                                : typeof headerDef === 'function'
                                                  ? cell.column.id
                                                  : cell.column.id || '';

                                        // Skip rendering if this column should be hidden on mobile
                                        const accessorKey =
                                            'accessorKey' in cell.column.columnDef
                                                ? (cell.column.columnDef.accessorKey as string)
                                                : '';
                                        if (hideColumnsOnMobile.includes(accessorKey)) {
                                            return null;
                                        }

                                        return (
                                            <div key={cell.id} className="flex flex-col gap-1">
                                                <span className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
                                                    {label}
                                                </span>
                                                <div
                                                    className="text-sm text-foreground break-words"
                                                    data-label={label}
                                                >
                                                    {flexRender(
                                                        cell.column.columnDef.cell,
                                                        cell.getContext()
                                                    )}
                                                </div>
                                            </div>
                                        );
                                    })}
                                </div>
                            ))
                        ) : (
                            <div className="bg-card border rounded-lg p-8 text-center">
                                <div className="text-muted-foreground">
                                    {loading
                                        ? t('common.loading', 'Loading...')
                                        : effectiveEmptyMessage}
                                </div>
                            </div>
                        )}
                    </div>
                ) : (
                    /* Desktop/Tablet Table Layout with horizontal scroll support */
                    <div className="rounded-md border overflow-x-auto">
                        <div className="overflow-x-auto w-full">
                            <Table
                                className={cn(
                                    'min-w-max',
                                    loading && 'pointer-events-none opacity-60'
                                )}
                            >
                                <TableHeader>
                                    {table.getHeaderGroups().map((headerGroup) => (
                                        <TableRow key={headerGroup.id}>
                                            {headerGroup.headers.map((header) => {
                                                // Skip rendering if this column should be hidden on mobile/tablet
                                                const accessorKey =
                                                    'accessorKey' in header.column.columnDef
                                                        ? (header.column.columnDef
                                                              .accessorKey as string)
                                                        : '';
                                                if (
                                                    isTablet &&
                                                    hideColumnsOnMobile.includes(accessorKey)
                                                ) {
                                                    return null;
                                                }

                                                return (
                                                    <TableHead
                                                        key={header.id}
                                                        className={cn(
                                                            compactMode ? 'px-2 py-2' : 'px-4 py-3'
                                                        )}
                                                    >
                                                        {header.isPlaceholder
                                                            ? null
                                                            : flexRender(
                                                                  header.column.columnDef.header,
                                                                  header.getContext()
                                                              )}
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
                                                data-state={row.getIsSelected() && 'selected'}
                                                className={cn(
                                                    'hover:bg-muted/50 transition-colors',
                                                    onRowClick && 'cursor-pointer'
                                                )}
                                                onClick={() => onRowClick?.(row.original)}
                                                onMouseEnter={(e) =>
                                                    handleRowMouseEnter(e, row.original)
                                                }
                                                onMouseLeave={handleRowMouseLeave}
                                            >
                                                {row.getVisibleCells().map((cell) => {
                                                    // Skip rendering if this column should be hidden on mobile/tablet
                                                    const accessorKey =
                                                        'accessorKey' in cell.column.columnDef
                                                            ? (cell.column.columnDef
                                                                  .accessorKey as string)
                                                            : '';
                                                    if (
                                                        isTablet &&
                                                        hideColumnsOnMobile.includes(accessorKey)
                                                    ) {
                                                        return null;
                                                    }

                                                    return (
                                                        <TableCell
                                                            key={cell.id}
                                                            className={cn(
                                                                compactMode
                                                                    ? 'px-2 py-2'
                                                                    : 'px-4 py-3'
                                                            )}
                                                        >
                                                            {flexRender(
                                                                cell.column.columnDef.cell,
                                                                cell.getContext()
                                                            )}
                                                        </TableCell>
                                                    );
                                                })}
                                            </TableRow>
                                        ))
                                    ) : (
                                        <TableRow>
                                            <TableCell
                                                colSpan={visibleColumns.length}
                                                className="h-24 text-center"
                                            >
                                                {loading
                                                    ? t('common.loading', 'Loading...')
                                                    : effectiveEmptyMessage}
                                            </TableCell>
                                        </TableRow>
                                    )}
                                </TableBody>
                            </Table>
                        </div>
                    </div>
                )}
            </div>

            {/* Advanced Pagination */}
            <AdvancedPagination
                currentPage={pageIndex + 1}
                totalPages={Math.ceil(total / pageSize)}
                pageSize={pageSize}
                totalItems={total}
                onPageChange={(page) => onPageChange?.(page - 1, pageSize)}
                onPageSizeChange={(newPageSize) => {
                    onPageSizeChange?.(newPageSize);
                    // Reset to first page when changing page size
                    onPageChange?.(0, newPageSize);
                }}
                loading={loading}
            />

            {/* Floating Row Actions */}
            {floatingRowActions &&
                hoveredRowData &&
                floatingPos &&
                createPortal(
                    <div
                        className="fixed z-50 transform -translate-y-1/2 bg-background border rounded-md shadow-lg p-1 animate-in fade-in zoom-in-95 duration-100"
                        style={{
                            top: floatingPos.top,
                            left: floatingPos.left,
                        }}
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
