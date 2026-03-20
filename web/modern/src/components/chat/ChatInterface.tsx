import { ImageAttachmentComponent, ImageAttachment as ImageAttachmentType } from '@/components/chat/ImageAttachment';
import { MessageList } from '@/components/chat/MessageList';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { MarkdownRenderer } from '@/components/ui/markdown';
import { Textarea } from '@/components/ui/textarea';
import { useIsTouchDevice, useResponsive } from '@/hooks/useResponsive';
import { Message } from '@/lib/utils';
import { Bot, Download, Eye, EyeOff, Send, Settings, Trash2, X } from 'lucide-react';
import React from 'react';
import { useTranslation } from 'react-i18next';

interface ChatInterfaceProps {
  // Messages
  messages: Message[];
  onClearConversation: () => void;
  onExportConversation: () => void;

  // Current input
  currentMessage: string;
  onCurrentMessageChange: (value: string) => void;
  onSendMessage: (message: string, images?: ImageAttachmentType[]) => void;

  // Chat state
  isStreaming: boolean;
  onStopGeneration: () => void;
  selectedModel: string;
  selectedToken: string;

  // Model capabilities
  supportsVision: boolean;

  // Image attachments
  attachedImages: ImageAttachmentType[];
  onAttachedImagesChange: (images: ImageAttachmentType[]) => void;

  // Preview
  showPreview: boolean;
  onPreviewChange: (show: boolean) => void;

  // Mobile
  onMobileMenuToggle: () => void;

  // Reasoning
  showReasoningContent: boolean;
  expandedReasonings: Record<number, boolean>;
  onToggleReasoning: (messageIndex: number) => void;

  // Focus mode
  focusModeEnabled: boolean;
  onFocusModeChange: (enabled: boolean) => void;

  // Message actions
  onCopyMessage?: (messageIndex: number, content: string) => void;
  onRegenerateMessage?: (messageIndex: number) => void;
  onEditMessage?: (messageIndex: number, newContent: string) => void;
  onDeleteMessage?: (messageIndex: number) => void;
}

export function ChatInterface({
  messages,
  onClearConversation,
  onExportConversation,
  currentMessage,
  onCurrentMessageChange,
  onSendMessage,
  isStreaming,
  onStopGeneration,
  selectedModel,
  selectedToken,
  supportsVision,
  attachedImages,
  onAttachedImagesChange,
  showPreview,
  onPreviewChange,
  onMobileMenuToggle,
  showReasoningContent,
  expandedReasonings,
  onToggleReasoning,
  focusModeEnabled,
  onFocusModeChange,
  onCopyMessage,
  onRegenerateMessage,
  onEditMessage,
  onDeleteMessage,
}: ChatInterfaceProps) {
  const { t } = useTranslation();
  const { isMobile, isTablet } = useResponsive();
  const isTouchDevice = useIsTouchDevice();

  const handleKeyPress = (e: React.KeyboardEvent) => {
    if (e.nativeEvent.isComposing || e.keyCode === 229) return;
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      handleSendMessage();
    }
    // Shift+Enter allows new lines, no action needed as it's default textarea behavior
  };

  const handleSendMessage = () => {
    onSendMessage(currentMessage, attachedImages);
    // Clear images after sending
    onAttachedImagesChange([]);
  };

  // Handle input change for preview functionality
  const handleInputChange = (e: React.ChangeEvent<HTMLTextAreaElement>) => {
    const value = e.target.value;
    onCurrentMessageChange(value);

    // Show preview for any content
    onPreviewChange(value.trim().length > 0);
  };

  return (
    <div className="flex-1 flex flex-col bg-background/50 min-h-0 p-3 space-y-3">
      {/* Header Card */}
      <Card className="flex-shrink-0">
        <CardHeader className="pb-3">
          <div className="space-y-3">
            {/* Top row: Menu button, title, and action buttons */}
            <div className="flex items-center justify-between gap-2">
              <div className="flex items-center gap-2 min-w-0 flex-1">
                {/* Mobile menu button */}
                <Button variant="ghost" size="sm" onClick={onMobileMenuToggle} className="lg:hidden p-2 flex-shrink-0">
                  <Settings className="h-4 w-4" />
                </Button>

                <div className="flex items-center gap-2 flex-shrink-0">
                  <Bot className="h-4 w-4 text-primary" />
                  <CardTitle className={`font-semibold text-primary ${isMobile ? 'text-base' : 'text-lg'}`}>
                    {t('playground.title')}
                  </CardTitle>
                </div>
              </div>

              {/* Action buttons - hide on mobile, show on larger screens */}
              <div className="hidden sm:flex items-center gap-2">
                <Button
                  variant={focusModeEnabled ? 'default' : 'outline'}
                  size="sm"
                  onClick={() => onFocusModeChange(!focusModeEnabled)}
                  className={`flex-shrink-0 ${focusModeEnabled ? 'bg-primary/10 border-primary/50 text-primary' : 'hover:bg-primary/10'}`}
                  title={focusModeEnabled ? t('playground.chat.disable_focus') : t('playground.chat.enable_focus')}
                >
                  {focusModeEnabled ? <EyeOff className="h-4 w-4 mr-1" /> : <Eye className="h-4 w-4 mr-1" />}
                  {t('playground.chat.focus')}
                </Button>
                <Button
                  variant="outline"
                  size="sm"
                  onClick={onExportConversation}
                  disabled={messages.length === 0}
                  className="hover:bg-primary/10 flex-shrink-0"
                >
                  <Download className="h-4 w-4 mr-1" />
                  {t('playground.chat.export')}
                </Button>
                <Button
                  variant="outline"
                  size="sm"
                  onClick={onClearConversation}
                  disabled={messages.length === 0 || isStreaming}
                  className="hover:bg-destructive/10 hover:text-destructive hover:border-destructive/50 flex-shrink-0"
                >
                  <Trash2 className="h-4 w-4 mr-1" />
                  {t('playground.chat.clear')}
                </Button>
              </div>
            </div>

            {/* Second row: Model info and status badges */}
            <div className="flex items-start justify-between gap-2 flex-wrap">
              <div className="flex items-start gap-2 min-w-0 flex-1">
                {selectedModel && (
                  <div className="flex flex-col sm:flex-row sm:items-center gap-1 sm:gap-2 min-w-0 flex-1">
                    <span className="text-sm text-muted-foreground flex-shrink-0">{t('playground.chat.model_label')}</span>
                    <div className="min-w-0 flex-1">
                      <Badge
                        variant="secondary"
                        className="font-medium text-xs w-full sm:w-auto inline-block break-all sm:break-normal"
                        title={selectedModel}
                      >
                        <span className={`${isMobile ? 'break-all' : 'truncate'} block`}>{selectedModel}</span>
                      </Badge>
                    </div>
                  </div>
                )}
                {isStreaming && (
                  <Badge
                    variant="outline"
                    className="animate-pulse border-success text-success text-xs flex-shrink-0 self-start sm:ml-auto"
                  >
                    <div className="w-1.5 h-1.5 bg-success rounded-full mr-1 animate-pulse"></div>
                    {isMobile ? t('playground.chat.generating_mobile') : t('playground.chat.generating')}
                  </Badge>
                )}
              </div>

              {/* Mobile action buttons - show only on mobile */}
              <div className="flex sm:hidden items-center gap-1">
                <Button
                  variant={focusModeEnabled ? 'default' : 'outline'}
                  size="icon"
                  onClick={() => onFocusModeChange(!focusModeEnabled)}
                  className={`flex-shrink-0 touch-target ${focusModeEnabled ? 'bg-primary/10 border-primary/50 text-primary' : 'hover:bg-primary/10'}`}
                  title={focusModeEnabled ? t('playground.chat.disable_focus') : t('playground.chat.enable_focus')}
                >
                  {focusModeEnabled ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
                </Button>
                <Button
                  variant="outline"
                  size="icon"
                  onClick={onExportConversation}
                  disabled={messages.length === 0}
                  className="hover:bg-primary/10 flex-shrink-0 touch-target"
                  title={t('playground.chat.export')}
                >
                  <Download className="h-4 w-4" />
                </Button>
                <Button
                  variant="outline"
                  size="icon"
                  onClick={onClearConversation}
                  disabled={messages.length === 0 || isStreaming}
                  className="hover:bg-destructive/10 hover:text-destructive hover:border-destructive/50 flex-shrink-0 touch-target"
                  title={t('playground.chat.clear')}
                >
                  <Trash2 className="h-4 w-4" />
                </Button>
              </div>
            </div>
          </div>
        </CardHeader>
      </Card>

      {/* Messages Card */}
      <Card className="flex-1 min-h-0">
        <CardContent className="p-0 h-full">
          <MessageList
            messages={messages}
            isStreaming={isStreaming}
            showReasoningContent={showReasoningContent}
            expandedReasonings={expandedReasonings}
            onToggleReasoning={onToggleReasoning}
            focusModeEnabled={focusModeEnabled}
            onCopyMessage={onCopyMessage}
            onRegenerateMessage={onRegenerateMessage}
            onEditMessage={onEditMessage}
            onDeleteMessage={onDeleteMessage}
          />
        </CardContent>
      </Card>

      {/* Preview Message Card */}
      {showPreview && currentMessage.trim() && (
        <Card className="flex-shrink-0 border-2 border-info-border bg-info-muted/50">
          <CardContent className="p-4">
            <div className="flex items-center gap-2 mb-3">
              <Badge variant="outline" className="text-xs border-info-border text-info">
                {t('playground.chat.preview')}
              </Badge>
              <span className="text-xs text-muted-foreground">{t('playground.chat.preview_desc')}</span>
            </div>
            <div className="max-h-[300px] overflow-y-auto">
              <div className="rounded-lg p-3 bg-background border">
                <MarkdownRenderer content={currentMessage} className="text-sm" />
              </div>
            </div>
          </CardContent>
        </Card>
      )}

      {/* Input Card */}
      <Card className="flex-shrink-0">
        <CardContent className={`${isMobile ? 'p-2' : 'p-4'}`}>
          <div className={`${isMobile ? 'space-y-2' : 'space-y-3'}`}>
            {/* Image Attachment - Only show for vision-capable models */}
            {supportsVision && (
              <ImageAttachmentComponent
                images={attachedImages}
                onImagesChange={onAttachedImagesChange}
                disabled={isStreaming || !selectedModel || !selectedToken}
                maxImages={5}
              />
            )}

            <div className="relative">
              <Textarea
                value={currentMessage}
                onChange={handleInputChange}
                onKeyDown={handleKeyPress}
                placeholder={
                  !selectedToken
                    ? t('playground.chat.input.placeholder_no_token')
                    : !selectedModel
                      ? t('playground.chat.input.placeholder_no_model')
                      : isStreaming
                        ? t('playground.chat.input.placeholder_generating')
                        : t('playground.chat.input.placeholder_default')
                }
                disabled={isStreaming || !selectedModel || !selectedToken}
                className={`
                  min-h-[80px] max-h-[200px] text-base border-2 focus:border-primary/50 transition-colors resize-none
                  ${isMobile || isTablet ? 'pr-12' : 'pr-20'}
                `}
                rows={3}
              />

              {/* Send/Stop Button positioned inside textarea */}
              <div
                className={`
                absolute flex items-center justify-center
                ${isMobile || isTablet ? 'bottom-2 right-2 h-10 w-10' : 'bottom-3 right-5 h-10 w-12'}
              `}
              >
                {isStreaming ? (
                  <Button
                    onClick={onStopGeneration}
                    variant="outline"
                    size={isMobile || isTablet ? 'sm' : 'md'}
                    className={`
                      ${isMobile || isTablet ? 'h-10 w-10 p-0' : 'h-10 w-12 px-3'}
                      hover:bg-destructive/10 hover:text-destructive hover:border-destructive/50
                      bg-background/95 border-border/50
                      ${isTouchDevice ? 'active:scale-95' : ''}
                      transition-all duration-200
                    `}
                  >
                    {isMobile || isTablet ? <X className="h-4 w-4" /> : t('playground.chat.input.stop')}
                  </Button>
                ) : (
                  <Button
                    onClick={handleSendMessage}
                    disabled={(!currentMessage.trim() && attachedImages.length === 0) || !selectedModel || !selectedToken}
                    size={isMobile || isTablet ? 'sm' : 'md'}
                    className={`
                      ${isMobile || isTablet ? 'h-10 w-10 p-0' : 'h-10 w-12 px-3'}
                      bg-primary hover:bg-primary/90 disabled:opacity-50
                      ${isTouchDevice ? 'active:scale-95' : ''}
                      transition-all duration-200
                      shadow-sm
                    `}
                  >
                    {isMobile || isTablet ? (
                      <Send className="h-4 w-4" />
                    ) : (
                      <>
                        <Send className="h-4 w-4 mr-1" />
                        {t('playground.chat.input.send')}
                      </>
                    )}
                  </Button>
                )}
              </div>
            </div>

            {(!selectedToken || !selectedModel) && (
              <div className="text-center">
                <span className="text-sm text-muted-foreground">
                  {!selectedToken ? t('playground.chat.input.hint_no_token') : t('playground.chat.input.hint_no_model')}
                </span>
              </div>
            )}
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
