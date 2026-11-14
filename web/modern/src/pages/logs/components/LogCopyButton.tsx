import { CopyButton } from '@/components/ui/copy-button';

/** LogCopyButtonProps supplies clipboard text and an accessible label to the log-table copy action. */
interface LogCopyButtonProps {
  text: string;
  label: string;
}

/**
 * logCopyFailed records a failed log reference copy as a plain string; the
 * shared CopyButton already shows and announces the failure to the user.
 */
function logCopyFailed() {
  console.error('Failed to copy log reference');
}

/**
 * LogCopyButton renders the log-table copy action on top of the shared
 * CopyButton, so the copy uses the plain-HTTP-safe clipboard helper, never
 * propagates to the row, and both success and failure are shown and announced.
 *
 * @param props - The text to copy and the caller-localized accessible label.
 * @returns The isolated, accessible copy control.
 */
export function LogCopyButton({ text, label }: LogCopyButtonProps) {
  return <CopyButton text={text} label={label} className="h-6 w-6 p-0" onCopyError={logCopyFailed} />;
}
