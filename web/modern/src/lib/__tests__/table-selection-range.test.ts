import { describe, expect, it } from 'vitest';
import { getSelectionRange } from '../table-selection-range';

const ids = ['alpha', 'bravo', 'charlie', 'delta', 'echo', 'foxtrot'];

describe('getSelectionRange', () => {
  it('includes both endpoints and all rows between them', () => {
    expect(getSelectionRange(ids, 'alpha', 'echo')).toEqual(ids.slice(0, 5));
  });

  it('supports ranges in the reverse direction', () => {
    expect(getSelectionRange(ids, 'echo', 'alpha')).toEqual(ids.slice(0, 5));
  });

  it('uses visual ordering rather than alphabetical or numeric identity order', () => {
    expect(getSelectionRange(['echo', 'alpha', 'foxtrot', 'bravo'], 'alpha', 'bravo'))
      .toEqual(['alpha', 'foxtrot', 'bravo']);
  });

  it('uses only the target when there is no anchor', () => {
    expect(getSelectionRange(ids, null, 'echo')).toEqual(['echo']);
  });

  it('does not select unseen rows when the anchor is on another page', () => {
    expect(getSelectionRange(ids, 'other-page', 'echo')).toEqual(['echo']);
  });

  it('handles a range with identical endpoints', () => {
    expect(getSelectionRange(ids, 'charlie', 'charlie')).toEqual(['charlie']);
  });

  it('rejects a missing or empty target', () => {
    expect(getSelectionRange(ids, 'alpha', 'missing')).toEqual([]);
    expect(getSelectionRange(ids, 'alpha', '')).toEqual([]);
  });

  it('deduplicates stable IDs and excludes empty identities', () => {
    expect(getSelectionRange(['alpha', '', 'bravo', 'bravo', 'echo'], 'alpha', 'echo'))
      .toEqual(['alpha', 'bravo', 'echo']);
  });

  it('returns no rows for an empty page without mutating the source', () => {
    expect(getSelectionRange([], 'alpha', 'echo')).toEqual([]);
    const frozen = Object.freeze([...ids]);
    expect(getSelectionRange(frozen, 'alpha', 'echo')).toEqual(ids.slice(0, 5));
    expect(frozen).toEqual(ids);
  });
});
