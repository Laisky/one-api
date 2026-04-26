import { ChatInterface } from '@/components/chat/ChatInterface';
import { EventLogPanel } from '@/components/chat/EventLogPanel';
import { ExportConversationDialog } from '@/components/chat/ExportConversationDialog';
import { ParametersPanel } from '@/components/chat/ParametersPanel';
import { codeBlockStyles } from '@/components/ui/markdown-css';
import { useNotifications } from '@/components/ui/notifications';
import { useEventLog } from '@/hooks/useEventLog';
import { usePlaygroundChat } from '@/hooks/usePlaygroundChat';
import { useRealtimeChat } from '@/hooks/useRealtimeChat';
import { useResponsive } from '@/hooks/useResponsive';
import { getModelCapabilities } from '@/lib/model-capabilities';
import 'highlight.js/styles/a11y-dark.css';
import 'katex/dist/katex.min.css';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { usePlaygroundActions } from './hooks/usePlaygroundActions';
import { usePlaygroundData } from './hooks/usePlaygroundData';
import { usePlaygroundState } from './hooks/usePlaygroundState';

if (typeof document !== 'undefined') {
  const styleElement = document.createElement('style');
  styleElement.textContent = codeBlockStyles;
  document.head.appendChild(styleElement);
}

export function PlaygroundPage() {
  const { t } = useTranslation();
  const { isMobile } = useResponsive();
  const { notify } = useNotifications();
  const eventLog = useEventLog();

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
  const [showEventLog, setShowEventLog] = useState(false);

  useEffect(() => {
    if (selectedModel) {
      const capabilities = getModelCapabilities(selectedModel);
      setModelCapabilities(capabilities);
    }
  }, [selectedModel]);

  const isRealtime = (modelCapabilities.isRealtime as boolean | undefined) ?? false;

  const sseChat = usePlaygroundChat({
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
    addEvent: eventLog.addEvent,
  });

  const realtimeChat = useRealtimeChat({
    selectedToken,
    selectedModel,
    systemMessage,
    temperature: Array.isArray(temperature) ? temperature[0] : temperature,
    maxCompletionTokens: Array.isArray(maxCompletionTokens) ? maxCompletionTokens[0] : maxCompletionTokens,
    messages,
    setMessages,
    addEvent: eventLog.addEvent,
  });

  const isStreaming = isRealtime ? realtimeChat.isStreaming : sseChat.isStreaming;
  const sendMessage = isRealtime ? realtimeChat.sendMessage : sseChat.sendMessage;
  const stopGeneration = isRealtime ? realtimeChat.stopGeneration : sseChat.stopGeneration;
  const regenerateMessage = isRealtime ? (_msgs: any) => realtimeChat.regenerateMessage('') : sseChat.regenerateMessage;

  const {
    exportConversation,
    toggleReasoning,
    handleCurrentMessageChange,
    handleSendMessage: baseHandleSendMessage,
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

  const handleSendMessage: typeof baseHandleSendMessage = (...args) => {
    if (isRealtime && realtimeChat.connectionStatus !== 'connected') {
      notify({
        title: t('playground.realtime.disconnected'),
        message: t('playground.realtime.error_send'),
        type: 'warning',
      });
      return;
    }
    return baseHandleSendMessage(...args);
  };

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
        connectionStatus={isRealtime ? realtimeChat.connectionStatus : undefined}
        onConnect={isRealtime ? realtimeChat.connect : undefined}
        onDisconnect={isRealtime ? realtimeChat.disconnect : undefined}
      />

      <div className="flex-1 flex min-h-0 relative">
        <div className="flex-1 flex min-h-0">
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
            supportsVision={(modelCapabilities.supportsVision || false) && !isRealtime}
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
            showEventLog={showEventLog}
            eventLogCount={eventLog.events.length}
            onToggleEventLog={() => setShowEventLog((v) => !v)}
          />
        </div>

        {showEventLog && (
          <aside className="hidden md:flex flex-col w-96 flex-shrink-0 border-l border-border/50 bg-background/50 p-3 min-h-0 overflow-hidden">
            <EventLogPanel
              events={eventLog.events}
              expandedEvents={eventLog.expandedEvents}
              onToggleExpand={eventLog.toggleExpand}
              onClear={eventLog.clearEvents}
              isMobile={isMobile}
              className="flex-1 min-h-0 flex flex-col"
            />
          </aside>
        )}

        {showEventLog && isMobile && (
          <div className="md:hidden fixed inset-x-0 bottom-0 z-40 max-h-[60vh] bg-background border-t border-border/50 p-3 overflow-hidden">
            <EventLogPanel
              events={eventLog.events}
              expandedEvents={eventLog.expandedEvents}
              onToggleExpand={eventLog.toggleExpand}
              onClear={eventLog.clearEvents}
              isMobile
            />
          </div>
        )}
      </div>

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
