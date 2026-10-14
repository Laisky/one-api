import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import {
  getTableSortStorageKey,
  isTableSortPreference,
  readTableSortPreference,
  writeTableSortPreference,
  type TableSortPreference,
} from '../table-sort-preference';

const fallback: TableSortPreference = { sortBy: 'id', sortOrder: 'desc' };
const chosen: TableSortPreference = { sortBy: 'name', sortOrder: 'asc' };
const key = getTableSortStorageKey('channels', 'account-a');

beforeEach(() => {
  localStorage.clear();
  vi.spyOn(console, 'warn').mockImplementation(() => {});
});

afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
  localStorage.clear();
});

describe('table sort preferences', () => {
  it('does not overwrite defaults when nothing has been saved', () => {
    expect(readTableSortPreference(key, fallback)).toEqual(fallback);
    expect(localStorage.getItem(key)).toBeNull();
  });

  it('round-trips both the column and direction', () => {
    writeTableSortPreference(key, chosen);
    expect(readTableSortPreference(key, fallback)).toEqual(chosen);
  });

  it('preserves an explicit return to default ordering', () => {
    const value: TableSortPreference = { sortBy: '', sortOrder: 'desc' };
    writeTableSortPreference(key, value);
    expect(readTableSortPreference(key, fallback)).toEqual(value);
  });

  it('isolates lists and principals with unambiguous storage keys', () => {
    writeTableSortPreference(key, chosen);
    expect(readTableSortPreference(getTableSortStorageKey('tokens', 'account-a'), fallback)).toEqual(fallback);
    expect(readTableSortPreference(getTableSortStorageKey('channels', 'account-b'), fallback)).toEqual(fallback);
    expect(getTableSortStorageKey('a:b', 'c')).not.toBe(getTableSortStorageKey('a', 'b:c'));
  });

  it.each([
    ['malformed JSON', '{'],
    ['null', 'null'],
    ['array', '[]'],
    ['primitive', 'true'],
    ['missing direction', '{"sortBy":"name"}'],
    ['unknown direction', '{"sortBy":"name","sortOrder":"sideways"}'],
    ['wrong column type', '{"sortBy":1,"sortOrder":"asc"}'],
    ['query string fragments', '{"sortBy":"name&size=999","sortOrder":"asc"}'],
    ['oversized value', ' '.repeat(513)],
  ])('falls back safely for %s', (_label, raw) => {
    localStorage.setItem(key, raw);
    expect(readTableSortPreference(key, fallback)).toEqual(fallback);
    expect(console.warn).toHaveBeenCalled();
  });

  it('stores and returns only sort fields, not extra properties or row data', () => {
    writeTableSortPreference(key, { ...chosen, rows: ['private-row'] } as TableSortPreference);
    expect(JSON.parse(localStorage.getItem(key)!)).toEqual(chosen);
    localStorage.setItem(key, JSON.stringify({ ...chosen, rows: ['private-row'] }));
    expect(readTableSortPreference(key, fallback)).toEqual(chosen);
  });

  it('rejects invalid writes instead of replacing a good saved preference', () => {
    writeTableSortPreference(key, chosen);
    writeTableSortPreference(key, { sortBy: 'name', sortOrder: 'invalid' } as unknown as TableSortPreference);
    expect(readTableSortPreference(key, fallback)).toEqual(chosen);
    expect(isTableSortPreference({ sortBy: 'a'.repeat(65), sortOrder: 'asc' })).toBe(false);
  });

  it('survives blocked storage reads without logging stored values or account IDs', () => {
    vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => { throw new Error('private-account-value'); });
    expect(readTableSortPreference(key, fallback)).toEqual(fallback);
    expect(JSON.stringify(vi.mocked(console.warn).mock.calls)).not.toContain('private-account-value');
    expect(JSON.stringify(vi.mocked(console.warn).mock.calls)).not.toContain('account-a');
  });

  it('survives blocked or full storage writes', () => {
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => { throw new Error('quota'); });
    expect(() => writeTableSortPreference(key, chosen)).not.toThrow();
    expect(console.warn).toHaveBeenCalled();
  });

  it('is safe when rendered without a browser window', () => {
    vi.stubGlobal('window', undefined);
    expect(readTableSortPreference(key, fallback)).toEqual(fallback);
    expect(() => writeTableSortPreference(key, chosen)).not.toThrow();
  });
});
