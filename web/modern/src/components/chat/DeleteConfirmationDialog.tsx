import React from 'react'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Trash2, AlertTriangle } from 'lucide-react'

interface DeleteConfirmationDialogProps {
  isOpen: boolean
  onClose: () => void
  onConfirm: () => void
  messageRole: 'user' | 'assistant' | 'system' | 'error'
  messagePreview?: string
}

export function DeleteConfirmationDialog({
  isOpen,
  onClose,
  onConfirm,
  messageRole,
  messagePreview
}: DeleteConfirmationDialogProps) {
  const handleConfirm = () => {
    onConfirm()
    onClose()
  }

  const handleKeyDown = (e: React.KeyboardEvent) => {
    // Confirm on Enter
    if (e.key === 'Enter') {
      e.preventDefault()
      handleConfirm()
    }
    // Cancel on Escape
    if (e.key === 'Escape') {
      e.preventDefault()
      onClose()
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

  const getTruncatedPreview = () => {
    if (!messagePreview) return ''
    return messagePreview.length > 100
      ? messagePreview.substring(0, 100) + '...'
      : messagePreview
  }

  return (
    <Dialog open={isOpen} onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="max-w-md" onKeyDown={handleKeyDown}>
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2 text-destructive">
            <AlertTriangle className="h-4 w-4" />
            Delete {getRoleDisplayName()} Message
          </DialogTitle>
          <DialogDescription className="space-y-3">
            <div>
              Are you sure you want to delete this message? This action cannot be undone.
            </div>

            {messagePreview && (
              <div className="p-3 bg-muted/50 rounded-lg border">
                <div className="flex items-center gap-2 mb-2">
                  <span className={`text-xs font-medium ${getRoleColor()}`}>
                    {getRoleDisplayName()} Message:
                  </span>
                </div>
                <div className="text-sm text-muted-foreground italic">
                  "{getTruncatedPreview()}"
                </div>
              </div>
            )}
          </DialogDescription>
        </DialogHeader>

        <DialogFooter className="gap-2">
          <Button
            variant="outline"
            onClick={onClose}
            autoFocus
          >
            Cancel
          </Button>
          <Button
            variant="destructive"
            onClick={handleConfirm}
            className="flex items-center gap-2"
          >
            <Trash2 className="h-4 w-4" />
            Delete Message
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
