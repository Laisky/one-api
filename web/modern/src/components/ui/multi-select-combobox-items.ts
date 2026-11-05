/**
 * ComboboxItem is one selectable row of a MultiSelectCombobox dropdown. An
 * `option` row toggles a known value, while a `create` row adds the typed text
 * as a new custom value.
 */
export interface ComboboxItem {
  kind: 'option' | 'create';
  value: string;
}

/**
 * mergeComboboxOptions returns the known options followed by any selected
 * values that are missing from them, trimmed, de-duplicated and with empty
 * strings removed. The original order is preserved so callers control ranking.
 *
 * @param options - The known values offered by the caller.
 * @param selected - The currently selected values, which may include custom ones.
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
 * buildComboboxItems filters the available values by the typed query and
 * returns the dropdown rows. An empty query lists every value. A non-empty
 * query keeps case-insensitive substring matches ranked as exact, prefix, then
 * substring matches (stable within each rank), and appends a `create` row when
 * custom values are allowed and no value equals the query exactly.
 *
 * @param values - The candidate values, usually from mergeComboboxOptions.
 * @param query - The raw text typed into the combobox input.
 * @param allowCustom - Whether a value outside `values` may be added.
 * @returns The ordered rows to render in the dropdown.
 */
export function buildComboboxItems(values: readonly string[], query: string, allowCustom: boolean): ComboboxItem[] {
  const trimmed = query.trim();
  if (trimmed === '') {
    return values.map((value) => ({ kind: 'option', value }));
  }

  const needle = trimmed.toLowerCase();
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

  const items: ComboboxItem[] = [...exact, ...prefix, ...substring].map((value) => ({ kind: 'option', value }));
  if (allowCustom && !values.includes(trimmed)) {
    items.push({ kind: 'create', value: trimmed });
  }
  return items;
}
