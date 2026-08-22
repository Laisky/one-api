import { NameWithId } from '@/components/shared/NameWithId';
import { TimestampDisplay } from '@/components/ui/timestamp';
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip';
import type { ModernColumnDef as ColumnDef } from '@/lib/table';
import { cn, renderQuota } from '@/lib/utils';
import type { LogEntry, LogMetadata } from '@/types/log';
import type { TFunction } from 'i18next';
import type { ReactNode } from 'react';

import { LogCopyButton } from './components/LogCopyButton';
import { LogModelCell } from './components/LogModelCell';

/** LogRow describes a row rendered by the log table. */
export type LogRow = LogEntry;

/** logRef returns a log's UUID-backed reference, falling back to its numeric ID. */
export const logRef = (log: Pick<LogRow, 'id' | 'uuid'>): string | number => log.uuid || log.id || '';

/** channelDisplayName returns the visible channel label for a log row. */
const channelDisplayName = (log: Pick<LogRow, 'channel_name' | 'channel_uuid' | 'channel'>, fallback: string) =>
  log.channel_name || log.channel_uuid || log.channel || fallback;

/** channelDisplayRef returns the external channel reference exposed on a channel-name click. */
const channelDisplayRef = (log: Pick<LogRow, 'channel_uuid' | 'channel'>): string | number | null =>
  log.channel_uuid || log.channel || null;

/** formatLatency formats millisecond latency for compact table and export display. */
export const formatLatency = (ms?: number, fallback: string = '-') => {
  if (!ms) return fallback;
  if (ms < 1000) return `${ms}ms`;
  return `${(ms / 1000).toFixed(1)}s`;
};

/** getLatencyColor returns the semantic text color for a latency value. */
const getLatencyColor = (ms?: number) => {
  if (!ms) return '';
  if (ms < 1000) return 'text-success';
  if (ms < 3000) return 'text-warning';
  return 'text-destructive';
};

/** coerceTokenCount converts finite numeric metadata values to integer token counts. */
const coerceTokenCount = (value: unknown) => {
  if (typeof value !== 'number' || !Number.isFinite(value)) return 0;
  return Math.trunc(value);
};

/** getCacheWriteSummaries extracts five-minute and one-hour cache-write token totals. */
export const getCacheWriteSummaries = (metadata?: LogMetadata) => {
  const details = metadata?.cache_write_tokens;
  if (!details) {
    return { fiveMinute: 0, oneHour: 0 };
  }

  return {
    fiveMinute: coerceTokenCount(details.ephemeral_5m),
    oneHour: coerceTokenCount(details.ephemeral_1h),
  };
};

/** LogColumnsOptions supplies current permissions, filters, and localization to the log columns. */
interface LogColumnsOptions {
  t: TFunction;
  isAdminOrRoot: boolean;
  filterType: string;
  currentUsername?: string;
  testLogType: number;
  renderLogTypeBadge: (type: number) => ReactNode;
}

/** createLogColumns creates the log table definition for the current permissions and filters. */
export const createLogColumns = ({
  t,
  isAdminOrRoot,
  filterType,
  currentUsername,
  testLogType,
  renderLogTypeBadge,
}: LogColumnsOptions): ColumnDef<LogRow>[] => [
  {
    accessorKey: 'created_at',
    header: t('logs.table.time'),
    cell: ({ row }) => (
      <div className="flex items-center gap-2">
        <TimestampDisplay timestamp={row.original.created_at} className="font-mono text-xs" title={row.original.request_id || undefined} />
        {row.original.request_id && <LogCopyButton text={row.original.request_id} label={t('common.copy_id', 'Copy ID')} />}
      </div>
    ),
  },
  ...(isAdminOrRoot
    ? [
        {
          accessorKey: 'channel',
          header: t('logs.table.channel'),
          cell: ({ row }) => (
            <NameWithId
              name={channelDisplayName(row.original, t('logs.labels.missing'))}
              refId={channelDisplayRef(row.original)}
              idLabel={t('logs.table.channel')}
            />
          ),
        } as ColumnDef<LogRow>,
      ]
    : []),
  {
    accessorKey: 'type',
    header: t('logs.table.type'),
    cell: ({ row }) => renderLogTypeBadge(row.original.type),
  },
  {
    accessorKey: 'model_name',
    header: t('logs.table.model'),
    cell: ({ row }) => (
      <LogModelCell
        modelName={row.original.model_name}
        originModelName={row.original.origin_model_name}
        targetLabel={t('logs.table.model')}
        originLabel={t('logs.details.origin_model')}
      />
    ),
  },
  ...(Number(filterType) !== testLogType
    ? [
        {
          accessorKey: 'username',
          header: t('logs.table.user'),
          cell: ({ row }) => <span className="text-sm">{row.original.username || currentUsername || t('logs.labels.missing')}</span>,
        } as ColumnDef<LogRow>,
        {
          accessorKey: 'token_name',
          header: t('logs.table.token'),
          cell: ({ row }) => <span className="text-sm">{row.original.token_name || t('logs.labels.missing')}</span>,
        } as ColumnDef<LogRow>,
        {
          accessorKey: 'prompt_tokens',
          header: t('logs.table.prompt'),
          cell: ({ row }) => (
            <TooltipProvider>
              <Tooltip>
                <TooltipTrigger asChild>
                  <span className="font-mono text-sm cursor-help">{row.original.prompt_tokens || 0}</span>
                </TooltipTrigger>
                <TooltipContent>
                  <div className="flex flex-col gap-1">
                    <div>{t('logs.tooltip.input_tokens', { value: row.original.prompt_tokens ?? 0 })}</div>
                    <div>{t('logs.tooltip.cached_tokens', { value: row.original.cached_prompt_tokens ?? 0 })}</div>
                  </div>
                </TooltipContent>
              </Tooltip>
            </TooltipProvider>
          ),
        } as ColumnDef<LogRow>,
        {
          accessorKey: 'completion_tokens',
          header: t('logs.table.completion'),
          cell: ({ row }) => {
            const { fiveMinute, oneHour } = getCacheWriteSummaries(row.original.metadata);
            return (
              <TooltipProvider>
                <Tooltip>
                  <TooltipTrigger asChild>
                    <span className="font-mono text-sm cursor-help">{row.original.completion_tokens || 0}</span>
                  </TooltipTrigger>
                  <TooltipContent>
                    <div className="flex flex-col gap-1">
                      <div>{t('logs.tooltip.output_tokens', { value: row.original.completion_tokens ?? 0 })}</div>
                      <div>{t('logs.tooltip.cache_write_5m', { value: fiveMinute })}</div>
                      <div>{t('logs.tooltip.cache_write_1h', { value: oneHour })}</div>
                    </div>
                  </TooltipContent>
                </Tooltip>
              </TooltipProvider>
            );
          },
        } as ColumnDef<LogRow>,
        {
          accessorKey: 'quota',
          header: t('logs.table.cost'),
          cell: ({ row }) => (
            <span className="font-mono text-sm" title={row.original.content || ''}>
              {renderQuota(row.original.quota)}
            </span>
          ),
        } as ColumnDef<LogRow>,
        {
          accessorKey: 'elapsed_time',
          header: t('logs.table.latency'),
          cell: ({ row }) => (
            <span className={cn('font-mono text-sm', getLatencyColor(row.original.elapsed_time))}>
              {formatLatency(row.original.elapsed_time, t('logs.labels.not_available'))}
            </span>
          ),
        } as ColumnDef<LogRow>,
      ]
    : []),
];
