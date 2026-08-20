import { rowPaginationFeature, rowSortingFeature, tableFeatures } from '@tanstack/react-table';
import type { ColumnDef, RowData } from '@tanstack/react-table';

export const modernTableFeatures = tableFeatures({
  rowPaginationFeature,
  rowSortingFeature,
});

export type ModernColumnDef<
  TData extends RowData,
  TValue = unknown,
> = ColumnDef<typeof modernTableFeatures, TData, TValue>;
