import { NameWithId } from '@/components/shared/NameWithId';
import { Button } from '@/components/ui/button';
import { ListActionButton } from '@/components/ui/list-action-button';
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip';
import { ResponsiveActionGroup } from '@/components/ui/responsive-action-group';
import { TimestampDisplay } from '@/components/ui/timestamp';
import type { ModernColumnDef as ColumnDef } from '@/lib/table';
import { cn, formatTimestamp } from '@/lib/utils';
import { Copy, FlaskConical, Info, RefreshCw, Settings, Trash2 } from 'lucide-react';
import type { ReactNode } from 'react';
import type { TFunction } from 'i18next';
import type { NavigateFunction } from 'react-router-dom';
import { ChannelPriorityCell } from './components/ChannelPriorityCell';

/** Channel describes a channel row returned by the channel list APIs. */
export interface Channel {
  id?: number;
  uuid?: string;
  name: string;
  type: number;
  status: number;
  response_time?: number;
  created_time: number;
  updated_time?: number;
  priority?: number;
  weight?: number;
  models?: string;
  test_models?: string[];
  group?: string;
  used_quota?: number;
  test_time?: number;
  testing_model?: string | null;
  balance?: number;
  balance_updated_time?: number;
}

/** channelRef returns the UUID-backed channel reference, falling back to the numeric ID. */
export const channelRef = (channel: Pick<Channel, 'id' | 'uuid'>): string | number => channel.uuid || channel.id || '';

/** channelRefPayload converts a channel reference into the API's identifier payload. */
export const channelRefPayload = (ref: string | number): { id: number } | { uuid: string } => {
  return typeof ref === 'string' ? { uuid: ref } : { id: ref };
};

/** sameChannelRef reports whether two channel records identify the same channel. */
export const sameChannelRef = (left: Pick<Channel, 'id' | 'uuid'>, right: Pick<Channel, 'id' | 'uuid'>) =>
  String(channelRef(left)) === String(channelRef(right));

/**
 * CHANNEL_TESTING_MODEL_SKIP mirrors model.ChannelTestingModelSkip in the backend.
 * Selecting it excludes the channel from health checking entirely.
 */
export const CHANNEL_TESTING_MODEL_SKIP = '__skip__';

/**
 * fallbackTestingModelMarkers is only consulted when the server did not send
 * test_models (an older backend). The server is otherwise authoritative: it
 * classifies by API format using provider metadata, which this list cannot do, and
 * re-filtering its answer here would hide models the backend accepts.
 */
const fallbackTestingModelMarkers = [
  'embedding',
  'rerank',
  'sora',
  'tts',
  'transcribe',
  'whisper',
  'dall-e',
  'gpt-image',
  'imagen',
  'veo',
];

/** isLikelyChatModelName is the offline fallback for the testing-model choices. */
const isLikelyChatModelName = (modelName: string) => {
  const lowerName = modelName.trim().toLowerCase();
  if (!lowerName) return false;
  return !fallbackTestingModelMarkers.some((marker) => lowerName.includes(marker));
};

/** formatResponseTime renders a response time with its latency severity color. */
const formatResponseTime = (time?: number) => {
  if (!time) return '-';
  const color = time < 1000 ? 'text-success' : time < 3000 ? 'text-warning' : 'text-destructive';
  return <span className={cn('font-mono text-sm', color)}>{time}ms</span>;
};

/** ChannelColumnsOptions supplies localized renderers and page actions to the channel table columns. */
interface ChannelColumnsOptions {
  t: TFunction;
  navigate: NavigateFunction;
  refreshingBalanceIds: Set<string | number>;
  renderChannelTypeBadge: (type: number) => ReactNode;
  renderStatusBadge: (status: number, priority?: number) => ReactNode;
  onPriorityUpdate: (channel: Channel, priority: number) => void;
  onBalanceRefresh: (channel: Channel) => void;
  onTestingModelUpdate: (channel: Channel, testingModel: string | null) => void;
  onDuplicate: (channel: Channel) => void;
  onManage: (id: string | number, action: 'enable' | 'disable' | 'delete' | 'test', index?: number) => void;
}

/** createChannelColumns creates the channel table definition from current state and action callbacks. */
export const createChannelColumns = ({
  t,
  navigate,
  refreshingBalanceIds,
  renderChannelTypeBadge,
  renderStatusBadge,
  onPriorityUpdate,
  onBalanceRefresh,
  onTestingModelUpdate,
  onDuplicate,
  onManage,
}: ChannelColumnsOptions): ColumnDef<Channel>[] => [
  {
    accessorKey: 'name',
    header: t('channels.columns.name'),
    cell: ({ row }) => <NameWithId name={row.original.name} refId={channelRef(row.original)} idLabel={t('channels.columns.id')} />,
  },
  {
    accessorKey: 'type',
    header: t('channels.columns.type'),
    cell: ({ row }) => renderChannelTypeBadge(row.original.type),
  },
  {
    accessorKey: 'status',
    header: t('channels.columns.status'),
    cell: ({ row }) => renderStatusBadge(row.original.status, row.original.priority),
  },
  {
    accessorKey: 'group',
    header: t('channels.columns.group'),
    cell: ({ row }) => <span className="text-sm">{row.original.group || t('channels.group_default')}</span>,
  },
  {
    accessorKey: 'priority',
    header: t('channels.columns.priority'),
    cell: ({ row }) => (
      <ChannelPriorityCell
        value={row.original.priority ?? 0}
        ariaLabel={t('channels.columns.priority_input_label', 'Priority for {{name}}', { name: row.original.name })}
        onCommit={(next) => onPriorityUpdate(row.original, next)}
      />
    ),
  },
  {
    accessorKey: 'weight',
    header: t('channels.columns.weight'),
    cell: ({ row }) => <span className="font-mono text-sm">{row.original.weight || 0}</span>,
  },
  {
    accessorKey: 'balance',
    header: t('channels.columns.balance'),
    cell: ({ row }) => {
      const channel = row.original;
      const refreshing = refreshingBalanceIds.has(channelRef(channel));
      const formatted = typeof channel.balance === 'number' ? channel.balance.toFixed(2) : '-';
      const updatedAt = channel.balance_updated_time ? channel.balance_updated_time * 1000 : null;
      return (
        <div className="flex items-center gap-2">
          <div className="font-mono text-sm">{formatted}</div>
          <Button
            variant="ghost"
            size="sm"
            className="h-7 w-7 p-0"
            onClick={() => onBalanceRefresh(channel)}
            disabled={refreshing}
            aria-label={t('channels.actions.refresh_balance', 'Refresh balance for {{name}}', { name: channel.name })}
            title={t('channels.actions.refresh_balance', 'Refresh balance for {{name}}', { name: channel.name })}
          >
            <RefreshCw className={cn('h-3.5 w-3.5', refreshing && 'animate-spin')} />
          </Button>
          {updatedAt && (
            <span className="text-xs text-muted-foreground">
              <TimestampDisplay timestamp={updatedAt} className="font-mono" />
            </span>
          )}
        </div>
      );
    },
  },
  {
    accessorKey: 'response_time',
    header: t('channels.columns.response'),
    cell: ({ row }) => {
      const responseTime = row.original.response_time;
      const testTime = row.original.test_time;
      const responseTitle = `${t('channels.response.prefix')} ${responseTime ? `${responseTime}ms` : t('channels.response.not_tested')}${
        testTime
          ? ` (${t('channels.response.tested_at', {
              local: formatTimestamp(testTime),
              utc: formatTimestamp(testTime, { timeZone: 'UTC' }),
            })})`
          : ''
      }`;
      return (
        <div className="text-center" title={responseTitle}>
          {formatResponseTime(responseTime)}
          {testTime && (
            <div className="text-xs text-muted-foreground">
              <TimestampDisplay timestamp={testTime} className="font-mono" />
            </div>
          )}
        </div>
      );
    },
  },
  {
    accessorKey: 'testing_model',
    header: () => (
      <TooltipProvider>
        <div className="flex items-center gap-1">
          <span>{t('channels.columns.testing_model')}</span>
          <Tooltip>
            <TooltipTrigger asChild>
              <button
                type="button"
                className="inline-flex items-center text-muted-foreground hover:text-foreground focus:outline-none"
                aria-label={t('channels.testing.help_label')}
              >
                <Info className="h-3.5 w-3.5" />
              </button>
            </TooltipTrigger>
            <TooltipContent side="top" align="start" className="max-w-[360px] whitespace-pre-line">
              {t('channels.testing.help')}
            </TooltipContent>
          </Tooltip>
        </div>
      </TooltipProvider>
    ),
    cell: ({ row }) => {
      const channel = row.original;
      const serverProvided = Array.isArray(channel.test_models);
      const models = (serverProvided ? channel.test_models! : (channel.models || '').split(','))
        .map((model) => String(model).trim())
        .filter(Boolean)
        // The server already filtered test_models by API format; only the offline
        // fallback needs the local heuristic.
        .filter((model) => (serverProvided ? true : isLikelyChatModelName(model)))
        .sort();
      const isSkipped = channel.testing_model === CHANNEL_TESTING_MODEL_SKIP;
      const value = isSkipped
        ? CHANNEL_TESTING_MODEL_SKIP
        : channel.testing_model && models.includes(channel.testing_model)
          ? channel.testing_model
          : '';
      return (
        <div className="w-[140px] md:w-[160px] max-w-[220px]">
          <select
            className="w-full border rounded px-2 py-1 text-sm bg-background"
            value={value}
            aria-label={t('channels.columns.testing_model')}
            onChange={(event) => onTestingModelUpdate(channel, event.target.value === '' ? null : event.target.value)}
          >
            <option value="">{t('channels.testing.auto')}</option>
            <option value={CHANNEL_TESTING_MODEL_SKIP}>{t('channels.testing.skip')}</option>
            {models.map((model) => (
              <option key={model} value={model}>
                {model}
              </option>
            ))}
          </select>
        </div>
      );
    },
  },
  {
    accessorKey: 'created_time',
    header: t('channels.columns.created'),
    cell: ({ row }) => <TimestampDisplay timestamp={row.original.created_time} className="text-sm font-mono" />,
  },
  {
    header: t('channels.columns.actions'),
    cell: ({ row }) => {
      const channel = row.original;
      return (
        <ResponsiveActionGroup className="sm:items-center">
          <ListActionButton
            variant="outline"
            size="sm"
            onClick={() => navigate(`/channels/edit/${channelRef(channel)}`)}
            className="gap-1"
            icon={<Settings className="h-3 w-3" />}
          >
            {t('channels.actions.edit')}
          </ListActionButton>
          <ListActionButton
            variant="outline"
            size="sm"
            onClick={() => onDuplicate(channel)}
            className="gap-1"
            icon={<Copy className="h-3 w-3" />}
          >
            {t('channels.actions.duplicate', 'Duplicate')}
          </ListActionButton>
          <ListActionButton
            variant="outline"
            size="sm"
            onClick={() => onManage(channelRef(channel), channel.status === 1 ? 'disable' : 'enable')}
            className={cn('gap-1', channel.status === 1 ? 'text-warning hover:text-warning/80' : 'text-success hover:text-success/80')}
          >
            {channel.status === 1 ? t('channels.actions.disable') : t('channels.actions.enable')}
          </ListActionButton>
          <ListActionButton
            variant="outline"
            size="sm"
            onClick={() => onManage(channelRef(channel), 'test', row.index)}
            className="gap-1"
            title={t('channels.actions.test_help')}
            icon={<FlaskConical className="h-3 w-3" />}
          >
            {t('channels.actions.test')}
          </ListActionButton>
          <ListActionButton
            variant="destructive"
            size="sm"
            onClick={() => onManage(channelRef(channel), 'delete')}
            className="gap-1"
            icon={<Trash2 className="h-3 w-3" />}
          >
            {t('channels.actions.delete')}
          </ListActionButton>
        </ResponsiveActionGroup>
      );
    },
  },
];
