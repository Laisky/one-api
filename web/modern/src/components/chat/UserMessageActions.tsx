import React from 'react'
import { Copy, Edit2, Trash2, MoreHorizontal } from 'lucide-react'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'

interface UserMessageActionsProps {
  onCopyMessage: () => void
  onEditMessage?: () => void
  onDeleteMessage?: () => void
}

export function UserMessageActions({
  onCopyMessage,
  onEditMessage,
  onDeleteMessage
}: UserMessageActionsProps) {
  return (
    <div className="opacity-0 group-hover:opacity-100 transition-opacity duration-200">
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button
            variant="ghost"
            size="sm"
            className="h-6 w-6 p-0 text-muted-foreground hover:text-foreground hover:bg-muted/50"
            aria-label="User message options"
          >
            <MoreHorizontal className="h-3 w-3" />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end" className="w-48">
          <DropdownMenuItem onClick={onCopyMessage}>
            <Copy className="mr-2 h-4 w-4" />
            Copy Message
          </DropdownMenuItem>
          {onEditMessage && (
            <DropdownMenuItem onClick={onEditMessage}>
              <Edit2 className="mr-2 h-4 w-4" />
              Edit Message
            </DropdownMenuItem>
          )}
          {onDeleteMessage && (
            <>
              <DropdownMenuSeparator />
              <DropdownMenuItem onClick={onDeleteMessage} className="text-destructive focus:text-destructive">
                <Trash2 className="mr-2 h-4 w-4" />
                Delete Message
              </DropdownMenuItem>
            </>
          )}
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
  )
}
