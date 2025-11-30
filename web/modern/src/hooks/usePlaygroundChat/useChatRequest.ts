import { useCallback } from "react";
import {
  getModelCapabilities,
  isOpenAIMediumOnlyReasoningModel,
} from "@/lib/model-capabilities";
import type { Message } from "@/lib/utils";
import type { ChatRequestConfig } from "./types";

interface ChatCallbacks {
  onUpdate: (content: string, reasoning: string) => void;
  onError: (error: Error) => void;
  onFinish: () => void;
}

export const useChatRequest = (config: ChatRequestConfig) => {
  const makeRequest = useCallback(
    async (
      messages: Message[],
      signal: AbortSignal,
      callbacks: ChatCallbacks
    ) => {
      const {
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
        reasoningEffort,
        thinkingEnabled,
        thinkingBudgetTokens,
        systemMessage,
      } = config;

      try {
        // Get model capabilities to determine which parameters to include
        const capabilities = getModelCapabilities(selectedModel);
        const effectiveReasoningEffort =
          capabilities.supportsReasoningEffort &&
          reasoningEffort &&
          reasoningEffort !== "none"
            ? selectedModel && isOpenAIMediumOnlyReasoningModel(selectedModel)
              ? "medium"
              : reasoningEffort
            : undefined;

        const response = await fetch("/v1/chat/completions", {
          method: "POST",
          headers: {
            Authorization: `Bearer ${selectedToken}`,
            "Content-Type": "application/json",
            Accept: "text/event-stream",
          },
          body: JSON.stringify({
            messages: (() => {
              // Filter out error messages
              const filteredMessages = messages
                .filter((msg) => msg.role !== "error")
                .map((msg) => ({ role: msg.role, content: msg.content }));

              // Prepend system message if it exists and isn't already at the start
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
            // Only include top_p if model supports it
            ...(capabilities.supportsTopP && { top_p: topP[0] }),
            // Only include max_completion_tokens if model supports it
            ...(capabilities.supportsMaxCompletionTokens && {
              max_completion_tokens: maxCompletionTokens[0],
            }),
            // Only include top_k if model supports it
            ...(capabilities.supportsTopK && { top_k: topK[0] }),
            // Only include frequency_penalty if model supports it
            ...(capabilities.supportsFrequencyPenalty && {
              frequency_penalty: frequencyPenalty[0],
            }),
            // Only include presence_penalty if model supports it
            ...(capabilities.supportsPresencePenalty && {
              presence_penalty: presencePenalty[0],
            }),
            // Only include stop sequences if model supports them and has values
            ...(capabilities.supportsStop &&
              stopSequences && {
                stop: stopSequences
                  .split(",")
                  .map((s) => s.trim())
                  .filter((s) => s),
              }),
            // Only include reasoning efforts if model supports them and has values
            ...(effectiveReasoningEffort && {
              reasoning_effort: effectiveReasoningEffort,
            }),
            // Only include thinking if model supports it and it's enabled
            ...(capabilities.supportsThinking &&
              thinkingEnabled && {
                thinking: {
                  type: "enabled",
                  budget_tokens: thinkingBudgetTokens[0],
                },
              }),
            stream: true,
          }),
          signal,
        });

        if (!response.ok) {
          // Try to parse JSON error response for detailed error information
          let errorMessage = `HTTP ${response.status}: ${response.statusText}`;
          try {
            const errorBody = await response.text();
            if (errorBody.trim()) {
              try {
                const errorJson = JSON.parse(errorBody);
                // Extract detailed error message from various possible JSON structures
                if (errorJson.error?.message) {
                  errorMessage = errorJson.error.message;
                } else if (
                  errorJson.error &&
                  typeof errorJson.error === "string"
                ) {
                  errorMessage = errorJson.error;
                } else if (errorJson.message) {
                  errorMessage = errorJson.message;
                } else if (errorJson.detail) {
                  errorMessage = errorJson.detail;
                } else {
                  // If we have JSON but no recognizable error field, show formatted JSON
                  errorMessage = `HTTP ${response.status}: ${JSON.stringify(errorJson, null, 2)}`;
                }
              } catch (_jsonParseError) {
                // If it's not JSON, use the raw text if it's more informative than the status
                if (errorBody.length > 0 && errorBody !== response.statusText) {
                  errorMessage = `HTTP ${response.status}: ${errorBody}`;
                }
              }
            }
          } catch (readError) {
            // If we can't read the response body, fall back to status
            console.warn("Failed to read error response body:", readError);
          }
          throw new Error(errorMessage);
        }

        const reader = response.body?.getReader();
        const decoder = new TextDecoder();

        if (!reader) {
          throw new Error("No response body");
        }

        let assistantContent = "";
        let reasoningContent = "";

        while (true) {
          const { done, value } = await reader.read();

          if (done) {
            callbacks.onFinish();
            break;
          }

          const chunk = decoder.decode(value);
          const lines = chunk.split("\n");

          for (const line of lines) {
            if (line.startsWith("data: ")) {
              const data = line.slice(6);

              if (data === "[DONE]") {
                continue;
              }

              try {
                const parsed = JSON.parse(data);

                if (parsed.type === "error") {
                  const errorMsg = parsed.error || "Stream error";
                  throw new Error(errorMsg);
                }

                if (parsed.choices?.[0]?.delta) {
                  const delta = parsed.choices[0].delta;

                  // Handle regular content
                  if (delta.content) {
                    // Check if content is a string (normal content) or array (Mistral thinking format)
                    if (typeof delta.content === "string") {
                      assistantContent += delta.content;
                    } else if (Array.isArray(delta.content)) {
                      // Handle Mistral's content array format
                      for (const contentItem of delta.content) {
                        if (
                          contentItem.type === "thinking" &&
                          contentItem.thinking
                        ) {
                          // Extract thinking content from Mistral format
                          for (const thinkingItem of contentItem.thinking) {
                            if (
                              thinkingItem.type === "text" &&
                              thinkingItem.text
                            ) {
                              reasoningContent += thinkingItem.text;
                            }
                          }
                        } else if (
                          contentItem.type === "text" &&
                          contentItem.text
                        ) {
                          // Regular text content
                          assistantContent += contentItem.text;
                        }
                      }
                    } else {
                      // Fallback for other content formats
                      assistantContent += String(delta.content);
                    }
                  }

                  // Handle reasoning content from different possible fields (for other providers)
                  if (delta.reasoning) {
                    reasoningContent += delta.reasoning;
                  }
                  if (delta.reasoning_content) {
                    reasoningContent += delta.reasoning_content;
                  }
                  if (delta.thinking) {
                    reasoningContent += delta.thinking;
                  }

                  callbacks.onUpdate(assistantContent, reasoningContent);
                }
              } catch (parseError) {
                console.warn("Failed to parse SSE data:", parseError);
              }
            }
          }
        }
      } catch (error) {
        callbacks.onError(error as Error);
      }
    },
    [config]
  );

  return { makeRequest };
};
