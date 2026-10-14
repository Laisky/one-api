import { useCallback, useEffect, useState } from 'react';
import { useAuthStore } from '@/lib/stores/auth';
import {
  getTableSortStorageKey,
  isTableSortPreference,
  readTableSortPreference,
  writeTableSortPreference,
  type TableSortPreference,
} from '@/lib/table-sort-preference';

/** StoredTableSort binds an in-memory sort pair to its owning storage key and explicit user changes. */
interface StoredTableSort {
  key: string;
  value: TableSortPreference;
  dirty: boolean;
}

/** useTableSort restores a table's account-scoped ordering synchronously and returns the sort pair and independent setters. */
export function useTableSort(table: string, defaultBy: string = '', defaultOrder: 'asc' | 'desc' = 'desc') {
  const principal = useAuthStore((state) => JSON.stringify([
    state.user?.uuid || state.user?.username || state.user?.id || null,
    state.user?.role ?? null,
  ]));
  const key = getTableSortStorageKey(table, principal);
  const [stored, setStored] = useState<StoredTableSort>(() => ({
    key,
    value: readTableSortPreference(key, { sortBy: defaultBy, sortOrder: defaultOrder }),
    dirty: false,
  }));
  // React retries this component before committing children or effects. A new
  // account/table therefore never fetches with, or persists, the old key's sort.
  let current = stored;
  if (stored.key !== key) {
    current = {
      key,
      value: readTableSortPreference(key, { sortBy: defaultBy, sortOrder: defaultOrder }),
      dirty: false,
    };
    setStored(current);
  }

  useEffect(() => {
    // Write the state's own key, not a possibly newer render's key. Do not save
    // defaults on mount, and keep storage side effects out of state updaters.
    if (stored.dirty) writeTableSortPreference(stored.key, stored.value);
  }, [stored]);

  /** setSortBy updates the column while composing with any queued direction change. */
  const setSortBy = useCallback((sortBy: string) => {
    setStored((previous) => {
      if (previous.key !== key || previous.value.sortBy === sortBy) return previous;
      const value = { ...previous.value, sortBy };
      return isTableSortPreference(value) ? { key, value, dirty: true } : previous;
    });
  }, [key]);

  /** setSortOrder updates the direction while composing with any queued column change. */
  const setSortOrder = useCallback((sortOrder: 'asc' | 'desc') => {
    setStored((previous) => {
      if (previous.key !== key || previous.value.sortOrder === sortOrder) return previous;
      const value = { ...previous.value, sortOrder };
      return isTableSortPreference(value) ? { key, value, dirty: true } : previous;
    });
  }, [key]);

  return { ...current.value, setSortBy, setSortOrder };
}
