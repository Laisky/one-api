import { api } from '@/lib/api';
import { buildCsv, fetchAllPaginatedResults, mapWithConcurrency } from '@/lib/export';
import { formatTimestamp, fromDateTimeLocal, renderQuota } from '@/lib/utils';
import { useCallback, useState } from 'react';
import { useTranslation } from 'react-i18next';

import { formatLatency, getCacheWriteSummaries, logRef, type LogRow } from './logs-page-columns';
import { logTypeTranslationKey, type ExportTracePayload } from './log-types';

/**
 * CSV export for the log list.
 *
 * Export deliberately keeps using the legacy offset route. It needs a
 * snapshot-shaped walk of the whole filtered set, which is a different job from
 * interactive paging, and the walker there is already the one under test.
 */

/** UseLogExportOptions describes the filtered set to export. */
export interface UseLogExportOptions {
  filters: {
    type: string;
    model_name: string;
    token_name: string;
    username: string;
    channel: string;
    start_timestamp: string;
    end_timestamp: string;
  };
  isAdminOrRoot: boolean;
  sortBy: string;
  sortOrder: 'asc' | 'desc';
}

/** UseLogExportResult is the hook's public surface. */
export interface UseLogExportResult {
  exporting: boolean;
  exportLogs: () => Promise<void>;
}

/**
 * useLogExport builds and downloads a CSV of the current filtered log set.
 *
 * @param options - the filtered set to export.
 * @returns the export state and its trigger.
 */
export function useLogExport(options: UseLogExportOptions): UseLogExportResult {
  const { filters, isAdminOrRoot, sortBy, sortOrder } = options;
  const { t } = useTranslation();
  const [exporting, setExporting] = useState(false);

  const getLogTypeLabelText = useCallback((typeValue: number) => t(`logs.types.${logTypeTranslationKey(typeValue)}`), [t]);

  const exportLogs = useCallback(async () => {
    setExporting(true);
    try {
      const params = new URLSearchParams();
      if (filters.type !== '0') params.set('type', filters.type);
      if (filters.model_name) params.set('model_name', filters.model_name);
      if (filters.token_name) params.set('token_name', filters.token_name);
      if (isAdminOrRoot && filters.username) params.set('username', filters.username);
      if (filters.channel && isAdminOrRoot) params.set('channel', filters.channel);
      if (filters.start_timestamp) params.set('start_timestamp', String(fromDateTimeLocal(filters.start_timestamp)));
      if (filters.end_timestamp) params.set('end_timestamp', String(fromDateTimeLocal(filters.end_timestamp)));
      if (sortBy) {
        params.set('sort', sortBy);
        params.set('order', sortOrder);
      }

      const exportPath = isAdminOrRoot ? '/api/log/' : '/api/log/self';
      const exportData = await fetchAllPaginatedResults<LogRow>((url) => api.get(url), exportPath, params);
      const logsWithTrace = exportData.filter((log) => log.trace_id?.trim());
      const traceEntries = await mapWithConcurrency(logsWithTrace, async (log) => {
        const ref = logRef(log);
        try {
          const traceResponse = await api.get(`/api/trace/log/${ref}`);
          if (traceResponse.data?.success === false) {
            return {
              logId: ref,
              trace: { error: traceResponse.data?.message || t('logs.details.load_failed') } as ExportTracePayload | { error: string },
            };
          }

          return {
            logId: ref,
            trace: (traceResponse.data?.data as ExportTracePayload | undefined) ?? null,
          };
        } catch (_error) {
          return {
            logId: ref,
            trace: { error: t('logs.details.load_failed') } as ExportTracePayload | { error: string },
          };
        }
      });
      const tracesByLogId = new Map<string | number, ExportTracePayload | { error: string } | null>(
        traceEntries.map((entry) => [entry.logId, entry.trace])
      );

      const csvHeaders = [
        t('logs.details.recorded_at'),
        t('logs.details.type'),
        t('logs.details.log_id'),
        t('logs.details.model'),
        t('logs.details.origin_model'),
        t('logs.details.token'),
        t('logs.details.user'),
        t('logs.details.channel'),
        t('logs.details.quota'),
        t('logs.details.quota_raw'),
        t('logs.details.prompt_tokens_input'),
        t('logs.details.completion_tokens_output'),
        t('logs.details.prompt_tokens_cached'),
        t('logs.details.cache_write_5m'),
        t('logs.details.cache_write_1h'),
        t('logs.details.total_tokens'),
        t('logs.details.latency'),
        t('logs.details.request_id'),
        t('logs.details.trace_id'),
        t('logs.details.stream'),
        t('logs.details.system_reset'),
        t('logs.details.content'),
        t('logs.details.metadata'),
        t('logs.details.tracing'),
      ];
      const csvData = exportData.map((log) => {
        const { fiveMinute, oneHour } = getCacheWriteSummaries(log.metadata);
        const totalTokens = (log.prompt_tokens ?? 0) + (log.completion_tokens ?? 0);
        const tracePayload = tracesByLogId.get(logRef(log)) ?? null;
        return [
          formatTimestamp(log.created_at),
          `${getLogTypeLabelText(log.type)} (${log.type})`,
          logRef(log),
          log.model_name,
          log.origin_model_name || '',
          log.token_name || '',
          log.username || '',
          log.channel_uuid || log.channel || '',
          renderQuota(log.quota),
          log.quota,
          log.prompt_tokens || 0,
          log.completion_tokens || 0,
          log.cached_prompt_tokens || 0,
          fiveMinute,
          oneHour,
          totalTokens,
          formatLatency(log.elapsed_time, t('logs.labels.not_available')),
          log.request_id || '',
          log.trace_id || '',
          Boolean(log.is_stream),
          Boolean(log.system_prompt_reset),
          log.content || '',
          log.metadata ?? null,
          tracePayload,
        ];
      });

      const csv = buildCsv([csvHeaders, ...csvData]);
      const blob = new Blob([csv], { type: 'text/csv' });
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = `logs_${new Date().toISOString().split('T')[0]}.csv`;
      a.click();
      URL.revokeObjectURL(url);
    } catch (error) {
      console.error('Failed to export logs:', error);
    } finally {
      setExporting(false);
    }
  }, [filters, isAdminOrRoot, sortBy, sortOrder, t, getLogTypeLabelText]);

  return { exporting, exportLogs };
}
