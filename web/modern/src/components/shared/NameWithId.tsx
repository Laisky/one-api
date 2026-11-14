import { Check, Copy, X } from 'lucide-react';
import { useEffect, useRef, useState, type MouseEvent, type ReactNode, type TouchEvent } from 'react';
import { useTranslation } from 'react-i18next';

import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip';
import { cn, copyToClipboard } from '@/lib/utils';

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
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const [touchIdVisible, setTouchIdVisible] = useState(false);
  const [copyStatus, setCopyStatus] = useState<'idle' | 'copied' | 'failed'>('idle');
  const [copyAnnouncement, setCopyAnnouncement] = useState('');
  const triggerRef = useRef<HTMLButtonElement | null>(null);
  const lastTouchAtRef = useRef(0);
  const copyResetRef = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  const copyCountRef = useRef(0);
  const mountedRef = useRef(false);
  const id = refId == null ? '' : String(refId);
  const copyLabel = t('common.copy_id', 'Copy ID');
  const copyStatusText =
    copyStatus === 'copied' ? t('common.copied', 'Copied!') : copyStatus === 'failed' ? t('common.copy_failed', 'Copy failed') : copyLabel;

  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
      if (copyResetRef.current) clearTimeout(copyResetRef.current);
    };
  }, []);

  /**
   * handleCopy copies the reference id and reports the real outcome through the
   * icon, the title and the polite status region, restarting one reset timer.
   *
   * @param event - The click event from the copy button.
   */
  const handleCopy = async (event: MouseEvent<HTMLButtonElement>) => {
    event.preventDefault();
    event.stopPropagation();
    let outcome: 'copied' | 'failed' = 'copied';
    try {
      await copyToClipboard(id);
    } catch {
      console.error('Failed to copy reference id');
      outcome = 'failed';
    }
    if (!mountedRef.current) return;

    // Alternate a no-break space so VoiceOver repeats an identical message.
    copyCountRef.current += 1;
    const message = outcome === 'copied' ? t('common.copied', 'Copied!') : t('common.copy_failed', 'Copy failed');
    setCopyStatus(outcome);
    setCopyAnnouncement(copyCountRef.current % 2 === 0 ? `${message}\u00A0` : message);
    if (copyResetRef.current) clearTimeout(copyResetRef.current);
    copyResetRef.current = setTimeout(() => {
      setCopyStatus('idle');
      setCopyAnnouncement('');
    }, 1500);
  };

  const copyButton = (
    <button
      type="button"
      onClick={handleCopy}
      aria-label={copyLabel}
      title={copyStatusText}
      data-copied={copyStatus === 'copied' ? '' : undefined}
      data-failed={copyStatus === 'failed' ? '' : undefined}
      className="inline-flex shrink-0 items-center border-0 bg-transparent p-0 text-current opacity-70 hover:opacity-100 data-[failed]:text-destructive"
    >
      {copyStatus === 'copied' ? (
        <Check className="h-3 w-3" aria-hidden="true" />
      ) : copyStatus === 'failed' ? (
        <X className="h-3 w-3" aria-hidden="true" />
      ) : (
        <Copy className="h-3 w-3" aria-hidden="true" />
      )}
    </button>
  );

  useEffect(() => {
    const trigger = triggerRef.current;
    if (!trigger) return undefined;

    const revealFromTouch = (event: globalThis.TouchEvent | PointerEvent) => {
      if ('pointerType' in event && event.pointerType !== 'touch' && event.pointerType !== 'pen') {
        return;
      }
      event.stopPropagation();
      lastTouchAtRef.current = Date.now();
      setOpen(false);
      setTouchIdVisible(true);
    };

    trigger.addEventListener('touchend', revealFromTouch);
    trigger.addEventListener('pointerup', revealFromTouch);

    return () => {
      trigger.removeEventListener('touchend', revealFromTouch);
      trigger.removeEventListener('pointerup', revealFromTouch);
    };
  }, []);

  if (!id) {
    return <span className={cn('font-medium', className)}>{name}</span>;
  }

  return (
    <TooltipProvider delayDuration={150}>
      <Tooltip open={open} onOpenChange={setOpen}>
        <TooltipTrigger asChild>
          <button
            ref={triggerRef}
            type="button"
            className={cn(
              'inline max-w-full border-0 bg-transparent p-0 text-left font-medium text-current cursor-help underline decoration-dotted underline-offset-4',
              className,
            )}
            onClick={(event: MouseEvent<HTMLButtonElement>) => {
              event.stopPropagation();
              if (Date.now() - lastTouchAtRef.current < 700) {
                setOpen(false);
                setTouchIdVisible(true);
                return;
              }
              setTouchIdVisible(false);
              setOpen(true);
            }}
            onTouchEnd={(event: TouchEvent<HTMLButtonElement>) => {
              event.stopPropagation();
              setOpen(false);
              setTouchIdVisible(true);
            }}
          >
            {name}
          </button>
        </TooltipTrigger>
        <TooltipContent align="start">
          <span className="inline-flex max-w-full items-center gap-2">
            <span className="font-mono text-xs break-all">
              {idLabel}: {id}
            </span>
            {copyButton}
          </span>
        </TooltipContent>
      </Tooltip>
      {touchIdVisible && (
        <span className="mt-1 flex max-w-full items-center gap-2 font-mono text-xs text-muted-foreground break-all sm:hidden">
          <span className="break-all">
            {idLabel}: {id}
          </span>
          {copyButton}
        </span>
      )}
      {/* Always-mounted polite live region outside the button, so copy outcomes are announced. */}
      <span role="status" className="sr-only">
        {copyAnnouncement}
      </span>
    </TooltipProvider>
  );
}

export default NameWithId;
