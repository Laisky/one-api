import { useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useConfirmDialog } from '@/components/ui/confirm-dialog';
import { useNotifications } from '@/components/ui/notifications';
import type { TableSelection } from '@/hooks/useTableSelection';
import { api } from '@/lib/api';
import { fromDateTimeLocal } from '@/lib/utils';
import type { LogCursorFilters } from '@/lib/logCursor';
import { logRef, type LogRow } from './logs-page-columns';
import { useLogExport } from './useLogExport';

/** useSelectedLogActions exports or deletes only selected records and freezes deletion targets before confirmation. */
export function useSelectedLogActions(
  selection: TableSelection,
  filters: LogCursorFilters,
  keyword: string,
  sort: string,
  order: 'asc' | 'desc',
  onDeleted: () => Promise<void>
) {
  const { t } = useTranslation();
  const { notify } = useNotifications();
  const [confirm, ConfirmDialog] = useConfirmDialog();
  const [deleting, setDeleting] = useState(false);
  const inFlight = useRef(false);
  const currentScope = useRef(selection.scopeVersion);
  currentScope.current = selection.scopeVersion;

  /** resolveSelected returns an authorized snapshot, not a page walker that could silently broaden the selection. */
  const resolveSelected = async (): Promise<LogRow[]> => {
    if (!selection.hasSelection || currentScope.current !== selection.scopeVersion) throw new Error(t('table_selection.empty'));
    const request = {
      selection: selection.snapshot,
      keyword,
      sort,
      order,
      type: Number(filters.type),
      model_name: filters.model_name,
      token_name: filters.token_name,
      username: filters.username,
      channel: filters.channel,
      start_timestamp: filters.start_timestamp ? fromDateTimeLocal(filters.start_timestamp) : 0,
      end_timestamp: filters.end_timestamp ? fromDateTimeLocal(filters.end_timestamp) : 0,
    };
    const result = (await api.post('/api/log/selection', request, { timeout: 120_000 })).data;
    if (currentScope.current !== selection.scopeVersion) throw new Error(t('table_selection.scope_changed'));
    if (!result?.success || !Array.isArray(result.data)) throw new Error(result?.message || t('table_selection.failed'));
    if (!result.data.length) throw new Error(t('table_selection.empty'));
    return result.data;
  };
  const exporter = useLogExport({ resolveSelected });

  /** deleteSelected revalidates authorization server-side and never sends a timestamp-only or all-matching deletion. */
  const deleteSelected = async () => {
    if (inFlight.current || exporter.exporting || !selection.hasSelection) return;
    inFlight.current = true;
    setDeleting(true);
    let attempted = false;
    try {
      const rows = await resolveSelected();
      const ids = rows.map((row) => String(logRef(row)));
      const confirmed = await confirm({
        title: t('table_selection.delete'),
        description: t('table_selection.confirm', { count: ids.length }),
        variant: 'destructive',
      });
      if (!confirmed || currentScope.current !== selection.scopeVersion) return;
      attempted = true;
      const result = (await api.post('/api/log/delete_selected', { selection: { mode: 'ids', ids } }, { timeout: 120_000 })).data;
      if (!result?.success || typeof result.data?.deleted !== 'number') throw new Error(result?.message || t('table_selection.failed'));
      selection.clear();
      notify({ type: 'success', message: t('table_selection.deleted', { count: result.data.deleted }) });
    } catch (error) {
      notify({
        type: 'error',
        message:
          (error as { response?: { data?: { message?: string } } }).response?.data?.message ||
          (error as Error)?.message ||
          t('table_selection.failed'),
      });
    } finally {
      try {
        if (attempted && currentScope.current === selection.scopeVersion) await onDeleted();
      } catch {
        notify({ type: 'error', message: t('table_selection.failed') });
      }
      inFlight.current = false;
      setDeleting(false);
    }
  };
  return { busy: deleting || exporter.exporting, exportSelected: exporter.exportLogs, deleteSelected, confirmation: <ConfirmDialog /> };
}
