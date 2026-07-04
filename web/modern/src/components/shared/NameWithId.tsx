import type { ReactNode } from 'react';

import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip';
import { cn } from '@/lib/utils';

/**
 * NameWithIdProps defines the visible label, optional resource id, tooltip
 * label, and additional class names used by NameWithId.
 */
interface NameWithIdProps {
  /** Primary display content for the row (name, username, ...). */
  name: ReactNode;
  /** External reference id (UUID, or numeric id fallback) surfaced on hover. */
  refId?: string | number | null;
  /** Short label shown before the id inside the tooltip (e.g. "ID"). */
  idLabel?: string;
  className?: string;
}

/**
 * NameWithId renders a resource's primary name and exposes its external id
 * through a hover tooltip, so the id no longer needs its own table column. It
 * falls back to a plain name when no id is available.
 */
export function NameWithId({ name, refId, idLabel = 'ID', className }: NameWithIdProps) {
  const id = refId == null ? '' : String(refId);
  if (!id) {
    return <span className={cn('font-medium', className)}>{name}</span>;
  }

  return (
    <TooltipProvider delayDuration={150}>
      <Tooltip>
        <TooltipTrigger asChild>
          <span
            className={cn(
              'font-medium cursor-help underline decoration-dotted underline-offset-4',
              className,
            )}
          >
            {name}
          </span>
        </TooltipTrigger>
        <TooltipContent align="start">
          <span className="font-mono text-xs break-all">
            {idLabel}: {id}
          </span>
        </TooltipContent>
      </Tooltip>
    </TooltipProvider>
  );
}

export default NameWithId;
