import { ImageAttachment as ImageAttachmentType } from "@/components/chat/ImageAttachment";
import { useNotifications } from "@/components/ui/notifications";
import {
  getModelCapabilities,
  isOpenAIMediumOnlyReasoningModel,
} from "@/lib/model-capabilities";
import { Message } from "@/lib/utils";
import { useCallback } from "react";
import { UsePlaygroundChatProps, UsePlaygroundChatReturn } from "./chat/types";
import { useChatCompletionStream } from "./chat/useChatCompletionStream";
import { useChatState } from "./chat/useChatState";
import { useChatStream } from "./chat/useChatStream";

export function usePlaygroundChat({
  selectedToken,
  selectedModel,
  temperature,
  maxTokens,
  maxCompletionTokens,
  topP,
  topK,
  frequencyPenalty,
  presencePenalty,
  stopSequences,
  thinkingEnabled,
  reasoningEffort,
  thinkingBudgetTokens,
  systemMessage,
  messages,
  setMessages,
  expandedReasonings,
  setExpandedReasonings,
}: UsePlaygroundChatProps): UsePlaygroundChatReturn {
  const { notify } = useNotifications();

  const {
    isStreaming,
    setIsStreaming,
    abortControllerRef,
    updateThrottleRef,
    throttledUpdateMessage,
    scheduleUpdate,
    addErrorMessage,
  } = useChatState({
    messages,
    setMessages,
    expandedReasonings,
    setExpandedReasonings,
  });

  const { streamResponse } = useChatStream({
    selectedToken,
    scheduleUpdate,
    throttledUpdateMessage,
    updateThrottleRef,
    setMessages,
  });

  const { streamChatCompletion } = useChatCompletionStream({
    selectedToken,
    scheduleUpdate,
    throttledUpdateMessage,
    updateThrottleRef,
    setMessages,
    setIsStreaming,
    abortControllerRef,
    notify,
    setExpandedReasonings,
  });

  const sendMessage = useCallback(
    async (messageContent: string, images?: ImageAttachmentType[]) => {
      if (
        (!messageContent.trim() && (!images || images.length === 0)) ||
        !selectedModel ||
        !selectedToken ||
        isStreaming
      ) {
        return;
      }

      const formatMessageContent = () => {
        const contentArray: any[] = [];
        if (messageContent.trim()) {
          contentArray.push({
            type: "text",
            text: messageContent.trim(),
          });
        }
        if (images && images.length > 0) {
          images.forEach((image) => {
            contentArray.push({
              type: "image_url",
              image_url: {
                url: image.base64,
              },
            });
          });
        }
        return contentArray.length === 1 && contentArray[0].type === "text"
          ? messageContent.trim()
          : contentArray;
      };

      const userMessage: Message = {
        role: "user",
        content: formatMessageContent(),
        timestamp: Date.now(),
      };

      const newMessages = [...messages, userMessage];
      setMessages(newMessages);
      setIsStreaming(true);

      const assistantMessage: Message = {
        role: "assistant",
        content: "",
        reasoning_content: null,
        timestamp: Date.now(),
        model: selectedModel,
      };
      setMessages([...newMessages, assistantMessage]);

      abortControllerRef.current = new AbortController();

      const capabilities = getModelCapabilities(selectedModel);
      const effectiveReasoningEffort =
        capabilities.supportsReasoningEffort && reasoningEffort !== "none"
          ? selectedModel && isOpenAIMediumOnlyReasoningModel(selectedModel)
            ? "medium"
            : reasoningEffort
          : undefined;

      const requestBody = {
        messages: (() => {
          const filteredMessages = newMessages
            .filter((msg) => msg.role !== "error")
            .map((msg) => ({ role: msg.role, content: msg.content }));
          if (systemMessage.trim()) {
            const hasSystemMessage = filteredMessages.some(
              (msg) => msg.role === "system"
            );
            if (!hasSystemMessage) {
              return [
                { role: "system", content: systemMessage.trim() },
                ...filteredMessages,
              ];
            }
          }
          return filteredMessages;
        })(),
        model: selectedModel,
        temperature: temperature[0],
        max_tokens: maxTokens[0],
        ...(capabilities.supportsTopP && { top_p: topP[0] }),
        ...(capabilities.supportsMaxCompletionTokens && {
          max_completion_tokens: maxCompletionTokens[0],
        }),
        ...(capabilities.supportsTopK && { top_k: topK[0] }),
        ...(capabilities.supportsFrequencyPenalty && {
          frequency_penalty: frequencyPenalty[0],
        }),
        ...(capabilities.supportsPresencePenalty && {
          presence_penalty: presencePenalty[0],
        }),
        ...(capabilities.supportsStop &&
          stopSequences && {
            stop: stopSequences
              .split(",")
              .map((s) => s.trim())
              .filter((s) => s),
          }),
        ...(effectiveReasoningEffort && {
          reasoning_effort: effectiveReasoningEffort,
        }),
        ...(capabilities.supportsThinking &&
          thinkingEnabled && {
            thinking: {
              type: "enabled",
              budget_tokens: thinkingBudgetTokens[0],
            },
          }),
        stream: true,
      };

      await streamChatCompletion(
        requestBody,
        abortControllerRef.current.signal
      );
    },
    [
      selectedModel,
      selectedToken,
      isStreaming,
      messages,
      temperature,
      maxTokens,
      maxCompletionTokens,
      topP,
      topK,
      frequencyPenalty,
      presencePenalty,
      stopSequences,
      reasoningEffort,
      thinkingEnabled,
      thinkingBudgetTokens,
      systemMessage,
      setMessages,
      streamChatCompletion,
    ]
  );

  const regenerateMessage = useCallback(
    async (existingMessages: Message[]) => {
      if (!selectedModel || !selectedToken || isStreaming) {
        return;
      }

      setIsStreaming(true);

      const assistantMessage: Message = {
        role: "assistant",
        content: "",
        reasoning_content: null,
        timestamp: Date.now(),
        model: selectedModel,
      };
      setMessages([...existingMessages, assistantMessage]);

      abortControllerRef.current = new AbortController();

      const capabilities = getModelCapabilities(selectedModel);
      const effectiveReasoningEffort =
        capabilities.supportsReasoningEffort &&
        reasoningEffort &&
        reasoningEffort !== "none"
          ? selectedModel && isOpenAIMediumOnlyReasoningModel(selectedModel)
            ? "medium"
            : reasoningEffort
          : undefined;

      const requestBody = {
        messages: (() => {
          const filteredMessages = existingMessages
            .filter((msg) => msg.role !== "error")
            .map((msg) => ({ role: msg.role, content: msg.content }));
          if (systemMessage.trim()) {
            const hasSystemMessage = filteredMessages.some(
              (msg) => msg.role === "system"
            );
            if (!hasSystemMessage) {
              return [
                { role: "system", content: systemMessage.trim() },
                ...filteredMessages,
              ];
            }
          }
          return filteredMessages;
        })(),
        model: selectedModel,
        temperature: temperature[0],
        max_tokens: maxTokens[0],
        ...(capabilities.supportsTopP && { top_p: topP[0] }),
        ...(capabilities.supportsMaxCompletionTokens && {
          max_completion_tokens: maxCompletionTokens[0],
        }),
        ...(capabilities.supportsTopK && { top_k: topK[0] }),
        ...(capabilities.supportsFrequencyPenalty && {
          frequency_penalty: frequencyPenalty[0],
        }),
        ...(capabilities.supportsPresencePenalty && {
          presence_penalty: presencePenalty[0],
        }),
        ...(capabilities.supportsStop &&
          stopSequences && {
            stop: stopSequences
              .split(",")
              .map((s) => s.trim())
              .filter((s) => s),
          }),
        ...(effectiveReasoningEffort && {
          reasoning_effort: effectiveReasoningEffort,
        }),
        ...(capabilities.supportsThinking &&
          thinkingEnabled && {
            thinking: {
              type: "enabled",
              budget_tokens: thinkingBudgetTokens[0],
            },
          }),
        stream: true,
      };

      await streamChatCompletion(
        requestBody,
        abortControllerRef.current.signal
      );
    },
    [
      selectedModel,
      selectedToken,
      isStreaming,
      temperature,
      maxTokens,
      maxCompletionTokens,
      topP,
      topK,
      frequencyPenalty,
      presencePenalty,
      stopSequences,
      thinkingEnabled,
      thinkingBudgetTokens,
      systemMessage,
      reasoningEffort,
      setMessages,
      streamChatCompletion,
    ]
  );

  const stopGeneration = useCallback(() => {
    if (abortControllerRef.current) {
      abortControllerRef.current.abort();
    }
  }, []);

  return {
    isStreaming,
    sendMessage,
    regenerateMessage,
    stopGeneration,
    addErrorMessage,
  };
}
