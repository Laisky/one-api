import { useCallback, useEffect, useRef, useState } from 'react';

/** TableSelectionSnapshot represents explicit stable IDs or a filtered universe minus exclusions. */
export type TableSelectionSnapshot = { mode: 'ids'; ids: string[] } | { mode: 'all_matching'; excluded_ids: string[] };

/** TableSelection exposes scope-bound selection operations without retaining row data or secrets. */
export interface TableSelection {
  snapshot: TableSelectionSnapshot;
  scopeVersion: number;
  hasSelection: boolean;
  isSelected: (id: string) => boolean;
  selectedCount: (total?: number | null) => number | null;
  setPage: (ids: string[], selected: boolean) => void;
  setRow: (id: string, selected: boolean) => void;
  selectAllMatching: () => void;
  clear: () => void;
}

/** isSelectedId checks stable identity against a captured selection, never a page-local row index. */
export function isSelectedId(selection: TableSelectionSnapshot, id: string): boolean {
  return Boolean(id) && (selection.mode === 'ids' ? selection.ids.includes(id) : !selection.excluded_ids.includes(id));
}

/** useTableSelection retains selection over page/sort changes and invalidates it when scope changes. */
export function useTableSelection(scope: string): TableSelection {
  const [stored, setStored] = useState<{ scope: string; snapshot: TableSelectionSnapshot }>({ scope, snapshot: { mode: 'ids', ids: [] } });
  const scopeState = useRef({ scope, version: 0 });
  if (scopeState.current.scope !== scope) scopeState.current = { scope, version: scopeState.current.version + 1 };
  const scopeVersion = scopeState.current.version;
  // Returning an empty selection during render prevents a stale batch action in
  // the render before the cleanup effect runs (including A -> B -> A changes).
  const snapshot: TableSelectionSnapshot = stored.scope === scope ? stored.snapshot : { mode: 'ids', ids: [] };
  useEffect(() => {
    setStored((previous) => (previous.scope === scope ? previous : { scope, snapshot: { mode: 'ids', ids: [] } }));
  }, [scope]);

  /** update applies a selection change only while its captured list scope is still current. */
  const update = useCallback(
    (change: (previous: TableSelectionSnapshot) => TableSelectionSnapshot) => {
      if (scopeState.current.version !== scopeVersion) return;
      setStored((previous) => ({ scope, snapshot: change(previous.scope === scope ? previous.snapshot : { mode: 'ids', ids: [] }) }));
    },
    [scope, scopeVersion]
  );

  /** setPage changes only the supplied page's IDs and preserves selections on other pages. */
  const setPage = (ids: string[], selected: boolean) =>
    update((previous) => {
      const values = new Set(previous.mode === 'ids' ? previous.ids : previous.excluded_ids);
      for (const id of ids.filter(Boolean)) {
        if (selected === (previous.mode === 'ids')) values.add(id);
        else values.delete(id);
      }
      return previous.mode === 'ids' ? { mode: 'ids', ids: [...values] } : { mode: 'all_matching', excluded_ids: [...values] };
    });

  return {
    snapshot,
    scopeVersion,
    hasSelection: snapshot.mode === 'all_matching' || snapshot.ids.length > 0,
    isSelected: (id) => isSelectedId(snapshot, id),
    selectedCount: (total) =>
      snapshot.mode === 'ids' ? snapshot.ids.length : total == null ? null : Math.max(0, total - snapshot.excluded_ids.length),
    setPage,
    setRow: (id, selected) => setPage([id], selected),
    selectAllMatching: () => update(() => ({ mode: 'all_matching', excluded_ids: [] })),
    clear: () => update(() => ({ mode: 'ids', ids: [] })),
  };
}
