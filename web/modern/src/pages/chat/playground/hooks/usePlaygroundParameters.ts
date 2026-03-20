import { useCallback, useEffect, useMemo, useRef, useState } from 'react';

import { getModelCapabilities, isOpenAIMediumOnlyReasoningModel } from '@/lib/model-capabilities';
import { STORAGE_KEYS } from '@/lib/storage';
import { loadFromStorage, saveToStorage } from '@/lib/utils';

const buildDefaultParameters = (systemPrompt: string) => ({
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
  systemMessage: systemPrompt,
  showReasoningContent: true,
  focusModeEnabled: true,
});

export interface PlaygroundParametersState {
  temperature: number[];
  maxTokens: number[];
  topP: number[];
  topK: number[];
  frequencyPenalty: number[];
  presencePenalty: number[];
  maxCompletionTokens: number[];
  stopSequences: string;
  reasoningEffort: string;
  thinkingEnabled: boolean;
  thinkingBudgetTokens: number[];
  systemMessage: string;
  showReasoningContent: boolean;
  focusModeEnabled: boolean;
}

interface UsePlaygroundParametersArgs {
  defaultSystemPrompt: string;
  selectedModel: string;
}

export interface UsePlaygroundParametersResult extends PlaygroundParametersState {
  modelCapabilities: Record<string, any>;
  handleReasoningEffortChange: (value: string) => void;
  setTemperature: React.Dispatch<React.SetStateAction<number[]>>;
  setMaxTokens: React.Dispatch<React.SetStateAction<number[]>>;
  setTopP: React.Dispatch<React.SetStateAction<number[]>>;
  setTopK: React.Dispatch<React.SetStateAction<number[]>>;
  setFrequencyPenalty: React.Dispatch<React.SetStateAction<number[]>>;
  setPresencePenalty: React.Dispatch<React.SetStateAction<number[]>>;
  setMaxCompletionTokens: React.Dispatch<React.SetStateAction<number[]>>;
  setStopSequences: React.Dispatch<React.SetStateAction<string>>;
  setReasoningEffort: React.Dispatch<React.SetStateAction<string>>;
  setThinkingEnabled: React.Dispatch<React.SetStateAction<boolean>>;
  setThinkingBudgetTokens: React.Dispatch<React.SetStateAction<number[]>>;
  setSystemMessage: React.Dispatch<React.SetStateAction<string>>;
  setShowReasoningContent: React.Dispatch<React.SetStateAction<boolean>>;
  setFocusModeEnabled: React.Dispatch<React.SetStateAction<boolean>>;
}

export const usePlaygroundParameters = (args: UsePlaygroundParametersArgs): UsePlaygroundParametersResult => {
  const { defaultSystemPrompt, selectedModel } = args;
  const initialParamsRef = useRef<PlaygroundParametersState | null>(null);
  if (!initialParamsRef.current) {
    const stored = loadFromStorage(STORAGE_KEYS.PARAMETERS, buildDefaultParameters(defaultSystemPrompt));
    initialParamsRef.current = stored;
  }
  const initialParams = initialParamsRef.current || buildDefaultParameters(defaultSystemPrompt);

  const [temperature, setTemperature] = useState(initialParams.temperature);
  const [maxTokens, setMaxTokens] = useState(initialParams.maxTokens);
  const [topP, setTopP] = useState(initialParams.topP);
  const [topK, setTopK] = useState(initialParams.topK);
  const [frequencyPenalty, setFrequencyPenalty] = useState(initialParams.frequencyPenalty);
  const [presencePenalty, setPresencePenalty] = useState(initialParams.presencePenalty);
  const [maxCompletionTokens, setMaxCompletionTokens] = useState(initialParams.maxCompletionTokens);
  const [stopSequences, setStopSequences] = useState(initialParams.stopSequences);
  const [reasoningEffort, setReasoningEffort] = useState(initialParams.reasoningEffort);
  const [thinkingEnabled, setThinkingEnabled] = useState(initialParams.thinkingEnabled);
  const [thinkingBudgetTokens, setThinkingBudgetTokens] = useState(initialParams.thinkingBudgetTokens);
  const [systemMessage, setSystemMessage] = useState(initialParams.systemMessage);
  const [showReasoningContent, setShowReasoningContent] = useState(initialParams.showReasoningContent);
  const [focusModeEnabled, setFocusModeEnabled] = useState(initialParams.focusModeEnabled);
  const [modelCapabilities, setModelCapabilities] = useState<Record<string, any>>({});

  const mediumOnlyReasoning = useMemo(() => {
    if (!selectedModel) {
      return false;
    }
    return isOpenAIMediumOnlyReasoningModel(selectedModel);
  }, [selectedModel]);

  const handleReasoningEffortChange = useCallback(
    (value: string) => {
      if (value === 'none') {
        setReasoningEffort(value);
        return;
      }

      if (mediumOnlyReasoning) {
        setReasoningEffort('medium');
        return;
      }

      setReasoningEffort(value);
    },
    [mediumOnlyReasoning]
  );

  // Aligns with the previous behavior where capability normalization only ran on model changes.
  // eslint-disable-next-line react-hooks/exhaustive-deps
  useEffect(() => {
    if (!selectedModel) {
      setModelCapabilities({});
      return;
    }

    const capabilities = getModelCapabilities(selectedModel);
    setModelCapabilities(capabilities);

    const defaults = buildDefaultParameters(defaultSystemPrompt);

    if (!capabilities.supportsTopK) {
      setTopK(defaults.topK);
    }
    if (!capabilities.supportsFrequencyPenalty) {
      setFrequencyPenalty(defaults.frequencyPenalty);
    }
    if (!capabilities.supportsPresencePenalty) {
      setPresencePenalty(defaults.presencePenalty);
    }
    if (!capabilities.supportsMaxCompletionTokens) {
      setMaxCompletionTokens(defaults.maxCompletionTokens);
    }
    if (!capabilities.supportsStop) {
      setStopSequences(defaults.stopSequences);
    }
    if (!capabilities.supportsReasoningEffort) {
      setReasoningEffort(defaults.reasoningEffort);
    } else if (mediumOnlyReasoning && reasoningEffort !== 'medium' && reasoningEffort !== 'none') {
      setReasoningEffort('medium');
    }
    if (!capabilities.supportsThinking) {
      setThinkingEnabled(defaults.thinkingEnabled);
      setThinkingBudgetTokens(defaults.thinkingBudgetTokens);
    }

    const persisted = {
      temperature,
      maxTokens,
      topP,
      topK: capabilities.supportsTopK ? topK : defaults.topK,
      frequencyPenalty: capabilities.supportsFrequencyPenalty ? frequencyPenalty : defaults.frequencyPenalty,
      presencePenalty: capabilities.supportsPresencePenalty ? presencePenalty : defaults.presencePenalty,
      maxCompletionTokens: capabilities.supportsMaxCompletionTokens ? maxCompletionTokens : defaults.maxCompletionTokens,
      stopSequences: capabilities.supportsStop ? stopSequences : defaults.stopSequences,
      reasoningEffort: capabilities.supportsReasoningEffort
        ? mediumOnlyReasoning && reasoningEffort !== 'none'
          ? 'medium'
          : reasoningEffort
        : defaults.reasoningEffort,
      thinkingEnabled: capabilities.supportsThinking ? thinkingEnabled : defaults.thinkingEnabled,
      thinkingBudgetTokens: capabilities.supportsThinking ? thinkingBudgetTokens : defaults.thinkingBudgetTokens,
      systemMessage,
      showReasoningContent,
      focusModeEnabled,
    };

    saveToStorage(STORAGE_KEYS.PARAMETERS, persisted);
  }, [
    selectedModel,
    defaultSystemPrompt,
    focusModeEnabled,
    frequencyPenalty,
    maxCompletionTokens,
    maxTokens,
    mediumOnlyReasoning,
    presencePenalty,
    reasoningEffort,
    showReasoningContent,
    stopSequences,
    systemMessage,
    temperature,
    thinkingBudgetTokens,
    thinkingEnabled,
    topK,
    topP,
  ]);

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

  return {
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
    modelCapabilities,
    handleReasoningEffortChange,
    setTemperature,
    setMaxTokens,
    setTopP,
    setTopK,
    setFrequencyPenalty,
    setPresencePenalty,
    setMaxCompletionTokens,
    setStopSequences,
    setReasoningEffort,
    setThinkingEnabled,
    setThinkingBudgetTokens,
    setSystemMessage,
    setShowReasoningContent,
    setFocusModeEnabled,
  };
};
