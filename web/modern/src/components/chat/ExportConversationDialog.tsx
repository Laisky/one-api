import React, { useState } from 'react'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { FileDown, FileJson } from 'lucide-react'
import {
  Message,
  generateSHA256Digest,
  formatTimestamp
} from '@/lib/utils'

interface ExportConversationDialogProps {
  isOpen: boolean
  onClose: () => void
  messages: Message[]
  selectedModel?: string
  conversationId?: string
  conversationCreated?: number
  conversationCreatedBy?: string
}

export function ExportConversationDialog({
  isOpen,
  onClose,
  messages,
  selectedModel,
  conversationId,
  conversationCreated,
  conversationCreatedBy
}: ExportConversationDialogProps) {
  const [customFilename, setCustomFilename] = useState('')
  const [isExporting, setIsExporting] = useState(false)

  const handleExport = async () => {
    if (messages.length === 0) return

    setIsExporting(true)
    try {
      // Current date formatted as YYYY-MM-DD
      const currentDate = new Date().toISOString().split('T')[0]

      // Export to JSON with full message structure
      const jsonData = {
        id: conversationId,
        timestamp: conversationCreated,
        createdBy: conversationCreatedBy || 'unknown',
        exportedAt: new Date().toISOString(),
        model: selectedModel || 'unknown',
        messageCount: messages.length,
        messages: messages.map((msg, index) => ({
          index: index + 1,
          role: msg.role,
          content: msg.content,
          timestamp: msg.timestamp,
          formattedTime: formatTimestamp(msg.timestamp),
          model: msg.model,
          reasoning_content: msg.reasoning_content,  // Now properly handles null values
          error: msg.error
        }))
      }

      const content = JSON.stringify(jsonData, null, 2)
      const mimeType = 'application/json'
      const fileExtension = 'json'

      // Generate digest from content using SHA-256
      const digest = await generateSHA256Digest(content)

      // Determine filename
      let filename: string
      if (customFilename.trim()) {
        // Remove any existing extension from custom filename
        const baseFilename = customFilename.replace(/\.json$/i, '')
        filename = `${baseFilename}_${currentDate}_${digest}.${fileExtension}`
      } else {
        filename = `conversation_${currentDate}_${digest}.${fileExtension}`
      }

      // Create and download file
      const blob = new Blob([content], { type: mimeType })
      const url = URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url
      a.download = filename
      document.body.appendChild(a)
      a.click()
      document.body.removeChild(a)
      URL.revokeObjectURL(url)

      // Close dialog after successful export
      onClose()
      setCustomFilename('')
    } catch (error) {
      console.error('Export failed:', error)
    } finally {
      setIsExporting(false)
    }
  }

  const handleClose = () => {
    if (!isExporting) {
      onClose()
      setCustomFilename('')
    }
  }

  const handleKeyDown = (e: React.KeyboardEvent) => {
    // Close on Escape if not exporting
    if (e.key === 'Escape' && !isExporting) {
      e.preventDefault()
      handleClose()
    }
  }

  return (
    <Dialog open={isOpen} onOpenChange={(open) => !open && handleClose()}>
      <DialogContent className="sm:max-w-md" onKeyDown={handleKeyDown}>
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <FileDown className="h-4 w-4" />
            Export Conversation
          </DialogTitle>
          <DialogDescription className="space-y-2">
            <div>
              Export your conversation in JSON format. The file will include a unique digest for verification.
            </div>
            {messages.length > 0 && (
              <div className="text-sm text-muted-foreground">
                Ready to export {messages.length} message{messages.length === 1 ? '' : 's'}
                {selectedModel && ` from ${selectedModel}`}
              </div>
            )}
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4 py-4">
          {/* Custom filename input */}
          <div className="space-y-2">
            <Label htmlFor="filename" className="text-sm font-medium">
              Custom Filename (optional)
            </Label>
            <Input
              id="filename"
              value={customFilename}
              onChange={(e) => setCustomFilename(e.target.value)}
              placeholder="my-conversation"
              disabled={isExporting}
              className="text-sm"
            />
            <p className="text-xs text-muted-foreground">
              Date and digest will be automatically added. Extension will be set to .json
            </p>
          </div>

          {/* Export button */}
          <div className="flex justify-center">
            <Button
              onClick={handleExport}
              disabled={messages.length === 0 || isExporting}
              variant="outline"
              className="flex flex-col items-center justify-center gap-2 h-28 w-48"
            >
              <FileJson className="h-8 w-8 text-green-600" />
              <div className="text-center">
                <div className="font-medium">Export as JSON</div>
                <div className="text-xs text-muted-foreground">Structured data format</div>
              </div>
            </Button>
          </div>

          {isExporting && (
            <div className="flex items-center justify-center gap-2 py-2">
              <div className="animate-spin h-4 w-4 border-2 border-primary border-t-transparent rounded-full"></div>
              <span className="text-sm text-muted-foreground">Generating export file...</span>
            </div>
          )}
        </div>

        <DialogFooter className="gap-2">
          <Button
            variant="secondary"
            onClick={handleClose}
            disabled={isExporting}
          >
            Cancel
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
