import type { Message } from '@/lib/utils';
import type { AddEventInput } from '@/types/realtime';
import { act, renderHook } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

const stableNotify = vi.fn();
const stableDismiss = vi.fn();
const stableNotificationsCtx = { notify: stableNotify, dismiss: stableDismiss };

vi.mock('@/components/ui/notifications', () => ({
  useNotifications: () => stableNotificationsCtx,
}));

import { useRealtimeChat, type UseRealtimeChatProps } from '../useRealtimeChat';

const sockets: MockWebSocket[] = [];

class MockWebSocket {
  static CONNECTING = 0;
  static OPEN = 1;
  static CLOSING = 2;
  static CLOSED = 3;

  CONNECTING = 0;
  OPEN = 1;
  CLOSING = 2;
  CLOSED = 3;

  url: string;
  protocols: string | string[] | undefined;
  readyState = 0;
  onopen: ((ev: Event) => void) | null = null;
  onmessage: ((ev: MessageEvent) => void) | null = null;
  onerror: ((ev: Event) => void) | null = null;
  onclose: ((ev: CloseEvent) => void) | null = null;

  sent: string[] = [];
  closeCalls: Array<{ code?: number; reason?: string }> = [];

  constructor(url: string, protocols?: string | string[]) {
    this.url = url;
    this.protocols = protocols;
    sockets.push(this);
  }

  send(data: string) {
    this.sent.push(data);
  }

  close(code = 1000, reason = '') {
    this.closeCalls.push({ code, reason });
    if (this.readyState === MockWebSocket.CLOSED) return;
    this.readyState = MockWebSocket.CLOSED;
    this.onclose?.(new CloseEvent('close', { code, reason }));
  }

  simulateOpen() {
    this.readyState = MockWebSocket.OPEN;
    this.onopen?.(new Event('open'));
  }

  simulateMessage(obj: unknown) {
    this.onmessage?.({ data: JSON.stringify(obj) } as MessageEvent);
  }

  simulateError() {
    this.onerror?.(new Event('error'));
  }
}

function makeProps(overrides: Partial<UseRealtimeChatProps> = {}): {
  props: UseRealtimeChatProps;
  addEvent: ReturnType<typeof vi.fn>;
  setMessages: ReturnType<typeof vi.fn>;
  messagesRef: { current: Message[] };
} {
  const messagesRef: { current: Message[] } = { current: [] };
  const setMessages = vi.fn((updater: any) => {
    if (typeof updater === 'function') {
      messagesRef.current = updater(messagesRef.current);
    } else {
      messagesRef.current = updater;
    }
  });
  const addEvent = vi.fn<(entry: AddEventInput) => void>();
  const props: UseRealtimeChatProps = {
    selectedToken: 'tok-test',
    selectedModel: 'gpt-4o-realtime-preview',
    systemMessage: '',
    messages: messagesRef.current,
    setMessages: setMessages as any,
    addEvent,
    ...overrides,
  };
  return { props, addEvent, setMessages, messagesRef };
}

beforeEach(() => {
  sockets.length = 0;
  vi.stubGlobal('WebSocket', MockWebSocket);
});

afterEach(() => {
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

describe('useRealtimeChat', () => {
  it('does not connect when token is empty', () => {
    const { props } = makeProps({ selectedToken: '' });
    renderHook(() => useRealtimeChat(props));
    expect(sockets.length).toBe(0);
  });

  it('does not connect when model is non-realtime', () => {
    const { props } = makeProps({ selectedModel: 'gpt-4o-mini' });
    renderHook(() => useRealtimeChat(props));
    expect(sockets.length).toBe(0);
  });

  it('auto-connects on mount when realtime model and token are set', () => {
    const { props } = makeProps();
    const { result } = renderHook(() => useRealtimeChat(props));
    expect(sockets.length).toBe(1);
    expect(result.current.connectionStatus).toBe('connecting');
  });

  it('uses correct subprotocols without openai-beta.realtime-v1', () => {
    const { props } = makeProps();
    renderHook(() => useRealtimeChat(props));
    expect(sockets[0].protocols).toEqual(['realtime', 'openai-insecure-api-key.tok-test']);
    expect(sockets[0].protocols).not.toContain('openai-beta.realtime-v1');
  });

  it('builds the WS URL with current host and encoded model', () => {
    const { props } = makeProps();
    renderHook(() => useRealtimeChat(props));
    expect(sockets[0].url).toMatch(/^wss?:\/\/[^/]+\/v1\/realtime\?model=gpt-4o-realtime-preview$/);
  });

  it('logs connection.open and does not send session.update without optional fields on open', () => {
    const { props, addEvent } = makeProps();
    const { result } = renderHook(() => useRealtimeChat(props));
    act(() => {
      sockets[0].simulateOpen();
    });
    expect(result.current.connectionStatus).toBe('connected');
    expect(sockets[0].sent).toHaveLength(0);
    const openCall = addEvent.mock.calls.find(
      ([entry]) => entry.type === 'connection.open' && entry.direction === 'in' && entry.transport === 'ws'
    );
    expect(openCall).toBeDefined();
  });

  it('sends session.update with instructions when systemMessage is set', () => {
    const { props } = makeProps({ systemMessage: 'be helpful' });
    renderHook(() => useRealtimeChat(props));
    act(() => {
      sockets[0].simulateOpen();
    });
    expect(sockets[0].sent).toHaveLength(1);
    const parsed = JSON.parse(sockets[0].sent[0]);
    expect(parsed.type).toBe('session.update');
    expect(parsed.session.instructions).toBe('be helpful');
  });

  it('sends session.update with temperature and max_response_output_tokens', () => {
    const { props } = makeProps({ temperature: 0.5, maxCompletionTokens: 256 });
    renderHook(() => useRealtimeChat(props));
    act(() => {
      sockets[0].simulateOpen();
    });
    expect(sockets[0].sent).toHaveLength(1);
    const parsed = JSON.parse(sockets[0].sent[0]);
    expect(parsed.type).toBe('session.update');
    expect(parsed.session.temperature).toBe(0.5);
    expect(parsed.session.max_response_output_tokens).toBe(256);
    expect(parsed.session.instructions).toBeUndefined();
  });

  it('handles GA response.output_text.delta events and accumulates assistant content', () => {
    const { props, messagesRef } = makeProps();
    renderHook(() => useRealtimeChat(props));
    act(() => {
      sockets[0].simulateOpen();
    });
    act(() => {
      sockets[0].simulateMessage({ type: 'response.created' });
    });
    act(() => {
      sockets[0].simulateMessage({ type: 'response.output_text.delta', delta: 'Hello' });
    });
    let last = messagesRef.current[messagesRef.current.length - 1];
    expect(last.role).toBe('assistant');
    expect(last.content).toBe('Hello');

    act(() => {
      sockets[0].simulateMessage({ type: 'response.output_text.delta', delta: ' world' });
    });
    last = messagesRef.current[messagesRef.current.length - 1];
    expect(last.role).toBe('assistant');
    expect(last.content).toBe('Hello world');
  });

  it('handles legacy response.text.delta events', () => {
    const { props, messagesRef } = makeProps();
    renderHook(() => useRealtimeChat(props));
    act(() => {
      sockets[0].simulateOpen();
    });
    act(() => {
      sockets[0].simulateMessage({ type: 'response.created' });
    });
    act(() => {
      sockets[0].simulateMessage({ type: 'response.text.delta', delta: 'foo' });
    });
    act(() => {
      sockets[0].simulateMessage({ type: 'response.text.delta', delta: 'bar' });
    });
    const last = messagesRef.current[messagesRef.current.length - 1];
    expect(last.role).toBe('assistant');
    expect(last.content).toBe('foobar');
  });

  it('flips isStreaming on response.created and back on response.done', () => {
    const { props } = makeProps();
    const { result } = renderHook(() => useRealtimeChat(props));
    act(() => {
      sockets[0].simulateOpen();
    });
    act(() => {
      sockets[0].simulateMessage({ type: 'response.created' });
    });
    expect(result.current.isStreaming).toBe(true);
    act(() => {
      sockets[0].simulateMessage({ type: 'response.done' });
    });
    expect(result.current.isStreaming).toBe(false);
  });

  it('logs error on sendMessage when not connected and does not append user message', async () => {
    const { props, addEvent, messagesRef } = makeProps();
    const { result } = renderHook(() => useRealtimeChat(props));
    await act(async () => {
      await result.current.sendMessage('hi');
    });
    const errCall = addEvent.mock.calls.find(
      ([entry]) => entry.direction === 'out' && entry.type === 'error'
    );
    expect(errCall).toBeDefined();
    expect(messagesRef.current.find((m) => m.role === 'user')).toBeUndefined();
    expect(sockets[0]?.sent ?? []).toHaveLength(0);
  });

  it('sends conversation.item.create then response.create on sendMessage when connected', async () => {
    const { props, messagesRef } = makeProps();
    const { result } = renderHook(() => useRealtimeChat(props));
    act(() => {
      sockets[0].simulateOpen();
    });
    await act(async () => {
      await result.current.sendMessage('hi');
    });
    const userMsg = messagesRef.current.find((m) => m.role === 'user');
    expect(userMsg?.content).toBe('hi');

    expect(sockets[0].sent).toHaveLength(2);
    const first = JSON.parse(sockets[0].sent[0]);
    const second = JSON.parse(sockets[0].sent[1]);
    expect(first).toEqual({
      type: 'conversation.item.create',
      item: {
        type: 'message',
        role: 'user',
        content: [{ type: 'input_text', text: 'hi' }],
      },
    });
    expect(second).toEqual({
      type: 'response.create',
      response: { output_modalities: ['text'] },
    });
  });

  it('logs warning when sendMessage receives attached images but still sends text', async () => {
    const { props, addEvent } = makeProps();
    const { result } = renderHook(() => useRealtimeChat(props));
    act(() => {
      sockets[0].simulateOpen();
    });
    await act(async () => {
      await result.current.sendMessage('hello', [{ url: 'data:image/png;base64,abc' }]);
    });
    const warnCall = addEvent.mock.calls.find(
      ([entry]) => entry.direction === 'out' && entry.type === 'warning'
    );
    expect(warnCall).toBeDefined();
    expect(sockets[0].sent).toHaveLength(2);
    const first = JSON.parse(sockets[0].sent[0]);
    expect(first.type).toBe('conversation.item.create');
    expect(first.item.content[0].text).toBe('hello');
  });

  it('sends response.cancel on stopGeneration while streaming', () => {
    const { props } = makeProps();
    const { result } = renderHook(() => useRealtimeChat(props));
    act(() => {
      sockets[0].simulateOpen();
    });
    act(() => {
      sockets[0].simulateMessage({ type: 'response.created' });
    });
    act(() => {
      result.current.stopGeneration();
    });
    const cancelFrame = sockets[0].sent
      .map((s) => JSON.parse(s))
      .find((p) => p.type === 'response.cancel');
    expect(cancelFrame).toBeDefined();
    expect(result.current.isStreaming).toBe(false);
  });

  it('does not send response.cancel on stopGeneration while idle', () => {
    const { props } = makeProps();
    const { result } = renderHook(() => useRealtimeChat(props));
    act(() => {
      sockets[0].simulateOpen();
    });
    act(() => {
      result.current.stopGeneration();
    });
    const cancelFrame = sockets[0].sent
      .map((s) => JSON.parse(s))
      .find((p) => p.type === 'response.cancel');
    expect(cancelFrame).toBeUndefined();
    expect(result.current.isStreaming).toBe(false);
  });

  it('disconnect closes the socket, sets status disconnected, and logs connection.close', () => {
    const { props, addEvent } = makeProps();
    const { result } = renderHook(() => useRealtimeChat(props));
    act(() => {
      sockets[0].simulateOpen();
    });
    act(() => {
      result.current.disconnect();
    });
    expect(sockets[0].readyState).toBe(MockWebSocket.CLOSED);
    expect(result.current.connectionStatus).toBe('disconnected');
    const closeCall = addEvent.mock.calls.find(
      ([entry]) => entry.type === 'connection.close' && entry.direction === 'in'
    );
    expect(closeCall).toBeDefined();
  });

  it('fires connection.timeout after 8s if socket has not opened', () => {
    vi.useFakeTimers();
    const { props, addEvent } = makeProps();
    const { result } = renderHook(() => useRealtimeChat(props));
    expect(sockets.length).toBe(1);
    act(() => {
      vi.advanceTimersByTime(8000);
    });
    const timeoutCall = addEvent.mock.calls.find(([entry]) => entry.type === 'connection.timeout');
    expect(timeoutCall).toBeDefined();
    expect(sockets[0].readyState).toBe(MockWebSocket.CLOSED);
    expect(result.current.connectionStatus).toBe('disconnected');
  });

  it('does not fire connection.timeout when socket opens before 8s', () => {
    vi.useFakeTimers();
    const { props, addEvent } = makeProps();
    renderHook(() => useRealtimeChat(props));
    act(() => {
      sockets[0].simulateOpen();
    });
    act(() => {
      vi.advanceTimersByTime(9000);
    });
    const timeoutCall = addEvent.mock.calls.find(([entry]) => entry.type === 'connection.timeout');
    expect(timeoutCall).toBeUndefined();
  });

  it('reconnects on token change with new subprotocols', () => {
    const initial = makeProps();
    const { rerender } = renderHook((p: UseRealtimeChatProps) => useRealtimeChat(p), {
      initialProps: initial.props,
    });
    expect(sockets.length).toBe(1);
    act(() => {
      sockets[0].simulateOpen();
    });
    const firstSocket = sockets[0];

    const next: UseRealtimeChatProps = {
      ...initial.props,
      selectedToken: 'tok-new',
    };
    rerender(next);

    expect(firstSocket.readyState).toBe(MockWebSocket.CLOSED);
    expect(sockets.length).toBeGreaterThanOrEqual(2);
    const newest = sockets[sockets.length - 1];
    expect(newest.protocols).toEqual(['realtime', 'openai-insecure-api-key.tok-new']);
  });
});
