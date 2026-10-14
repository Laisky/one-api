/** TableSortPreference contains only a column identifier and its ordering, never row data. */
export interface TableSortPreference {
  sortBy: string;
  sortOrder: 'asc' | 'desc';
}

/** getTableSortStorageKey returns a versioned browser key scoped to one table and authenticated principal. */
export function getTableSortStorageKey(table: string, principal: string): string {
  return `oneapi_table_sort_v1:${JSON.stringify([table, principal])}`;
}

/** isTableSortPreference validates an unknown value before its fields can enter API query strings. */
export function isTableSortPreference(value: unknown): value is TableSortPreference {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return false;
  const candidate = value as Partial<TableSortPreference>;
  return typeof candidate.sortBy === 'string'
    && (candidate.sortBy === '' || /^[a-zA-Z_][a-zA-Z0-9_]{0,63}$/.test(candidate.sortBy))
    && (candidate.sortOrder === 'asc' || candidate.sortOrder === 'desc');
}

/** readTableSortPreference returns validated saved ordering or the supplied default when storage is unavailable or invalid. */
export function readTableSortPreference(key: string, fallback: TableSortPreference): TableSortPreference {
  if (typeof window === 'undefined') return fallback;
  try {
    const raw = window.localStorage.getItem(key);
    if (raw === null) return fallback;
    if (raw.length > 512) {
      console.warn('Ignoring an oversized table sort preference.');
      return fallback;
    }
    const parsed: unknown = JSON.parse(raw);
    if (isTableSortPreference(parsed)) return { sortBy: parsed.sortBy, sortOrder: parsed.sortOrder };
    console.warn('Ignoring an invalid table sort preference.');
  } catch {
    // Keep diagnostics useful without printing account IDs, stored values, or browser error objects.
    console.warn('Unable to restore a table sort preference; using the default ordering.');
  }
  return fallback;
}

/** writeTableSortPreference saves a validated sort pair without interrupting in-memory sorting if storage fails. */
export function writeTableSortPreference(key: string, value: TableSortPreference): void {
  if (typeof window === 'undefined' || !isTableSortPreference(value)) return;
  try {
    window.localStorage.setItem(key, JSON.stringify({ sortBy: value.sortBy, sortOrder: value.sortOrder }));
  } catch {
    console.warn('Unable to save a table sort preference; sorting remains available for this page.');
  }
}
