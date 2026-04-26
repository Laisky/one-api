import { ChevronDown, ChevronRight, Trash2 } from 'lucide-react';
import { useEffect, useRef } from 'react';
import { useTranslation } from 'react-i18next';

import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { ScrollArea } from '@/components/ui/scroll-area';
import { cn } from '@/lib/utils';
import type { EventLogEntry } from '@/types/realtime';

interface EventLogPanelProps {
  events: EventLogEntry[];
  expandedEvents: Set<string>;
  onToggleExpand: (id: string) => void;
  onClear: () => void;
  isMobile?: boolean;
  className?: string;
}

export function EventLogPanel({ events, expandedEvents, onToggleExpand, onClear, isMobile = false, className }: EventLogPanelProps) {
  const { t } = useTranslation();
  const eventsEndRef = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    eventsEndRef.current?.scrollIntoView({ behavior: 'smooth', block: 'end' });
  }, [events.length]);

  return (
    <Card className={className}>
      <CardHeader className="pb-2 flex-row items-center justify-between space-y-0">
        <CardTitle className="text-base">{t('playground.realtime.event_log')}</CardTitle>
        <div className="flex items-center gap-2">
          <Badge variant="secondary" className="text-xs">
            {events.length}
          </Badge>
          {events.length > 0 && (
            <Button variant="ghost" size="sm" onClick={onClear} className="h-7 gap-1 text-xs text-muted-foreground">
              <Trash2 className="h-3 w-3" />
              {t('playground.realtime.clear_events')}
            </Button>
          )}
        </div>
      </CardHeader>
      <CardContent className="p-0">
        <ScrollArea style={{ height: isMobile ? '250px' : '300px' }}>
          {events.length === 0 ? (
            <div className="flex items-center justify-center py-12">
              <p className="text-sm text-muted-foreground">{t('playground.realtime.events')}</p>
            </div>
          ) : (
            <div className="divide-y">
              {events.map((evt) => {
                const isExpanded = expandedEvents.has(evt.id);
                const isOut = evt.direction === 'out';
                return (
                  <div key={evt.id} className="group">
                    <button
                      type="button"
                      className="flex items-center gap-2 w-full text-left px-4 py-2 hover:bg-muted/50 transition-colors text-sm"
                      onClick={() => onToggleExpand(evt.id)}
                    >
                      {isExpanded ? (
                        <ChevronDown className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
                      ) : (
                        <ChevronRight className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
                      )}
                      <Badge
                        variant={isOut ? 'default' : 'outline'}
                        className={cn(
                          'text-[10px] px-1.5 py-0 shrink-0',
                          isOut ? 'bg-blue-500/15 text-blue-600 border-blue-500/30' : 'bg-green-500/15 text-green-600 border-green-500/30'
                        )}
                      >
                        {isOut ? t('playground.realtime.sent') : t('playground.realtime.received')}
                      </Badge>
                      <Badge
                        variant="outline"
                        className="text-[9px] px-1 py-0 shrink-0 font-mono uppercase tracking-wide text-muted-foreground border-muted-foreground/30"
                      >
                        {evt.transport.toUpperCase()}
                      </Badge>
                      <span className="font-mono text-xs truncate flex-1">{evt.type}</span>
                      <span className="text-[10px] text-muted-foreground shrink-0">{evt.timestamp.toLocaleTimeString()}</span>
                    </button>
                    {isExpanded && (
                      <div className="px-4 pb-3 pl-10">
                        <pre className="text-xs bg-muted/50 rounded-md p-3 overflow-x-auto max-h-48 whitespace-pre-wrap break-all">
                          {JSON.stringify(evt.payload, null, 2)}
                        </pre>
                      </div>
                    )}
                  </div>
                );
              })}
              <div ref={eventsEndRef} />
            </div>
          )}
        </ScrollArea>
      </CardContent>
    </Card>
  );
}
