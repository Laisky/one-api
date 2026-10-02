import { Button } from '@/components/ui/button';
import { Copy } from 'lucide-react';
import type { MouseEvent } from 'react';

/** LogCopyButtonProps supplies clipboard text and an accessible label to the log-table copy action. */
interface LogCopyButtonProps {
  text: string;
  label: string;
}

/** LogCopyButton accepts clipboard text and a label, then returns an isolated, accessible copy control. */
export function LogCopyButton({ text, label }: LogCopyButtonProps) {
  /** handleCopy accepts a button click, stops row propagation, starts the copy, and returns no value. */
  const handleCopy = (event: MouseEvent<HTMLButtonElement>) => {
    event.stopPropagation();
    void navigator.clipboard.writeText(text).catch(() => {
      console.error('Failed to copy log reference');
    });
  };

  return (
    <Button size="sm" variant="ghost" className="h-6 w-6 p-0" aria-label={label} title={label} onClick={handleCopy}>
      <Copy className="h-3 w-3" />
    </Button>
  );
}
