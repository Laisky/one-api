import { Input } from '@/components/ui/input';
import { useEffect, useState } from 'react';

/** ChannelPriorityCellProps configures an editable priority cell and its commit callback. */
interface ChannelPriorityCellProps {
  value: number;
  ariaLabel: string;
  onCommit: (value: number) => void;
}

/** ChannelPriorityCell renders an editable priority that commits changed numeric values on blur or Enter. */
export function ChannelPriorityCell({ value, ariaLabel, onCommit }: ChannelPriorityCellProps) {
  const [draft, setDraft] = useState<string>(String(value));
  useEffect(() => {
    setDraft(String(value));
  }, [value]);

  /** commit validates the draft and reports a changed numeric priority to the parent page. */
  const commit = () => {
    const parsed = parseInt(draft.trim(), 10);
    if (!Number.isFinite(parsed)) {
      setDraft(String(value));
      return;
    }
    if (parsed === value) return;
    onCommit(parsed);
  };

  return (
    <Input
      type="number"
      value={draft}
      aria-label={ariaLabel}
      className="h-8 w-20 font-mono text-sm"
      onChange={(event) => setDraft(event.target.value)}
      onBlur={commit}
      onKeyDown={(event) => {
        if (event.key === 'Enter') {
          event.preventDefault();
          (event.target as HTMLInputElement).blur();
        }
      }}
    />
  );
}
