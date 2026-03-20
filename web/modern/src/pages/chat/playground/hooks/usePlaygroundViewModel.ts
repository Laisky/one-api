import { useCallback, useState } from 'react';
import { useTranslation } from 'react-i18next';

import type { ImageAttachment as ImageAttachmentType } from '@/components/chat/ImageAttachment';
import { useNotifications } from '@/components/ui/notifications';
import { usePlaygroundChat } from '@/hooks/usePlaygroundChat';
import { useAuthStore } from '@/lib/stores/auth';
import type { Message } from '@/lib/utils';

import { useConversationPersistence } from './useConversationPersistence';
import { type UseModelAndTokenBrowserResult, useModelAndTokenBrowser } from './useModelAndTokenBrowser';
import { type UsePlaygroundParametersResult, usePlaygroundParameters } from './usePlaygroundParameters';

interface UsePlaygroundViewModelResult {
  isMobileSidebarOpen: boolean;
  setIsMobileSidebarOpen: React.Dispatch<React.SetStateAction<boolean>>;
  tokens: UseModelAndTokenBrowserResult['tokens'];
  isLoadingTokens: boolean;
  models: UseModelAndTokenBrowserResult['models'];
  isLoadingModels: boolean;
  isLoadingChannels: boolean;
  selectedToken: string;
  setSelectedToken: React.Dispatch<React.SetStateAction<string>>;
  selectedModel: string;
  setSelectedModel: React.Dispatch<React.SetStateAction<string>>;
  selectedChannel: string;
  channelInputValue: string;
  modelInputValue: string;
  channelSuggestions: UseModelAndTokenBrowserResult['channelSuggestions'];
  modelSuggestions: UseModelAndTokenBrowserResult['modelSuggestions'];
  handleChannelQueryChange: (value: string) => void;
  handleChannelSelect: (key: string) => void;
  handleChannelClear: () => void;
  handleModelQueryChange: (value: string) => void;
  handleModelSelect: (key: string) => void;
  handleModelClear: () => void;
  parameters: UsePlaygroundParametersResult;
  messages: Message[];
  clearConversation: () => void;
  exportDialogOpen: boolean;
  setExportDialogOpen: React.Dispatch<React.SetStateAction<boolean>>;
  conversationId: string;
  conversationCreated: number;
  conversationCreatedBy: string;
  currentMessage: string;
  setCurrentMessage: React.Dispatch<React.SetStateAction<string>>;
  isStreaming: boolean;
  handleSendMessage: (message: string, images?: ImageAttachmentType[]) => Promise<void>;
  stopGeneration: () => void;
  showReasoningContent: boolean;
  expandedReasonings: Record<number, boolean>;
  toggleReasoning: (index: number) => void;
  focusModeEnabled: boolean;
  setFocusModeEnabled: React.Dispatch<React.SetStateAction<boolean>>;
  showPreview: boolean;
  setShowPreview: React.Dispatch<React.SetStateAction<boolean>>;
  attachedImages: ImageAttachmentType[];
  setAttachedImages: React.Dispatch<React.SetStateAction<ImageAttachmentType[]>>;
  handleCopyMessage: (index: number, content: string) => Promise<void>;
  handleRegenerateMessage: (index: number) => Promise<void>;
  handleEditMessage: (index: number, newContent: string | any[]) => void;
  handleDeleteMessage: (index: number) => void;
}

export const usePlaygroundViewModel = (): UsePlaygroundViewModelResult => {
  const { t } = useTranslation();
  const defaultSystemPrompt = t('playground.system_prompt');
  const { notify } = useNotifications();
  const { user } = useAuthStore();

  const conversation = useConversationPersistence({ username: user?.username });

  const modelBrowser = useModelAndTokenBrowser({ t, notify });

  const parameters = usePlaygroundParameters({
    defaultSystemPrompt,
    selectedModel: modelBrowser.selectedModel,
  });

  const [isMobileSidebarOpen, setIsMobileSidebarOpen] = useState(false);
  const [expandedReasonings, setExpandedReasonings] = useState<Record<number, boolean>>({});
  const [currentMessage, setCurrentMessage] = useState('');
  const [showPreview, setShowPreview] = useState(false);
  const [exportDialogOpen, setExportDialogOpen] = useState(false);
  const [attachedImages, setAttachedImages] = useState<ImageAttachmentType[]>([]);

  const { isStreaming, sendMessage, regenerateMessage, stopGeneration } = usePlaygroundChat({
    selectedToken: modelBrowser.selectedToken,
    selectedModel: modelBrowser.selectedModel,
    temperature: parameters.temperature,
    maxTokens: parameters.maxTokens,
    maxCompletionTokens: parameters.maxCompletionTokens,
    topP: parameters.topP,
    topK: parameters.topK,
    frequencyPenalty: parameters.frequencyPenalty,
    presencePenalty: parameters.presencePenalty,
    stopSequences: parameters.stopSequences,
    reasoningEffort: parameters.reasoningEffort,
    thinkingEnabled: parameters.thinkingEnabled,
    thinkingBudgetTokens: parameters.thinkingBudgetTokens,
    systemMessage: parameters.systemMessage,
    messages: conversation.messages,
    setMessages: conversation.setMessages,
    expandedReasonings,
    setExpandedReasonings,
  });

  const toggleReasoning = useCallback((messageIndex: number) => {
    setExpandedReasonings((prev) => ({
      ...prev,
      [messageIndex]: !prev[messageIndex],
    }));
  }, []);

  const handleSendMessage = useCallback(
    async (message: string, images?: ImageAttachmentType[]) => {
      if (message.trim() || (images && images.length > 0)) {
        setCurrentMessage('');
        await sendMessage(message, images);
      }
    },
    [sendMessage]
  );

  const handleCopyMessage = useCallback(
    async (_index: number, content: string) => {
      try {
        await navigator.clipboard.writeText(content);
        notify({
          title: t('playground.notifications.copied_title'),
          message: t('playground.notifications.copied_message'),
          type: 'success',
        });
      } catch {
        notify({
          title: t('playground.notifications.copy_failed_title'),
          message: t('playground.notifications.copy_failed_message'),
          type: 'error',
        });
      }
    },
    [notify, t]
  );

  const handleRegenerateMessage = useCallback(
    async (messageIndex: number) => {
      if (messageIndex < 1 || isStreaming) {
        return;
      }

      const targetMessage = conversation.messages[messageIndex];
      if (!targetMessage || targetMessage.role !== 'assistant') {
        return;
      }

      let userMessageIndex = -1;
      for (let i = messageIndex - 1; i >= 0; i -= 1) {
        if (conversation.messages[i]?.role === 'user') {
          userMessageIndex = i;
          break;
        }
      }

      if (userMessageIndex === -1) {
        return;
      }

      const newMessages = conversation.messages.slice(0, userMessageIndex + 1);
      conversation.setMessages(newMessages);
      await regenerateMessage(newMessages);
    },
    [conversation, isStreaming, regenerateMessage]
  );

  const handleEditMessage = useCallback(
    (messageIndex: number, newContent: string | any[]) => {
      conversation.setMessages((prev) => {
        const next = [...prev];
        if (!next[messageIndex]) {
          return prev;
        }
        next[messageIndex] = {
          ...next[messageIndex],
          content: newContent,
          timestamp: Date.now(),
        };
        return next;
      });

      notify({
        title: t('playground.notifications.message_edited_title'),
        message: t('playground.notifications.message_edited_message'),
        type: 'success',
      });
    },
    [conversation, notify, t]
  );

  const handleDeleteMessage = useCallback(
    (messageIndex: number) => {
      conversation.setMessages((prev) => prev.filter((_, index) => index !== messageIndex));

      notify({
        title: t('playground.notifications.message_deleted_title'),
        message: t('playground.notifications.message_deleted_message'),
        type: 'success',
      });
    },
    [conversation, notify, t]
  );

  return {
    isMobileSidebarOpen,
    setIsMobileSidebarOpen,
    tokens: modelBrowser.tokens,
    isLoadingTokens: modelBrowser.isLoadingTokens,
    models: modelBrowser.models,
    isLoadingModels: modelBrowser.isLoadingModels,
    isLoadingChannels: modelBrowser.isLoadingChannels,
    selectedToken: modelBrowser.selectedToken,
    setSelectedToken: modelBrowser.setSelectedToken,
    selectedModel: modelBrowser.selectedModel,
    setSelectedModel: modelBrowser.setSelectedModel,
    selectedChannel: modelBrowser.selectedChannel,
    channelInputValue: modelBrowser.channelInputValue,
    modelInputValue: modelBrowser.modelInputValue,
    channelSuggestions: modelBrowser.channelSuggestions,
    modelSuggestions: modelBrowser.modelSuggestions,
    handleChannelQueryChange: modelBrowser.handleChannelQueryChange,
    handleChannelSelect: modelBrowser.handleChannelSelect,
    handleChannelClear: modelBrowser.handleChannelClear,
    handleModelQueryChange: modelBrowser.handleModelQueryChange,
    handleModelSelect: modelBrowser.handleModelSelect,
    handleModelClear: modelBrowser.handleModelClear,
    parameters,
    messages: conversation.messages,
    clearConversation: conversation.clearConversation,
    exportDialogOpen,
    setExportDialogOpen,
    conversationId: conversation.conversationId,
    conversationCreated: conversation.conversationCreated,
    conversationCreatedBy: conversation.conversationCreatedBy,
    currentMessage,
    setCurrentMessage,
    isStreaming,
    handleSendMessage,
    stopGeneration,
    showReasoningContent: parameters.showReasoningContent,
    expandedReasonings,
    toggleReasoning,
    focusModeEnabled: parameters.focusModeEnabled,
    setFocusModeEnabled: parameters.setFocusModeEnabled,
    showPreview,
    setShowPreview,
    attachedImages,
    setAttachedImages,
    handleCopyMessage,
    handleRegenerateMessage,
    handleEditMessage,
    handleDeleteMessage,
  };
};
