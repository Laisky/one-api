import { useEffect, useRef, useState, type MouseEvent } from 'react';
import { Button } from '@/components/ui/button';
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip';
import { Copy, Check, X } from 'lucide-react';
import { cn, copyToClipboard } from '@/lib/utils';
import { useTranslation } from 'react-i18next';

/**
 * CopyButtonProps describes the text to copy, optional accessible labels,
 * visual button options, and callbacks invoked after copy attempts.
 */
interface CopyButtonProps {
  text: string;
  label?: string;
  variant?: 'ghost' | 'outline' | 'default' | 'destructive' | 'secondary';
  size?: 'sm' | 'md' | 'lg' | 'icon';
  className?: string;
  successMessage?: string;
  onCopySuccess?: () => void;
  onCopyError?: (error: Error) => void;
}

/** CopyStatus is the visible and announced outcome of the latest copy attempt. */
type CopyStatus = 'idle' | 'copied' | 'failed';

/** COPY_FEEDBACK_MS is how long the copied/failed feedback stays visible and announced. */
const COPY_FEEDBACK_MS = 2000;

/**
 * CopyButton renders an icon button that copies text to the clipboard. It
 * reports both success and failure through the icon, a tooltip bubble, and an
 * always-mounted polite live region placed outside the button, so sighted and
 * screen reader users get the same message. The button is never disabled while
 * feedback shows, and repeated clicks restart a single feedback timer.
 *
 * @param props - The text to copy, the accessible label, styling options, and outcome callbacks.
 * @returns The copy button with its tooltip and status live region.
 */
export function CopyButton({
  text,
  label,
  variant = 'ghost',
  size = 'sm',
  className = 'h-6 w-6 p-0',
  successMessage,
  onCopySuccess,
  onCopyError,
}: CopyButtonProps) {
  const { t } = useTranslation();
  const [status, setStatus] = useState<CopyStatus>('idle');
  const [announcement, setAnnouncement] = useState('');
  const [tooltipOpen, setTooltipOpen] = useState(false);
  const timeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const reportCountRef = useRef(0);
  const mountedRef = useRef(true);

  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
      if (timeoutRef.current) {
        clearTimeout(timeoutRef.current);
        timeoutRef.current = null;
      }
    };
  }, []);

  const accessibleLabel = label ?? t('common.copy_to_clipboard', 'Copy to clipboard');
  const copiedMessage = successMessage ?? t('common.copied', 'Copied!');
  const failedMessage = t('common.copy_failed', 'Copy failed');

  /**
   * report shows the outcome of one copy attempt and restarts the single feedback timer.
   * Consecutive messages alternate a trailing no-break space so assistive technologies
   * that skip an identical repeated message (VoiceOver) still announce a repeat copy.
   *
   * @param ok - True when the text reached the clipboard.
   */
  const report = (ok: boolean) => {
    if (!mountedRef.current) return;
    if (timeoutRef.current) {
      clearTimeout(timeoutRef.current);
    }
    reportCountRef.current += 1;
    const message = ok ? copiedMessage : failedMessage;
    setAnnouncement(reportCountRef.current % 2 === 0 ? `${message}\u00A0` : message);
    setStatus(ok ? 'copied' : 'failed');
    setTooltipOpen(true);
    timeoutRef.current = setTimeout(() => {
      setStatus('idle');
      setAnnouncement('');
      setTooltipOpen(false);
      timeoutRef.current = null;
    }, COPY_FEEDBACK_MS);
  };

  /**
   * handleCopy copies the text and reports the outcome; it stops propagation so
   * parent row handlers never fire.
   *
   * @param event - The click event from the copy button.
   */
  const handleCopy = async (event: MouseEvent<HTMLButtonElement>) => {
    event.stopPropagation(); // Prevent row selection or other parent click handlers

    try {
      await copyToClipboard(text);
    } catch (error) {
      report(false);
      if (onCopyError) {
        onCopyError(error instanceof Error ? error : new Error(failedMessage));
      } else {
        console.error(`Failed to copy to clipboard: ${error instanceof Error ? error.message : String(error)}`);
      }
      return;
    }

    report(true);
    onCopySuccess?.();
  };

  const StatusIcon = status === 'copied' ? Check : status === 'failed' ? X : Copy;

  return (
    <>
      <TooltipProvider>
        <Tooltip
          open={tooltipOpen}
          onOpenChange={(open) => {
            // Only allow closing the tooltip; it opens programmatically after a copy attempt.
            if (!open) {
              setTooltipOpen(false);
            }
          }}
        >
          <TooltipTrigger asChild>
            <Button
              type="button"
              variant={variant}
              size={size}
              onPointerDown={(e) => e.stopPropagation()}
              onClick={handleCopy}
              className={cn(
                className,
                'transition-colors duration-200',
                status === 'copied' && 'text-success hover:text-success/80',
                status === 'failed' && 'text-destructive hover:text-destructive/80'
              )}
              data-copied={status === 'copied' ? '' : undefined}
              data-failed={status === 'failed' ? '' : undefined}
              title={accessibleLabel}
              aria-label={accessibleLabel}
            >
              <StatusIcon className="h-3 w-3" aria-hidden="true" />
            </Button>
          </TooltipTrigger>
          <TooltipContent className="flex items-center gap-1">
            {status === 'failed' ? <X className="h-3 w-3" aria-hidden="true" /> : <Check className="h-3 w-3" aria-hidden="true" />}
            {status === 'failed' ? failedMessage : copiedMessage}
          </TooltipContent>
        </Tooltip>
      </TooltipProvider>
      {/* The live region stays mounted and outside the button: button children are presentational. */}
      <span role="status" className="sr-only">
        {announcement}
      </span>
    </>
  );
}

export default CopyButton;
