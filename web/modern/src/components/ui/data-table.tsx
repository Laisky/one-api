import * as React from 'react'
import {
  flexRender,
  getCoreRowModel,
  useReactTable,
} from '@tanstack/react-table'
import type { ColumnDef, SortingState } from '@tanstack/react-table'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { Button } from '@/components/ui/button'
import { AdvancedPagination } from '@/components/ui/advanced-pagination'
import { ArrowUpDown, ArrowUp, ArrowDown } from 'lucide-react'
import { useTranslation } from 'react-i18next'

export interface DataTableProps<TData, TValue> {
  columns: ColumnDef<TData, TValue>[]
  data: TData[]
  pageIndex?: number
  pageSize?: number
  total?: number
  onPageChange?: (pageIndex: number, pageSize: number) => void
  onPageSizeChange?: (pageSize: number) => void
  // Server-side sorting support
  sortBy?: string
  sortOrder?: 'asc' | 'desc'
  onSortChange?: (sortBy: string, sortOrder: 'asc' | 'desc') => void
  loading?: boolean
}

export function DataTable<TData, TValue>({
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
  loading = false,
}: DataTableProps<TData, TValue>) {
  const { t } = useTranslation()
  // Client-side sorting state (for display only when no server-side sorting)
  const [sorting, setSorting] = React.useState<SortingState>([])

  // Handle column header click for server-side sorting
  const handleSort = (accessorKey: string) => {
    if (!onSortChange) return
    if (loading) return // Prevent repeated actions while loading

    // If clicking the same column, toggle order
    if (sortBy === accessorKey) {
      const newOrder = sortOrder === 'desc' ? 'asc' : 'desc'
      onSortChange(accessorKey, newOrder)
    } else {
      // New column, default to desc
      onSortChange(accessorKey, 'desc')
    }
  }

  const getSortIcon = (accessorKey: string) => {
    if (!onSortChange) return null

    if (sortBy === accessorKey) {
      return sortOrder === 'asc' ? (
        <ArrowUp className="ml-2 h-4 w-4" />
      ) : (
        <ArrowDown className="ml-2 h-4 w-4" />
      )
    }
    return <ArrowUpDown className="ml-2 h-4 w-4 opacity-50" />
  }

  // Enhanced columns with sorting support
  const enhancedColumns = columns.map((column) => {
    // Check if column has accessorKey for sorting
    const hasAccessorKey = 'accessorKey' in column && typeof column.accessorKey === 'string'
    const accessorKey = hasAccessorKey ? column.accessorKey as string : ''

    if (!accessorKey || !onSortChange) return column

    return {
      ...column,
      header: () => {
        const headerContent = typeof column.header === 'string' ? column.header : accessorKey

        return (
          <Button
            variant="ghost"
            onClick={() => handleSort(accessorKey)}
            className="h-auto p-0 font-semibold hover:bg-transparent"
          >
            <span>{headerContent}</span>
            {getSortIcon(accessorKey)}
          </Button>
        )
      },
    } as ColumnDef<TData, TValue>
  })

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
  })

  return (
    <div className="space-y-2">
      <div className="relative rounded-md border overflow-x-auto">
        {/* Loading overlay to prevent repeated actions */}
        {loading && (
          <div className="absolute inset-0 z-10 flex items-center justify-center bg-background/60 backdrop-blur-sm">
            <div className="text-sm text-muted-foreground">{t('common.loading', 'Loading...')}</div>
          </div>
        )}
        <Table className={loading ? 'pointer-events-none opacity-60' : ''}>
          <TableHeader>
            {table.getHeaderGroups().map((headerGroup) => (
              <TableRow key={headerGroup.id}>
                {headerGroup.headers.map((header) => (
                  <TableHead key={header.id} className="text-left mobile:whitespace-normal mobile:break-words">
                    {header.isPlaceholder
                      ? null
                      : flexRender(header.column.columnDef.header, header.getContext())}
                  </TableHead>
                ))}
              </TableRow>
            ))}
          </TableHeader>
          <TableBody>
            {table.getRowModel().rows?.length ? (
              table.getRowModel().rows.map((row) => (
                <TableRow key={row.id} data-state={row.getIsSelected() && 'selected'} className="mobile-table-row">
                  {row.getVisibleCells().map((cell) => {
                    const meta = cell.column.columnDef.meta as { mobileLabel?: string } | undefined
                    const headerDef = cell.column.columnDef.header
                    const label = meta?.mobileLabel || (typeof headerDef === 'string' ? headerDef : (cell.column.id || ''))
                    return (
                      <TableCell key={cell.id} data-label={label} className="mobile-table-cell break-words break-all">
                        {flexRender(cell.column.columnDef.cell, cell.getContext())}
                      </TableCell>
                    )
                  })}
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

      {/* Advanced Pagination */}
      <AdvancedPagination
        currentPage={pageIndex + 1}
        totalPages={Math.ceil(total / pageSize)}
        pageSize={pageSize}
        totalItems={total}
        onPageChange={(page) => onPageChange?.(page - 1, pageSize)}
        onPageSizeChange={(newPageSize) => {
          onPageSizeChange?.(newPageSize)
          // Reset to first page when changing page size
          onPageChange?.(0, newPageSize)
        }}
        loading={loading}
      />
    </div>
  )
}
