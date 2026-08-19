import { Button } from '@/components/ui/button';
import { FormItem, FormMessage } from '@/components/ui/form';
import { Input } from '@/components/ui/input';
import { Plus, Trash2 } from 'lucide-react';
import { useEffect, useId, useRef, useState } from 'react';
import type { UseFormReturn } from 'react-hook-form';
import type { ChannelForm } from '../schemas';
import { LabelWithHelp } from './LabelWithHelp';

const COMMON_HEADER_KEYS = [
  'Authorization',
  'api-key',
  'Api-Key',
  'X-Api-Key',
  'x-api-key',
  'X-API-Key',
  'X-Goog-Api-Key',
  'anthropic-version',
  'anthropic-beta',
  'OpenAI-Beta',
  'HTTP-Referer',
  'X-Title',
  'User-Agent',
];

// CustomHeaderRow stores one editable custom-header row before it is normalized
// into the channel config record.
interface CustomHeaderRow {
  id: string;
  key: string;
  value: string;
}

// ChannelCustomHeadersProps describes the form bindings and translator used by
// the custom upstream-header editor.
interface ChannelCustomHeadersProps {
  form: UseFormReturn<ChannelForm>;
  tr: (key: string, defaultValue: string, options?: Record<string, unknown>) => string;
  /** Grid span for the outer FormItem; defaults to the two-column Basic Info grid. */
  className?: string;
}

// normalizeCustomHeaders returns a clean string map from unknown form state.
const normalizeCustomHeaders = (value: unknown): Record<string, string> => {
  if (!value || typeof value !== 'object' || Array.isArray(value)) {
    return {};
  }

  const normalized: Record<string, string> = {};
  Object.entries(value as Record<string, unknown>).forEach(([key, entry]) => {
    const trimmedKey = key.trim();
    if (trimmedKey === '') {
      return;
    }
    normalized[trimmedKey] = typeof entry === 'string' ? entry : String(entry ?? '');
  });
  return normalized;
};

// recordToRows converts the persisted custom-header map into stable UI rows.
const recordToRows = (record: Record<string, string>): CustomHeaderRow[] =>
  Object.entries(record).map(([key, value], index) => ({
    id: `${key}-${index}`,
    key,
    value,
  }));

// rowsToRecord converts UI rows into the channel config map, skipping blank keys.
const rowsToRecord = (rows: CustomHeaderRow[]): Record<string, string> => {
  const record: Record<string, string> = {};
  rows.forEach((row) => {
    const key = row.key.trim();
    if (key === '') {
      return;
    }
    record[key] = row.value;
  });
  return record;
};

// stableRecordKey serializes a record deterministically for form-state sync.
const stableRecordKey = (record: Record<string, string>): string =>
  JSON.stringify(
    Object.keys(record)
      .sort()
      .map((key) => [key, record[key]])
  );

// createHeaderRow returns a fresh editable custom-header row.
const createHeaderRow = (): CustomHeaderRow => ({
  id: globalThis.crypto?.randomUUID?.() ?? `${Date.now()}-${Math.random()}`,
  key: '',
  value: '',
});

// ChannelCustomHeaders renders channel-owned upstream headers with prefix
// autocomplete for common header keys.
export const ChannelCustomHeaders = ({ form, tr, className = 'col-span-1 md:col-span-2' }: ChannelCustomHeadersProps) => {
  const datalistId = useId().replaceAll(':', '-');
  const watchedHeaders = form.watch('config.custom_headers');
  const initialHeaders = useRef<Record<string, string> | null>(null);
  if (initialHeaders.current === null) {
    initialHeaders.current = normalizeCustomHeaders(watchedHeaders);
  }
  const [rows, setRows] = useState<CustomHeaderRow[]>(() => recordToRows(initialHeaders.current ?? {}));
  const lastSyncedRecord = useRef(stableRecordKey(initialHeaders.current ?? {}));

  useEffect(() => {
    const incoming = normalizeCustomHeaders(watchedHeaders);
    const incomingKey = stableRecordKey(incoming);
    if (incomingKey === lastSyncedRecord.current) {
      return;
    }
    lastSyncedRecord.current = incomingKey;
    setRows(recordToRows(incoming));
  }, [watchedHeaders]);

  const commitRows = (nextRows: CustomHeaderRow[]) => {
    setRows(nextRows);
    const nextRecord = rowsToRecord(nextRows);
    lastSyncedRecord.current = stableRecordKey(nextRecord);
    form.setValue('config.custom_headers', nextRecord, {
      shouldDirty: true,
      shouldValidate: true,
    });
  };

  const addRow = () => {
    commitRows([...rows, createHeaderRow()]);
  };

  const updateRow = (id: string, patch: Partial<Pick<CustomHeaderRow, 'key' | 'value'>>) => {
    commitRows(rows.map((row) => (row.id === id ? { ...row, ...patch } : row)));
  };

  const removeRow = (id: string) => {
    commitRows(rows.filter((row) => row.id !== id));
  };

  const customHeadersError = (form.formState.errors as any)?.config?.custom_headers?.message;

  return (
    <FormItem className={className}>
      <div className="flex flex-col gap-3">
        <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-2">
          <LabelWithHelp
            label={tr('custom_headers.label', 'Custom Headers')}
            help={tr(
              'custom_headers.help',
              'Headers configured here are always sent upstream and override client-provided headers. Use {{key}} to insert the channel API key.'
            )}
          />
          <Button type="button" variant="outline" size="sm" className="self-start sm:self-auto gap-2" onClick={addRow}>
            <Plus className="h-4 w-4" />
            {tr('custom_headers.add', 'Add Header')}
          </Button>
        </div>

        <datalist id={datalistId}>
          {COMMON_HEADER_KEYS.map((headerKey) => (
            <option key={headerKey} value={headerKey} />
          ))}
        </datalist>

        {rows.length === 0 ? (
          <div className="rounded-md border border-dashed px-3 py-3 text-sm text-muted-foreground">
            {tr('custom_headers.empty', 'No custom headers configured.')}
          </div>
        ) : (
          <div className="space-y-2">
            {rows.map((row) => (
              <div key={row.id} className="grid grid-cols-1 md:grid-cols-[minmax(0,0.75fr)_minmax(0,1fr)_2.5rem] gap-2">
                <Input
                  list={datalistId}
                  value={row.key}
                  onChange={(event) => updateRow(row.id, { key: event.target.value })}
                  placeholder={tr('custom_headers.key_placeholder', 'Header key')}
                  autoComplete="off"
                />
                <Input
                  value={row.value}
                  onChange={(event) => updateRow(row.id, { value: event.target.value })}
                  placeholder={tr('custom_headers.value_placeholder', 'Header value, e.g. {{key}}')}
                  autoComplete="off"
                />
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  className="h-10 w-10 sm:h-9 sm:w-9 text-muted-foreground hover:text-destructive"
                  onClick={() => removeRow(row.id)}
                  aria-label={tr('custom_headers.remove', 'Remove header')}
                >
                  <Trash2 className="h-4 w-4" />
                </Button>
              </div>
            ))}
          </div>
        )}
        {customHeadersError && <FormMessage>{customHeadersError}</FormMessage>}
      </div>
    </FormItem>
  );
};
