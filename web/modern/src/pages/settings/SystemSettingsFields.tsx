import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { useNotifications } from '@/components/ui/notifications';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import { Info, X } from 'lucide-react';
import { useCallback, useEffect, useMemo, useState, type KeyboardEvent, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';

export interface OptionRow {
  key: string;
  value: string;
}

export interface EnumChoice {
  // value is what the Radix Select renders (must be non-empty; Radix forbids '').
  value: string;
  // storedValue is what gets persisted; defaults to `value`. Use '' for an
  // "auto/unset" choice so the empty backend state stays selectable and restorable.
  storedValue?: string;
  labelKey: string;
}

interface OptionItemProps {
  option: OptionRow;
  description?: string;
  onSave: (key: string, value: string | string[]) => Promise<void>;
  onClear?: (key: string) => Promise<void>;
  isSensitive?: boolean;
  isBoolean?: boolean;
  enumChoices?: EnumChoice[];
  extraAction?: ReactNode;
}

export function OptionItem({ option, description, onSave, onClear, isSensitive, isBoolean, enumChoices, extraAction }: OptionItemProps) {
  const { t } = useTranslation();
  const [value, setValue] = useState(option.value);
  const [isSaving, setIsSaving] = useState(false);

  useEffect(() => {
    setValue(option.value);
  }, [option.value]);

  const handleSave = useCallback(
    async (overrideValue?: string) => {
      const nextValue = overrideValue ?? value;
      if (isSaving || nextValue === option.value) return;
      setIsSaving(true);
      try {
        await onSave(option.key, nextValue);
        if (isSensitive) {
          setValue('');
        } else {
          setValue(nextValue);
        }
      } catch (_error) {
        setValue(option.value);
      } finally {
        setIsSaving(false);
      }
    },
    [isSaving, isSensitive, onSave, option.key, option.value, value]
  );

  const handleBlur = useCallback(async () => {
    if (isSensitive || value === option.value) return;
    await handleSave();
  }, [handleSave, isSensitive, option.value, value]);

  const handleBooleanChange = useCallback(
    (newValue: string) => {
      setValue(newValue);
      handleSave(newValue);
    },
    [handleSave]
  );

  // handleEnumChange maps the Radix Select UI value back to the persisted value
  // (e.g. the Auto sentinel maps to an empty string) before saving.
  const handleEnumChange = useCallback(
    (uiValue: string) => {
      const choice = enumChoices?.find((c) => c.value === uiValue);
      const stored = choice ? (choice.storedValue ?? choice.value) : uiValue;
      setValue(stored);
      handleSave(stored);
    },
    [enumChoices, handleSave]
  );

  const handleClear = useCallback(async () => {
    if (isSaving || !onClear) return;
    setIsSaving(true);
    try {
      await onClear(option.key);
      setValue('');
    } catch (_error) {
      // Leave the current input untouched on failure.
    } finally {
      setIsSaving(false);
    }
  }, [isSaving, onClear, option.key]);

  // Map the persisted value to the Select's UI value (inverse of storedValue).
  const enumSelected = enumChoices?.find((choice) => (choice.storedValue ?? choice.value) === value)?.value;

  const placeholder = isSensitive ? t('system_settings.sensitive_placeholder') : undefined;
  const optionValueAriaLabel = t('system_settings.option_value_aria', {
    key: option.key,
  });

  return (
    <div className="border rounded-lg p-4 space-y-3">
      <div className="text-sm font-medium text-muted-foreground flex items-center gap-2">
        <span>{option.key}</span>
        <Tooltip>
          <TooltipTrigger asChild>
            <button
              type="button"
              className="inline-flex items-center text-muted-foreground hover:text-foreground focus:outline-none"
              aria-label={t('system_settings.info_about', { key: option.key })}
            >
              <Info className="h-4 w-4" />
            </button>
          </TooltipTrigger>
          <TooltipContent side="top" align="start" className="max-w-[320px]">
            {description || t('system_settings.no_description')}
          </TooltipContent>
        </Tooltip>
      </div>
      <div className="flex flex-col gap-2 sm:flex-row">
        {isBoolean ? (
          <Select value={value === '' ? undefined : value} onValueChange={handleBooleanChange} disabled={isSaving}>
            <SelectTrigger className="flex-1" aria-label={optionValueAriaLabel} disabled={isSaving}>
              <SelectValue placeholder={t('system_settings.select_value')} />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="true">{t('system_settings.enabled')}</SelectItem>
              <SelectItem value="false">{t('system_settings.disabled')}</SelectItem>
            </SelectContent>
          </Select>
        ) : enumChoices && enumChoices.length > 0 ? (
          <Select value={enumSelected} onValueChange={handleEnumChange} disabled={isSaving}>
            <SelectTrigger className="flex-1" aria-label={optionValueAriaLabel} disabled={isSaving}>
              <SelectValue placeholder={t('system_settings.select_value')} />
            </SelectTrigger>
            <SelectContent>
              {enumChoices.map((choice) => (
                <SelectItem key={choice.value} value={choice.value}>
                  {t(choice.labelKey)}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        ) : (
          <Input
            type={isSensitive ? 'password' : undefined}
            value={value}
            onChange={(e) => setValue(e.target.value)}
            onBlur={handleBlur}
            className="flex-1"
            aria-label={optionValueAriaLabel}
            placeholder={placeholder}
            disabled={isSaving}
          />
        )}
        <Button type="button" variant="outline" onClick={() => handleSave()} disabled={isSaving}>
          {isSaving ? t('system_settings.saving') : t('system_settings.save')}
        </Button>
        {isSensitive && onClear && (
          <Button
            type="button"
            variant="ghost"
            onClick={handleClear}
            disabled={isSaving}
            aria-label={t('system_settings.clear_aria', { key: option.key })}
          >
            {t('system_settings.clear')}
          </Button>
        )}
        {extraAction}
      </div>
      {isSensitive && <p className="text-xs text-muted-foreground">{t('system_settings.sensitive_hint')}</p>}
    </div>
  );
}

interface EmailDomainWhitelistItemProps {
  option: OptionRow;
  description?: string;
  onSave: (key: string, value: string | string[]) => Promise<void>;
}

const EMAIL_DOMAIN_REGEX = /^[a-z0-9.-]+\.[a-z]{2,}$/i;

export function EmailDomainWhitelistItem({ option, description, onSave }: EmailDomainWhitelistItemProps) {
  const { t } = useTranslation();
  const { notify } = useNotifications();
  const initialDomains = useMemo(
    () =>
      option.value
        .split(',')
        .map((d) => d.trim())
        .filter(Boolean),
    [option.value]
  );
  const [domains, setDomains] = useState<string[]>(initialDomains);
  const [draft, setDraft] = useState('');
  const [isSaving, setIsSaving] = useState(false);

  useEffect(() => {
    setDomains(initialDomains);
  }, [initialDomains]);

  const isDirty = useMemo(() => {
    const a = domains.join(',');
    const b = initialDomains.join(',');
    return a !== b;
  }, [domains, initialDomains]);

  const handleAdd = useCallback(() => {
    const candidate = draft.trim();
    if (!candidate) return;
    if (!EMAIL_DOMAIN_REGEX.test(candidate)) {
      notify({
        type: 'error',
        title: t('system_settings.email_whitelist.invalid_title'),
        message: t('system_settings.email_whitelist.invalid_domain', { domain: candidate }),
      });
      return;
    }
    if (domains.some((d) => d.toLowerCase() === candidate.toLowerCase())) {
      notify({
        type: 'warning',
        title: t('system_settings.email_whitelist.duplicate_title'),
        message: t('system_settings.email_whitelist.duplicate_domain', { domain: candidate }),
      });
      setDraft('');
      return;
    }
    setDomains((prev) => [...prev, candidate]);
    setDraft('');
  }, [domains, draft, notify, t]);

  const handleRemove = useCallback((domain: string) => {
    setDomains((prev) => prev.filter((d) => d !== domain));
  }, []);

  const handleSave = useCallback(async () => {
    if (isSaving) return;
    setIsSaving(true);
    try {
      await onSave(option.key, domains);
    } catch (_error) {
      // Restore on error.
      setDomains(initialDomains);
    } finally {
      setIsSaving(false);
    }
  }, [domains, initialDomains, isSaving, onSave, option.key]);

  const handleKeyDown = (e: KeyboardEvent<HTMLInputElement>) => {
    if (e.key === 'Enter') {
      e.preventDefault();
      handleAdd();
    }
  };

  return (
    <div className="border rounded-lg p-4 space-y-3 md:col-span-2">
      <div className="text-sm font-medium text-muted-foreground flex items-center gap-2">
        <span>{option.key}</span>
        <Tooltip>
          <TooltipTrigger asChild>
            <button
              type="button"
              className="inline-flex items-center text-muted-foreground hover:text-foreground focus:outline-none"
              aria-label={t('system_settings.info_about', { key: option.key })}
            >
              <Info className="h-4 w-4" />
            </button>
          </TooltipTrigger>
          <TooltipContent side="top" align="start" className="max-w-[320px]">
            {description || t('system_settings.no_description')}
          </TooltipContent>
        </Tooltip>
      </div>
      <div className="flex flex-wrap gap-2 min-h-[28px]" data-testid="email-domain-list">
        {domains.length === 0 ? (
          <span className="text-xs text-muted-foreground">{t('system_settings.email_whitelist.empty')}</span>
        ) : (
          domains.map((domain) => (
            <Badge key={domain} variant="secondary" className="gap-1">
              <span>{domain}</span>
              <button
                type="button"
                className="ml-1 inline-flex items-center text-muted-foreground hover:text-destructive focus:outline-none"
                aria-label={t('system_settings.email_whitelist.remove_aria', { domain })}
                onClick={() => handleRemove(domain)}
              >
                <X className="h-3 w-3" />
              </button>
            </Badge>
          ))
        )}
      </div>
      <div className="flex flex-col gap-2 sm:flex-row">
        <Input
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          onKeyDown={handleKeyDown}
          className="flex-1"
          aria-label={t('system_settings.email_whitelist.input_aria')}
          placeholder={t('system_settings.email_whitelist.placeholder')}
          disabled={isSaving}
        />
        <Button type="button" variant="outline" onClick={handleAdd} disabled={isSaving}>
          {t('system_settings.email_whitelist.add')}
        </Button>
        <Button type="button" onClick={handleSave} disabled={isSaving || !isDirty}>
          {isSaving ? t('system_settings.saving') : t('system_settings.save')}
        </Button>
      </div>
    </div>
  );
}
