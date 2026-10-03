import { act, renderHook } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { useTableSelection } from '../useTableSelection';

describe('scope-bound table selection', () => {
  it('preserves other pages when selecting and clearing the current page', () => {
    const { result } = renderHook(() => useTableSelection('filter-A'));
    act(() => result.current.setPage(['a', 'b'], true));
    act(() => result.current.setRow('off-page', true));
    act(() => result.current.setPage(['a', 'b'], false));
    expect(result.current.snapshot).toEqual({ mode: 'ids', ids: ['off-page'] });
    act(() => result.current.setPage(['a', 'a', ''], true));
    expect(result.current.selectedCount()).toBe(2);
  });

  it('represents all matching pages with exclusions instead of enumerating unseen rows', () => {
    const { result } = renderHook(() => useTableSelection('filter-A'));
    act(() => result.current.selectAllMatching());
    act(() => result.current.setRow('except-me', false));
    expect(result.current.isSelected('unseen-page')).toBe(true);
    expect(result.current.isSelected('except-me')).toBe(false);
    expect(result.current.selectedCount(25)).toBe(24);
    expect(result.current.selectedCount(null)).toBeNull();
    expect(result.current.snapshot).toEqual({ mode: 'all_matching', excluded_ids: ['except-me'] });
    act(() => result.current.setPage(['except-me', 'another'], true));
    expect(result.current.snapshot).toEqual({ mode: 'all_matching', excluded_ids: [] });
  });

  it('clears immediately on filter changes and ignores callbacks from the old scope', () => {
    const { result, rerender } = renderHook(({ scope }) => useTableSelection(scope), { initialProps: { scope: 'A' } });
    act(() => result.current.setRow('a', true));
    const old = result.current;
    rerender({ scope: 'B' });
    expect(result.current.hasSelection).toBe(false);
    act(() => old.selectAllMatching());
    expect(result.current.hasSelection).toBe(false);
    rerender({ scope: 'A' });
    act(() => old.selectAllMatching());
    expect(result.current.hasSelection).toBe(false);
  });
});
