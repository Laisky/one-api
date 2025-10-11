import React, { useState, useRef, useEffect, useCallback } from 'react'
import { Bot, User, AlertCircle, Settings, Copy, RotateCcw, Edit2, Trash2, MoreHorizontal, X } from 'lucide-react'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { useNotifications } from '@/components/ui/notifications'
import { MarkdownRenderer } from '@/components/ui/markdown'
import ThinkingBubble from '@/components/chat/ThinkingBubble'
import { UserMessageActions } from '@/components/chat/UserMessageActions'
import { AssistantMessageActions } from '@/components/chat/AssistantMessageActions'
import { EditMessageDialog } from '@/components/chat/EditMessageDialog'
import { DeleteConfirmationDialog } from '@/components/chat/DeleteConfirmationDialog'
import { useAuthStore } from '@/lib/stores/auth'
import { Message, getMessageStringContent, hasMultiModalContent } from '@/lib/utils'

// Simple image component without lazy loading for better performance
interface SimpleImageProps {
  src: string
  alt: string
  className?: string
  onDelete?: () => void
}

function SimpleImage({ src, alt, className = '', onDelete }: SimpleImageProps) {
  return (
    <div className="relative group">
      <img
        src={src}
        alt={alt}
        className={className}
        style={{
          maxWidth: '100%',
          height: 'auto'
        }}
      />

      {/* Delete button */}
      {onDelete && (
        <Button
          variant="destructive"
          size="icon"
          onClick={onDelete}
          className="absolute top-2 right-2 h-8 w-8 p-0 opacity-100 sm:opacity-0 sm:group-hover:opacity-100 transition-opacity duration-200 shadow-lg rounded-full"
          title="Delete image"
        >
          <X className="h-4 w-4" />
        </Button>
      )}
    </div>
  )
}

interface MessageItemProps {
  message: Message
  messageIndex: number
  isStreaming: boolean
  isLastMessage: boolean
  showReasoningContent: boolean
  expandedReasonings: Record<number, boolean>
  onToggleReasoning: (messageIndex: number) => void
  // Message actions
  onCopyMessage?: (messageIndex: number, content: string) => void
  onRegenerateMessage?: (messageIndex: number) => void
  onEditMessage?: (messageIndex: number, newContent: string | any[]) => void
  onDeleteMessage?: (messageIndex: number) => void
}

// Component to render mixed content (text + images)
function MixedContentRenderer({
  content,
  className = "",
  onDeleteImage
}: {
  content: string | any[],
  className?: string,
  onDeleteImage?: (imageIndex: number) => void
}) {
  if (typeof content === 'string') {
    return (
      <MarkdownRenderer
        content={content}
        className={className}
      />
    )
  }

  if (Array.isArray(content)) {
    let imageIndex = 0 // Track image indices separately from content indices

    return (
      <div className="space-y-3">
        {content.map((item, index) => {
          if (item?.type === 'text') {
            return (
              <MarkdownRenderer
                key={index}
                content={item.text || ''}
                className={className}
              />
            )
          } else if (item?.type === 'image_url' && item?.image_url?.url) {
            const currentImageIndex = imageIndex++
            return (
              <div key={index} className="mt-2">
                <SimpleImage
                  src={item.image_url.url}
                  alt="Attached image"
                  className="max-w-full h-auto rounded-lg border shadow-sm max-h-96 object-contain"
                  onDelete={onDeleteImage ? () => onDeleteImage(currentImageIndex) : undefined}
                />
              </div>
            )
          }
          return null
        })}
      </div>
    )
  }

  return null
}

export function MessageItem({
  message,
  messageIndex,
  isStreaming,
  isLastMessage,
  showReasoningContent,
  expandedReasonings,
  onToggleReasoning,
  onCopyMessage,
  onRegenerateMessage,
  onEditMessage,
  onDeleteMessage
}: MessageItemProps) {
  const { notify } = useNotifications()
  const { user } = useAuthStore()
  const [isEditDialogOpen, setIsEditDialogOpen] = useState(false)
  const [isDeleteDialogOpen, setIsDeleteDialogOpen] = useState(false)

  const handleCopyMessage = async () => {
    const stringContent = getMessageStringContent(message.content)
    if (onCopyMessage) {
      onCopyMessage(messageIndex, stringContent)
    } else {
      // Fallback copy functionality
      try {
        await navigator.clipboard.writeText(stringContent)
        notify({
          title: "Copied!",
          message: "Message copied to clipboard",
          type: "success",
        })
      } catch (error) {
        notify({
          title: "Copy failed",
          message: "Failed to copy message to clipboard",
          type: "error",
        })
      }
    }
  }

  const handleRegenerateMessage = () => {
    if (onRegenerateMessage && !isStreaming) {
      onRegenerateMessage(messageIndex)
    }
  }

  const handleEditMessage = () => {
    if (onEditMessage) {
      setIsEditDialogOpen(true)
    }
  }

  const handleSaveEdit = (newContent: string | any[]) => {
    const currentStringContent = getMessageStringContent(message.content)
    const newStringContent = typeof newContent === 'string' ? newContent : getMessageStringContent(newContent)

    // Check for both text changes and content structure changes (like attachment removal)
    const textChanged = newStringContent.trim() !== currentStringContent.trim()
    const contentStructureChanged = JSON.stringify(newContent) !== JSON.stringify(message.content)

    if (onEditMessage && (textChanged || contentStructureChanged)) {
      onEditMessage(messageIndex, newContent)
    }
  }

  const handleCloseEditDialog = () => {
    setIsEditDialogOpen(false)
  }

  const handleDeleteMessage = () => {
    if (onDeleteMessage) {
      setIsDeleteDialogOpen(true)
    }
  }

  const handleConfirmDelete = () => {
    if (onDeleteMessage) {
      onDeleteMessage(messageIndex)
    }
  }

  const handleCloseDeleteDialog = () => {
    setIsDeleteDialogOpen(false)
  }

  const handleDeleteImage = (imageIndex: number) => {
    if (!onEditMessage || typeof message.content === 'string') return

    // Remove the specific image from the content array
    const contentArray = Array.isArray(message.content) ? [...message.content] : []
    let currentImageIndex = 0
    const updatedContent = contentArray.filter((item) => {
      if (item?.type === 'image_url') {
        if (currentImageIndex === imageIndex) {
          currentImageIndex++
          return false // Remove this image
        }
        currentImageIndex++
      }
      return true // Keep all other items (text and other images)
    })

    // If no content left, don't update
    if (updatedContent.length === 0) return

    // Update the message with the new content
    onEditMessage(messageIndex, updatedContent)
  }
  if (message.role === 'user') {
    return (
      <>
        <div className="flex justify-end group">
          <div className="max-w-3xl space-y-3">
            {/* Header section with user icon, display name, and actions */}
            <div className="flex items-center justify-between mb-2">
              {/* Message Actions */}
              <UserMessageActions
                onCopyMessage={handleCopyMessage}
                onEditMessage={onEditMessage ? handleEditMessage : undefined}
                onDeleteMessage={onDeleteMessage ? handleDeleteMessage : undefined}
              />

              <div className="flex items-center gap-2">
                <div className="text-xs text-muted-foreground font-medium">
                  {user?.display_name || "User"}
                </div>
                <div className="w-8 h-8 bg-primary rounded-full flex items-center justify-center">
                  <User className="h-4 w-4 text-primary-foreground" />
                </div>
              </div>
            </div>

            {/* Message content card */}
            <div className="rounded-lg px-4 py-3 bg-primary text-primary-foreground shadow-md">
              <MixedContentRenderer
                content={message.content}
                className="prose-invert text-primary-foreground"
                onDeleteImage={onEditMessage ? handleDeleteImage : undefined}
              />
            </div>
          </div>
        </div>

        {/* Edit Message Dialog */}
        <EditMessageDialog
          isOpen={isEditDialogOpen}
          onClose={handleCloseEditDialog}
          onSave={handleSaveEdit}
          currentContent={getMessageStringContent(message.content)}
          originalContent={message.content}
          messageRole={message.role}
        />

        {/* Delete Confirmation Dialog */}
        <DeleteConfirmationDialog
          isOpen={isDeleteDialogOpen}
          onClose={handleCloseDeleteDialog}
          onConfirm={handleConfirmDelete}
          messageRole={message.role}
          messagePreview={getMessageStringContent(message.content)}
        />
      </>
    )
  }

  if (message.role === 'system') {
    return (
      <>
        <div className="flex justify-center mb-6 group">
          <div className="flex gap-3 max-w-4xl w-full">
            <div className="flex-shrink-0">
              <div className="w-8 h-8 bg-gradient-to-br from-indigo-500 to-purple-600 rounded-full flex items-center justify-center shadow-sm">
                <Settings className="h-4 w-4 text-white" />
              </div>
            </div>
            <div className="flex-1 rounded-lg px-4 py-3 bg-gradient-to-r from-indigo-50/80 to-purple-50/80 dark:from-indigo-950/40 dark:to-purple-950/40 border border-indigo-200/50 dark:border-indigo-800/50 shadow-sm backdrop-blur-sm">
              <div className="flex items-center justify-between mb-2">
                <div className="flex items-center gap-2">
                  <Settings className="h-4 w-4 text-indigo-600 dark:text-indigo-400" />
                  <span className="font-medium text-sm text-indigo-800 dark:text-indigo-200">System Message</span>
                </div>

                {/* Message Actions */}
                <div className="opacity-0 group-hover:opacity-100 transition-opacity duration-200">
                  <DropdownMenu>
                    <DropdownMenuTrigger asChild>
                      <Button variant="ghost" size="sm" className="h-6 w-6 p-0 text-indigo-600/70 dark:text-indigo-400/70 hover:text-indigo-600 dark:hover:text-indigo-400">
                        <MoreHorizontal className="h-3 w-3" />
                      </Button>
                    </DropdownMenuTrigger>
                    <DropdownMenuContent align="end" className="w-40">
                      <DropdownMenuItem onClick={handleCopyMessage}>
                        <Copy className="mr-2 h-4 w-4" />
                        Copy
                      </DropdownMenuItem>
                      {onEditMessage && (
                        <DropdownMenuItem onClick={handleEditMessage}>
                          <Edit2 className="mr-2 h-4 w-4" />
                          Edit
                        </DropdownMenuItem>
                      )}
                      {onDeleteMessage && (
                        <>
                          <DropdownMenuSeparator />
                          <DropdownMenuItem onClick={handleDeleteMessage} className="text-destructive">
                            <Trash2 className="mr-2 h-4 w-4" />
                            Delete
                          </DropdownMenuItem>
                        </>
                      )}
                    </DropdownMenuContent>
                  </DropdownMenu>
                </div>
              </div>
              <div className="text-indigo-900 dark:text-indigo-100">
                <MarkdownRenderer
                  content={getMessageStringContent(message.content)}
                  className="text-sm [&>*:first-child]:mt-0 [&>*:last-child]:mb-0"
                />
              </div>
            </div>
          </div>
        </div>

        {/* Edit Message Dialog */}
        <EditMessageDialog
          isOpen={isEditDialogOpen}
          onClose={handleCloseEditDialog}
          onSave={handleSaveEdit}
          currentContent={getMessageStringContent(message.content)}
          originalContent={message.content}
          messageRole={message.role}
        />

        {/* Delete Confirmation Dialog */}
        <DeleteConfirmationDialog
          isOpen={isDeleteDialogOpen}
          onClose={handleCloseDeleteDialog}
          onConfirm={handleConfirmDelete}
          messageRole={message.role}
          messagePreview={getMessageStringContent(message.content)}
        />
      </>
    )
  }

  if (message.role === 'error') {
    return (
      <>
        <div className="space-y-2 group">
          <div className="max-w-3xl space-y-3">
            {/* Header section with error icon and actions */}
            <div className="flex items-center justify-between mb-2">
              <div className="flex items-center gap-2">
                <div className="w-8 h-8 bg-red-500 rounded-full flex items-center justify-center">
                  <AlertCircle className="h-4 w-4 text-white" />
                </div>
                <div className="text-xs text-muted-foreground font-medium">
                  Error
                </div>
              </div>

              {/* Message Actions */}
              <div className="opacity-0 group-hover:opacity-100 transition-opacity duration-200">
                <DropdownMenu>
                  <DropdownMenuTrigger asChild>
                    <Button variant="ghost" size="sm" className="h-6 w-6 p-0 text-red-500/70 hover:text-red-500">
                      <MoreHorizontal className="h-3 w-3" />
                    </Button>
                  </DropdownMenuTrigger>
                  <DropdownMenuContent align="end" className="w-40">
                    <DropdownMenuItem onClick={handleCopyMessage}>
                      <Copy className="mr-2 h-4 w-4" />
                      Copy
                    </DropdownMenuItem>
                    {onRegenerateMessage && !isStreaming && (
                      <DropdownMenuItem onClick={handleRegenerateMessage}>
                        <RotateCcw className="mr-2 h-4 w-4" />
                        Retry
                      </DropdownMenuItem>
                    )}
                    {onDeleteMessage && (
                      <>
                        <DropdownMenuSeparator />
                        <DropdownMenuItem onClick={handleDeleteMessage} className="text-destructive">
                          <Trash2 className="mr-2 h-4 w-4" />
                          Delete
                        </DropdownMenuItem>
                      </>
                    )}
                  </DropdownMenuContent>
                </DropdownMenu>
              </div>
            </div>

            {/* Error message content card */}
            <div className="rounded-lg px-4 py-3 bg-red-50 border border-red-200 text-red-800 dark:bg-red-900/20 dark:border-red-800 dark:text-red-300 shadow-md">
              <div className="whitespace-pre-wrap">{getMessageStringContent(message.content)}</div>
            </div>
          </div>
        </div>

        {/* Delete Confirmation Dialog */}
        <DeleteConfirmationDialog
          isOpen={isDeleteDialogOpen}
          onClose={handleCloseDeleteDialog}
          onConfirm={handleConfirmDelete}
          messageRole={message.role}
          messagePreview={getMessageStringContent(message.content)}
        />
      </>
    )
  }

  // Assistant message with thinking bubble above content
  return (
    <>
      <div className="space-y-2 group">
        <div className="max-w-3xl space-y-3">
          {/* Display reasoning content as thinking bubble above message */}
          {message.reasoning_content && showReasoningContent && (
            <ThinkingBubble
              content={message.reasoning_content}
              isExpanded={expandedReasonings[messageIndex] ?? true}
              onToggle={() => onToggleReasoning(messageIndex)}
              isStreaming={isStreaming && isLastMessage}
            />
          )}

          {/* Header section with bot icon, model name, and actions */}
          <div className="flex items-center justify-between mb-2">
            <div className="flex items-center gap-2">
              <div className="w-8 h-8 bg-secondary rounded-full flex items-center justify-center">
                <Bot className="h-4 w-4 text-secondary-foreground" />
              </div>
              {message.model && (
                <div className="text-xs text-muted-foreground font-medium">
                  {message.model}
                </div>
              )}
            </div>

            {/* Message Actions */}
            <AssistantMessageActions
              onCopyMessage={handleCopyMessage}
              onRegenerateMessage={onRegenerateMessage ? handleRegenerateMessage : undefined}
              onEditMessage={onEditMessage ? handleEditMessage : undefined}
              onDeleteMessage={onDeleteMessage ? handleDeleteMessage : undefined}
              isStreaming={isStreaming}
            />
          </div>

          {/* Message content card */}
          <div className="rounded-lg px-4 py-3 bg-secondary shadow-md">
            {/* Show loading indicator when content is empty and streaming */}
            {!getMessageStringContent(message.content) && isStreaming && isLastMessage ? (
              <div className="flex items-center justify-center p-4 text-sm text-muted-foreground">
                <svg className="animate-spin -ml-1 mr-3 h-5 w-5 text-current" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
                  <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4"></circle>
                  <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
                </svg>
                {message.model} is processing...
              </div>
            ) : (
              <MarkdownRenderer
                content={getMessageStringContent(message.content)}
                className=""
              />
            )}
          </div>
        </div>
      </div>

      {/* Edit Message Dialog */}
      <EditMessageDialog
        isOpen={isEditDialogOpen}
        onClose={handleCloseEditDialog}
        onSave={handleSaveEdit}
        currentContent={getMessageStringContent(message.content)}
        originalContent={message.content}
        messageRole={message.role}
      />

      {/* Delete Confirmation Dialog */}
      <DeleteConfirmationDialog
        isOpen={isDeleteDialogOpen}
        onClose={handleCloseDeleteDialog}
        onConfirm={handleConfirmDelete}
        messageRole={message.role}
        messagePreview={getMessageStringContent(message.content)}
      />
    </>
  )
}
