import { Alert, AlertDescription } from '@/components/ui/alert';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import { ScrollArea } from '@/components/ui/scroll-area';
import { Separator } from '@/components/ui/separator';
import { Skeleton } from '@/components/ui/skeleton';
import { TimestampDisplay } from '@/components/ui/timestamp';
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip';
import { api } from '@/lib/api';
import { LOG_TYPES, getLogTypeLabel } from '@/lib/constants/logs';
import { useAuthStore } from '@/lib/stores/auth';
import { cn, renderQuota } from '@/lib/utils';
import type { LogEntry, LogMetadata } from '@/types/log';
import {
  Activity,
  ArrowRight,
  CheckCircle,
  Clock,
  Copy,
  FileText,
  Flag,
  Globe,
  Hash,
  Play,
  Plug,
  Reply,
  Send,
  User,
  Zap,
} from 'lucide-react';
import { useEffect, useMemo, useState, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';

interface LogDetailsModalProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  log: LogEntry | null;
}

const getCacheWriteSummaries = (metadata?: LogMetadata) => {
  const details = metadata?.cache_write_tokens;
  if (!details) {
    return { fiveMinute: 0, oneHour: 0 };
  }

  const safeNumber = (value: unknown) => (typeof value === 'number' && Number.isFinite(value) ? Math.trunc(value) : 0);

  return {
    fiveMinute: safeNumber(details.ephemeral_5m),
    oneHour: safeNumber(details.ephemeral_1h),
  };
};

const formatLatency = (ms?: number) => {
  if (!ms) return '-';
  if (ms < 1000) return `${ms}ms`;
  return `${(ms / 1000).toFixed(1)}s`;
};

const getLatencyColor = (ms?: number) => {
  if (!ms) return '';
  if (ms < 1000) return 'text-green-600';
  if (ms < 3000) return 'text-yellow-600';
  return 'text-red-600';
};

const DetailItem = ({ label, value }: { label: string; value: ReactNode }) => (
  <div className="space-y-1">
    <span className="text-xs font-semibold text-muted-foreground uppercase tracking-wide">{label}</span>
    <div className="text-sm break-words leading-relaxed">{value}</div>
  </div>
);

interface TraceTimestamps {
  request_received?: number;
  request_forwarded?: number;
  first_upstream_response?: number;
  first_client_response?: number;
  upstream_completed?: number;
  request_completed?: number;
  external_calls?: TraceExternalCall[];
}

interface TraceExternalCall {
  key?: string;
  source?: string;
  tool?: string;
  server_id?: number;
  server_label?: string;
  started_at?: number;
  ended_at?: number;
  duration_ms?: number;
  is_error?: boolean;
}

interface TraceDurations {
  processing_time?: number;
  upstream_response_time?: number;
  response_processing_time?: number;
  streaming_time?: number;
  total_time?: number;
}

interface TraceData {
  id: number;
  trace_id: string;
  url: string;
  method: string;
  body_size: number;
  status: number;
  created_at: number;
  updated_at: number;
  timestamps: TraceTimestamps;
  durations?: TraceDurations;
  log?: {
    id: number;
    user_id: number;
    username: string;
    content: string;
    type: number;
  };
}

const formatDuration = (milliseconds?: number): string => {
  if (!milliseconds) return 'N/A';
  if (milliseconds < 1000) {
    return `${milliseconds}ms`;
  }
  return `${(milliseconds / 1000).toFixed(2)}s`;
};

const getStatusColor = (status: number): string => {
  if (status >= 200 && status < 300) return 'bg-green-500 text-white';
  if (status >= 300 && status < 400) return 'bg-yellow-500 text-white';
  if (status >= 400 && status < 500) return 'bg-orange-500 text-white';
  if (status >= 500) return 'bg-red-500 text-white';
  return 'bg-gray-500 text-white';
};

const getMethodColor = (method: string): string => {
  switch (method.toUpperCase()) {
    case 'GET':
      return 'bg-blue-500 text-white';
    case 'POST':
      return 'bg-green-500 text-white';
    case 'PUT':
      return 'bg-yellow-500 text-white';
    case 'DELETE':
      return 'bg-red-500 text-white';
    case 'PATCH':
      return 'bg-purple-500 text-white';
    default:
      return 'bg-gray-500 text-white';
  }
};

// LogDetailsModal renders a scrollable dialog containing the full details of a log entry, including metadata and content.
export function LogDetailsModal({ open, onOpenChange, log }: LogDetailsModalProps) {
  const { t } = useTranslation();
  const { user } = useAuthStore();
  const metadataJSON = useMemo(() => (log?.metadata ? JSON.stringify(log.metadata, null, 2) : null), [log]);
  const cacheWriteSummary = useMemo(() => getCacheWriteSummaries(log?.metadata), [log]);
  const [traceData, setTraceData] = useState<TraceData | null>(null);
  const [traceLoading, setTraceLoading] = useState(false);
  const [traceError, setTraceError] = useState<string | null>(null);
  const [traceCopied, setTraceCopied] = useState(false);
  const hasTrace = Boolean(
    log && log.trace_id && log.trace_id.trim() !== '' && typeof log.id === 'number' && log.type === LOG_TYPES.CONSUME
  );

  const timelineEvents = useMemo(
    () => [
      {
        key: 'request_received' as keyof TraceTimestamps,
        title: t('logs.details.events.request_received', 'Request Received'),
        icon: Play,
        color: 'text-blue-500',
        description: t('logs.details.events.request_received_desc', 'Initial request received by the gateway'),
      },
      {
        key: 'request_forwarded' as keyof TraceTimestamps,
        title: t('logs.details.events.request_forwarded', 'Forwarded to Upstream'),
        icon: ArrowRight,
        color: 'text-teal-500',
        description: t('logs.details.events.request_forwarded_desc', 'Request forwarded to upstream service'),
      },
      {
        key: 'first_upstream_response' as keyof TraceTimestamps,
        title: t('logs.details.events.first_upstream_response', 'First Upstream Response'),
        icon: Reply,
        color: 'text-purple-500',
        description: t('logs.details.events.first_upstream_response_desc', 'First response received from upstream'),
      },
      {
        key: 'first_client_response' as keyof TraceTimestamps,
        title: t('logs.details.events.first_client_response', 'First Client Response'),
        icon: Send,
        color: 'text-orange-500',
        description: t('logs.details.events.first_client_response_desc', 'First response sent to client'),
      },
      {
        key: 'upstream_completed' as keyof TraceTimestamps,
        title: t('logs.details.events.upstream_completed', 'Upstream Completed'),
        icon: CheckCircle,
        color: 'text-green-500',
        description: t('logs.details.events.upstream_completed_desc', 'Upstream response completed (streaming)'),
      },
      {
        key: 'request_completed' as keyof TraceTimestamps,
        title: t('logs.details.events.request_completed', 'Request Completed'),
        icon: Flag,
        color: 'text-green-600',
        description: t('logs.details.events.request_completed_desc', 'Request fully completed'),
      },
    ],
    [t]
  );

  useEffect(() => {
    let active = true;
    const loadTrace = async () => {
      if (!open || !hasTrace || !log) {
        return;
      }

      setTraceLoading(true);
      try {
        const response = await api.get(`/api/trace/log/${log.id}`);
        if (active) {
          setTraceData(response.data.data);
        }
      } catch (error: any) {
        if (active) {
          setTraceError(t('logs.details.load_failed'));
        }
      } finally {
        if (active) {
          setTraceLoading(false);
        }
      }
    };

    loadTrace();

    return () => {
      active = false;
    };
  }, [open, hasTrace, log, t]);

  const handleCopy = async (value?: string) => {
    if (!value) return;
    try {
      await navigator.clipboard.writeText(value);
    } catch (error) {
      console.error('Failed to copy value to clipboard:', error);
    }
  };

  const handleTraceCopy = async (value?: string) => {
    if (!value) {
      return;
    }
    try {
      await navigator.clipboard.writeText(value);
      setTraceCopied(true);
      setTimeout(() => setTraceCopied(false), 2000);
    } catch (error) {
      console.error('Failed to copy trace ID:', error);
    }
  };

  useEffect(() => {
    if (!open) {
      setTraceCopied(false);
    }
  }, [open]);

  const renderTraceSummary = (trace: TraceData) => (
    <div className="rounded border bg-muted/30 p-4 space-y-4">
      <div className="flex items-center gap-2 text-sm font-semibold text-muted-foreground uppercase tracking-wide">
        <Globe className="h-4 w-4" />
        {t('logs.details.request_info', 'Request Information')}
      </div>

      <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
        <div className="space-y-2">
          <div className="flex items-center gap-2 text-sm text-muted-foreground">
            <Globe className="h-4 w-4" />
            {t('logs.details.url', 'URL')}
          </div>
          <div className="font-mono text-sm bg-background p-2 rounded border break-all">{trace.url || 'N/A'}</div>
        </div>

        <div className="space-y-2">
          <div className="flex items-center gap-2 text-sm text-muted-foreground">
            <Activity className="h-4 w-4" />
            {t('logs.details.method_status', 'Method & Status')}
          </div>
          <div className="flex items-center gap-2">
            <Badge className={getMethodColor(trace.method)}>{trace.method || 'N/A'}</Badge>
            <Badge className={getStatusColor(trace.status)}>{trace.status || 'N/A'}</Badge>
          </div>
        </div>

        <div className="space-y-2">
          <div className="flex items-center gap-2 text-sm text-muted-foreground">
            <FileText className="h-4 w-4" />
            {t('logs.details.request_size', 'Request Size')}
          </div>
          <div className="text-sm">{trace.body_size ? `${trace.body_size} bytes` : 'N/A'}</div>
        </div>

        <div className="space-y-2">
          <div className="flex items-center gap-2 text-sm text-muted-foreground">
            <User className="h-4 w-4" />
            {t('logs.details.user', 'User')}
          </div>
          <div className="text-sm">{trace.log?.username || log?.username || user?.username || 'N/A'}</div>
        </div>

        <div className="space-y-2 md:col-span-2">
          <div className="flex items-center gap-2 text-sm text-muted-foreground">
            <Hash className="h-4 w-4" />
            {t('logs.details.trace_id', 'Trace ID')}
          </div>
          <TooltipProvider>
            <Tooltip>
              <TooltipTrigger asChild>
                <div
                  role="button"
                  tabIndex={0}
                  className="font-mono text-xs bg-background border rounded px-2 py-2 break-all cursor-pointer hover:bg-muted transition-colors"
                  onClick={() => handleTraceCopy(trace.trace_id)}
                  onKeyDown={(event) => {
                    if (event.key === 'Enter' || event.key === ' ') {
                      event.preventDefault();
                      handleTraceCopy(trace.trace_id);
                    }
                  }}
                >
                  {trace.trace_id || 'N/A'}
                </div>
              </TooltipTrigger>
              <TooltipContent>
                <span>{traceCopied ? t('logs.details.copied', 'Copied!') : t('logs.details.copy_trace', 'Click to copy trace ID')}</span>
              </TooltipContent>
            </Tooltip>
          </TooltipProvider>
        </div>
      </div>
    </div>
  );

  const renderTimeline = (trace: TraceData) => {
    if (!trace.timestamps) {
      return (
        <p className="text-sm text-muted-foreground">{t('logs.details.no_timeline', 'Timeline data is not available for this trace.')}</p>
      );
    }

    const { timestamps, durations } = trace;
    const baseEvents = timelineEvents
      .map((event) => {
        const timestamp = timestamps[event.key];
        if (!timestamp) return null;

        let duration: number | undefined;
        if (event.key === 'request_forwarded') duration = durations?.processing_time;
        else if (event.key === 'first_upstream_response') duration = durations?.upstream_response_time;
        else if (event.key === 'first_client_response') duration = durations?.response_processing_time;
        else if (event.key === 'upstream_completed') duration = durations?.streaming_time;

        return {
          ...event,
          timestamp,
          duration,
        };
      })
      .filter(Boolean);

    const externalEvents = (timestamps.external_calls || [])
      .map((call, index) => {
        if (!call.started_at) return null;
        const duration = call.duration_ms ?? (call.ended_at && call.started_at ? call.ended_at - call.started_at : undefined);
        const descriptionParts = [t('logs.details.events.mcp_call_desc', 'MCP tool call')];
        if (call.tool) {
          descriptionParts.push(t('logs.details.events.mcp_call_tool', 'Tool: {{tool}}', { tool: call.tool }));
        }
        if (call.server_label || call.server_id) {
          descriptionParts.push(
            t('logs.details.events.mcp_call_server', 'Server: {{server}}', {
              server: call.server_label || call.server_id,
            })
          );
        }
        return {
          key: call.key || `mcp_call_${index}`,
          title: t('logs.details.events.mcp_call', 'MCP Call'),
          icon: Plug,
          color: call.is_error ? 'text-red-500' : 'text-indigo-500',
          description: descriptionParts.join(' • '),
          timestamp: call.started_at,
          duration,
        };
      })
      .filter(Boolean);

    const activeEvents = [...baseEvents, ...externalEvents].sort((a, b) => {
      if (!a || !b) return 0;
      const aTimestamp = typeof a.timestamp === 'number' ? a.timestamp : Number(a.timestamp || 0);
      const bTimestamp = typeof b.timestamp === 'number' ? b.timestamp : Number(b.timestamp || 0);
      return aTimestamp - bTimestamp;
    });

    if (activeEvents.length === 0) {
      return (
        <p className="text-sm text-muted-foreground">{t('logs.details.no_timeline', 'Timeline data is not available for this trace.')}</p>
      );
    }

    return (
      <div className="space-y-6">
        <div className="flex items-center gap-2 text-sm font-semibold text-muted-foreground uppercase tracking-wide">
          <Clock className="h-4 w-4" />
          {t('logs.details.timeline', 'Request Timeline')}
        </div>
        <div className="space-y-4">
          {activeEvents.map((event: any, index: number) => {
            const timestamp = event.timestamp;
            const Icon = event.icon;
            const isLast = index === activeEvents.length - 1;
            const duration = event.duration;

            return (
              <div key={event.key} className="relative pl-10">
                <div className="absolute left-0 top-0 flex items-center justify-center w-8 h-8 rounded-full border-2 border-border bg-background">
                  <Icon className={cn('h-4 w-4', event.color)} />
                </div>
                <div className="space-y-1">
                  <div className="flex flex-wrap items-center gap-3">
                    <span className="font-medium">{event.title}</span>
                    <TimestampDisplay
                      timestamp={timestamp ? Math.floor(timestamp / 1000) : null}
                      className="font-mono text-xs text-muted-foreground"
                      fallback="N/A"
                    />
                    {duration && (
                      <Badge variant="outline" className="text-xs">
                        +{formatDuration(duration)}
                      </Badge>
                    )}
                  </div>
                  <p className="text-sm text-muted-foreground">{event.description}</p>
                </div>
                {!isLast && <div className="absolute left-3.5 top-8 h-6 w-px bg-border" />}
              </div>
            );
          })}
        </div>

        {durations?.total_time && (
          <div className="flex items-center gap-2 border rounded-lg bg-primary/5 border-primary/20 px-4 py-3">
            <Zap className="h-4 w-4 text-primary" />
            <span className="text-sm font-semibold">{t('logs.details.total_time', 'Total Request Time')}:</span>
            <Badge variant="default">{formatDuration(durations.total_time)}</Badge>
          </div>
        )}
      </div>
    );
  };

  const renderIdentifier = (value?: string) => (
    <div className="flex items-center gap-2">
      <span className="font-mono text-xs bg-muted rounded px-2 py-1 break-all flex-1">{value || '—'}</span>
      {value && (
        <Button size="icon" variant="ghost" className="h-7 w-7" onClick={() => handleCopy(value)}>
          <Copy className="h-3.5 w-3.5" />
        </Button>
      )}
    </div>
  );

  const renderSummary = () => {
    if (!log) return null;

    const username = log.username || user?.username || '—';
    const channelDisplay = log.channel ?? '—';
    const promptTokens = log.prompt_tokens ?? 0;
    const cachedPromptTokens = log.cached_prompt_tokens ?? 0;
    const completionTokens = log.completion_tokens ?? 0;
    const cachedCompletionTokens = log.cached_completion_tokens ?? 0;
    const totalTokens = promptTokens + completionTokens;
    const totalCachedTokens = cachedPromptTokens + cachedCompletionTokens;
    const quotaDisplay = renderQuota(log.quota ?? 0);
    const rawQuota = Number.isFinite(log.quota) ? log.quota : 0;
    const latencyValue = formatLatency(log.elapsed_time);
    const latencyColor = getLatencyColor(log.elapsed_time);
    const logTypeLabel = getLogTypeLabel(log.type);
    return (
      <div className="space-y-5">
        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
          <DetailItem label={t('logs.details.log_id', 'Log ID')} value={<span className="font-mono text-sm">{log.id}</span>} />
          <DetailItem label={t('logs.details.type', 'Type')} value={<Badge variant="outline">{logTypeLabel}</Badge>} />
          <DetailItem
            label={t('logs.details.recorded_at', 'Recorded At')}
            value={<TimestampDisplay timestamp={log.created_at} className="font-mono text-sm" />}
          />
          <DetailItem label={t('logs.details.model', 'Model')} value={log.model_name || '—'} />
          <DetailItem label={t('logs.details.user', 'User')} value={username} />
          <DetailItem label={t('logs.details.token', 'Token')} value={log.token_name || '—'} />
          <DetailItem label={t('logs.details.channel', 'Channel')} value={<span className="font-mono text-sm">{channelDisplay}</span>} />
          <DetailItem label={t('logs.details.quota', 'Quota')} value={<span className="font-mono text-sm">{quotaDisplay}</span>} />
          <DetailItem
            label={t('logs.details.quota_raw', 'Quota (raw units)')}
            value={<span className="font-mono text-sm">{rawQuota}</span>}
          />
          <DetailItem
            label={t('logs.details.latency', 'Latency')}
            value={<span className={cn('font-mono text-sm', latencyColor)}>{latencyValue}</span>}
          />
        </div>

        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
          <DetailItem
            label={t('logs.details.prompt_tokens_input', 'Prompt Tokens (input)')}
            value={<span className="font-mono text-sm">{promptTokens}</span>}
          />
          <DetailItem
            label={t('logs.details.prompt_tokens_cached', 'Prompt Tokens (cached)')}
            value={<span className="font-mono text-sm">{cachedPromptTokens}</span>}
          />
          <DetailItem
            label={t('logs.details.completion_tokens_output', 'Completion Tokens (output)')}
            value={<span className="font-mono text-sm">{completionTokens}</span>}
          />
          <DetailItem
            label={t('logs.details.completion_tokens_cached', 'Completion Tokens (cached)')}
            value={<span className="font-mono text-sm">{cachedCompletionTokens}</span>}
          />
          <DetailItem
            label={t('logs.details.cache_write_5m', 'Cache Write 5m Tokens')}
            value={<span className="font-mono text-sm">{cacheWriteSummary.fiveMinute}</span>}
          />
          <DetailItem
            label={t('logs.details.cache_write_1h', 'Cache Write 1h Tokens')}
            value={<span className="font-mono text-sm">{cacheWriteSummary.oneHour}</span>}
          />
          <DetailItem
            label={t('logs.details.total_tokens', 'Total Tokens')}
            value={<span className="font-mono text-sm">{totalTokens}</span>}
          />
          <DetailItem
            label={t('logs.details.total_cached_tokens', 'Total Cached Tokens')}
            value={<span className="font-mono text-sm">{totalCachedTokens}</span>}
          />
        </div>
      </div>
    );
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-3xl max-h-[90vh]">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <FileText className="h-5 w-5" />
            {t('logs.details.title', 'Log Entry Details')}
            {log && (
              <Badge variant="secondary" className="ml-2">
                {getLogTypeLabel(log.type)}
              </Badge>
            )}
          </DialogTitle>
          {log && (
            <DialogDescription className="flex items-center gap-2 text-sm">
              <Hash className="h-4 w-4" />
              {t('logs.details.recorded_at', 'Recorded at')} <TimestampDisplay timestamp={log.created_at} className="font-mono text-xs" />
            </DialogDescription>
          )}
        </DialogHeader>

        <ScrollArea className="max-h-[calc(90vh-8rem)] pr-2">
          <div className="space-y-6">
            {!log && (
              <p className="text-sm text-muted-foreground">{t('logs.details.select_hint', 'Select a log entry to view full details.')}</p>
            )}

            {log && (
              <>
                <section className="space-y-3">
                  <h3 className="text-sm font-semibold text-muted-foreground uppercase tracking-wide">
                    {t('logs.details.summary', 'Summary')}
                  </h3>
                  {renderSummary()}
                </section>

                <section className="space-y-3">
                  <h3 className="text-sm font-semibold text-muted-foreground uppercase tracking-wide">
                    {t('logs.details.identifiers', 'Identifiers')}
                  </h3>
                  <div className="space-y-3">
                    <div className="space-y-1">
                      <span className="text-xs font-semibold text-muted-foreground uppercase tracking-wide">
                        {t('logs.details.request_id', 'Request ID')}
                      </span>
                      {renderIdentifier(log.request_id)}
                    </div>
                    <div className="space-y-1">
                      <span className="text-xs font-semibold text-muted-foreground uppercase tracking-wide">
                        {t('logs.details.trace_id', 'Trace ID')}
                      </span>
                      {renderIdentifier(log.trace_id)}
                    </div>
                  </div>
                </section>

                {(log.is_stream || log.system_prompt_reset) && (
                  <section className="space-y-2">
                    <h3 className="text-sm font-semibold text-muted-foreground uppercase tracking-wide">
                      {t('logs.details.flags', 'Flags')}
                    </h3>
                    <div className="flex gap-2 flex-wrap">
                      {log.is_stream && <Badge variant="secondary">{t('logs.details.stream', 'Stream')}</Badge>}
                      {log.system_prompt_reset && <Badge variant="destructive">{t('logs.details.system_reset', 'System Reset')}</Badge>}
                    </div>
                  </section>
                )}

                <Separator />

                <section className="space-y-3">
                  <h3 className="text-sm font-semibold text-muted-foreground uppercase tracking-wide">
                    {t('logs.details.content', 'Content')}
                  </h3>
                  <div className="rounded border bg-muted/40 p-3">
                    <pre className="whitespace-pre-wrap text-sm leading-relaxed">
                      {log.content || t('logs.details.no_content', 'No content recorded.')}
                    </pre>
                  </div>
                </section>

                {metadataJSON && (
                  <section className="space-y-3">
                    <h3 className="text-sm font-semibold text-muted-foreground uppercase tracking-wide">
                      {t('logs.details.metadata', 'Metadata')}
                    </h3>
                    <div className="rounded border bg-muted/40 p-3">
                      <pre className="whitespace-pre-wrap text-sm leading-relaxed">{metadataJSON}</pre>
                    </div>
                  </section>
                )}

                <section className="space-y-3">
                  <h3 className="text-sm font-semibold text-muted-foreground uppercase tracking-wide">
                    {t('logs.details.tracing', 'Tracing')}
                  </h3>
                  {!hasTrace && (
                    <p className="text-sm text-muted-foreground">
                      {t('logs.details.no_tracing', 'Tracing data is not available for this log entry.')}
                    </p>
                  )}

                  {hasTrace && traceLoading && (
                    <div className="space-y-3">
                      <Skeleton className="h-20 w-full" />
                      <Skeleton className="h-32 w-full" />
                    </div>
                  )}

                  {hasTrace && !traceLoading && traceError && (
                    <Alert variant="destructive">
                      <AlertDescription>{traceError}</AlertDescription>
                    </Alert>
                  )}

                  {hasTrace && !traceLoading && traceData && (
                    <div className="space-y-6">
                      {renderTraceSummary(traceData)}
                      <Separator />
                      {renderTimeline(traceData)}
                    </div>
                  )}
                </section>
              </>
            )}
          </div>
        </ScrollArea>
      </DialogContent>
    </Dialog>
  );
}
