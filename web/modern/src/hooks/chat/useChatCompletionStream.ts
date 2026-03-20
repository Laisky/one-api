import { Message, getMessageStringContent } from '@/lib/utils';
import { useCallback } from 'react';

interface UseChatCompletionStreamProps {
  selectedToken: string;
  scheduleUpdate: (content: string, reasoning_content: string) => void;
  throttledUpdateMessage: () => void;
  updateThrottleRef: React.MutableRefObject<number | null>;
  setMessages: (messages: Message[] | ((prev: Message[]) => Message[])) => void;
  setIsStreaming: (isStreaming: boolean) => void;
  abortControllerRef: React.MutableRefObject<AbortController | null>;
  notify: (options: any) => void;
  setExpandedReasonings: (expanded: Record<number, boolean> | ((prev: Record<number, boolean>) => Record<number, boolean>)) => void;
}

export const useChatCompletionStream = ({
  selectedToken,
  scheduleUpdate,
  throttledUpdateMessage,
  updateThrottleRef,
  setMessages,
  setIsStreaming,
  abortControllerRef,
  notify,
  setExpandedReasonings,
}: UseChatCompletionStreamProps) => {
  const streamChatCompletion = useCallback(
    async (requestBody: Record<string, any>, signal: AbortSignal) => {
      try {
        const response = await fetch('/v1/chat/completions', {
          method: 'POST',
          headers: {
            Authorization: `Bearer ${selectedToken}`,
            'Content-Type': 'application/json',
            Accept: 'text/event-stream',
          },
          body: JSON.stringify(requestBody),
          signal,
        });

        if (!response.ok) {
          let errorMessage = `HTTP ${response.status}: ${response.statusText}`;
          try {
            const errorBody = await response.text();
            if (errorBody.trim()) {
              try {
                const errorJson = JSON.parse(errorBody);
                if (errorJson.error?.message) {
                  errorMessage = errorJson.error.message;
                } else if (errorJson.error && typeof errorJson.error === 'string') {
                  errorMessage = errorJson.error;
                } else if (errorJson.message) {
                  errorMessage = errorJson.message;
                } else if (errorJson.detail) {
                  errorMessage = errorJson.detail;
                } else {
                  errorMessage = `HTTP ${response.status}: ${JSON.stringify(errorJson, null, 2)}`;
                }
              } catch (jsonParseError) {
                if (errorBody.length > 0 && errorBody !== response.statusText) {
                  errorMessage = `HTTP ${response.status}: ${errorBody}`;
                }
              }
            }
          } catch (readError) {
            console.warn('Failed to read error response body:', readError);
          }
          throw new Error(errorMessage);
        }

        const reader = response.body?.getReader();
        const decoder = new TextDecoder();

        if (!reader) {
          throw new Error('No response body');
        }

        let assistantContent = '';
        let reasoningContent = '';

        while (true) {
          const { done, value } = await reader.read();

          if (done) {
            if (updateThrottleRef.current !== null) {
              cancelAnimationFrame(updateThrottleRef.current);
              throttledUpdateMessage();
            }
            break;
          }

          const chunk = decoder.decode(value);
          const lines = chunk.split('\n');

          for (const line of lines) {
            if (line.startsWith('data: ')) {
              const data = line.slice(6);

              if (data === '[DONE]') {
                continue;
              }

              try {
                const parsed = JSON.parse(data);

                if (parsed.type === 'error') {
                  const errorMsg = parsed.error || 'Stream error';
                  setMessages((prev) => {
                    const messagesWithoutLastAssistant = prev.slice(0, -1);
                    const streamErrorMessage: Message = {
                      role: 'error',
                      content: errorMsg,
                      timestamp: Date.now(),
                      error: true,
                    };
                    return [...messagesWithoutLastAssistant, streamErrorMessage];
                  });
                  throw new Error(errorMsg);
                }

                if (parsed.choices && parsed.choices[0]?.delta) {
                  const delta = parsed.choices[0].delta;

                  if (delta.content) {
                    if (typeof delta.content === 'string') {
                      assistantContent += delta.content;
                    } else if (Array.isArray(delta.content)) {
                      for (const contentItem of delta.content) {
                        if (contentItem.type === 'thinking' && contentItem.thinking) {
                          for (const thinkingItem of contentItem.thinking) {
                            if (thinkingItem.type === 'text' && thinkingItem.text) {
                              reasoningContent += thinkingItem.text;
                            }
                          }
                        } else if (contentItem.type === 'text' && contentItem.text) {
                          assistantContent += contentItem.text;
                        }
                      }
                    } else {
                      assistantContent += String(delta.content);
                    }
                  }

                  if (delta.reasoning) {
                    reasoningContent += delta.reasoning;
                  }
                  if (delta.reasoning_content) {
                    reasoningContent += delta.reasoning_content;
                  }
                  if (delta.thinking) {
                    reasoningContent += delta.thinking;
                  }

                  scheduleUpdate(assistantContent, reasoningContent);
                }
              } catch (parseError) {
                console.warn('Failed to parse SSE data:', parseError);
              }
            }
          }
        }
      } catch (error: any) {
        if (error.name === 'AbortError') {
          notify({
            title: 'Request Cancelled',
            message: 'The request was cancelled by the user',
            type: 'info',
          });
          setMessages((prev) => prev.slice(0, -1));
        } else {
          const errorMessage = error.message || 'Failed to send message';
          setMessages((prev) => {
            const messagesWithoutAssistant = prev.slice(0, -1);
            const errorMsg: Message = {
              role: 'error',
              content: errorMessage,
              timestamp: Date.now(),
              error: true,
            };
            return [...messagesWithoutAssistant, errorMsg];
          });
          notify({
            title: 'Error',
            message: errorMessage,
            type: 'error',
          });
        }
      } finally {
        setIsStreaming(false);
        abortControllerRef.current = null;

        setMessages((prev) => {
          if (prev.length > 0) {
            const lastMessage = prev[prev.length - 1];
            const lastMessageIndex = prev.length - 1;

            if (
              lastMessage.role === 'assistant' &&
              lastMessage.content &&
              getMessageStringContent(lastMessage.content).trim().length > 0 &&
              lastMessage.reasoning_content &&
              lastMessage.reasoning_content.trim().length > 0
            ) {
              setExpandedReasonings((prevExpanded) => ({
                ...prevExpanded,
                [lastMessageIndex]: false,
              }));
            }
          }
          return prev;
        });
      }
    },
    [
      selectedToken,
      scheduleUpdate,
      throttledUpdateMessage,
      updateThrottleRef,
      setMessages,
      setIsStreaming,
      abortControllerRef,
      notify,
      setExpandedReasonings,
    ]
  );

  return { streamChatCompletion };
};
