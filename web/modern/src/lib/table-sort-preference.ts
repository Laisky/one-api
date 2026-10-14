/** TableSortPreference contains only a column identifier and its ordering, never row data. */
export interface TableSortPreference {
  sortBy: string;
  sortOrder: 'asc' | 'desc';
}

// This registry covers existing header and toolbar choices, including default
// IDs that are not visible columns. Page contract tests prevent column drift.
const TABLE_SORT_COLUMNS: Readonly<Record<string, readonly string[]>> = {
  channels: ['id', 'name', 'type', 'status', 'group', 'priority', 'weight', 'balance', 'response_time', 'testing_model', 'created_time'],
  users: ['id', 'username', 'display_name', 'group', 'used_quota', 'quota', 'created_at'],
  tokens: ['id', 'name', 'key', 'status', 'remain_quota', 'used_quota', 'created_time', 'accessed_time', 'expired_time'],
  redemptions: ['id', 'name', 'key', 'quota', 'status', 'created_time', 'redeemed_time'],
  mcp: ['id', 'name', 'status', 'priority', 'base_url', 'protocol', 'auth_type', 'tool_count', 'last_sync_at'],
  logs: ['created_at', 'channel', 'type', 'model_name', 'username', 'token_name', 'prompt_tokens', 'completion_tokens', 'quota', 'elapsed_time'],
};

/** getTableSortColumns returns the supported fields for a built-in list, or undefined for a caller-defined table. */
export function getTableSortColumns(table: string): readonly string[] | undefined {
  return Object.hasOwn(TABLE_SORT_COLUMNS, table) ? TABLE_SORT_COLUMNS[table] : undefined;
}

/** getTableSortStorageKey returns a versioned browser key scoped to one table and authenticated principal. */
export function getTableSortStorageKey(table: string, principal: string): string {
  return `oneapi_table_sort_v1:${JSON.stringify([table, principal])}`;
}

/** isTableSortPreference validates an unknown value before its fields can enter API query strings. */
export function isTableSortPreference(value: unknown, allowedColumns?: readonly string[]): value is TableSortPreference {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return false;
  const candidate = value as Partial<TableSortPreference>;
  return typeof candidate.sortBy === 'string'
    && (candidate.sortBy === '' || /^[a-zA-Z_][a-zA-Z0-9_]{0,63}$/.test(candidate.sortBy))
    && (candidate.sortBy === '' || !allowedColumns || allowedColumns.includes(candidate.sortBy))
    && (candidate.sortOrder === 'asc' || candidate.sortOrder === 'desc');
}

/** readTableSortPreference returns validated saved ordering or the supplied default when storage is unavailable or invalid. */
export function readTableSortPreference(key: string, fallback: TableSortPreference, allowedColumns?: readonly string[]): TableSortPreference {
  if (typeof window === 'undefined') return fallback;
  try {
    const raw = window.localStorage.getItem(key);
    if (raw === null) return fallback;
    if (raw.length > 512) {
      console.warn('Ignoring an oversized table sort preference.');
      return fallback;
    }
    const parsed: unknown = JSON.parse(raw);
    if (isTableSortPreference(parsed, allowedColumns)) return { sortBy: parsed.sortBy, sortOrder: parsed.sortOrder };
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
