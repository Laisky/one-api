import { Button } from '@/components/ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { Copy, Edit2, MoreHorizontal, RotateCcw, Trash2 } from 'lucide-react';
import { useTranslation } from 'react-i18next';

interface AssistantMessageActionsProps {
  onCopyMessage: () => void;
  onRegenerateMessage?: () => void;
  onEditMessage?: () => void;
  onDeleteMessage?: () => void;
  isStreaming?: boolean;
}

export function AssistantMessageActions({
  onCopyMessage,
  onRegenerateMessage,
  onEditMessage,
  onDeleteMessage,
  isStreaming = false,
}: AssistantMessageActionsProps) {
  const { t } = useTranslation();

  return (
    <div className="opacity-0 group-hover:opacity-100 transition-opacity duration-200">
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button
            variant="ghost"
            size="sm"
            className="h-6 w-6 p-0 text-muted-foreground hover:text-foreground hover:bg-muted/50"
            aria-label={t('playground.actions.assistant_message_options')}
          >
            <MoreHorizontal className="h-3 w-3" />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end" className="w-52">
          <DropdownMenuItem onClick={onCopyMessage}>
            <Copy className="mr-2 h-4 w-4" />
            {t('playground.actions.copy_response')}
          </DropdownMenuItem>
          {onRegenerateMessage && !isStreaming && (
            <DropdownMenuItem onClick={onRegenerateMessage}>
              <RotateCcw className="mr-2 h-4 w-4" />
              {t('playground.actions.regenerate_response')}
            </DropdownMenuItem>
          )}
          {onEditMessage && (
            <DropdownMenuItem onClick={onEditMessage}>
              <Edit2 className="mr-2 h-4 w-4" />
              {t('playground.actions.edit_response')}
            </DropdownMenuItem>
          )}
          {onDeleteMessage && (
            <>
              <DropdownMenuSeparator />
              <DropdownMenuItem onClick={onDeleteMessage} className="text-destructive focus:text-destructive">
                <Trash2 className="mr-2 h-4 w-4" />
                {t('playground.actions.delete_response')}
              </DropdownMenuItem>
            </>
          )}
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
  );
}
