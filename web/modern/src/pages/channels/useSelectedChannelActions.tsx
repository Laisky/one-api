import { useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/button';
import { useConfirmDialog } from '@/components/ui/confirm-dialog';
import { useNotifications } from '@/components/ui/notifications';
import type { TableSelection } from '@/hooks/useTableSelection';
import { api } from '@/lib/api';
import { mapWithConcurrency } from '@/lib/export';
import { resetFailureMessage } from './useChannelModelReset';

/** ChannelTarget is the non-secret UUID snapshot returned before batch confirmation. */
interface ChannelTarget {
  uuid: string;
  name: string;
}
/** ChannelOutcome reports each selected channel's result, including intentional status skips. */
interface ChannelOutcome extends ChannelTarget {
  success: boolean;
  skipped?: boolean;
  message?: string;
  conflict?: { code: string; field?: string; models?: string[] };
}
/** ChannelBatchAction names the existing record operations now restricted to selected channels. */
export type ChannelBatchAction = 'reset' | 'test' | 'enable' | 'disable' | 'delete_disabled';

/** useSelectedChannelActions resolves a stable selected UUID set, confirms its size, and never calls an unscoped mutation. */
export function useSelectedChannelActions(selection: TableSelection, keyword: string, onCompleted: () => Promise<void>) {
  const { t } = useTranslation();
  const { notify } = useNotifications();
  const [confirm, ConfirmDialog] = useConfirmDialog();
  const [busy, setBusy] = useState(false);
  const [results, setResults] = useState<ChannelOutcome[] | null>(null);
  const inFlight = useRef(false);
  const current = useRef({ keyword, onCompleted, version: selection.scopeVersion });
  current.current = { keyword, onCompleted, version: selection.scopeVersion };

  /** run freezes target UUIDs before confirmation and reports failures without widening the selection. */
  const run = async (action: ChannelBatchAction) => {
    if (inFlight.current || !selection.hasSelection) return;
    inFlight.current = true;
    setBusy(true);
    let attempted = false;
    try {
      const resolved = (await api.post('/api/channel/selection', { selection: selection.snapshot, keyword })).data;
      if (!resolved?.success || !Array.isArray(resolved.data)) throw new Error(resolved?.message || t('table_selection.failed'));
      const targets = resolved.data as ChannelTarget[];
      if (!targets.length) {
        notify({ type: 'info', message: t('table_selection.empty') });
        return;
      }
      if (current.current.version !== selection.scopeVersion) return;
      const changesStatus = action === 'enable' || action === 'disable';
      const confirmationKey = changesStatus
        ? `table_selection.confirm_${action}`
        : action === 'delete_disabled'
          ? 'table_selection.confirm_disabled'
          : 'table_selection.confirm';
      const confirmed = await confirm({
        title: t(`table_selection.${action}`),
        description: t(confirmationKey, {
          count: targets.length,
        }),
        details: [{ label: t('table_selection.scope'), value: keyword || t('table_selection.unfiltered') }],
        confirmLabel: t('common.confirm'),
        variant: action === 'test' || action === 'enable' ? 'default' : 'destructive',
      });
      if (!confirmed || current.current.version !== selection.scopeVersion) return;
      setResults(null);
      attempted = true;
      const payload = { selection: { mode: 'ids', ids: targets.map((target) => target.uuid) } };
      let outcomes: ChannelOutcome[];
      if (action === 'test' || changesStatus) {
        outcomes = await mapWithConcurrency(
          targets,
          async (target) => {
            // Stop queued work if filters or the authenticated principal change.
            // Requests already in flight may have committed; report them normally.
            if (current.current.version !== selection.scopeVersion) {
              return { ...target, success: false, message: t('table_selection.scope_changed') };
            }
            try {
              const result = changesStatus
                ? (await api.put('/api/channel/?status_only=1', { uuid: target.uuid, status: action === 'enable' ? 1 : 2 })).data
                : (await api.get(`/api/channel/test/${encodeURIComponent(target.uuid)}`)).data;
              return {
                ...target,
                success: Boolean(result?.success) && !result?.skipped,
                skipped: Boolean(result?.skipped),
                message: result?.message,
              };
            } catch (error) {
              return {
                ...target,
                success: false,
                message: (error as { response?: { data?: { message?: string } } }).response?.data?.message || t('table_selection.failed'),
              };
            }
          },
          3
        );
      } else {
        const path = action === 'reset' ? '/api/channel/reset_models' : '/api/channel/delete_selected_disabled';
        const result = (await api.post(path, payload, { timeout: 120_000 })).data;
        if (!result?.success) throw new Error(result?.message || t('table_selection.failed'));
        const returned = action === 'reset' ? result.data?.results : result.data;
        if (!Array.isArray(returned)) throw new Error(t('table_selection.failed'));
        // A concurrently removed row must not disappear from the report or become
        // a claimed success. Every confirmed target gets an explicit outcome.
        const byId = new Map<string, ChannelOutcome>(returned.map((row: ChannelOutcome) => [row.uuid, row]));
        outcomes = targets.map((target) => byId.get(target.uuid) || { ...target, success: false, message: t('table_selection.missing') });
      }
      setResults(outcomes);
      selection.clear();
      notify({
        type: outcomes.some((result) => !result.success && !result.skipped) ? 'error' : 'success',
        message: t('table_selection.finished'),
      });
    } catch (error) {
      notify({
        type: 'error',
        message:
          (error as { response?: { data?: { message?: string } }; message?: string }).response?.data?.message ||
          (error as Error)?.message ||
          t('table_selection.failed'),
      });
    } finally {
      try {
        if (attempted) await current.current.onCompleted();
      } catch {
        notify({ type: 'error', message: t('channel_reset.refresh_failed') });
      }
      inFlight.current = false;
      setBusy(false);
    }
  };

  return {
    busy,
    run,
    confirmation: <ConfirmDialog />,
    report: results && (
      <section className="mb-4 space-y-2 rounded border p-3" aria-label={t('table_selection.results')}>
        <p role="status">
          {t('table_selection.summary', {
            total: results.length,
            success: results.filter((row) => row.success).length,
            skipped: results.filter((row) => row.skipped).length,
            failed: results.filter((row) => !row.success && !row.skipped).length,
          })}
        </p>
        <ul className="max-h-80 overflow-y-auto space-y-2 text-sm">
          {results
            .filter((row) => !row.success)
            .map((row) => (
              <li key={row.uuid}>
                <strong>{row.name}</strong> ({row.uuid}):{' '}
                {row.skipped ? row.message || t('table_selection.skipped_enabled') : resetFailureMessage(t, row)}
              </li>
            ))}
        </ul>
        <Button variant="ghost" size="sm" onClick={() => setResults(null)}>
          {t('channel_reset.dismiss')}
        </Button>
      </section>
    ),
  };
}
