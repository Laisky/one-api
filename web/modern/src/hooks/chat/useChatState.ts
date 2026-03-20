import { Message } from '@/lib/utils';
import { useCallback, useEffect, useRef, useState } from 'react';

interface UseChatStateProps {
  messages: Message[];
  setMessages: (messages: Message[] | ((prev: Message[]) => Message[])) => void;
  expandedReasonings: Record<number, boolean>;
  setExpandedReasonings: (expanded: Record<number, boolean> | ((prev: Record<number, boolean>) => Record<number, boolean>)) => void;
}

export const useChatState = ({ messages, setMessages, expandedReasonings, setExpandedReasonings }: UseChatStateProps) => {
  const [isStreaming, setIsStreaming] = useState(false);
  const abortControllerRef = useRef<AbortController | null>(null);
  const updateThrottleRef = useRef<number | null>(null);
  const pendingUpdateRef = useRef<{ content: string; reasoning_content: string } | null>(null);

  // Throttled update function to reduce rendering frequency during streaming
  const throttledUpdateMessage = useCallback(() => {
    if (pendingUpdateRef.current) {
      const { content, reasoning_content } = pendingUpdateRef.current;
      setMessages((prev) => {
        const updated = [...prev];
        if (updated.length > 0) {
          updated[updated.length - 1] = {
            ...updated[updated.length - 1],
            content,
            reasoning_content: reasoning_content.trim() || null, // Convert empty reasoning to null
          };
        }
        return updated;
      });
      pendingUpdateRef.current = null;
    }
    updateThrottleRef.current = null;
  }, [setMessages]);

  // Schedule a throttled update using requestAnimationFrame
  const scheduleUpdate = useCallback(
    (content: string, reasoning_content: string) => {
      pendingUpdateRef.current = { content, reasoning_content };

      // Auto-collapse thinking bubble when main content starts appearing
      if (content.trim().length > 0 && reasoning_content.trim().length > 0) {
        const lastMessageIndex = messages.length - 1;
        if (lastMessageIndex >= 0 && expandedReasonings[lastMessageIndex] !== false) {
          setExpandedReasonings((prev) => ({
            ...prev,
            [lastMessageIndex]: false,
          }));
        }
      }

      if (updateThrottleRef.current === null) {
        updateThrottleRef.current = requestAnimationFrame(throttledUpdateMessage);
      }
    },
    [throttledUpdateMessage, messages.length, expandedReasonings, setExpandedReasonings]
  );

  // Cleanup animation frames on unmount
  useEffect(() => {
    return () => {
      if (updateThrottleRef.current !== null) {
        cancelAnimationFrame(updateThrottleRef.current);
      }
    };
  }, []);

  // Helper function to add error message to chat
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

  return {
    isStreaming,
    setIsStreaming,
    abortControllerRef,
    updateThrottleRef,
    throttledUpdateMessage,
    scheduleUpdate,
    addErrorMessage,
  };
};
