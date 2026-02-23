import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { X } from 'lucide-react';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';

interface ToolListEditorProps {
  label: string;
  description?: string;
  value: string[];
  onChange: (next: string[]) => void;
  placeholder?: string;
  disabled?: boolean;
  addLabel?: string;
}

export function ToolListEditor({ label, description, value, onChange, placeholder, disabled, addLabel }: ToolListEditorProps) {
  const [draft, setDraft] = useState('');
  const { t } = useTranslation();

  const normalized = value.map((item) => item.trim()).filter((item) => item.length > 0);

  const addItem = () => {
    const next = draft.trim();
    if (!next) return;
    if (!normalized.includes(next)) {
      onChange([...normalized, next]);
    }
    setDraft('');
  };

  const removeItem = (item: string) => {
    onChange(normalized.filter((entry) => entry !== item));
  };

  return (
    <div className="space-y-2">
      <div>
        <Label className="text-sm font-medium">{label}</Label>
        {description && <p className="text-xs text-muted-foreground mt-1">{description}</p>}
      </div>
      <div className="flex gap-2">
        <Input
          value={draft}
          onChange={(event) => setDraft(event.target.value)}
          placeholder={placeholder}
          disabled={disabled}
          onKeyDown={(event) => {
            if (event.key === 'Enter') {
              event.preventDefault();
              addItem();
            }
          }}
        />
        <Button type="button" variant="secondary" onClick={addItem} disabled={disabled}>
          {addLabel || 'Add'}
        </Button>
      </div>
      <div className="flex flex-wrap gap-2">
        {normalized.length === 0 && <span className="text-xs text-muted-foreground">{t('common.no_items', 'No items')}</span>}
        {normalized.map((item) => (
          <Badge key={item} variant="secondary" className="gap-1 max-w-full">
            <span className="truncate" title={item}>{item}</span>
            <button
              type="button"
              onClick={() => removeItem(item)}
              className="ml-1 inline-flex shrink-0"
              aria-label={t('common.remove_item', 'Remove {{item}}', { item })}
              disabled={disabled}
            >
              <X className="h-3 w-3" />
            </button>
          </Badge>
        ))}
      </div>
    </div>
  );
}
