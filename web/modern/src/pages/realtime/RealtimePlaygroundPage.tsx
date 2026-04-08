import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { useNotifications } from '@/components/ui/notifications';
import { ResponsivePageContainer } from '@/components/ui/responsive-container';
import { ScrollArea } from '@/components/ui/scroll-area';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { Textarea } from '@/components/ui/textarea';
import { useResponsive } from '@/hooks/useResponsive';
import { api } from '@/lib/api';
import { cn } from '@/lib/utils';
import { ArrowDown, ChevronDown, ChevronRight, Loader2, Radio, Send, Trash2, Wifi, WifiOff } from 'lucide-react';
import { useCallback, useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

interface Token {
  id: number;
  name: string;
  key: string;
  status: number;
  models?: string | null;
}

const TOKEN_STATUS_ENABLED = 1;

interface ConversationMessage {
  id: string;
  role: 'user' | 'assistant' | 'system';
  text: string;
  timestamp: Date;
}

interface EventLogEntry {
  id: string;
  direction: 'sent' | 'received';
  type: string;
  data: unknown;
  timestamp: Date;
}

type ConnectionStatus = 'disconnected' | 'connecting' | 'connected';

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export function RealtimePlaygroundPage() {
  const { t } = useTranslation();
  const { notify } = useNotifications();
  const { isMobile } = useResponsive();

  // ------ Data loading ------
  const [tokens, setTokens] = useState<Token[]>([]);
  const [selectedTokenKey, setSelectedTokenKey] = useState('');
  const [isLoadingTokens, setIsLoadingTokens] = useState(true);

  const [models, setModels] = useState<string[]>([]);
  const [selectedModel, setSelectedModel] = useState('');
  const [isLoadingModels, setIsLoadingModels] = useState(false);

  // ------ Connection ------
  const [connectionStatus, setConnectionStatus] = useState<ConnectionStatus>('disconnected');
  const wsRef = useRef<WebSocket | null>(null);

  // ------ Session config ------
  const [instructions, setInstructions] = useState('');

  // ------ Conversation ------
  const [messages, setMessages] = useState<ConversationMessage[]>([]);
  const [inputText, setInputText] = useState('');
  const messagesEndRef = useRef<HTMLDivElement>(null);
  const pendingAssistantRef = useRef<string>('');
  const pendingAssistantIdRef = useRef<string>('');

  // ------ Event log ------
  const [events, setEvents] = useState<EventLogEntry[]>([]);
  const [expandedEvents, setExpandedEvents] = useState<Set<string>>(new Set());
  const eventsEndRef = useRef<HTMLDivElement>(null);
  const [showEventsPanel, setShowEventsPanel] = useState(!isMobile);

  // Auto-scroll conversation
  const scrollToBottom = useCallback(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, []);

  useEffect(() => {
    scrollToBottom();
  }, [messages, scrollToBottom]);

  // Auto-scroll events
  useEffect(() => {
    eventsEndRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, [events]);

  // ------ Fetch tokens ------
  useEffect(() => {
    const fetchTokens = async () => {
      setIsLoadingTokens(true);
      try {
        const res = await api.get('/api/token/?p=0&size=100');
        if (res.data.success && res.data.data) {
          const enabled = (res.data.data as Token[]).filter((tk) => tk.status === TOKEN_STATUS_ENABLED);
          setTokens(enabled);
          if (enabled.length > 0 && !selectedTokenKey) {
            setSelectedTokenKey(enabled[0].key);
          }
        }
      } catch {
        notify({
          title: 'Error',
          message: 'Failed to load tokens',
          type: 'error',
        });
      } finally {
        setIsLoadingTokens(false);
      }
    };
    fetchTokens();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // ------ Fetch models when token changes ------
  useEffect(() => {
    if (!selectedTokenKey) {
      setModels([]);
      setSelectedModel('');
      return;
    }

    const fetchModels = async () => {
      setIsLoadingModels(true);
      try {
        const token = tokens.find((tk) => tk.key === selectedTokenKey);
        if (!token) {
          setModels([]);
          setSelectedModel('');
          return;
        }

        // Get models from the token itself
        const rawModels = typeof token.models === 'string' ? token.models : '';
        let modelNames = rawModels
          .split(',')
          .map((n) => n.trim())
          .filter((n) => n.length > 0);

        // If token has no model restriction, fetch user available models
        if (modelNames.length === 0) {
          try {
            const response = await api.get('/api/user/available_models');
            if (response.data?.success && Array.isArray(response.data.data)) {
              modelNames = response.data.data.filter((m: unknown): m is string => typeof m === 'string' && m.trim().length > 0);
            }
          } catch {
            // Silently fallback
          }
        }

        // Filter for realtime models
        const realtimeModels = modelNames.filter((m) => m.includes('realtime'));

        // If no realtime models found, still show all models so user can try
        const finalModels = realtimeModels.length > 0 ? realtimeModels : modelNames;
        setModels(finalModels);

        if (finalModels.length > 0) {
          // Prefer a realtime model if available
          const preferred = finalModels.find((m) => m.includes('realtime'));
          setSelectedModel(preferred ?? finalModels[0]);
        } else {
          setSelectedModel('');
        }
      } catch {
        notify({
          title: 'Error',
          message: 'Failed to load models',
          type: 'error',
        });
        setModels([]);
        setSelectedModel('');
      } finally {
        setIsLoadingModels(false);
      }
    };

    fetchModels();
  }, [selectedTokenKey, tokens, notify]);

  // ------ Helpers ------
  const generateId = () => `${Date.now()}-${Math.random().toString(36).slice(2, 9)}`;

  const addEvent = useCallback((direction: 'sent' | 'received', type: string, data: unknown) => {
    setEvents((prev) => [
      ...prev,
      {
        id: generateId(),
        direction,
        type,
        data,
        timestamp: new Date(),
      },
    ]);
  }, []);

  // ------ WebSocket handlers ------
  const handleWsMessage = useCallback(
    (event: MessageEvent) => {
      try {
        const data = JSON.parse(event.data as string);
        const eventType = data.type as string;
        addEvent('received', eventType, data);

        switch (eventType) {
          case 'session.created':
          case 'session.updated':
            // Session info — no conversation update needed
            break;

          case 'response.text.delta': {
            const delta = data.delta as string | undefined;
            if (delta) {
              pendingAssistantRef.current += delta;
              // Update the existing assistant message in-place
              setMessages((prev) => {
                const idx = prev.findIndex((m) => m.id === pendingAssistantIdRef.current);
                if (idx === -1) {
                  // Create new assistant message
                  const newId = generateId();
                  pendingAssistantIdRef.current = newId;
                  return [
                    ...prev,
                    {
                      id: newId,
                      role: 'assistant',
                      text: pendingAssistantRef.current,
                      timestamp: new Date(),
                    },
                  ];
                }
                const updated = [...prev];
                updated[idx] = {
                  ...updated[idx],
                  text: pendingAssistantRef.current,
                };
                return updated;
              });
            }
            break;
          }

          case 'response.text.done': {
            // Finalize the assistant message
            const fullText = (data.text as string) ?? pendingAssistantRef.current;
            setMessages((prev) => {
              const idx = prev.findIndex((m) => m.id === pendingAssistantIdRef.current);
              if (idx === -1) return prev;
              const updated = [...prev];
              updated[idx] = { ...updated[idx], text: fullText };
              return updated;
            });
            break;
          }

          case 'response.done': {
            // Response complete — reset pending state
            pendingAssistantRef.current = '';
            pendingAssistantIdRef.current = '';
            break;
          }

          case 'response.created': {
            // A new response is starting — prepare for text deltas
            pendingAssistantRef.current = '';
            pendingAssistantIdRef.current = '';
            break;
          }

          case 'error': {
            const errorMsg = (data.error as { message?: string })?.message ?? JSON.stringify(data);
            setMessages((prev) => [
              ...prev,
              {
                id: generateId(),
                role: 'system',
                text: `Error: ${errorMsg}`,
                timestamp: new Date(),
              },
            ]);
            break;
          }

          default:
            // Other events are logged but not explicitly handled
            break;
        }
      } catch {
        // Non-JSON message, log raw
        addEvent('received', 'raw', event.data);
      }
    },
    [addEvent]
  );

  const connect = useCallback(() => {
    if (!selectedTokenKey || !selectedModel) {
      notify({
        title: t('realtime.error_connect'),
        message: t('realtime.error_connect'),
        type: 'error',
      });
      return;
    }

    // Disconnect existing connection
    if (wsRef.current) {
      wsRef.current.close();
      wsRef.current = null;
    }

    setConnectionStatus('connecting');

    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const host = window.location.host;
    const url = `${protocol}//${host}/v1/realtime?model=${encodeURIComponent(selectedModel)}`;

    const subprotocols = ['realtime', `openai-insecure-api-key.${selectedTokenKey}`, 'openai-beta.realtime-v1'];

    try {
      const ws = new WebSocket(url, subprotocols);
      wsRef.current = ws;

      ws.onopen = () => {
        setConnectionStatus('connected');
        addEvent('received', 'connection.open', { url, model: selectedModel });

        // Optionally send session.update with instructions
        if (instructions.trim()) {
          const sessionUpdate = {
            type: 'session.update',
            session: {
              instructions: instructions.trim(),
              modalities: ['text'],
            },
          };
          ws.send(JSON.stringify(sessionUpdate));
          addEvent('sent', 'session.update', sessionUpdate);
        }
      };

      ws.onmessage = handleWsMessage;

      ws.onerror = () => {
        setConnectionStatus('disconnected');
        addEvent('received', 'connection.error', {});
        notify({
          title: t('realtime.connection_error'),
          message: t('realtime.error_connect'),
          type: 'error',
        });
      };

      ws.onclose = (e) => {
        setConnectionStatus('disconnected');
        wsRef.current = null;
        addEvent('received', 'connection.close', {
          code: e.code,
          reason: e.reason,
        });
      };
    } catch {
      setConnectionStatus('disconnected');
      notify({
        title: t('realtime.connection_error'),
        message: t('realtime.error_connect'),
        type: 'error',
      });
    }
  }, [selectedTokenKey, selectedModel, instructions, handleWsMessage, addEvent, notify, t]);

  const disconnect = useCallback(() => {
    if (wsRef.current) {
      wsRef.current.close();
      wsRef.current = null;
    }
    setConnectionStatus('disconnected');
  }, []);

  // Cleanup on unmount
  useEffect(() => {
    return () => {
      if (wsRef.current) {
        wsRef.current.close();
        wsRef.current = null;
      }
    };
  }, []);

  const sendMessage = useCallback(() => {
    if (!inputText.trim()) return;
    if (connectionStatus !== 'connected' || !wsRef.current) {
      notify({
        title: t('realtime.error_send'),
        message: t('realtime.error_send'),
        type: 'error',
      });
      return;
    }

    const text = inputText.trim();

    // Add user message to conversation
    setMessages((prev) => [
      ...prev,
      {
        id: generateId(),
        role: 'user',
        text,
        timestamp: new Date(),
      },
    ]);

    // Send conversation.item.create
    const createEvent = {
      type: 'conversation.item.create',
      item: {
        type: 'message',
        role: 'user',
        content: [{ type: 'input_text', text }],
      },
    };
    wsRef.current.send(JSON.stringify(createEvent));
    addEvent('sent', 'conversation.item.create', createEvent);

    // Request a response
    const responseEvent = { type: 'response.create' };
    wsRef.current.send(JSON.stringify(responseEvent));
    addEvent('sent', 'response.create', responseEvent);

    setInputText('');
  }, [inputText, connectionStatus, addEvent, notify, t]);

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      sendMessage();
    }
  };

  const toggleEventExpand = (id: string) => {
    setExpandedEvents((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  };

  const clearEvents = () => {
    setEvents([]);
    setExpandedEvents(new Set());
  };

  const clearConversation = () => {
    setMessages([]);
    pendingAssistantRef.current = '';
    pendingAssistantIdRef.current = '';
  };

  // ------ Status badge ------
  const statusBadge = () => {
    switch (connectionStatus) {
      case 'connected':
        return (
          <Badge className="bg-emerald-500/15 text-emerald-600 border-emerald-500/30 gap-1.5">
            <Wifi className="h-3 w-3" />
            {t('realtime.connected')}
          </Badge>
        );
      case 'connecting':
        return (
          <Badge className="bg-amber-500/15 text-amber-600 border-amber-500/30 gap-1.5">
            <Loader2 className="h-3 w-3 animate-spin" />
            {t('realtime.connecting')}
          </Badge>
        );
      default:
        return (
          <Badge variant="secondary" className="gap-1.5">
            <WifiOff className="h-3 w-3" />
            {t('realtime.disconnected')}
          </Badge>
        );
    }
  };

  // ------ Render ------
  return (
    <ResponsivePageContainer title={t('realtime.title')} description={t('realtime.description')} actions={statusBadge()}>
      <div className={cn('grid gap-4', isMobile ? 'grid-cols-1' : 'grid-cols-[320px_1fr]')}>
        {/* ===== Left Panel: Settings ===== */}
        <div className="space-y-4">
          <Card>
            <CardHeader className="pb-3">
              <CardTitle className="text-base flex items-center gap-2">
                <Radio className="h-4 w-4" />
                {t('realtime.settings')}
              </CardTitle>
            </CardHeader>
            <CardContent className="space-y-4">
              {/* Token select */}
              <div className="space-y-1.5">
                <label className="text-sm font-medium">{t('realtime.token')}</label>
                {isLoadingTokens ? (
                  <div className="flex items-center gap-2 text-sm text-muted-foreground">
                    <Loader2 className="h-4 w-4 animate-spin" />
                    Loading...
                  </div>
                ) : tokens.length === 0 ? (
                  <p className="text-sm text-muted-foreground">{t('realtime.no_tokens')}</p>
                ) : (
                  <Select value={selectedTokenKey} onValueChange={setSelectedTokenKey} disabled={connectionStatus === 'connected'}>
                    <SelectTrigger>
                      <SelectValue placeholder={t('realtime.select_token')} />
                    </SelectTrigger>
                    <SelectContent>
                      {tokens.map((tk) => (
                        <SelectItem key={tk.key} value={tk.key}>
                          {tk.name}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                )}
              </div>

              {/* Model select */}
              <div className="space-y-1.5">
                <label className="text-sm font-medium">{t('realtime.model')}</label>
                {isLoadingModels ? (
                  <div className="flex items-center gap-2 text-sm text-muted-foreground">
                    <Loader2 className="h-4 w-4 animate-spin" />
                    Loading...
                  </div>
                ) : models.length === 0 ? (
                  <p className="text-sm text-muted-foreground">{selectedTokenKey ? t('realtime.no_models') : t('realtime.select_token')}</p>
                ) : (
                  <Select value={selectedModel} onValueChange={setSelectedModel} disabled={connectionStatus === 'connected'}>
                    <SelectTrigger>
                      <SelectValue placeholder={t('realtime.select_model')} />
                    </SelectTrigger>
                    <SelectContent>
                      {models.map((m) => (
                        <SelectItem key={m} value={m}>
                          {m}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                )}
              </div>

              {/* Instructions */}
              <div className="space-y-1.5">
                <label className="text-sm font-medium">{t('realtime.instructions')}</label>
                <Textarea
                  value={instructions}
                  onChange={(e) => setInstructions(e.target.value)}
                  placeholder={t('realtime.session_instructions_placeholder')}
                  rows={3}
                  disabled={connectionStatus === 'connected'}
                  className="resize-none text-sm"
                />
              </div>

              {/* Connect / Disconnect */}
              <div className="pt-1">
                {connectionStatus === 'disconnected' ? (
                  <Button className="w-full gap-2" onClick={connect} disabled={!selectedTokenKey || !selectedModel}>
                    <Wifi className="h-4 w-4" />
                    {t('realtime.connect')}
                  </Button>
                ) : connectionStatus === 'connecting' ? (
                  <Button className="w-full gap-2" disabled>
                    <Loader2 className="h-4 w-4 animate-spin" />
                    {t('realtime.connecting')}
                  </Button>
                ) : (
                  <Button className="w-full gap-2" variant="destructive" onClick={disconnect}>
                    <WifiOff className="h-4 w-4" />
                    {t('realtime.disconnect')}
                  </Button>
                )}
              </div>
            </CardContent>
          </Card>

          {/* Events Toggle (mobile) */}
          {isMobile && (
            <Button variant="outline" className="w-full gap-2" onClick={() => setShowEventsPanel((p) => !p)}>
              <ArrowDown className={cn('h-4 w-4 transition-transform', showEventsPanel && 'rotate-180')} />
              {t('realtime.event_log')}
            </Button>
          )}
        </div>

        {/* ===== Right Panel: Conversation + Events ===== */}
        <div className="space-y-4">
          {/* Conversation Card */}
          <Card className="flex flex-col" style={{ minHeight: isMobile ? '400px' : '500px' }}>
            <CardHeader className="pb-2 flex-row items-center justify-between space-y-0">
              <CardTitle className="text-base">{t('realtime.conversation')}</CardTitle>
              {messages.length > 0 && (
                <Button variant="ghost" size="sm" onClick={clearConversation} className="h-7 gap-1 text-xs text-muted-foreground">
                  <Trash2 className="h-3 w-3" />
                  {t('realtime.clear_events')}
                </Button>
              )}
            </CardHeader>
            <CardContent className="flex-1 flex flex-col p-0">
              {/* Messages */}
              <ScrollArea className="flex-1 px-6" style={{ height: isMobile ? '300px' : '380px' }}>
                {messages.length === 0 ? (
                  <div className="flex items-center justify-center h-full py-16">
                    <p className="text-sm text-muted-foreground text-center">{t('realtime.no_messages')}</p>
                  </div>
                ) : (
                  <div className="space-y-4 py-4">
                    {messages.map((msg) => (
                      <div key={msg.id} className={cn('flex', msg.role === 'user' ? 'justify-end' : 'justify-start')}>
                        <div
                          className={cn(
                            'max-w-[85%] rounded-lg px-4 py-2.5 text-sm whitespace-pre-wrap',
                            msg.role === 'user'
                              ? 'bg-primary text-primary-foreground'
                              : msg.role === 'assistant'
                                ? 'bg-muted'
                                : 'bg-destructive/10 text-destructive border border-destructive/20'
                          )}
                        >
                          <div className="flex items-center gap-2 mb-1">
                            <span className="text-xs font-medium opacity-70">
                              {msg.role === 'user'
                                ? t('realtime.user')
                                : msg.role === 'assistant'
                                  ? t('realtime.assistant')
                                  : t('realtime.system')}
                            </span>
                            <span className="text-xs opacity-50">{msg.timestamp.toLocaleTimeString()}</span>
                          </div>
                          {msg.text}
                        </div>
                      </div>
                    ))}
                    <div ref={messagesEndRef} />
                  </div>
                )}
              </ScrollArea>

              {/* Input area */}
              <div className="border-t p-4 flex gap-2 items-end">
                <Textarea
                  value={inputText}
                  onChange={(e) => setInputText(e.target.value)}
                  onKeyDown={handleKeyDown}
                  placeholder={t('realtime.placeholder')}
                  rows={1}
                  className="resize-none min-h-[40px] text-sm flex-1"
                  disabled={connectionStatus !== 'connected'}
                />
                <Button
                  size="icon"
                  onClick={sendMessage}
                  disabled={connectionStatus !== 'connected' || !inputText.trim()}
                  className="shrink-0"
                >
                  <Send className="h-4 w-4" />
                </Button>
              </div>
            </CardContent>
          </Card>

          {/* Event Log */}
          {(showEventsPanel || !isMobile) && (
            <Card>
              <CardHeader className="pb-2 flex-row items-center justify-between space-y-0">
                <CardTitle className="text-base">{t('realtime.event_log')}</CardTitle>
                <div className="flex items-center gap-2">
                  <Badge variant="secondary" className="text-xs">
                    {events.length}
                  </Badge>
                  {events.length > 0 && (
                    <Button variant="ghost" size="sm" onClick={clearEvents} className="h-7 gap-1 text-xs text-muted-foreground">
                      <Trash2 className="h-3 w-3" />
                      {t('realtime.clear_events')}
                    </Button>
                  )}
                </div>
              </CardHeader>
              <CardContent className="p-0">
                <ScrollArea style={{ height: isMobile ? '250px' : '300px' }}>
                  {events.length === 0 ? (
                    <div className="flex items-center justify-center py-12">
                      <p className="text-sm text-muted-foreground">{t('realtime.events')}</p>
                    </div>
                  ) : (
                    <div className="divide-y">
                      {events.map((evt) => {
                        const isExpanded = expandedEvents.has(evt.id);
                        return (
                          <div key={evt.id} className="group">
                            <button
                              className="flex items-center gap-2 w-full text-left px-4 py-2 hover:bg-muted/50 transition-colors text-sm"
                              onClick={() => toggleEventExpand(evt.id)}
                            >
                              {isExpanded ? (
                                <ChevronDown className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
                              ) : (
                                <ChevronRight className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
                              )}
                              <Badge
                                variant={evt.direction === 'sent' ? 'default' : 'outline'}
                                className={cn(
                                  'text-[10px] px-1.5 py-0 shrink-0',
                                  evt.direction === 'sent'
                                    ? 'bg-blue-500/15 text-blue-600 border-blue-500/30'
                                    : 'bg-green-500/15 text-green-600 border-green-500/30'
                                )}
                              >
                                {evt.direction === 'sent' ? t('realtime.sent') : t('realtime.received')}
                              </Badge>
                              <span className="font-mono text-xs truncate flex-1">{evt.type}</span>
                              <span className="text-[10px] text-muted-foreground shrink-0">{evt.timestamp.toLocaleTimeString()}</span>
                            </button>
                            {isExpanded && (
                              <div className="px-4 pb-3 pl-10">
                                <pre className="text-xs bg-muted/50 rounded-md p-3 overflow-x-auto max-h-48 whitespace-pre-wrap break-all">
                                  {JSON.stringify(evt.data, null, 2)}
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
          )}
        </div>
      </div>
    </ResponsivePageContainer>
  );
}

export default RealtimePlaygroundPage;
