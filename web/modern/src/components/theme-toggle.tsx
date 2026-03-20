import { Check, Laptop, Moon, Sun, type LucideIcon } from 'lucide-react';
import { useTranslation } from 'react-i18next';

import { Button } from '@/components/ui/button';
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@/components/ui/dropdown-menu';
import { cn } from '@/lib/utils';
import { useTheme } from './theme-provider';

type ThemeValue = 'light' | 'dark' | 'system';

const THEME_ICONS: Record<ThemeValue, LucideIcon> = {
  light: Sun,
  dark: Moon,
  system: Laptop,
};

const THEME_VALUES: ThemeValue[] = ['light', 'dark', 'system'];

// ThemeToggle renders a minimal dropdown to switch between light, dark, and system themes.
export function ThemeToggle() {
  const { theme, setTheme } = useTheme();
  const { t } = useTranslation();
  const ActiveIcon = THEME_ICONS[theme] ?? THEME_ICONS.system;

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button variant="ghost" size="icon" aria-label={t('theme.toggle')} className="h-9 w-9">
          <ActiveIcon className="h-[1.2rem] w-[1.2rem]" aria-hidden="true" />
          <span className="sr-only">{t('theme.toggle')}</span>
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-44">
        {THEME_VALUES.map((value) => {
          const isActive = value === theme;
          const Icon = THEME_ICONS[value];
          const label = t(`theme.${value}`);

          return (
            <DropdownMenuItem
              key={value}
              onSelect={() => setTheme(value)}
              className={cn('flex items-center gap-2', isActive && 'bg-muted text-foreground focus:bg-muted')}
              role="menuitemradio"
              aria-checked={isActive}
            >
              <Icon className="h-4 w-4" aria-hidden="true" />
              <span className="flex-1 text-left">{label}</span>
              {isActive ? <Check className="h-4 w-4 text-primary" aria-hidden="true" /> : null}
            </DropdownMenuItem>
          );
        })}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
