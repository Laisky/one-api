import { defaultSelectionId } from '@/components/ui/table-selection';

const anonymousRecords = new WeakMap<object, number>();
let nextAnonymousId = 0;

/** getMobileRecordId uses public identity or object identity without serializing possibly sensitive row data. */
export function getMobileRecordId<TData>(row: TData, fallback: string, getId?: (row: TData) => string): string {
  const id = (getId ?? defaultSelectionId)(row);
  if (id) return `id:${id}`;
  if (row !== null && typeof row === 'object') {
    let anonymousId = anonymousRecords.get(row);
    if (anonymousId === undefined) {
      anonymousId = ++nextAnonymousId;
      anonymousRecords.set(row, anonymousId);
    }
    return `object:${anonymousId}`;
  }
  return `row:${fallback}`;
}
