import React, { useState, useEffect } from 'react'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Textarea } from '@/components/ui/textarea'
import { Label } from '@/components/ui/label'
import { Card, CardContent } from '@/components/ui/card'
import { Edit2, X, Image as ImageIcon } from 'lucide-react'

interface EditMessageDialogProps {
  isOpen: boolean
  onClose: () => void
  onSave: (newContent: string | any[]) => void
  currentContent: string
  originalContent: string | any[] // The full message content including attachments
  messageRole: 'user' | 'assistant' | 'system' | 'error'
}

export function EditMessageDialog({
  isOpen,
  onClose,
  onSave,
  currentContent,
  originalContent,
  messageRole
}: EditMessageDialogProps) {
  const [editedContent, setEditedContent] = useState(currentContent)
  const [editedAttachments, setEditedAttachments] = useState<any[]>([])
  const [hasChanges, setHasChanges] = useState(false)

  // Update edited content when dialog opens with new content
  useEffect(() => {
    setEditedContent(currentContent)

    // Extract attachments from originalContent
    if (Array.isArray(originalContent)) {
      const attachments = originalContent.filter(item => item?.type === 'image_url')
      setEditedAttachments(attachments)
    } else {
      setEditedAttachments([])
    }

    setHasChanges(false)
  }, [currentContent, originalContent, isOpen])

  // Track changes
  useEffect(() => {
    const textChanged = editedContent.trim() !== currentContent.trim()
    const attachmentsChanged = Array.isArray(originalContent) &&
      JSON.stringify(editedAttachments) !== JSON.stringify(originalContent.filter(item => item?.type === 'image_url'))

    setHasChanges(textChanged || attachmentsChanged)
  }, [editedContent, currentContent, editedAttachments, originalContent])

  const handleSave = () => {
    const trimmedContent = editedContent.trim()

    // Always save if there are changes (text or attachments)
    if (hasChanges) {
      // Create the new content structure
      if (editedAttachments.length > 0) {
        // Mixed content: combine text and attachments
        const newContent: any[] = []

        if (trimmedContent) {
          newContent.push({
            type: 'text',
            text: trimmedContent
          })
        }

        // Add attachments
        editedAttachments.forEach(attachment => {
          newContent.push(attachment)
        })

        onSave(newContent)
      } else if (trimmedContent) {
        // Text only
        onSave(trimmedContent)
      } else {
        // Edge case: no text and no attachments (shouldn't happen with hasChanges check)
        // But handle it gracefully by saving empty string
        onSave('')
      }
    }

    onClose()
  }

  const handleCancel = () => {
    setEditedContent(currentContent)
    setHasChanges(false)
    onClose()
  }

  const handleDeleteAttachment = (attachmentIndex: number) => {
    const updatedAttachments = editedAttachments.filter((_, index) => index !== attachmentIndex)
    setEditedAttachments(updatedAttachments)
  }

  const handleKeyDown = (e: React.KeyboardEvent) => {
    // Save on Ctrl/Cmd + Enter
    if ((e.ctrlKey || e.metaKey) && e.key === 'Enter' && hasChanges) {
      e.preventDefault()
      handleSave()
    }
    // Cancel on Escape
    if (e.key === 'Escape') {
      e.preventDefault()
      handleCancel()
    }
  }

  const getRoleDisplayName = () => {
    switch (messageRole) {
      case 'user': return 'User'
      case 'assistant': return 'Assistant'
      case 'system': return 'System'
      case 'error': return 'Error'
      default: return 'Message'
    }
  }

  const getRoleColor = () => {
    switch (messageRole) {
      case 'user': return 'text-primary'
      case 'assistant': return 'text-secondary-foreground'
      case 'system': return 'text-indigo-600 dark:text-indigo-400'
      case 'error': return 'text-red-600 dark:text-red-400'
      default: return 'text-foreground'
    }
  }

  return (
    <Dialog open={isOpen} onOpenChange={(open) => !open && handleCancel()}>
      <DialogContent className="w-[95vw] max-w-lg sm:max-w-2xl lg:max-w-4xl max-h-[90vh] sm:max-h-[85vh] flex flex-col overflow-hidden p-4 sm:p-6">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <Edit2 className="h-4 w-4" />
            Edit {getRoleDisplayName()} Message
          </DialogTitle>
          <DialogDescription>
            Make changes to the message content. You can use{' '}
            <kbd className="px-1.5 py-0.5 text-xs bg-muted rounded">Ctrl+Enter</kbd> to save or{' '}
            <kbd className="px-1.5 py-0.5 text-xs bg-muted rounded">Esc</kbd> to cancel.
          </DialogDescription>
        </DialogHeader>

        <div className="flex-1 space-y-3 sm:space-y-4 overflow-auto min-w-0 w-full">
          <div className="space-y-3">
            <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-1 sm:gap-2">
              <Label htmlFor="message-content" className={`text-sm font-semibold ${getRoleColor()} flex-shrink-0`}>
                {getRoleDisplayName()} Message Content
              </Label>
              <div className="flex items-center gap-1 sm:gap-2 text-xs flex-shrink-0 flex-wrap">
                <span className="text-muted-foreground">
                  {editedContent.length} chars
                </span>
                <span className="text-muted-foreground">•</span>
                <span className="text-muted-foreground">
                  {editedContent.trim() ? editedContent.trim().split(/\s+/).length : 0} words
                </span>
                {hasChanges && (
                  <span className="flex items-center gap-1 text-orange-600 dark:text-orange-400">
                    <span className="w-2 h-2 bg-orange-500 rounded-full animate-pulse"></span>
                    Modified
                  </span>
                )}
              </div>
            </div>
            <Textarea
              id="message-content"
              value={editedContent}
              onChange={(e) => setEditedContent(e.target.value)}
              onKeyDown={handleKeyDown}
              placeholder={`Enter ${messageRole} message content...`}
              className="min-h-[120px] sm:min-h-[180px] max-h-[250px] sm:max-h-[350px] resize-y text-sm leading-relaxed
                border-2 border-border hover:border-border/80 focus:border-primary focus:ring-2 focus:ring-primary/20
                transition-all duration-200 rounded-lg p-3
                bg-background/50 w-full"
              autoFocus
              spellCheck={messageRole === 'user' || messageRole === 'assistant'}
            />
          </div>

          <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-2 text-xs">
            <div className="flex items-center gap-2 text-muted-foreground flex-wrap">
              <kbd className="px-1.5 py-0.5 text-xs bg-muted rounded border">Ctrl+Enter</kbd>
              <span>to save</span>
              <span>•</span>
              <kbd className="px-1.5 py-0.5 text-xs bg-muted rounded border">Esc</kbd>
              <span>to cancel</span>
            </div>
          </div>

          {/* Attachments section */}
          {editedAttachments.length > 0 && (
            <div className="space-y-2">
              <Label className="text-sm font-medium flex items-center gap-2">
                <ImageIcon className="h-4 w-4" />
                Message Attachments ({editedAttachments.length})
              </Label>
              <div className="max-h-[120px] sm:max-h-[180px] overflow-y-auto">
                <div className="grid grid-cols-1 sm:grid-cols-2 md:grid-cols-3 lg:grid-cols-4 gap-3 sm:gap-4 p-3 bg-muted/30 rounded-lg">
                  {editedAttachments.map((attachment, index) => (
                    <Card key={index} className="relative group overflow-hidden">
                      <CardContent className="p-0">
                        <div className="relative aspect-square">
                          <img
                            src={attachment.image_url?.url}
                            alt={`Attachment ${index + 1}`}
                            className="w-full h-full object-cover rounded-lg"
                            loading="lazy"
                          />
                          <Button
                            type="button"
                            variant="destructive"
                            size="icon"
                            onClick={() => handleDeleteAttachment(index)}
                            className="absolute top-2 right-2 h-8 w-8 p-0 opacity-100 sm:opacity-0 sm:group-hover:opacity-100 transition-opacity duration-200 shadow-lg rounded-full"
                            title="Delete attachment"
                          >
                            <X className="h-4 w-4" />
                          </Button>
                        </div>
                      </CardContent>
                    </Card>
                  ))}
                </div>
              </div>
              <div className="text-xs text-muted-foreground">
                You can delete individual attachments by clicking the × button that appears when you hover over them.
              </div>
            </div>
          )}
        </div>

        <DialogFooter className="gap-2 sm:gap-3">
          <Button
            variant="outline"
            onClick={handleCancel}
            className="px-3 py-2 text-sm"
          >
            Cancel
          </Button>
          <Button
            onClick={handleSave}
            disabled={!hasChanges}
            className="px-3 py-2 text-sm"
          >
            Save Changes
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
