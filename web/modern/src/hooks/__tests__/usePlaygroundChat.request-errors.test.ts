import { act, cleanup, renderHook } from '@testing-library/react';
import { afterEach, expect, it, vi } from 'vitest';
import { registerRequestErrorCapture, reportUnexpectedRequestError } from '../../../../legacy/browser-request-errors.mjs';
import { privateResponseError } from '../../../../legacy/browser-sentry-privacy.mjs';

const stream = vi.hoisted(() => vi.fn());
vi.mock('@/components/ui/notifications', () => ({ useNotifications: () => ({ notify: vi.fn(), dismiss: vi.fn() }) }));
vi.mock('../chat/useChatCompletionStream', () => ({ useChatCompletionStream: () => ({ streamChatCompletion: stream }) }));
vi.mock('../chat/useChatStream', () => ({ useChatStream: () => ({ streamResponse: vi.fn() }) }));
import { usePlaygroundChat } from '../usePlaygroundChat';

let unregister = () => {};
afterEach(() => {
  unregister();
  cleanup();
  vi.clearAllMocks();
});

/** mountPlayground supplies valid UI state without performing a real chat or telemetry request. */
function mountPlayground() {
  const addEvent = vi.fn();
  const hook = renderHook(() =>
    usePlaygroundChat({
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
      messages: [],
      setMessages: vi.fn(),
      expandedReasonings: {},
      setExpandedReasonings: vi.fn(),
      addEvent,
    })
  );
  return { hook, addEvent };
}

it('reports an unexpected outer failure and preserves the existing UI event', async () => {
  const capture = vi.fn();
  unregister = registerRequestErrorCapture(capture);
  const error = new TypeError('ordinary unexpected stream failure');
  stream.mockRejectedValueOnce(error);
  const { hook, addEvent } = mountPlayground();
  await act(async () => {
    await hook.result.current.sendMessage('fixture');
  });
  expect(capture).toHaveBeenCalledTimes(1);
  expect(capture).toHaveBeenCalledWith(error);
  expect(addEvent).toHaveBeenCalledWith(
    expect.objectContaining({
      type: 'error',
      payload: { message: error.message },
    })
  );
});

it('does not duplicate an inner capture when the same error reaches the outer UI catch', async () => {
  const capture = vi.fn();
  unregister = registerRequestErrorCapture(capture);
  const error = new TypeError('ordinary inner stream failure');
  stream.mockImplementationOnce(async () => {
    reportUnexpectedRequestError(error);
    throw error;
  });
  const { hook, addEvent } = mountPlayground();
  await act(async () => {
    await hook.result.current.sendMessage('fixture');
  });
  expect(capture).toHaveBeenCalledTimes(1);
  expect(addEvent).toHaveBeenCalledWith(expect.objectContaining({ type: 'error' }));
});

it('keeps an expected private provider response in the existing UI without telemetry', async () => {
  const capture = vi.fn();
  unregister = registerRequestErrorCapture(capture);
  const error = privateResponseError('synthetic-private-provider-response', 'Chat provider failed');
  stream.mockRejectedValueOnce(error);
  const { hook, addEvent } = mountPlayground();
  await act(async () => {
    await hook.result.current.sendMessage('fixture');
  });
  expect(capture).not.toHaveBeenCalled();
  expect(addEvent).toHaveBeenCalledWith(
    expect.objectContaining({
      type: 'error',
      payload: { message: error.message },
    })
  );
});

it('preserves the existing outer UI event when SDK capture is absent', async () => {
  const error = new TypeError('ordinary offline stream failure');
  stream.mockRejectedValueOnce(error);
  const { hook, addEvent } = mountPlayground();
  await act(async () => {
    await hook.result.current.sendMessage('fixture');
  });
  expect(addEvent).toHaveBeenCalledWith(
    expect.objectContaining({
      type: 'error',
      payload: { message: error.message },
    })
  );
});
