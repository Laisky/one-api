import { Button } from '@/components/ui/button';
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@/components/ui/dropdown-menu';
import { changeAppLanguage } from '@/i18n';
import { cn } from '@/lib/utils';
import { Check, Languages } from 'lucide-react';
import { useTranslation } from 'react-i18next';

const languages = [
  { code: 'en', label: 'English' },
  { code: 'zh', label: '简体中文' },
  { code: 'fr', label: 'Français' },
  { code: 'es', label: 'Español' },
  { code: 'ja', label: '日本語' },
];

/**
 * LanguageSelector renders the header language menu. Each option shows its native name marked with its
 * own lang attribute so screen readers pronounce it correctly, and the active one is exposed as a
 * checked radio item rather than by color and an icon alone.
 *
 * @returns The language dropdown menu.
 */
export function LanguageSelector() {
  const { i18n, t } = useTranslation();

  const handleLanguageChange = (value: string) => {
    void changeAppLanguage(value);
  };

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button variant="ghost" size="icon" aria-label={t('a11y.select_language')} className="h-9 w-9 touch-target">
          <Languages className="h-[1.2rem] w-[1.2rem]" aria-hidden="true" />
          <span className="sr-only">{t('a11y.select_language')}</span>
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-44">
        {languages.map((lang) => {
          const isActive = i18n.language === lang.code || (i18n.language && i18n.language.startsWith(lang.code));

          return (
            <DropdownMenuItem
              key={lang.code}
              onSelect={() => handleLanguageChange(lang.code)}
              className={cn('flex items-center gap-2', isActive && 'bg-muted text-foreground focus:bg-muted')}
              role="menuitemradio"
              aria-checked={Boolean(isActive)}
            >
              <span className="flex-1 text-left" lang={lang.code}>
                {lang.label}
              </span>
              {isActive && <Check className="h-4 w-4 text-primary" aria-hidden="true" />}
            </DropdownMenuItem>
          );
        })}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
