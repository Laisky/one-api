import { useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import type { TFunction } from 'i18next';
import { RotateCcw } from 'lucide-react';

import { Button } from '@/components/ui/button';
import { useConfirmDialog } from '@/components/ui/confirm-dialog';
import { ListActionButton } from '@/components/ui/list-action-button';
import { useNotifications } from '@/components/ui/notifications';
import { api } from '@/lib/api';
import { channelRef, type Channel } from './channels-page-columns';

/** ResetConflict contains only safe server-side policy diagnostics. */
interface ResetConflict {
  code: string;
  field?: string;
  models?: string[];
}

/** ResetOutcome describes one channel without credentials or internal IDs. */
interface ResetOutcome {
  uuid: string;
  name: string;
  success: boolean;
  model_count?: number;
  message?: string;
  conflict?: ResetConflict;
}

/** ResetSummary describes every channel considered by the server-side batch. */
interface ResetSummary {
  total: number;
  reset: number;
  rejected: number;
  failed: number;
  results: ResetOutcome[];
}

/** ResetResponse is the shared single/batch response and conflict envelope. */
interface ResetResponse {
  success: boolean;
  message?: string;
  conflict?: ResetConflict;
  data?: ResetOutcome | ResetSummary;
}

/** resetFailureMessage localizes known policy codes while retaining safe model details. */
export function resetFailureMessage(t: TFunction, outcome: Pick<ResetOutcome, 'conflict' | 'message'>): string {
  const conflict = outcome.conflict;
  if (!conflict) return outcome.message || t('channel_reset.failed_message');
  const reason = t(`channel_reset.reasons.${conflict.code}`, outcome.message || t('channel_reset.failed_message'));
  const field = conflict.field ? t('channel_reset.field', { field: conflict.field }) : '';
  const models = conflict.models?.length ? t('channel_reset.models', { models: conflict.models.join(', ') }) : '';
  return [reason, field, models].filter(Boolean).join(' ');
}

/** ChannelModelResetButton renders the same reset action in regular and floating rows. */
export function ChannelModelResetButton({
  channel,
  disabled,
  compact = false,
  onReset,
}: {
  channel: Channel;
  disabled: boolean;
  compact?: boolean;
  onReset: (channel: Channel) => Promise<void>;
}) {
  const { t } = useTranslation();
  return (
    <ListActionButton
      variant={compact ? 'ghost' : 'outline'}
      size="sm"
      className="gap-1"
      onClick={() => onReset(channel)}
      disabled={disabled}
      title={t('channel_reset.action')}
      aria-label={t('channel_reset.action_label', { name: channel.name })}
      icon={<RotateCcw className="h-4 w-4" />}
    >
      {compact ? null : t('channel_reset.action')}
    </ListActionButton>
  );
}

/** ChannelModelResetReport retains batch failures on the page instead of hiding
 * potentially long conflict details inside a transient toast. */
function ChannelModelResetReport({ summary, onDismiss }: { summary: ResetSummary; onDismiss: () => void }) {
  const { t } = useTranslation();
  const failures = summary.results.filter((result) => !result.success);
  return (
    <section className="mb-4 rounded-lg border p-4 space-y-3" aria-label={t('channel_reset.result_title')}>
      <div className="flex items-start justify-between gap-3">
        <div>
          <h2 className="font-semibold">{t('channel_reset.result_title')}</h2>
          <p role="status" className="text-sm">
            {t('channel_reset.summary', { ...summary })}
          </p>
        </div>
        <Button variant="ghost" size="sm" onClick={onDismiss}>
          {t('channel_reset.dismiss')}
        </Button>
      </div>
      {failures.length > 0 && (
        <ul className="max-h-80 overflow-y-auto space-y-3 text-sm">
          {failures.map((result) => (
            <li key={result.uuid} className="break-words">
              <span className="font-medium">{result.name}</span> <span className="text-muted-foreground">({result.uuid})</span>
              <p>{resetFailureMessage(t, result)}</p>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}

/** useChannelModelReset coordinates confirmation, duplicate-click protection,
 * guarded API calls, result reporting, and a reload of the current list/filter. */
export function useChannelModelReset(onCompleted: () => Promise<void>) {
  const { t } = useTranslation();
  const { notify } = useNotifications();
  const [confirm, ConfirmResetDialog] = useConfirmDialog();
  const [busy, setBusy] = useState(false);
  const [summary, setSummary] = useState<ResetSummary | null>(null);
  const inFlight = useRef(false);
  const reload = useRef(onCompleted);
  reload.current = onCompleted;

  /** reportFailure displays a rejection without suggesting any unsafe automatic retry. */
  const reportFailure = (response: ResetResponse, channel?: Channel) => {
    notify({ type: 'error', title: t('channel_reset.failed_title'), message: resetFailureMessage(t, response) });
    if (channel) {
      setSummary({
        total: 1,
        reset: 0,
        rejected: response.conflict ? 1 : 0,
        failed: response.conflict ? 0 : 1,
        results: [
          {
            uuid: String(channelRef(channel)),
            name: channel.name,
            success: false,
            message: response.message,
            conflict: response.conflict,
          },
        ],
      });
    }
  };

  /** reset performs a single-channel reset only after explicit confirmation. */
  const reset = async (channel: Channel): Promise<void> => {
    if (inFlight.current) return;
    inFlight.current = true;
    setBusy(true);
    let attempted = false;
    try {
      const confirmed = await confirm({
        title: t('channel_reset.confirm_title'),
        description: t('channel_reset.confirm_body', { name: channel.name }),
        confirmLabel: t('channel_reset.confirm'),
        variant: 'destructive',
        details: channel ? [{ label: t('channel_reset.channel'), value: `${channel.name} (${channelRef(channel)})` }] : undefined,
      });
      if (!confirmed) return;
      setSummary(null);
      attempted = true;
      const endpoint = `/api/channel/${encodeURIComponent(channelRef(channel))}/reset_models`;
      const response = (await api.post(endpoint, undefined, { timeout: 120_000 })).data as ResetResponse;
      if (!response.success || !response.data) {
        reportFailure(response, channel);
        return;
      }
      const result = response.data as ResetOutcome;
      notify({ type: 'success', message: t('channel_reset.single_success', { name: channel.name, count: result.model_count }) });
    } catch (error) {
      const response = (error as { response?: { data?: ResetResponse } } | null)?.response?.data;
      reportFailure(response || { success: false, message: t('channel_reset.failed_message') }, channel);
    } finally {
      // A connection can fail after the server committed a reset. Always reload
      // after an attempted POST rather than keeping a misleading stale row.
      try {
        if (attempted) await reload.current();
      } catch {
        notify({ type: 'error', message: t('channel_reset.refresh_failed') });
      } finally {
        inFlight.current = false;
        setBusy(false);
      }
    }
  };

  return {
    busy,
    resetChannel: (channel: Channel) => reset(channel),
    confirmation: <ConfirmResetDialog />,
    report: summary ? <ChannelModelResetReport summary={summary} onDismiss={() => setSummary(null)} /> : null,
  };
}
