import React from 'react'
import { Copy, RotateCcw, Edit2, Trash2, MoreHorizontal } from 'lucide-react'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'

interface AssistantMessageActionsProps {
  onCopyMessage: () => void
  onRegenerateMessage?: () => void
  onEditMessage?: () => void
  onDeleteMessage?: () => void
  isStreaming?: boolean
}

export function AssistantMessageActions({
  onCopyMessage,
  onRegenerateMessage,
  onEditMessage,
  onDeleteMessage,
  isStreaming = false
}: AssistantMessageActionsProps) {
  return (
    <div className="opacity-0 group-hover:opacity-100 transition-opacity duration-200">
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button
            variant="ghost"
            size="sm"
            className="h-6 w-6 p-0 text-muted-foreground hover:text-foreground hover:bg-muted/50"
            aria-label="Assistant message options"
          >
            <MoreHorizontal className="h-3 w-3" />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end" className="w-52">
          <DropdownMenuItem onClick={onCopyMessage}>
            <Copy className="mr-2 h-4 w-4" />
            Copy Response
          </DropdownMenuItem>
          {onRegenerateMessage && !isStreaming && (
            <DropdownMenuItem onClick={onRegenerateMessage}>
              <RotateCcw className="mr-2 h-4 w-4" />
              Regenerate Response
            </DropdownMenuItem>
          )}
          {onEditMessage && (
            <DropdownMenuItem onClick={onEditMessage}>
              <Edit2 className="mr-2 h-4 w-4" />
              Edit Response
            </DropdownMenuItem>
          )}
          {onDeleteMessage && (
            <>
              <DropdownMenuSeparator />
              <DropdownMenuItem onClick={onDeleteMessage} className="text-destructive focus:text-destructive">
                <Trash2 className="mr-2 h-4 w-4" />
                Delete Response
              </DropdownMenuItem>
            </>
          )}
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
  )
}
