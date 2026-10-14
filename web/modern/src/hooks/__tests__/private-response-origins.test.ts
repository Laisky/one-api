import { act, cleanup, renderHook } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import * as privacy from '../../../../legacy/browser-sentry-privacy.mjs';
import { useChatCompletionStream } from '../chat/useChatCompletionStream';
import { useChatStream } from '../chat/useChatStream';
import { useChatRequest } from '../usePlaygroundChat/useChatRequest';
import { useStreamResponse } from '../usePlaygroundChat/useStreamResponse';

vi.mock('../../../../legacy/browser-sentry-privacy.mjs', { spy: true });

const kinds = ['chat-completion', 'chat-response', 'playground-completion', 'playground-response'] as const;
type StreamKind = (typeof kinds)[number];

/** createHarness mounts an actual stream hook and preserves its existing callback/error contract. */
function createHarness(kind: StreamKind) {
  const notify = vi.fn();
  const onError = vi.fn();
  const props = {
    selectedToken: 'synthetic-token',
    scheduleUpdate: vi.fn(),
    throttledUpdateMessage: vi.fn(),
    updateThrottleRef: { current: null },
    setMessages: vi.fn(),
    setIsStreaming: vi.fn(),
    abortControllerRef: { current: null },
    notify,
    setExpandedReasonings: vi.fn(),
  };
  const config = {
    selectedToken: 'synthetic-token',
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
    kind === 'chat-completion'
      ? (() => {
          const hook = renderHook(() => useChatCompletionStream(props));
          return () => hook.result.current.streamChatCompletion({}, new AbortController().signal);
        })()
      : kind === 'chat-response'
        ? (() => {
            const hook = renderHook(() => useChatStream(props));
            return () => hook.result.current.streamResponse({}, new AbortController().signal);
          })()
        : kind === 'playground-response'
          ? (() => {
              const hook = renderHook(() => useStreamResponse(props));
              return () => hook.result.current.streamResponse({}, new AbortController().signal);
            })()
          : (() => {
              const hook = renderHook(() => useChatRequest(config));
              return () =>
                hook.result.current.makeRequest([], new AbortController().signal, {
                  onError,
                  onUpdate: vi.fn(),
                  onFinish: vi.fn(),
                });
            })();
  /** run invokes the mounted hook while retaining a rejection for identity assertions. */
  const run = async () => {
    let failure: unknown;
    await act(async () => {
      try {
        await invoke();
      } catch (error) {
        failure = error;
      }
    });
    return failure;
  };
  return { run, notify, onError };
}

/** streamFixture builds an offline stream response without issuing a network request. */
function streamFixture(text: string) {
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

beforeEach(() => {
  vi.clearAllMocks();
});
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

describe.each(kinds)('%s private provider origins', (kind) => {
  it('marks a private HTTP body while preserving the UI message and original request', async () => {
    const raw = 'synthetic-private-provider-response';
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({ error: { message: raw } }), { status: 403 }));
    vi.stubGlobal('fetch', fetchMock);
    const harness = createHarness(kind);
    const failure = await harness.run();
    expect(privacy.privateResponseError).toHaveBeenCalledWith(raw, expect.stringContaining('HTTP 403'));
    const marked = vi.mocked(privacy.privateResponseError).mock.results[0].value as Error;
    expect(marked.message).toBe(raw);
    expect(marked.stack).toContain(raw);
    expect(privacy.isExpectedRequestError(marked)).toBe(true);
    if (kind.endsWith('response')) expect(failure).toBe(marked);
    if (kind === 'playground-completion') expect(harness.onError).toHaveBeenCalledWith(marked);
    if (kind === 'chat-completion') expect(harness.notify).toHaveBeenCalledWith(expect.objectContaining({ message: raw }));
    expect(fetchMock).toHaveBeenCalledWith(
      kind.endsWith('response') ? '/v1/responses' : '/v1/chat/completions',
      expect.objectContaining({ headers: expect.objectContaining({ Authorization: 'Bearer synthetic-token' }) })
    );
  });

  it('marks provider SSE errors without replacing their existing UI text', async () => {
    const raw = 'synthetic-private-stream-response';
    const text = kind.endsWith('response')
      ? 'event: response.error\ndata: ' + JSON.stringify({ error: { message: raw } }) + '\n\n'
      : 'data: ' + JSON.stringify({ type: 'error', error: raw }) + '\n\n';
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(streamFixture(text)));
    vi.spyOn(console, 'warn').mockImplementation(() => {});
    const harness = createHarness(kind);
    const failure = await harness.run();
    expect(privacy.privateResponseError).toHaveBeenCalledWith(raw, expect.stringContaining('stream'));
    const marked = vi.mocked(privacy.privateResponseError).mock.results[0].value as Error;
    expect(marked.message).toBe(raw);
    expect(privacy.isExpectedRequestError(marked)).toBe(true);
    if (kind.endsWith('response')) expect(failure).toBe(marked);
  });

  it('marks only the native malformed provider JSON error before the existing warning', async () => {
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {});
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(streamFixture('data: synthetic-private-malformed-json\n\n')));
    await createHarness(kind).run();
    const error = warn.mock.calls[0][1] as Error;
    expect(error).toBeInstanceOf(SyntaxError);
    expect(privacy.markPrivateError).toHaveBeenCalledWith(error, 'Chat response decoding failed');
    expect(error.message).not.toBe('Chat response decoding failed');
    expect(privacy.isExpectedRequestError(error)).toBe(false);
  });

  it('keeps ordinary transport TypeErrors unmarked', async () => {
    const ordinary = new TypeError('Failed to fetch');
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(ordinary));
    await createHarness(kind).run();
    expect(privacy.markPrivateError).not.toHaveBeenCalled();
    expect(privacy.privateResponseError).not.toHaveBeenCalled();
    expect(ordinary.message).toBe('Failed to fetch');
    expect(privacy.isExpectedRequestError(ordinary)).toBe(false);
  });

  it('does not mark a valid provider payload', async () => {
    const text = kind.endsWith('response')
      ? 'event: response.output_text.delta\ndata: {"delta":"ordinary reply"}\n\n'
      : 'data: {"choices":[{"delta":{"content":"ordinary reply"}}]}\n\n';
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(streamFixture(text)));
    await createHarness(kind).run();
    expect(privacy.markPrivateError).not.toHaveBeenCalled();
    expect(privacy.privateResponseError).not.toHaveBeenCalled();
  });
});
