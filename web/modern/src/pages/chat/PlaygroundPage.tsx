import { ChatInterface } from '@/components/chat/ChatInterface';
import { ExportConversationDialog } from '@/components/chat/ExportConversationDialog';
import { ParametersPanel } from '@/components/chat/ParametersPanel';
import { codeBlockStyles } from '@/components/ui/markdown-css';
import { usePlaygroundChat } from '@/hooks/usePlaygroundChat';
import { getModelCapabilities } from '@/lib/model-capabilities';
import 'highlight.js/styles/a11y-dark.css';
import 'katex/dist/katex.min.css';
import { useEffect, useState } from 'react';
import { usePlaygroundActions } from './hooks/usePlaygroundActions';
import { usePlaygroundData } from './hooks/usePlaygroundData';
import { usePlaygroundState } from './hooks/usePlaygroundState';

// Inject styles into document head
if (typeof document !== 'undefined') {
  const styleElement = document.createElement('style');
  styleElement.textContent = codeBlockStyles;
  document.head.appendChild(styleElement);
}

export function PlaygroundPage() {
  const {
    messages,
    setMessages,
    conversationId,
    conversationCreated,
    conversationCreatedBy,
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
  } = usePlaygroundState();

  const {
    models,
    selectedModel,
    setSelectedModel,
    isLoadingModels,
    tokens,
    selectedToken,
    setSelectedToken,
    isLoadingTokens,
    isLoadingChannels,
    channelInputValue,
    setChannelInputValue,
    selectedChannel,
    setSelectedChannel,
    modelInputValue,
    setModelInputValue,
    channelSuggestions,
    modelSuggestions,
    handleModelQueryChange,
    handleModelSelect,
    handleModelClear,
    handleChannelQueryChange,
    handleChannelSelect,
    handleChannelClear,
  } = usePlaygroundData();

  const [modelCapabilities, setModelCapabilities] = useState<Record<string, any>>({});

  useEffect(() => {
    if (selectedModel) {
      const capabilities = getModelCapabilities(selectedModel);
      setModelCapabilities(capabilities);
    }
  }, [selectedModel]);

  const { isStreaming, sendMessage, regenerateMessage, stopGeneration } = usePlaygroundChat({
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
    messages,
    setMessages,
    expandedReasonings,
    setExpandedReasonings,
  });

  const {
    exportConversation,
    toggleReasoning,
    handleCurrentMessageChange,
    handleSendMessage,
    handleCopyMessage,
    handleRegenerateMessage,
    handleEditMessage,
    handleDeleteMessage,
  } = usePlaygroundActions({
    messages,
    setMessages,
    setCurrentMessage,
    sendMessage,
    regenerateMessage,
    isStreaming,
    setExpandedReasonings,
    setExportDialogOpen,
  });

  return (
    <div className="flex h-screen bg-gradient-to-br from-background to-muted/20 relative">
      {isMobileSidebarOpen && <div className="fixed inset-0 bg-black/50 z-40 lg:hidden" onClick={() => setIsMobileSidebarOpen(false)} />}

      <ParametersPanel
        isMobileSidebarOpen={isMobileSidebarOpen}
        onMobileSidebarClose={() => setIsMobileSidebarOpen(false)}
        isLoadingTokens={isLoadingTokens}
        isLoadingModels={isLoadingModels}
        isLoadingChannels={isLoadingChannels}
        tokens={tokens}
        models={models}
        selectedToken={selectedToken}
        selectedModel={selectedModel}
        selectedChannel={selectedChannel}
        channelInputValue={channelInputValue}
        channelSuggestions={channelSuggestions}
        modelInputValue={modelInputValue}
        modelSuggestions={modelSuggestions}
        onChannelQueryChange={handleChannelQueryChange}
        onChannelSelect={handleChannelSelect}
        onChannelClear={handleChannelClear}
        onTokenChange={setSelectedToken}
        onModelQueryChange={handleModelQueryChange}
        onModelSelect={handleModelSelect}
        onModelClear={handleModelClear}
        temperature={temperature}
        maxTokens={maxTokens}
        topP={topP}
        topK={topK}
        frequencyPenalty={frequencyPenalty}
        presencePenalty={presencePenalty}
        maxCompletionTokens={maxCompletionTokens}
        stopSequences={stopSequences}
        reasoningEffort={reasoningEffort}
        thinkingEnabled={thinkingEnabled}
        thinkingBudgetTokens={thinkingBudgetTokens}
        systemMessage={systemMessage}
        showReasoningContent={showReasoningContent}
        onTemperatureChange={setTemperature}
        onMaxTokensChange={setMaxTokens}
        onTopPChange={setTopP}
        onTopKChange={setTopK}
        onFrequencyPenaltyChange={setFrequencyPenalty}
        onPresencePenaltyChange={setPresencePenalty}
        onMaxCompletionTokensChange={setMaxCompletionTokens}
        onStopSequencesChange={setStopSequences}
        onReasoningEffortChange={setReasoningEffort}
        onThinkingEnabledChange={setThinkingEnabled}
        onThinkingBudgetTokensChange={setThinkingBudgetTokens}
        onSystemMessageChange={setSystemMessage}
        onShowReasoningContentChange={setShowReasoningContent}
        modelCapabilities={modelCapabilities}
      />

      <ChatInterface
        messages={messages}
        onClearConversation={clearConversation}
        onExportConversation={exportConversation}
        currentMessage={currentMessage}
        onCurrentMessageChange={handleCurrentMessageChange}
        onSendMessage={handleSendMessage}
        isStreaming={isStreaming}
        onStopGeneration={stopGeneration}
        selectedModel={selectedModel}
        selectedToken={selectedToken}
        supportsVision={modelCapabilities.supportsVision || false}
        attachedImages={attachedImages}
        onAttachedImagesChange={setAttachedImages}
        showPreview={showPreview}
        onPreviewChange={setShowPreview}
        onMobileMenuToggle={() => setIsMobileSidebarOpen(true)}
        showReasoningContent={showReasoningContent}
        expandedReasonings={expandedReasonings}
        onToggleReasoning={toggleReasoning}
        focusModeEnabled={focusModeEnabled}
        onFocusModeChange={setFocusModeEnabled}
        onCopyMessage={handleCopyMessage}
        onRegenerateMessage={handleRegenerateMessage}
        onEditMessage={handleEditMessage}
        onDeleteMessage={handleDeleteMessage}
      />

      <ExportConversationDialog
        isOpen={exportDialogOpen}
        onClose={() => setExportDialogOpen(false)}
        messages={messages}
        selectedModel={selectedModel}
        conversationId={conversationId}
        conversationCreated={conversationCreated}
        conversationCreatedBy={conversationCreatedBy}
      />
    </div>
  );
}

export default PlaygroundPage;
