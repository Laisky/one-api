import { Button } from '@/components/ui/button';
import { Copy } from 'lucide-react';

/** LogCopyButtonProps supplies clipboard text to a compact log-table copy action. */
interface LogCopyButtonProps {
  text: string;
}

/** LogCopyButton copies its supplied text to the browser clipboard. */
export function LogCopyButton({ text }: LogCopyButtonProps) {
  return (
    <Button size="sm" variant="ghost" className="h-6 w-6 p-0" onClick={() => navigator.clipboard.writeText(text)}>
      <Copy className="h-3 w-3" />
    </Button>
  );
}
