import { useCallback } from "react";
import { extractTextAndReasoningFromOutput } from "./helpers";

export interface StreamResponseResult {
  assistantContent: string;
  reasoningContent: string;
  status: string | null;
  incompleteDetails: unknown;
}

interface UseStreamResponseProps {
  selectedToken: string;
  scheduleUpdate: (content: string, reasoning_content: string) => void;
  throttledUpdateMessage: () => void;
  updateThrottleRef: React.MutableRefObject<number | null>;
}

export const useStreamResponse = ({
  selectedToken,
  scheduleUpdate,
  throttledUpdateMessage,
  updateThrottleRef,
}: UseStreamResponseProps) => {
  const streamResponse = useCallback(
    async (
      requestBody: Record<string, unknown>,
      signal: AbortSignal
    ): Promise<StreamResponseResult> => {
      const response = await fetch("/v1/responses", {
        method: "POST",
        headers: {
          Authorization: `Bearer ${selectedToken}`,
          "Content-Type": "application/json",
          Accept: "text/event-stream",
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
              } else if (typeof errorJson.error === "string") {
                errorMessage = errorJson.error;
              } else if (errorJson.message) {
                errorMessage = errorJson.message;
              } else if (errorJson.detail) {
                errorMessage = errorJson.detail;
              } else {
                errorMessage = `HTTP ${response.status}: ${JSON.stringify(errorJson, null, 2)}`;
              }
            } catch {
              if (errorBody && errorBody !== response.statusText) {
                errorMessage = `HTTP ${response.status}: ${errorBody}`;
              }
            }
          }
        } catch {
          // ignore secondary errors while reading response body
        }
        throw new Error(errorMessage);
      }

      const reader = response.body?.getReader();
      if (!reader) {
        throw new Error("No response body");
      }

      const decoder = new TextDecoder();
      let buffer = "";
      let assistantContent = "";
      let reasoningContent = "";
      let finalStatus: string | null = null;
      let incompleteDetails: unknown = null;

      const appendTextDelta = (delta: string | undefined) => {
        if (!delta) {
          return;
        }
        assistantContent += delta;
        scheduleUpdate(assistantContent, reasoningContent);
      };

      const appendReasoningDelta = (delta: string | undefined) => {
        if (!delta) {
          return;
        }
        reasoningContent += delta;
        scheduleUpdate(assistantContent, reasoningContent);
      };

      // biome-ignore lint/suspicious/noExplicitAny: payload is dynamic
      const applyResponsePayload = (payload: any) => {
        if (!payload || typeof payload !== "object") {
          return;
        }

        if (typeof payload.status === "string") {
          finalStatus = payload.status;
        }
        if (payload.incomplete_details) {
          incompleteDetails = payload.incomplete_details;
        }

        const { text, reasoning } = extractTextAndReasoningFromOutput(
          payload.output
        );
        if (text !== null) {
          assistantContent = text;
          scheduleUpdate(assistantContent, reasoningContent);
        }
        if (reasoning !== null) {
          reasoningContent = reasoning;
          scheduleUpdate(assistantContent, reasoningContent);
        }
      };

      const processEvent = (rawEvent: string): boolean => {
        const sanitized = rawEvent.replace(/\r/g, "");
        const lines = sanitized.split("\n");
        let eventType = "";
        const dataLines: string[] = [];

        for (const line of lines) {
          if (line.startsWith("event:")) {
            eventType = line.slice(6).trim();
            continue;
          }
          if (line.startsWith("data:")) {
            let value = line.slice(5);
            if (value.startsWith(" ")) {
              value = value.slice(1);
            }
            dataLines.push(value);
            continue;
          }
          if (line.startsWith(":")) {
          }
        }

        const dataString = dataLines.join("\n");
        if (dataString === "") {
          return false;
        }
        if (dataString === "[DONE]") {
          return true;
        }

        // biome-ignore lint/suspicious/noExplicitAny: JSON.parse returns any
        let payload: any;
        try {
          payload = JSON.parse(dataString);
        } catch (parseError) {
          console.warn("Failed to parse SSE data:", parseError);
          return false;
        }

        const resolvedType =
          eventType || (typeof payload?.type === "string" ? payload.type : "");

        switch (resolvedType) {
          case "response.output_text.delta":
            appendTextDelta(
              typeof payload.delta === "string" ? payload.delta : undefined
            );
            if (payload.response) {
              applyResponsePayload(payload.response);
            }
            break;
          case "response.reasoning_summary_text.delta":
            appendReasoningDelta(
              typeof payload.delta === "string" ? payload.delta : undefined
            );
            if (payload.response) {
              applyResponsePayload(payload.response);
            }
            break;
          case "response.output_text.done":
            if (typeof payload.text === "string") {
              assistantContent = payload.text;
              scheduleUpdate(assistantContent, reasoningContent);
            }
            if (payload.response) {
              applyResponsePayload(payload.response);
            }
            break;
          case "response.reasoning_summary_text.done":
            if (typeof payload.text === "string") {
              reasoningContent = payload.text;
              scheduleUpdate(assistantContent, reasoningContent);
            }
            if (payload.response) {
              applyResponsePayload(payload.response);
            }
            break;
          case "response.output_item.done":
          case "response.content_part.done":
            if (payload.item) {
              const { text, reasoning } = extractTextAndReasoningFromOutput([
                payload.item,
              ]);
              if (text !== null) {
                assistantContent = text;
                scheduleUpdate(assistantContent, reasoningContent);
              }
              if (reasoning !== null) {
                reasoningContent = reasoning;
                scheduleUpdate(assistantContent, reasoningContent);
              }
            }
            if (payload.response) {
              applyResponsePayload(payload.response);
            }
            break;
          case "response.completed":
            if (payload.response) {
              applyResponsePayload(payload.response);
            }
            break;
          case "response.error": {
            const errorMessage =
              typeof payload?.error?.message === "string"
                ? payload.error.message
                : typeof payload?.error === "string"
                  ? payload.error
                  : "Stream error";
            throw new Error(errorMessage);
          }
          default:
            if (payload?.response) {
              applyResponsePayload(payload.response);
            } else if (payload?.output) {
              const { text, reasoning } = extractTextAndReasoningFromOutput(
                payload.output
              );
              if (text !== null) {
                assistantContent = text;
                scheduleUpdate(assistantContent, reasoningContent);
              }
              if (reasoning !== null) {
                reasoningContent = reasoning;
                scheduleUpdate(assistantContent, reasoningContent);
              }
            }
            break;
        }

        return false;
      };

      const processPendingEvents = (): boolean => {
        let shouldStop = false;
        let boundary = buffer.indexOf("\n\n");
        while (boundary !== -1) {
          const rawEvent = buffer.slice(0, boundary);
          buffer = buffer.slice(boundary + 2);
          if (rawEvent.trim().length > 0) {
            if (processEvent(rawEvent)) {
              shouldStop = true;
              break;
            }
          }
          boundary = buffer.indexOf("\n\n");
        }
        return shouldStop;
      };

      let reachedEnd = false;

      while (!reachedEnd) {
        const { done, value } = await reader.read();
        if (done) {
          buffer += decoder.decode();
          reachedEnd = true;
          processPendingEvents();
          break;
        }

        if (value) {
          buffer += decoder.decode(value, { stream: true });
          if (processPendingEvents()) {
            reachedEnd = true;
            break;
          }
        }
      }

      if (updateThrottleRef.current !== null) {
        cancelAnimationFrame(updateThrottleRef.current);
        throttledUpdateMessage();
      }

      return {
        assistantContent,
        reasoningContent,
        status: finalStatus,
        incompleteDetails,
      };
    },
    [selectedToken, scheduleUpdate, throttledUpdateMessage, updateThrottleRef]
  );

  return { streamResponse };
};
