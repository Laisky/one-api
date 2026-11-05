/**
 * mergeComboboxOptions returns the known options followed by any selected
 * values that are missing from them, trimmed, de-duplicated and with empty
 * strings removed. The original order is preserved so callers control ranking.
 * Selected values outside `options` stay listed so they can still be
 * deselected; once deselected they drop out and cannot be picked again.
 *
 * @param options - The known values offered by the caller.
 * @param selected - The currently selected values, which may include stale ones.
 * @returns The ordered, unique list of values the dropdown can display.
 */
export function mergeComboboxOptions(options: readonly string[], selected: readonly string[]): string[] {
  const seen = new Set<string>();
  const merged: string[] = [];
  for (const raw of [...options, ...selected]) {
    const value = raw.trim();
    if (value === '' || seen.has(value)) {
      continue;
    }
    seen.add(value);
    merged.push(value);
  }
  return merged;
}

/**
 * filterComboboxValues filters the available values by the typed query. An
 * empty query keeps every value. A non-empty query keeps case-insensitive
 * substring matches ranked as exact, prefix, then substring matches (stable
 * within each rank). Only values from `values` are ever returned, so typed
 * text can never become a new value.
 *
 * @param values - The candidate values, usually from mergeComboboxOptions.
 * @param query - The raw text typed into the combobox input.
 * @returns The ordered values to render in the dropdown.
 */
export function filterComboboxValues(values: readonly string[], query: string): string[] {
  const needle = query.trim().toLowerCase();
  if (needle === '') {
    return [...values];
  }

  const exact: string[] = [];
  const prefix: string[] = [];
  const substring: string[] = [];
  for (const value of values) {
    const haystack = value.toLowerCase();
    if (haystack === needle) {
      exact.push(value);
    } else if (haystack.startsWith(needle)) {
      prefix.push(value);
    } else if (haystack.includes(needle)) {
      substring.push(value);
    }
  }
  return [...exact, ...prefix, ...substring];
}
