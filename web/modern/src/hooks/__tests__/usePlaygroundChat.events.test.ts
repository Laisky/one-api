import { act, renderHook } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

vi.mock('@/components/ui/notifications', () => ({
  useNotifications: () => ({ notify: vi.fn(), dismiss: vi.fn() }),
}));

const streamChatCompletionMock = vi.fn(() => Promise.resolve());

vi.mock('../chat/useChatCompletionStream', () => ({
  useChatCompletionStream: () => ({ streamChatCompletion: streamChatCompletionMock }),
}));

vi.mock('../chat/useChatStream', () => ({
  useChatStream: () => ({ streamResponse: vi.fn(() => Promise.resolve()) }),
}));

import { usePlaygroundChat } from '../usePlaygroundChat';

const baseProps = (overrides: Record<string, unknown> = {}) => ({
  selectedToken: 'token-1',
  selectedModel: 'gpt-4o-mini',
  temperature: [0.7],
  maxTokens: [1024],
  maxCompletionTokens: [1024],
  topP: [1],
  topK: [40],
  frequencyPenalty: [0],
  presencePenalty: [0],
  stopSequences: '',
  reasoningEffort: 'none',
  thinkingEnabled: false,
  thinkingBudgetTokens: [1024],
  systemMessage: '',
  messages: [],
  setMessages: vi.fn(),
  expandedReasonings: {},
  setExpandedReasonings: vi.fn(),
  ...overrides,
});

describe('usePlaygroundChat addEvent emissions', () => {
  it('emits a cancel event via addEvent when stopGeneration is called after a request', async () => {
    const addEvent = vi.fn();
    const setMessages = vi.fn();

    const { result } = renderHook(() =>
      usePlaygroundChat(baseProps({ addEvent, setMessages }) as any)
    );

    await act(async () => {
      await result.current.sendMessage('hello');
    });

    addEvent.mockClear();

    act(() => {
      result.current.stopGeneration();
    });

    expect(addEvent).toHaveBeenCalledWith({
      direction: 'out',
      type: 'cancel',
      payload: {},
      transport: 'sse',
    });
  });

  it('emits request and done events around a successful sendMessage', async () => {
    const addEvent = vi.fn();
    const setMessages = vi.fn();
    streamChatCompletionMock.mockResolvedValueOnce(undefined as unknown as void);

    const { result } = renderHook(() =>
      usePlaygroundChat(baseProps({ addEvent, setMessages }) as any)
    );

    await act(async () => {
      await result.current.sendMessage('hi there');
    });

    const calls = addEvent.mock.calls.map((c) => c[0]);
    const requestCall = calls.find((c) => c.type === 'request');
    const doneCall = calls.find((c) => c.type === 'done');

    expect(requestCall).toBeDefined();
    expect(requestCall.direction).toBe('out');
    expect(requestCall.transport).toBe('sse');
    expect(requestCall.payload).toMatchObject({ model: 'gpt-4o-mini', stream: true });

    expect(doneCall).toBeDefined();
    expect(doneCall.direction).toBe('in');
    expect(doneCall.transport).toBe('sse');
  });

  it('emits an error event when streamChatCompletion rejects in sendMessage', async () => {
    const addEvent = vi.fn();
    const setMessages = vi.fn();
    streamChatCompletionMock.mockRejectedValueOnce(new Error('boom'));

    const { result } = renderHook(() =>
      usePlaygroundChat(baseProps({ addEvent, setMessages }) as any)
    );

    await act(async () => {
      await result.current.sendMessage('hi there');
    });

    const calls = addEvent.mock.calls.map((c) => c[0]);
    const errorCall = calls.find((c) => c.type === 'error');

    expect(errorCall).toBeDefined();
    expect(errorCall.direction).toBe('in');
    expect(errorCall.transport).toBe('sse');
    expect(errorCall.payload).toMatchObject({ message: 'boom' });
  });

  it('emits request and done events for a successful regenerateMessage', async () => {
    const addEvent = vi.fn();
    const setMessages = vi.fn();
    streamChatCompletionMock.mockResolvedValueOnce(undefined as unknown as void);

    const { result } = renderHook(() =>
      usePlaygroundChat(baseProps({ addEvent, setMessages }) as any)
    );

    await act(async () => {
      await result.current.regenerateMessage([
        { role: 'user', content: 'hello', timestamp: Date.now() } as any,
      ]);
    });

    const calls = addEvent.mock.calls.map((c) => c[0]);
    expect(calls.some((c) => c.type === 'request' && c.direction === 'out')).toBe(true);
    expect(calls.some((c) => c.type === 'done' && c.direction === 'in')).toBe(true);
  });
});
