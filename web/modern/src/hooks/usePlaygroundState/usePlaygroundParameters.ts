import { getModelCapabilities } from "@/lib/model-capabilities";
import { STORAGE_KEYS } from "@/lib/storage";
import { loadFromStorage, saveToStorage } from "@/lib/utils";
import { useCallback, useEffect, useState } from "react";

export function usePlaygroundParameters(selectedModel: string) {
  const [temperature, setTemperature] = useState<number[]>([0.7]);
  const [maxTokens, setMaxTokens] = useState<number[]>([4096]);
  const [topP, setTopP] = useState<number[]>([1.0]);
  const [topK, setTopK] = useState<number[]>([40]);
  const [frequencyPenalty, setFrequencyPenalty] = useState<number[]>([0.0]);
  const [presencePenalty, setPresencePenalty] = useState<number[]>([0.0]);
  const [maxCompletionTokens, setMaxCompletionTokens] = useState<number[]>([
    4096,
  ]);
  const [stopSequences, setStopSequences] = useState<string>("");
  const [reasoningEffort, setReasoningEffort] = useState<string>("high");
  const [thinkingEnabled, setThinkingEnabled] = useState<boolean>(false);
  const [thinkingBudgetTokens, setThinkingBudgetTokens] = useState<number[]>([
    10000,
  ]);
  const [systemMessage, setSystemMessage] = useState<string>("");
  const [showReasoningContent, setShowReasoningContent] =
    useState<boolean>(true);
  const [focusModeEnabled, setFocusModeEnabled] = useState<boolean>(false);

  const modelCapabilities = getModelCapabilities(selectedModel);

  // Load parameters from storage
  useEffect(() => {
    const savedParams = loadFromStorage(STORAGE_KEYS.PARAMETERS, {});
    if (savedParams) {
      if (savedParams.temperature) setTemperature(savedParams.temperature);
      if (savedParams.maxTokens) setMaxTokens(savedParams.maxTokens);
      if (savedParams.topP) setTopP(savedParams.topP);
      if (savedParams.topK) setTopK(savedParams.topK);
      if (savedParams.frequencyPenalty)
        setFrequencyPenalty(savedParams.frequencyPenalty);
      if (savedParams.presencePenalty)
        setPresencePenalty(savedParams.presencePenalty);
      if (savedParams.maxCompletionTokens)
        setMaxCompletionTokens(savedParams.maxCompletionTokens);
      if (savedParams.stopSequences)
        setStopSequences(savedParams.stopSequences);
      if (savedParams.reasoningEffort)
        setReasoningEffort(savedParams.reasoningEffort);
      if (savedParams.thinkingEnabled !== undefined)
        setThinkingEnabled(savedParams.thinkingEnabled);
      if (savedParams.thinkingBudgetTokens)
        setThinkingBudgetTokens(savedParams.thinkingBudgetTokens);
      if (savedParams.systemMessage)
        setSystemMessage(savedParams.systemMessage);
      if (savedParams.showReasoningContent !== undefined)
        setShowReasoningContent(savedParams.showReasoningContent);
      if (savedParams.focusModeEnabled !== undefined)
        setFocusModeEnabled(savedParams.focusModeEnabled);
    }
  }, []);

  // Validate parameters against model capabilities
  useEffect(() => {
    if (!selectedModel) return;

    let hasChanges = false;
    const updates: Record<string, unknown> = {};

    if (!modelCapabilities.supportsTopK && topK[0] !== 40) {
      setTopK([40]);
      updates.topK = [40];
      hasChanges = true;
    }

    if (!modelCapabilities.supportsThinking && thinkingEnabled) {
      setThinkingEnabled(false);
      updates.thinkingEnabled = false;
      hasChanges = true;
    }

    if (
      !modelCapabilities.supportsReasoningEffort &&
      reasoningEffort !== "high"
    ) {
      setReasoningEffort("high");
      updates.reasoningEffort = "high";
      hasChanges = true;
    }

    if (
      !modelCapabilities.supportsMaxCompletionTokens &&
      maxCompletionTokens[0] !== 4096
    ) {
      setMaxCompletionTokens([4096]);
      updates.maxCompletionTokens = [4096];
      hasChanges = true;
    }

    if (hasChanges) {
      const currentParams = loadFromStorage(STORAGE_KEYS.PARAMETERS, {});
      saveToStorage(STORAGE_KEYS.PARAMETERS, { ...currentParams, ...updates });
    }
  }, [
    selectedModel,
    modelCapabilities,
    topK,
    thinkingEnabled,
    reasoningEffort,
    maxCompletionTokens,
  ]);

  // Save parameters to storage
  useEffect(() => {
    saveToStorage(STORAGE_KEYS.PARAMETERS, {
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
    });
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

  const handleReasoningEffortChange = useCallback((value: string) => {
    setReasoningEffort(value);
  }, []);

  return {
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
    handleReasoningEffortChange,
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
    modelCapabilities,
  };
}
