import React, { useState, useEffect, useRef } from 'react'
import { Button } from '@/components/ui/button'
import { Textarea } from '@/components/ui/textarea'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Send, Trash2, Download, Bot, Menu, X, Eye, EyeOff, Settings } from 'lucide-react'
import { useResponsive, useIsTouchDevice } from '@/hooks/useResponsive'
import { MarkdownRenderer } from '@/components/ui/markdown'
import { MessageList } from '@/components/chat/MessageList'
import { ImageAttachmentComponent, ImageAttachment as ImageAttachmentType } from '@/components/chat/ImageAttachment'
import { Message } from '@/lib/utils'

interface ChatInterfaceProps {
  // Messages
  messages: Message[]
  onClearConversation: () => void
  onExportConversation: () => void

  // Current input
  currentMessage: string
  onCurrentMessageChange: (value: string) => void
  onSendMessage: (message: string, images?: ImageAttachmentType[]) => void

  // Chat state
  isStreaming: boolean
  onStopGeneration: () => void
  selectedModel: string
  selectedToken: string

  // Model capabilities
  supportsVision: boolean

  // Image attachments
  attachedImages: ImageAttachmentType[]
  onAttachedImagesChange: (images: ImageAttachmentType[]) => void

  // Preview
  showPreview: boolean
  onPreviewChange: (show: boolean) => void

  // Mobile
  onMobileMenuToggle: () => void

  // Reasoning
  showReasoningContent: boolean
  expandedReasonings: Record<number, boolean>
  onToggleReasoning: (messageIndex: number) => void

  // Focus mode
  focusModeEnabled: boolean
  onFocusModeChange: (enabled: boolean) => void

  // Message actions
  onCopyMessage?: (messageIndex: number, content: string) => void
  onRegenerateMessage?: (messageIndex: number) => void
  onEditMessage?: (messageIndex: number, newContent: string) => void
  onDeleteMessage?: (messageIndex: number) => void
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
  onDeleteMessage
}: ChatInterfaceProps) {
  const { isMobile, isTablet } = useResponsive()
  const isTouchDevice = useIsTouchDevice()

  const handleKeyPress = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      handleSendMessage()
    }
    // Shift+Enter allows new lines, no action needed as it's default textarea behavior
  }

  const handleSendMessage = () => {
    onSendMessage(currentMessage, attachedImages)
    // Clear images after sending
    onAttachedImagesChange([])
  }

  // Handle input change for preview functionality
  const handleInputChange = (e: React.ChangeEvent<HTMLTextAreaElement>) => {
    const value = e.target.value
    onCurrentMessageChange(value)

    // Show preview for any content
    onPreviewChange(value.trim().length > 0)
  }

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
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={onMobileMenuToggle}
                  className="lg:hidden p-2 flex-shrink-0"
                >
                  <Settings className="h-4 w-4" />
                </Button>

                <div className="flex items-center gap-2 flex-shrink-0">
                  <Bot className="h-4 w-4 text-primary" />
                  <CardTitle className={`font-semibold bg-gradient-to-r from-primary to-primary/80 bg-clip-text text-transparent ${isMobile ? 'text-base' : 'text-lg'
                    }`}>
                    AI Playground
                  </CardTitle>
                </div>
              </div>

              {/* Action buttons - hide on mobile, show on larger screens */}
              <div className="hidden sm:flex items-center gap-2">
                <Button
                  variant={focusModeEnabled ? "default" : "outline"}
                  size="sm"
                  onClick={() => onFocusModeChange(!focusModeEnabled)}
                  className={`flex-shrink-0 ${focusModeEnabled ? 'bg-primary/10 border-primary/50 text-primary' : 'hover:bg-primary/10'}`}
                  title={focusModeEnabled ? "Disable Focus Mode" : "Enable Focus Mode"}
                >
                  {focusModeEnabled ? <EyeOff className="h-4 w-4 mr-1" /> : <Eye className="h-4 w-4 mr-1" />}
                  Focus
                </Button>
                <Button
                  variant="outline"
                  size="sm"
                  onClick={onExportConversation}
                  disabled={messages.length === 0}
                  className="hover:bg-primary/10 flex-shrink-0"
                >
                  <Download className="h-4 w-4 mr-1" />
                  Export
                </Button>
                <Button
                  variant="outline"
                  size="sm"
                  onClick={onClearConversation}
                  disabled={messages.length === 0 || isStreaming}
                  className="hover:bg-destructive/10 hover:text-destructive hover:border-destructive/50 flex-shrink-0"
                >
                  <Trash2 className="h-4 w-4 mr-1" />
                  Clear
                </Button>
              </div>
            </div>

            {/* Second row: Model info and status badges */}
            <div className="flex items-start justify-between gap-2 flex-wrap">
              <div className="flex items-start gap-2 min-w-0 flex-1">
                {selectedModel && (
                  <div className="flex flex-col sm:flex-row sm:items-center gap-1 sm:gap-2 min-w-0 flex-1">
                    <span className="text-sm text-muted-foreground flex-shrink-0">Model:</span>
                    <div className="min-w-0 flex-1">
                      <Badge
                        variant="secondary"
                        className="font-medium text-xs w-full sm:w-auto inline-block break-all sm:break-normal"
                        title={selectedModel}
                      >
                        <span className={`${isMobile ? 'break-all' : 'truncate'} block`}>
                          {selectedModel}
                        </span>
                      </Badge>
                    </div>
                  </div>
                )}
                {isStreaming && (
                  <Badge variant="outline" className="animate-pulse border-green-500 text-green-600 text-xs flex-shrink-0 self-start sm:ml-auto">
                    <div className="w-1.5 h-1.5 bg-green-500 rounded-full mr-1 animate-pulse"></div>
                    {isMobile ? 'Gen...' : 'Generating...'}
                  </Badge>
                )}
              </div>

              {/* Mobile action buttons - show only on mobile */}
              <div className="flex sm:hidden items-center gap-2">
                <Button
                  variant={focusModeEnabled ? "default" : "outline"}
                  size="sm"
                  onClick={() => onFocusModeChange(!focusModeEnabled)}
                  className={`flex-shrink-0 p-2 ${focusModeEnabled ? 'bg-primary/10 border-primary/50 text-primary' : 'hover:bg-primary/10'}`}
                  title={focusModeEnabled ? "Disable Focus Mode" : "Enable Focus Mode"}
                >
                  {focusModeEnabled ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
                </Button>
                <Button
                  variant="outline"
                  size="sm"
                  onClick={onExportConversation}
                  disabled={messages.length === 0}
                  className="hover:bg-primary/10 flex-shrink-0 p-2"
                  title="Export"
                >
                  <Download className="h-4 w-4" />
                </Button>
                <Button
                  variant="outline"
                  size="sm"
                  onClick={onClearConversation}
                  disabled={messages.length === 0 || isStreaming}
                  className="hover:bg-destructive/10 hover:text-destructive hover:border-destructive/50 flex-shrink-0 p-2"
                  title="Clear"
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
        <Card className="flex-shrink-0 border-2 border-blue-200 dark:border-blue-800 bg-blue-50/50 dark:bg-blue-900/20">
          <CardContent className="p-4">
            <div className="flex items-center gap-2 mb-3">
              <Badge variant="outline" className="text-xs border-blue-300 text-blue-600 dark:border-blue-600 dark:text-blue-400">
                Preview
              </Badge>
              <span className="text-xs text-muted-foreground">
                This is how your message will be rendered
              </span>
            </div>
            <div className="max-h-[300px] overflow-y-auto">
              <div className="rounded-lg p-3 bg-background border">
                <MarkdownRenderer
                  content={currentMessage}
                  className="text-sm"
                />
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
                    ? "Select an API token to start chatting..."
                    : !selectedModel
                      ? "Select a model to start chatting..."
                      : isStreaming
                        ? "Generating response..."
                        : "Type your message... (Shift+Enter for new line)"
                }
                disabled={isStreaming || !selectedModel || !selectedToken}
                className={`
                  min-h-[80px] max-h-[200px] text-base border-2 focus:border-primary/50 transition-colors resize-none
                  ${isMobile || isTablet ? 'pr-12' : 'pr-20'}
                `}
                rows={3}
              />

              {/* Send/Stop Button positioned inside textarea */}
              <div className={`
                absolute flex items-center justify-center
                ${isMobile || isTablet
                  ? 'bottom-3 right-3 h-8 w-8'
                  : 'bottom-3 right-5 h-10 w-12'
                }
              `}>
                {isStreaming ? (
                  <Button
                    onClick={onStopGeneration}
                    variant="outline"
                    size={isMobile || isTablet ? "sm" : "md"}
                    className={`
                      ${isMobile || isTablet ? 'h-8 w-8 p-0' : 'h-10 w-12 px-3'}
                      hover:bg-destructive/10 hover:text-destructive hover:border-destructive/50
                      bg-background/95 backdrop-blur-sm border-border/50
                      ${isTouchDevice ? 'active:scale-95' : ''}
                      transition-all duration-200
                    `}
                  >
                    {isMobile || isTablet ? (
                      <X className="h-4 w-4" />
                    ) : (
                      "Stop"
                    )}
                  </Button>
                ) : (
                  <Button
                    onClick={handleSendMessage}
                    disabled={(!currentMessage.trim() && attachedImages.length === 0) || !selectedModel || !selectedToken}
                    size={isMobile || isTablet ? "sm" : "md"}
                    className={`
                      ${isMobile || isTablet ? 'h-8 w-8 p-0' : 'h-10 w-12 px-3'}
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
                        Send
                      </>
                    )}
                  </Button>
                )}
              </div>
            </div>

            {(!selectedToken || !selectedModel) && (
              <div className="text-center">
                <span className="text-sm text-muted-foreground">
                  {!selectedToken
                    ? "Please select an API token from the sidebar to begin"
                    : "Please select a model from the sidebar to begin"
                  }
                </span>
              </div>
            )}
          </div>
        </CardContent>
      </Card>
    </div>
  )
}
