import { ImageAttachment as ImageAttachmentType } from '@/components/chat/ImageAttachment';
import { getModelCapabilities, isOpenAIMediumOnlyReasoningModel } from '@/lib/model-capabilities';
import { STORAGE_KEYS } from '@/lib/storage';
import { useAuthStore } from '@/lib/stores/auth';
import { clearStorage, generateUUIDv4, loadFromStorage, Message, saveToStorage } from '@/lib/utils';
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';

export const usePlaygroundState = () => {
  const { t } = useTranslation();
  const defaultSystemPrompt = t('playground.system_prompt');
  const { user } = useAuthStore();

  // Conversation state
  const [messages, setMessages] = useState<Message[]>([]);
  const [conversationId, setConversationId] = useState<string>('');
  const [conversationCreated, setConversationCreated] = useState<number>(0);
  const [conversationCreatedBy, setConversationCreatedBy] = useState<string>('');
  const [currentMessage, setCurrentMessage] = useState('');

  // Model parameters
  const [temperature, setTemperature] = useState([0.7]);
  const [maxTokens, setMaxTokens] = useState([4096]);
  const [topP, setTopP] = useState([1.0]);
  const [topK, setTopK] = useState([40]);
  const [frequencyPenalty, setFrequencyPenalty] = useState([0.0]);
  const [presencePenalty, setPresencePenalty] = useState([0.0]);
  const [maxCompletionTokens, setMaxCompletionTokens] = useState([4096]);
  const [stopSequences, setStopSequences] = useState('');
  const [reasoningEffort, setReasoningEffort] = useState('high');
  const [thinkingEnabled, setThinkingEnabled] = useState(false);
  const [thinkingBudgetTokens, setThinkingBudgetTokens] = useState([10000]);
  const [systemMessage, setSystemMessage] = useState('');

  // Configuration settings
  const [showReasoningContent, setShowReasoningContent] = useState(true);
  const [focusModeEnabled, setFocusModeEnabled] = useState(false);

  // UI State
  const [isMobileSidebarOpen, setIsMobileSidebarOpen] = useState(false);
  const [expandedReasonings, setExpandedReasonings] = useState<Record<number, boolean>>({});
  const [showPreview, setShowPreview] = useState(false);
  const [exportDialogOpen, setExportDialogOpen] = useState(false);
  const [attachedImages, setAttachedImages] = useState<ImageAttachmentType[]>([]);

  // Load saved data from localStorage on mount
  useEffect(() => {
    // Load conversation from storage
    const savedConversation = loadFromStorage(STORAGE_KEYS.CONVERSATION, null);
    let savedMessages = [];
    let savedConversationId = '';
    let savedConversationCreated = 0;
    let savedConversationCreatedBy = '';

    if (savedConversation && savedConversation.id && savedConversation.messages) {
      savedMessages = savedConversation.messages;
      savedConversationId = savedConversation.id;
      savedConversationCreated = savedConversation.timestamp || Date.now();
      savedConversationCreatedBy = savedConversation.createdBy || user?.username || 'unknown';
    } else {
      savedMessages = [];
      savedConversationId = generateUUIDv4();
      savedConversationCreated = Date.now();
      savedConversationCreatedBy = user?.username || 'unknown';
    }

    const savedModel = loadFromStorage(STORAGE_KEYS.MODEL, '');
    const savedParams = loadFromStorage(STORAGE_KEYS.PARAMETERS, {
      temperature: [0.7],
      maxTokens: [4096],
      topP: [1.0],
      topK: [40],
      frequencyPenalty: [0.0],
      presencePenalty: [0.0],
      maxCompletionTokens: [4096],
      stopSequences: '',
      reasoningEffort: 'high',
      thinkingEnabled: false,
      thinkingBudgetTokens: [10000],
      systemMessage: defaultSystemPrompt,
      showReasoningContent: true,
      focusModeEnabled: true,
    });

    // Validate saved parameters against model capabilities if model is saved
    let validatedParams = savedParams;
    if (savedModel) {
      const capabilities = getModelCapabilities(savedModel);
      const mediumOnly = isOpenAIMediumOnlyReasoningModel(savedModel);

      const defaults = {
        topK: [40],
        frequencyPenalty: [0.0],
        presencePenalty: [0.0],
        maxCompletionTokens: [4096],
        stopSequences: '',
        reasoningEffort: 'high',
        thinkingEnabled: false,
        thinkingBudgetTokens: [10000],
      };

      validatedParams = {
        ...savedParams,
        topK: capabilities.supportsTopK ? savedParams.topK : defaults.topK,
        frequencyPenalty: capabilities.supportsFrequencyPenalty ? savedParams.frequencyPenalty : defaults.frequencyPenalty,
        presencePenalty: capabilities.supportsPresencePenalty ? savedParams.presencePenalty : defaults.presencePenalty,
        maxCompletionTokens: capabilities.supportsMaxCompletionTokens ? savedParams.maxCompletionTokens : defaults.maxCompletionTokens,
        stopSequences: capabilities.supportsStop ? savedParams.stopSequences : defaults.stopSequences,
        reasoningEffort: capabilities.supportsReasoningEffort
          ? mediumOnly
            ? savedParams.reasoningEffort === 'none'
              ? 'none'
              : 'medium'
            : savedParams.reasoningEffort
          : defaults.reasoningEffort,
        thinkingEnabled: capabilities.supportsThinking ? savedParams.thinkingEnabled : defaults.thinkingEnabled,
        thinkingBudgetTokens: capabilities.supportsThinking ? savedParams.thinkingBudgetTokens : defaults.thinkingBudgetTokens,
      };

      if (JSON.stringify(validatedParams) !== JSON.stringify(savedParams)) {
        saveToStorage(STORAGE_KEYS.PARAMETERS, validatedParams);
      }
    }

    setMessages(savedMessages);
    setConversationId(savedConversationId);
    setConversationCreated(savedConversationCreated);
    setConversationCreatedBy(savedConversationCreatedBy);
    setTemperature(validatedParams.temperature);
    setMaxTokens(validatedParams.maxTokens);
    setTopP(validatedParams.topP);
    setTopK(validatedParams.topK);
    setFrequencyPenalty(validatedParams.frequencyPenalty);
    setPresencePenalty(validatedParams.presencePenalty);
    setMaxCompletionTokens(validatedParams.maxCompletionTokens);
    setStopSequences(validatedParams.stopSequences);
    setReasoningEffort(validatedParams.reasoningEffort);
    setThinkingEnabled(validatedParams.thinkingEnabled);
    setThinkingBudgetTokens(validatedParams.thinkingBudgetTokens);
    setSystemMessage(validatedParams.systemMessage);
    setShowReasoningContent(validatedParams.showReasoningContent);
    setFocusModeEnabled(validatedParams.focusModeEnabled);
  }, []);

  // Save data to localStorage when it changes
  useEffect(() => {
    if (messages.length > 0 && conversationId) {
      const conversation = {
        id: conversationId,
        timestamp: conversationCreated,
        createdBy: conversationCreatedBy,
        messages: messages,
      };
      saveToStorage(STORAGE_KEYS.CONVERSATION, conversation);
    }
  }, [messages, conversationId, conversationCreated, conversationCreatedBy]);

  useEffect(() => {
    const params = {
      temperature,
      maxTokens,
      topP,
      topK,
      frequencyPenalty,
      presencePenalty,
      maxCompletionTokens,
      stopSequences,
      reasoningEffort,
      thinkingEnabled,
      thinkingBudgetTokens,
      systemMessage,
      showReasoningContent,
      focusModeEnabled,
    };
    saveToStorage(STORAGE_KEYS.PARAMETERS, params);
  }, [
    temperature,
    maxTokens,
    topP,
    topK,
    frequencyPenalty,
    presencePenalty,
    maxCompletionTokens,
    stopSequences,
    reasoningEffort,
    thinkingEnabled,
    thinkingBudgetTokens,
    systemMessage,
    showReasoningContent,
    focusModeEnabled,
  ]);

  const clearConversation = useCallback(() => {
    setMessages([]);
    setConversationId(generateUUIDv4());
    setConversationCreated(Date.now());
    setConversationCreatedBy(user?.username || 'unknown');
    clearStorage(STORAGE_KEYS.CONVERSATION);
  }, [user]);

  return {
    messages,
    setMessages,
    conversationId,
    setConversationId,
    conversationCreated,
    setConversationCreated,
    conversationCreatedBy,
    setConversationCreatedBy,
    currentMessage,
    setCurrentMessage,
    temperature,
    setTemperature,
    maxTokens,
    setMaxTokens,
    topP,
    setTopP,
    topK,
    setTopK,
    frequencyPenalty,
    setFrequencyPenalty,
    presencePenalty,
    setPresencePenalty,
    maxCompletionTokens,
    setMaxCompletionTokens,
    stopSequences,
    setStopSequences,
    reasoningEffort,
    setReasoningEffort,
    thinkingEnabled,
    setThinkingEnabled,
    thinkingBudgetTokens,
    setThinkingBudgetTokens,
    systemMessage,
    setSystemMessage,
    showReasoningContent,
    setShowReasoningContent,
    focusModeEnabled,
    setFocusModeEnabled,
    isMobileSidebarOpen,
    setIsMobileSidebarOpen,
    expandedReasonings,
    setExpandedReasonings,
    showPreview,
    setShowPreview,
    exportDialogOpen,
    setExportDialogOpen,
    attachedImages,
    setAttachedImages,
    clearConversation,
    defaultSystemPrompt,
  };
};
