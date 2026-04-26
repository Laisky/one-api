import { useNotifications } from '@/components/ui/notifications';
import { Message } from '@/lib/utils';
import type { AddEventInput, ConnectionStatus } from '@/types/realtime';
import { useCallback, useEffect, useRef, useState } from 'react';

export interface UseRealtimeChatProps {
  selectedToken: string;
  selectedModel: string;
  systemMessage: string;
  temperature?: number;
  maxCompletionTokens?: number;
  messages: Message[];
  setMessages: React.Dispatch<React.SetStateAction<Message[]>>;
  addEvent: (entry: AddEventInput) => void;
  expandedReasonings?: Set<string>;
  setExpandedReasonings?: React.Dispatch<React.SetStateAction<Set<string>>>;
}

export interface UseRealtimeChatReturn {
  isStreaming: boolean;
  sendMessage: (text: string, attachedImages?: any[]) => Promise<void>;
  regenerateMessage: (messageId: string) => Promise<void>;
  stopGeneration: () => void;
  connectionStatus: ConnectionStatus;
  connect: () => void;
  disconnect: () => void;
}

const REALTIME_MODEL_REGEX = /realtime/i;
const CONNECT_TIMEOUT_MS = 8000;

export function useRealtimeChat({
  selectedToken,
  selectedModel,
  systemMessage,
  temperature,
  maxCompletionTokens,
  messages,
  setMessages,
  addEvent,
}: UseRealtimeChatProps): UseRealtimeChatReturn {
  const { notify } = useNotifications();

  const [connectionStatus, setConnectionStatus] = useState<ConnectionStatus>('disconnected');
  const [isStreaming, setIsStreaming] = useState(false);

  const wsRef = useRef<WebSocket | null>(null);
  const connectTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const pendingAssistantTextRef = useRef<string>('');
  const pendingAssistantTimestampRef = useRef<number | null>(null);
  const messagesRef = useRef<Message[]>(messages);
  const isStreamingRef = useRef(false);
  const explicitDisconnectRef = useRef(false);

  useEffect(() => {
    messagesRef.current = messages;
  }, [messages]);

  useEffect(() => {
    isStreamingRef.current = isStreaming;
  }, [isStreaming]);

  const addErrorMessage = useCallback(
    (errorText: string) => {
      const errorMessage: Message = {
        role: 'error',
        content: errorText,
        timestamp: Date.now(),
        error: true,
      };
      setMessages((prev) => [...prev, errorMessage]);
    },
    [setMessages]
  );

  const upsertAssistantText = useCallback(
    (nextText: string) => {
      setMessages((prev) => {
        const stamp = pendingAssistantTimestampRef.current;
        if (stamp === null) {
          const newStamp = Date.now();
          pendingAssistantTimestampRef.current = newStamp;
          const assistantMessage: Message = {
            role: 'assistant',
            content: nextText,
            reasoning_content: null,
            timestamp: newStamp,
            model: selectedModel,
          };
          return [...prev, assistantMessage];
        }
        const idx = prev.findIndex((m) => m.role === 'assistant' && m.timestamp === stamp);
        if (idx === -1) {
          const assistantMessage: Message = {
            role: 'assistant',
            content: nextText,
            reasoning_content: null,
            timestamp: stamp,
            model: selectedModel,
          };
          return [...prev, assistantMessage];
        }
        const updated = [...prev];
        updated[idx] = { ...updated[idx], content: nextText };
        return updated;
      });
    },
    [selectedModel, setMessages]
  );

  const handleWsMessage = useCallback(
    (event: MessageEvent) => {
      let data: any;
      try {
        data = JSON.parse(event.data as string);
      } catch {
        addEvent({ direction: 'in', type: 'raw', payload: event.data, transport: 'ws' });
        return;
      }

      const eventType = (data?.type as string) ?? 'unknown';
      addEvent({ direction: 'in', type: eventType, payload: data, transport: 'ws' });

      switch (eventType) {
        case 'session.created':
        case 'session.updated':
          break;

        case 'response.created': {
          pendingAssistantTextRef.current = '';
          pendingAssistantTimestampRef.current = null;
          setIsStreaming(true);
          break;
        }

        case 'response.output_text.delta':
        case 'response.text.delta': {
          const delta = typeof data?.delta === 'string' ? data.delta : '';
          if (delta) {
            pendingAssistantTextRef.current += delta;
            upsertAssistantText(pendingAssistantTextRef.current);
          }
          break;
        }

        case 'response.output_text.done':
        case 'response.text.done': {
          const finalText =
            typeof data?.text === 'string' && data.text.length > 0 ? data.text : pendingAssistantTextRef.current;
          pendingAssistantTextRef.current = finalText;
          upsertAssistantText(finalText);
          break;
        }

        case 'response.done':
        case 'response.cancelled': {
          pendingAssistantTextRef.current = '';
          pendingAssistantTimestampRef.current = null;
          setIsStreaming(false);
          break;
        }

        case 'error': {
          const errorMsg =
            (data?.error as { message?: string })?.message ?? data?.message ?? JSON.stringify(data);
          addErrorMessage(`Error: ${errorMsg}`);
          setIsStreaming(false);
          break;
        }

        default:
          break;
      }
    },
    [addEvent, addErrorMessage, upsertAssistantText]
  );

  const disconnect = useCallback(() => {
    explicitDisconnectRef.current = true;
    if (connectTimeoutRef.current) {
      clearTimeout(connectTimeoutRef.current);
      connectTimeoutRef.current = null;
    }
    const ws = wsRef.current;
    if (ws) {
      try {
        ws.close();
      } catch {
        // ignore
      }
      wsRef.current = null;
      addEvent({ direction: 'in', type: 'connection.close', payload: { reason: 'client' }, transport: 'ws' });
    }
    setConnectionStatus('disconnected');
    setIsStreaming(false);
    pendingAssistantTextRef.current = '';
    pendingAssistantTimestampRef.current = null;
  }, [addEvent]);

  const connect = useCallback(() => {
    if (!selectedToken || !selectedModel) {
      return;
    }

    if (wsRef.current) {
      try {
        wsRef.current.close();
      } catch {
        // ignore
      }
      wsRef.current = null;
    }

    explicitDisconnectRef.current = false;
    setConnectionStatus('connecting');

    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const host = window.location.host;
    const url = `${protocol}//${host}/v1/realtime?model=${encodeURIComponent(selectedModel)}`;
    const model = selectedModel;
    const subprotocols = ['realtime', `openai-insecure-api-key.${selectedToken}`];

    let ws: WebSocket;
    try {
      ws = new WebSocket(url, subprotocols);
    } catch (err) {
      setConnectionStatus('disconnected');
      const message = err instanceof Error ? err.message : 'failed to open websocket';
      addEvent({ direction: 'in', type: 'connection.error', payload: { message }, transport: 'ws' });
      notify({ title: 'Connection error', message, type: 'error' });
      return;
    }
    wsRef.current = ws;

    if (connectTimeoutRef.current) {
      clearTimeout(connectTimeoutRef.current);
    }
    connectTimeoutRef.current = setTimeout(() => {
      connectTimeoutRef.current = null;
      if (wsRef.current === ws && ws.readyState !== WebSocket.OPEN) {
        addEvent({
          direction: 'in',
          type: 'connection.timeout',
          payload: { url, model, readyState: ws.readyState },
          transport: 'ws',
        });
        try {
          ws.close();
        } catch {
          // ignore
        }
        if (wsRef.current === ws) {
          wsRef.current = null;
        }
        setConnectionStatus('disconnected');
      }
    }, CONNECT_TIMEOUT_MS);

    ws.onopen = () => {
      if (connectTimeoutRef.current) {
        clearTimeout(connectTimeoutRef.current);
        connectTimeoutRef.current = null;
      }
      setConnectionStatus('connected');
      addEvent({
        direction: 'in',
        type: 'connection.open',
        payload: { url, model: selectedModel },
        transport: 'ws',
      });
    };

    ws.onmessage = handleWsMessage;

    ws.onerror = () => {
      if (connectTimeoutRef.current) {
        clearTimeout(connectTimeoutRef.current);
        connectTimeoutRef.current = null;
      }
      addEvent({ direction: 'in', type: 'connection.error', payload: {}, transport: 'ws' });
    };

    ws.onclose = (e) => {
      if (connectTimeoutRef.current) {
        clearTimeout(connectTimeoutRef.current);
        connectTimeoutRef.current = null;
      }
      const wasExplicit = explicitDisconnectRef.current;
      wsRef.current = null;
      setConnectionStatus('disconnected');
      setIsStreaming(false);
      pendingAssistantTextRef.current = '';
      pendingAssistantTimestampRef.current = null;
      if (!wasExplicit) {
        addEvent({
          direction: 'in',
          type: 'connection.close',
          payload: { code: e.code, reason: e.reason },
          transport: 'ws',
        });
      }
    };
  }, [selectedToken, selectedModel, handleWsMessage, addEvent, notify]);

  useEffect(() => {
    if (connectionStatus !== 'connected') return;
    const ws = wsRef.current;
    if (!ws || ws.readyState !== WebSocket.OPEN) return;
    const sessionFields: Record<string, unknown> = {};
    const trimmedSystem = systemMessage?.trim();
    if (trimmedSystem) sessionFields.instructions = trimmedSystem;
    if (typeof temperature === 'number') sessionFields.temperature = temperature;
    if (typeof maxCompletionTokens === 'number') sessionFields.max_response_output_tokens = maxCompletionTokens;
    if (Object.keys(sessionFields).length === 0) return;
    const sessionUpdate = { type: 'session.update', session: sessionFields };
    try {
      ws.send(JSON.stringify(sessionUpdate));
      addEvent({ direction: 'out', type: 'session.update', payload: sessionUpdate, transport: 'ws' });
    } catch (err) {
      const message = err instanceof Error ? err.message : 'failed to send session.update';
      addEvent({ direction: 'out', type: 'error', payload: { message }, transport: 'ws' });
    }
  }, [systemMessage, temperature, maxCompletionTokens, connectionStatus, addEvent]);

  useEffect(() => {
    if (selectedToken && selectedModel && REALTIME_MODEL_REGEX.test(selectedModel)) {
      connect();
    }
    return () => {
      explicitDisconnectRef.current = true;
      if (connectTimeoutRef.current) {
        clearTimeout(connectTimeoutRef.current);
        connectTimeoutRef.current = null;
      }
      const ws = wsRef.current;
      if (ws) {
        try {
          ws.close();
        } catch {
          // ignore
        }
        wsRef.current = null;
      }
    };
  }, [selectedToken, selectedModel, connect]);

  const sendMessage = useCallback(
    async (text: string, attachedImages?: any[]) => {
      const ws = wsRef.current;
      if (!ws || connectionStatus !== 'connected' || ws.readyState !== WebSocket.OPEN) {
        addEvent({
          direction: 'out',
          type: 'error',
          payload: { message: 'not connected' },
          transport: 'ws',
        });
        return;
      }

      const trimmed = text.trim();
      if (!trimmed) return;

      if (attachedImages && attachedImages.length > 0) {
        addEvent({
          direction: 'out',
          type: 'warning',
          payload: { message: 'images not supported in realtime mode, ignored' },
          transport: 'ws',
        });
      }

      const userMessage: Message = {
        role: 'user',
        content: trimmed,
        timestamp: Date.now(),
      };
      setMessages((prev) => [...prev, userMessage]);

      const createEvent = {
        type: 'conversation.item.create',
        item: {
          type: 'message',
          role: 'user',
          content: [{ type: 'input_text', text: trimmed }],
        },
      };
      const responseEvent = {
        type: 'response.create',
        response: { output_modalities: ['text'] },
      };

      try {
        ws.send(JSON.stringify(createEvent));
        addEvent({ direction: 'out', type: 'conversation.item.create', payload: createEvent, transport: 'ws' });
        ws.send(JSON.stringify(responseEvent));
        addEvent({ direction: 'out', type: 'response.create', payload: responseEvent, transport: 'ws' });
      } catch (err) {
        const message = err instanceof Error ? err.message : 'failed to send';
        addEvent({ direction: 'out', type: 'error', payload: { message }, transport: 'ws' });
        addErrorMessage(`Send failed: ${message}`);
      }
    },
    [connectionStatus, addEvent, addErrorMessage, setMessages]
  );

  const regenerateMessage = useCallback(
    async (messageId: string) => {
      const ws = wsRef.current;
      if (!ws || connectionStatus !== 'connected' || ws.readyState !== WebSocket.OPEN) {
        addEvent({
          direction: 'out',
          type: 'error',
          payload: { message: 'not connected' },
          transport: 'ws',
        });
        return;
      }

      const current = messagesRef.current;
      const targetIdx = current.findIndex((m) => String(m.timestamp) === messageId);
      let truncatedTo: number;
      if (targetIdx === -1) {
        let lastUser = -1;
        for (let i = current.length - 1; i >= 0; i--) {
          if (current[i].role === 'user') {
            lastUser = i;
            break;
          }
        }
        if (lastUser === -1) {
          addEvent({
            direction: 'out',
            type: 'warning',
            payload: { message: 'no prior user message to regenerate from' },
            transport: 'ws',
          });
          return;
        }
        truncatedTo = lastUser + 1;
      } else {
        truncatedTo = targetIdx;
        let foundUser = false;
        for (let i = targetIdx - 1; i >= 0; i--) {
          if (current[i].role === 'user') {
            foundUser = true;
            break;
          }
        }
        if (!foundUser) {
          addEvent({
            direction: 'out',
            type: 'warning',
            payload: { message: 'no prior user message to regenerate from' },
            transport: 'ws',
          });
          return;
        }
      }

      setMessages((prev) => prev.slice(0, truncatedTo));
      pendingAssistantTextRef.current = '';
      pendingAssistantTimestampRef.current = null;

      const responseEvent = {
        type: 'response.create',
        response: { output_modalities: ['text'] },
      };
      try {
        ws.send(JSON.stringify(responseEvent));
        addEvent({ direction: 'out', type: 'response.create', payload: responseEvent, transport: 'ws' });
      } catch (err) {
        const message = err instanceof Error ? err.message : 'failed to send';
        addEvent({ direction: 'out', type: 'error', payload: { message }, transport: 'ws' });
        addErrorMessage(`Regenerate failed: ${message}`);
      }
    },
    [connectionStatus, addEvent, addErrorMessage, setMessages]
  );

  const stopGeneration = useCallback(() => {
    const ws = wsRef.current;
    if (isStreamingRef.current && ws && ws.readyState === WebSocket.OPEN) {
      const cancelEvent = { type: 'response.cancel' };
      try {
        ws.send(JSON.stringify(cancelEvent));
        addEvent({ direction: 'out', type: 'response.cancel', payload: cancelEvent, transport: 'ws' });
      } catch (err) {
        const message = err instanceof Error ? err.message : 'failed to send response.cancel';
        addEvent({ direction: 'out', type: 'error', payload: { message }, transport: 'ws' });
      }
    }
    setIsStreaming(false);
  }, [addEvent]);

  return {
    isStreaming,
    sendMessage,
    regenerateMessage,
    stopGeneration,
    connectionStatus,
    connect,
    disconnect,
  };
}

export default useRealtimeChat;
