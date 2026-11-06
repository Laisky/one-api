import { Input } from '@/components/ui/input';
import { Popover, PopoverAnchor, PopoverContent } from '@/components/ui/popover';
import { cn } from '@/lib/utils';
import { Check, ChevronsUpDown } from 'lucide-react';
import * as React from 'react';
import { filterComboboxValues, mergeComboboxOptions } from './multi-select-combobox-items';

export interface MultiSelectComboboxProps {
  /** options lists the values that may be selected; typed text is only a filter and never becomes a value. */
  options: readonly string[];
  /** selected lists the currently chosen values; they are shown with a check mark. */
  selected: readonly string[];
  /** onAdd is called with an option the user picked. */
  onAdd: (value: string) => void;
  /** onRemove is called with a selected value the user picked again to deselect it. */
  onRemove: (value: string) => void;
  /** placeholder is the input placeholder text. */
  placeholder?: string;
  /** emptyLabel is shown when no option matches the typed text. */
  emptyLabel: string;
  /** id is forwarded to the input so a label can reference it. */
  id?: string;
  /** className is applied to the input wrapper. */
  className?: string;
  /** disabled disables the input and keeps the dropdown closed. */
  disabled?: boolean;
  /** 'aria-label' names the input for assistive technology when no visible label references it. */
  'aria-label'?: string;
}

/**
 * MultiSelectCombobox renders a text input with an autocomplete dropdown for
 * choosing several values. Focusing or clicking the input opens the dropdown
 * with every option, typing filters it, and Enter or a click toggles the
 * highlighted option. Only values from `options` can be added: typed text that
 * matches nothing selects nothing. Selected values stay in the list with a
 * check mark (including selected values missing from `options`, which can be
 * deselected but not re-added); rendering the selection itself (for example as
 * removable badges) is left to the caller.
 *
 * @param props - The options, current selection, change callbacks and labels.
 * @returns The combobox element.
 */
export function MultiSelectCombobox({
  options,
  selected,
  onAdd,
  onRemove,
  placeholder,
  emptyLabel,
  id,
  className,
  disabled = false,
  'aria-label': ariaLabel,
}: MultiSelectComboboxProps) {
  const listboxId = React.useId();
  const [open, setOpen] = React.useState(false);
  const [query, setQuery] = React.useState('');
  const [activeIndex, setActiveIndex] = React.useState(-1);
  const anchorRef = React.useRef<HTMLDivElement>(null);
  const inputRef = React.useRef<HTMLInputElement>(null);

  const selectedSet = React.useMemo(() => new Set(selected), [selected]);
  const values = React.useMemo(() => mergeComboboxOptions(options, selected), [options, selected]);
  const items = React.useMemo(() => filterComboboxValues(values, query), [values, query]);
  const isOpen = open && !disabled;
  const highlighted = activeIndex >= 0 && activeIndex < items.length ? activeIndex : -1;
  /** optionId returns the DOM id of the dropdown row at `index`, used for aria-activedescendant. */
  const optionId = (index: number) => `${listboxId}-option-${index}`;

  React.useEffect(() => {
    if (!isOpen || highlighted < 0) {
      return;
    }
    document.getElementById(`${listboxId}-option-${highlighted}`)?.scrollIntoView({ block: 'nearest' });
  }, [isOpen, highlighted, listboxId]);

  /** commit toggles one listed value, then resets the query. */
  const commit = (value: string) => {
    if (selectedSet.has(value)) {
      onRemove(value);
    } else {
      onAdd(value);
    }
    setQuery('');
    setActiveIndex(-1);
    inputRef.current?.focus();
  };

  /** moveHighlight shifts the highlighted row by `step`, wrapping at both ends. */
  const moveHighlight = (step: 1 | -1) => {
    if (items.length === 0) {
      return;
    }
    setOpen(true);
    if (highlighted < 0) {
      setActiveIndex(step === 1 ? 0 : items.length - 1);
      return;
    }
    setActiveIndex((highlighted + step + items.length) % items.length);
  };

  /** handleKeyDown implements arrow navigation, Enter to pick, and Escape to close; IME composition is ignored. */
  const handleKeyDown = (event: React.KeyboardEvent<HTMLInputElement>) => {
    if (event.nativeEvent.isComposing || event.keyCode === 229) {
      return;
    }
    switch (event.key) {
      case 'ArrowDown':
        event.preventDefault();
        moveHighlight(1);
        break;
      case 'ArrowUp':
        event.preventDefault();
        moveHighlight(-1);
        break;
      case 'Enter': {
        // Never let Enter submit the surrounding form from this input.
        event.preventDefault();
        if (isOpen && highlighted >= 0) {
          commit(items[highlighted]);
          return;
        }
        const typed = query.trim();
        if (typed === '') {
          setOpen(true);
          return;
        }
        // Typed text is only a filter: commit it solely when it names a listed value exactly.
        if (items.includes(typed)) {
          commit(typed);
        }
        break;
      }
      case 'Escape':
        if (isOpen) {
          event.preventDefault();
          setOpen(false);
          setActiveIndex(-1);
        }
        break;
      default:
        break;
    }
  };

  return (
    <Popover open={isOpen} onOpenChange={setOpen}>
      <PopoverAnchor asChild>
        <div ref={anchorRef} className={cn('relative', className)}>
          <Input
            ref={inputRef}
            id={id}
            role="combobox"
            aria-label={ariaLabel}
            aria-expanded={isOpen}
            aria-controls={listboxId}
            aria-autocomplete="list"
            aria-activedescendant={isOpen && highlighted >= 0 ? optionId(highlighted) : undefined}
            autoComplete="off"
            className="pr-8"
            disabled={disabled}
            placeholder={placeholder}
            value={query}
            onChange={(event) => {
              const next = event.target.value;
              setQuery(next);
              setActiveIndex(next.trim() === '' ? -1 : 0);
              setOpen(true);
            }}
            onFocus={() => setOpen(true)}
            onClick={() => setOpen(true)}
            onKeyDown={handleKeyDown}
          />
          <ChevronsUpDown className="pointer-events-none absolute right-2 top-1/2 h-4 w-4 -translate-y-1/2 opacity-50" />
        </div>
      </PopoverAnchor>
      <PopoverContent
        align="start"
        className="w-[var(--radix-popover-trigger-width)] p-1"
        onOpenAutoFocus={(event) => event.preventDefault()}
        onCloseAutoFocus={(event) => event.preventDefault()}
        onInteractOutside={(event) => {
          // The input lives outside the popover content; interacting with it must not dismiss the list.
          if (anchorRef.current?.contains(event.target as Node)) {
            event.preventDefault();
          }
        }}
      >
        {items.length === 0 ? (
          <p className="py-6 text-center text-sm text-muted-foreground">{emptyLabel}</p>
        ) : (
          <ul id={listboxId} role="listbox" aria-multiselectable="true" className="max-h-60 overflow-y-auto">
            {items.map((value, index) => {
              const isSelected = selectedSet.has(value);
              return (
                <li
                  key={value}
                  id={optionId(index)}
                  role="option"
                  aria-selected={isSelected}
                  className={cn(
                    'flex cursor-pointer select-none items-center gap-2 rounded-sm px-2 py-1.5 text-sm',
                    index === highlighted && 'bg-accent text-accent-foreground'
                  )}
                  // Keep focus in the input so the dropdown stays open for further picks.
                  onMouseDown={(event) => event.preventDefault()}
                  onMouseMove={() => {
                    if (index !== highlighted) {
                      setActiveIndex(index);
                    }
                  }}
                  onClick={() => commit(value)}
                >
                  <Check className={cn('h-4 w-4 shrink-0', isSelected ? 'opacity-100' : 'opacity-0')} />
                  <span className="truncate" title={value}>
                    {value}
                  </span>
                </li>
              );
            })}
          </ul>
        )}
      </PopoverContent>
    </Popover>
  );
}
