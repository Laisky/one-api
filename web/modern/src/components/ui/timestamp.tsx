import { type ReactNode } from 'react';
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip';
import { cn, formatTimestamp } from '@/lib/utils';

// TimestampDisplayProps describes the configuration options for rendering a timestamp with its UTC tooltip counterpart.
export interface TimestampDisplayProps {
  timestamp?: number | null;
  className?: string;
  children?: ReactNode;
  fallback?: ReactNode;
  title?: string;
  tooltipPrefix?: string;
}

// TimestampDisplay renders a formatted local timestamp and shows the UTC equivalent in a tooltip when hovered.
export function TimestampDisplay({ timestamp, className, children, fallback = '-', title, tooltipPrefix }: TimestampDisplayProps) {
  const isValid = typeof timestamp === 'number' && Number.isFinite(timestamp) && timestamp > 0;
  if (!isValid) {
    return (
      <span className={cn('inline-flex', className)} title={title}>
        {children ?? fallback}
      </span>
    );
  }

  const localDisplay = formatTimestamp(timestamp);
  const utcDisplay = formatTimestamp(timestamp, { timeZone: 'UTC' });
  const tooltipLabel = tooltipPrefix ? `${tooltipPrefix}: ${utcDisplay}` : `${utcDisplay}Z`;

  return (
    <TooltipProvider delayDuration={150}>
      <Tooltip>
        <TooltipTrigger asChild>
          <span className={cn('inline-flex', className)} title={title}>
            {children ?? localDisplay}
          </span>
        </TooltipTrigger>
        <TooltipContent>
          <span className="font-mono text-xs">{tooltipLabel}</span>
        </TooltipContent>
      </Tooltip>
    </TooltipProvider>
  );
}
