import { act, cleanup, renderHook } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { registerRequestErrorCapture } from '../../../../legacy/browser-request-errors.mjs';
import { useChatCompletionStream } from '../chat/useChatCompletionStream';
import { useChatRequest } from '../usePlaygroundChat/useChatRequest';
import { useChatStream } from '../chat/useChatStream';
import { useStreamResponse } from '../usePlaygroundChat/useStreamResponse';

let unregister = () => {};
afterEach(() => {
  unregister();
  cleanup();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

/** responseFixture supplies an offline stream with no external HTTP or telemetry. */
function responseFixture(text: string) {
  return {
    ok: true,
    body: new ReadableStream({
      start(controller) {
        controller.enqueue(new TextEncoder().encode(text));
        controller.close();
      },
    }),
  };
}

/** mountCompletion mounts the actual caught-error hooks with their unchanged UI callbacks. */
function mountCompletion(kind: 'chat' | 'playground', onUpdate: (content: string, reasoning: string) => void = vi.fn()) {
  const notify = vi.fn();
  const onError = vi.fn();
  const config = {
    selectedToken: 'fixture',
    selectedModel: 'gpt-4o-mini',
    temperature: [0.7],
    maxTokens: [128],
    maxCompletionTokens: [128],
    topP: [1],
    topK: [40],
    frequencyPenalty: [0],
    presencePenalty: [0],
    stopSequences: '',
    reasoningEffort: 'none',
    thinkingEnabled: false,
    thinkingBudgetTokens: [128],
    systemMessage: '',
  };
  const invoke =
    kind === 'chat'
      ? (() => {
          const hook = renderHook(() =>
            useChatCompletionStream({
              selectedToken: 'fixture',
              scheduleUpdate: onUpdate,
              throttledUpdateMessage: vi.fn(),
              updateThrottleRef: { current: null },
              setMessages: vi.fn(),
              setIsStreaming: vi.fn(),
              abortControllerRef: { current: null },
              notify,
              setExpandedReasonings: vi.fn(),
            })
          );
          return () => hook.result.current.streamChatCompletion({}, new AbortController().signal);
        })()
      : (() => {
          const hook = renderHook(() => useChatRequest(config));
          return () => hook.result.current.makeRequest([], new AbortController().signal, { onError, onUpdate, onFinish: vi.fn() });
        })();
  /** run awaits the existing UI-handled request without introducing an outer telemetry callback. */
  const run = () =>
    act(async () => {
      await invoke();
    });
  return { run, notify, onError };
}

describe.each(['chat', 'playground'] as const)('%s unexpected caught requests', (kind) => {
  it('reports the same transport error once while preserving repeated UI callbacks', async () => {
    const error = new TypeError('Failed to fetch');
    const capture = vi.fn();
    unregister = registerRequestErrorCapture(capture);
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(error));
    const harness = mountCompletion(kind);
    await harness.run();
    await harness.run();
    expect(capture).toHaveBeenCalledTimes(1);
    expect(capture).toHaveBeenCalledWith(error);
    if (kind === 'playground') expect(harness.onError).toHaveBeenCalledTimes(2);
    else expect(harness.notify).toHaveBeenCalledTimes(2);
    expect(error.message).toBe('Failed to fetch');
  });

  it('keeps expected HTTP responses in UI without capture', async () => {
    const capture = vi.fn();
    unregister = registerRequestErrorCapture(capture);
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response('{"error":{"message":"synthetic-private-response"}}', { status: 403 })));
    const harness = mountCompletion(kind);
    await harness.run();
    expect(capture).not.toHaveBeenCalled();
    if (kind === 'playground') expect(harness.onError.mock.calls[0][0].message).toBe('synthetic-private-response');
    else expect(harness.notify).toHaveBeenCalledWith(expect.objectContaining({ message: 'synthetic-private-response' }));
  });

  it('keeps provider errors and cancellation out of telemetry', async () => {
    const capture = vi.fn();
    unregister = registerRequestErrorCapture(capture);
    vi.spyOn(console, 'warn').mockImplementation(() => {});
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(responseFixture('data: {"type":"error","error":"synthetic-private-provider"}\n\n')));
    await mountCompletion(kind).run();
    expect(capture).not.toHaveBeenCalled();
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new DOMException('fixture cancellation', 'AbortError')));
    await mountCompletion(kind).run();
    expect(capture).not.toHaveBeenCalled();
  });

  it('reports one malformed-stream diagnostic across multiple invalid frames', async () => {
    const capture = vi.fn();
    unregister = registerRequestErrorCapture(capture);
    vi.spyOn(console, 'warn').mockImplementation(() => {});
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(responseFixture('data: synthetic-private-frame-one\n\ndata: synthetic-private-frame-two\n\n'))
    );
    await mountCompletion(kind).run();
    expect(capture).toHaveBeenCalledTimes(1);
    expect(capture.mock.calls[0][0]).toBeInstanceOf(SyntaxError);
  });

  it('preserves and captures an ordinary programmer failure after successful decoding', async () => {
    const capture = vi.fn();
    unregister = registerRequestErrorCapture(capture);
    vi.spyOn(console, 'warn').mockImplementation(() => {});
    const error = new TypeError('ordinary update implementation failure');
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(responseFixture('data: {"choices":[{"delta":{"content":"fixture"}}]}\n\n')));
    await mountCompletion(
      kind,
      vi.fn(() => {
        throw error;
      })
    ).run();
    expect(capture).toHaveBeenCalledWith(error);
    expect(error.message).toBe('ordinary update implementation failure');
  });

  it('preserves UI error behavior when SDK capture is absent', async () => {
    const error = new TypeError('Failed to fetch');
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(error));
    const harness = mountCompletion(kind);
    await harness.run();
    if (kind === 'playground') expect(harness.onError).toHaveBeenCalledWith(error);
    else expect(harness.notify).toHaveBeenCalledWith(expect.objectContaining({ message: error.message }));
  });

  it('preserves UI behavior when capture throws and logs only a static failure diagnostic', async () => {
    unregister = registerRequestErrorCapture(() => {
      throw new Error('synthetic-private-reporter-failure');
    });
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {});
    const error = new TypeError('Failed to fetch');
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(error));
    const harness = mountCompletion(kind);
    await harness.run();
    expect(warn).toHaveBeenCalledWith('Browser request error reporting unavailable.');
    if (kind === 'playground') expect(harness.onError).toHaveBeenCalledWith(error);
    else expect(harness.notify).toHaveBeenCalledWith(expect.objectContaining({ message: error.message }));
  });

  it('does not capture valid stream content', async () => {
    const capture = vi.fn();
    unregister = registerRequestErrorCapture(capture);
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(responseFixture('data: {"choices":[{"delta":{"content":"fixture"}}]}\n\n')));
    await mountCompletion(kind).run();
    expect(capture).not.toHaveBeenCalled();
  });
});

it.each(['chat', 'playground'] as const)('%s response decoding reports only the first malformed frame', async (kind) => {
  const capture = vi.fn();
  unregister = registerRequestErrorCapture(capture);
  vi.spyOn(console, 'warn').mockImplementation(() => {});
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue(responseFixture('data: bad-one\n\ndata: bad-two\n\n')));
  const props = {
    selectedToken: 'fixture',
    scheduleUpdate: vi.fn(),
    throttledUpdateMessage: vi.fn(),
    updateThrottleRef: { current: null },
  };
  const run =
    kind === 'chat'
      ? (() => {
          const hook = renderHook(() => useChatStream({ ...props, setMessages: vi.fn() }));
          return () => hook.result.current.streamResponse({}, new AbortController().signal);
        })()
      : (() => {
          const hook = renderHook(() => useStreamResponse(props));
          return () => hook.result.current.streamResponse({}, new AbortController().signal);
        })();
  await act(async () => {
    await run();
  });
  expect(capture).toHaveBeenCalledTimes(1);
  expect(capture.mock.calls[0][0]).toBeInstanceOf(SyntaxError);
});
