import { Copy, Loader2 } from 'lucide-react';
import { useTranslation } from 'react-i18next';

import { ListActionButton } from '@/components/ui/list-action-button';
import { cn } from '@/lib/utils';

/** DuplicateActionProps describes the same duplicate interaction in full and compact row layouts. */
interface DuplicateActionProps {
  onDuplicate: () => void | Promise<void>;
  pending?: boolean;
  compact?: boolean;
  className?: string;
}

/** DuplicateAction renders shared labels, icons, touch targets, and accessible progress for tokens and channels. */
export function DuplicateAction({ onDuplicate, pending = false, compact = false, className }: DuplicateActionProps) {
  const { t } = useTranslation();
  const label = pending ? t('duplicate_action.pending', 'Duplicating...') : t('duplicate_action.action', 'Duplicate');
  return (
    <ListActionButton
      type="button"
      variant={compact ? 'ghost' : 'outline'}
      size={compact ? 'icon' : 'sm'}
      className={cn('touch-target gap-1', className)}
      onClick={() => void onDuplicate()}
      disabled={pending}
      aria-busy={pending}
      aria-label={label}
      title={label}
      icon={
        pending ? (
          <Loader2 aria-hidden="true" className="h-4 w-4 animate-spin" />
        ) : (
          <Copy aria-hidden="true" className="h-4 w-4" />
        )
      }
    >
      {compact ? null : label}
    </ListActionButton>
  );
}
