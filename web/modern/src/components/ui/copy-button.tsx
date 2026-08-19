import { useState, useEffect, useRef } from 'react';
import { Button } from '@/components/ui/button';
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip';
import { Copy, Check } from 'lucide-react';
import { copyToClipboard } from '@/lib/utils';
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

/**
 * CopyButton renders an icon button that copies text to the clipboard and
 * returns immediate visual feedback through its icon and tooltip.
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
  const [copied, setCopied] = useState(false);
  const [copying, setCopying] = useState(false);
  const [tooltipOpen, setTooltipOpen] = useState(false);
  const timeoutRef = useRef<NodeJS.Timeout | null>(null);

  // Effect to manage icon revert timer and tooltip
  useEffect(() => {
    if (copied) {
      // Clear any existing timeout
      if (timeoutRef.current) {
        clearTimeout(timeoutRef.current);
      }

      // Show tooltip immediately when copied
      setTooltipOpen(true);

      // Set new timeout to revert icon back to copy and hide tooltip after 2 seconds
      timeoutRef.current = setTimeout(() => {
        setCopied(false);
        setTooltipOpen(false);
        timeoutRef.current = null;
      }, 2000);
    }

    // Cleanup function to clear timeout on unmount or when copied state changes
    return () => {
      if (timeoutRef.current) {
        clearTimeout(timeoutRef.current);
        timeoutRef.current = null;
      }
    };
  }, [copied]);

  const handleCopy = async (e: React.MouseEvent) => {
    e.stopPropagation(); // Prevent row selection or other parent click handlers

    if (copying || copied) return;

    setCopying(true);
    try {
      await copyToClipboard(text);

      // Show success icon immediately after successful copy
      setCopied(true);

      onCopySuccess?.();
    } catch (error) {
      console.error('Failed to copy to clipboard:', error);
      onCopyError?.(error instanceof Error ? error : new Error(t('common.copy_failed', 'Copy failed')));
    } finally {
      setCopying(false);
    }
  };

  return (
    <TooltipProvider>
      <Tooltip
        open={tooltipOpen}
        onOpenChange={(open) => {
          // Only allow closing the tooltip, not opening it manually
          // Tooltip should only open programmatically when copy succeeds
          if (!open) {
            setTooltipOpen(false);
          }
        }}
      >
        <TooltipTrigger asChild>
          <Button
            variant={variant}
            size={size}
            onPointerDown={(e) => e.stopPropagation()}
            onClick={handleCopy}
            className={`${className} transition-colors duration-200 ${copied ? 'text-success hover:text-success/80' : ''}`}
            disabled={copying}
            title={label ?? t('common.copy_to_clipboard', 'Copy to clipboard')}
            aria-label={label ?? t('common.copy_to_clipboard', 'Copy to clipboard')}
          >
            {copied ? <Check className="h-3 w-3" /> : <Copy className="h-3 w-3" />}
          </Button>
        </TooltipTrigger>
        <TooltipContent className="flex items-center gap-1">
          <Check className="h-3 w-3" />
          {successMessage ?? t('common.copied', 'Copied!')}
        </TooltipContent>
      </Tooltip>
    </TooltipProvider>
  );
}

export default CopyButton;
