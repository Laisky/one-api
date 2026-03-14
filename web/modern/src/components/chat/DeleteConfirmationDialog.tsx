import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { AlertTriangle, Trash2 } from 'lucide-react'
import React from 'react'
import { useTranslation } from 'react-i18next'

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
  const { t } = useTranslation()

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
      case 'user': return t('playground.roles.user')
      case 'assistant': return t('playground.roles.assistant')
      case 'system': return t('playground.roles.system')
      case 'error': return t('playground.roles.error')
      default: return t('playground.roles.message')
    }
  }

  const getRoleColor = () => {
    switch (messageRole) {
      case 'user': return 'text-primary'
      case 'assistant': return 'text-secondary-foreground'
      case 'system': return 'text-info'
      case 'error': return 'text-destructive'
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
            {t('playground.delete.title', { role: getRoleDisplayName() })}
          </DialogTitle>
          <DialogDescription className="space-y-3">
            <div>
              {t('playground.delete.description')}
            </div>

            {messagePreview && (
              <div className="p-3 bg-muted/50 rounded-lg border">
                <div className="flex items-center gap-2 mb-2">
                  <span className={`text-xs font-medium ${getRoleColor()}`}>
                    {t('playground.delete.preview_label', { role: getRoleDisplayName() })}
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
            {t('playground.delete.cancel')}
          </Button>
          <Button
            variant="destructive"
            onClick={handleConfirm}
            className="flex items-center gap-2"
          >
            <Trash2 className="h-4 w-4" />
            {t('playground.delete.confirm')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
