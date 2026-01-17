import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip';
import { cn } from '@/lib/utils';
import { Info, X } from 'lucide-react';
import type { ReactNode } from 'react';
import { useMemo, useState } from 'react';

// SelectionListOption describes a selectable option displayed in the manager.
export interface SelectionListOption {
  value: string;
  label?: string;
}

// SelectionListManagerProps configures labels, data, and behavior for the manager.
export interface SelectionListManagerProps {
  label: string;
  help?: string;
  options: SelectionListOption[];
  selected: string[];
  onChange: (next: string[]) => void;
  searchPlaceholder?: string;
  customPlaceholder?: string;
  addLabel?: string;
  actions?: ReactNode;
  selectedSummaryLabel?: (count: number) => string;
  emptySelectedLabel?: string;
  noOptionsLabel?: string;
  disabled?: boolean;
}

/**
 * SelectionListManager renders a searchable multi-select list with optional custom additions.
 * It supports toggling from a catalog, adding ad-hoc values, and showing selected items.
 */
export function SelectionListManager({
  label,
  help,
  options,
  selected,
  onChange,
  searchPlaceholder,
  customPlaceholder,
  addLabel,
  actions,
  selectedSummaryLabel,
  emptySelectedLabel,
  noOptionsLabel,
  disabled,
}: SelectionListManagerProps) {
  const [searchTerm, setSearchTerm] = useState('');
  const [customValue, setCustomValue] = useState('');

  const normalizedSelected = useMemo(() => selected.map((item) => item.trim()).filter((item) => item.length > 0), [selected]);

  const filteredOptions = useMemo(() => {
    if (!searchTerm.trim()) return options;
    const keyword = searchTerm.trim().toLowerCase();
    return options.filter((option) => {
      const labelText = option.label ?? option.value;
      return labelText.toLowerCase().includes(keyword) || option.value.toLowerCase().includes(keyword);
    });
  }, [options, searchTerm]);

  const toggleOption = (value: string) => {
    if (disabled) return;
    if (normalizedSelected.includes(value)) {
      onChange(normalizedSelected.filter((item) => item !== value));
      return;
    }
    onChange([...normalizedSelected, value]);
  };

  const addCustomValue = () => {
    if (disabled) return;
    const next = customValue.trim();
    if (!next) return;
    if (!normalizedSelected.includes(next)) {
      onChange([...normalizedSelected, next]);
    }
    setCustomValue('');
  };

  const removeSelected = (value: string) => {
    if (disabled) return;
    onChange(normalizedSelected.filter((item) => item !== value));
  };

  const selectedSummary = selectedSummaryLabel
    ? selectedSummaryLabel(normalizedSelected.length)
    : `Selected (${normalizedSelected.length})`;

  return (
    <TooltipProvider>
      <div className="space-y-4">
        <div className="flex items-center gap-1">
          <Label className="text-sm font-medium">{label}</Label>
          {help && (
            <Tooltip>
              <TooltipTrigger asChild>
                <Info className="h-4 w-4 text-muted-foreground cursor-help" aria-label={`Help: ${label}`} />
              </TooltipTrigger>
              <TooltipContent className="max-w-xs whitespace-pre-line">{help}</TooltipContent>
            </Tooltip>
          )}
        </div>

        {actions && <div className="flex flex-wrap gap-2">{actions}</div>}

        <div className="flex gap-2 flex-wrap">
          <Input
            value={searchTerm}
            onChange={(event) => setSearchTerm(event.target.value)}
            placeholder={searchPlaceholder}
            disabled={disabled}
            className="flex-1 min-w-[220px]"
          />
          {customPlaceholder && (
            <div className="flex gap-2 flex-1 min-w-[260px]">
              <Input
                value={customValue}
                onChange={(event) => setCustomValue(event.target.value)}
                placeholder={customPlaceholder}
                disabled={disabled}
                onKeyDown={(event) => {
                  if (event.key === 'Enter') {
                    event.preventDefault();
                    addCustomValue();
                  }
                }}
              />
              <Button type="button" variant="secondary" onClick={addCustomValue} disabled={disabled}>
                {addLabel || 'Add'}
              </Button>
            </div>
          )}
        </div>

        <div className="max-h-[200px] overflow-y-auto border rounded p-2 bg-muted/10">
          <div className="flex flex-wrap gap-2">
            {filteredOptions.length === 0 && <span className="text-xs text-muted-foreground">{noOptionsLabel || 'No options'}</span>}
            {filteredOptions.map((option) => {
              const isSelected = normalizedSelected.includes(option.value);
              return (
                <Badge
                  key={option.value}
                  variant={isSelected ? 'default' : 'outline'}
                  className={cn('cursor-pointer hover:bg-primary/90', disabled && 'opacity-60')}
                  onClick={() => toggleOption(option.value)}
                >
                  {option.label ?? option.value}
                </Badge>
              );
            })}
          </div>
        </div>

        <div className="space-y-2">
          <div className="text-sm font-medium text-muted-foreground">{selectedSummary}</div>
          <div className="flex flex-wrap gap-2 min-h-[40px] p-2 border rounded bg-background">
            {normalizedSelected.length === 0 && (
              <span className="text-sm text-muted-foreground italic p-1">{emptySelectedLabel || 'No selections'}</span>
            )}
            {normalizedSelected.map((item) => (
              <Badge key={item} variant="secondary" className="gap-1">
                {item}
                <button
                  type="button"
                  onClick={() => removeSelected(item)}
                  className="ml-1 inline-flex"
                  aria-label={`Remove ${item}`}
                  disabled={disabled}
                >
                  <X className="h-3 w-3" />
                </button>
              </Badge>
            ))}
          </div>
        </div>
      </div>
    </TooltipProvider>
  );
}
