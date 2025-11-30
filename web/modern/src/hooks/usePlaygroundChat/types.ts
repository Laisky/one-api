import type { ImageAttachment as ImageAttachmentType } from "@/components/chat/ImageAttachment";
import type { Message } from "@/lib/utils";

export interface UsePlaygroundChatProps {
  selectedToken: string;
  selectedModel: string;
  temperature: number[];
  maxTokens: number[];
  maxCompletionTokens: number[];
  topP: number[];
  topK: number[];
  frequencyPenalty: number[];
  presencePenalty: number[];
  stopSequences: string;
  reasoningEffort: string;
  thinkingEnabled: boolean;
  thinkingBudgetTokens: number[];
  systemMessage: string;
  messages: Message[];
  setMessages: (messages: Message[] | ((prev: Message[]) => Message[])) => void;
  expandedReasonings: Record<number, boolean>;
  setExpandedReasonings: (
    expanded:
      | Record<number, boolean>
      | ((prev: Record<number, boolean>) => Record<number, boolean>)
  ) => void;
}

export interface UsePlaygroundChatReturn {
  isStreaming: boolean;
  sendMessage: (
    messageContent: string,
    images?: ImageAttachmentType[]
  ) => Promise<void>;
  regenerateMessage: (messages: Message[]) => Promise<void>;
  stopGeneration: () => void;
  addErrorMessage: (errorText: string) => void;
}

export interface ChatRequestConfig {
  selectedToken: string;
  selectedModel: string;
  temperature: number[];
  maxTokens: number[];
  maxCompletionTokens: number[];
  topP: number[];
  topK: number[];
  frequencyPenalty: number[];
  presencePenalty: number[];
  stopSequences: string;
  reasoningEffort: string;
  thinkingEnabled: boolean;
  thinkingBudgetTokens: number[];
  systemMessage: string;
}
