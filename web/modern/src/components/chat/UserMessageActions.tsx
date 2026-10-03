import { Button } from '@/components/ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { Copy, Edit2, MoreHorizontal, Trash2 } from 'lucide-react';
import { useTranslation } from 'react-i18next';

interface UserMessageActionsProps {
  onCopyMessage: () => void;
  onEditMessage?: () => void;
  onDeleteMessage?: () => void;
}

export function UserMessageActions({ onCopyMessage, onEditMessage, onDeleteMessage }: UserMessageActionsProps) {
  const { t } = useTranslation();

  return (
    <div className="opacity-0 group-hover:opacity-100 transition-opacity duration-200">
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button
            variant="ghost"
            size="sm"
            className="h-6 w-6 p-0 text-muted-foreground hover:text-foreground hover:bg-muted/50"
            aria-label={t('playground.actions.user_message_options')}
          >
            <MoreHorizontal className="h-3 w-3" />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end" className="w-48">
          <DropdownMenuItem onClick={onCopyMessage}>
            <Copy className="mr-2 h-4 w-4" />
            {t('playground.actions.copy_message')}
          </DropdownMenuItem>
          {onEditMessage && (
            <DropdownMenuItem onClick={onEditMessage}>
              <Edit2 className="mr-2 h-4 w-4" />
              {t('playground.actions.edit_message')}
            </DropdownMenuItem>
          )}
          {onDeleteMessage && (
            <>
              <DropdownMenuSeparator />
              <DropdownMenuItem onClick={onDeleteMessage} className="text-destructive focus:text-destructive">
                <Trash2 className="mr-2 h-4 w-4" />
                {t('playground.actions.delete_message')}
              </DropdownMenuItem>
            </>
          )}
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
  );
}
