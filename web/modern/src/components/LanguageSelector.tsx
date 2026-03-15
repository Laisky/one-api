import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { cn } from "@/lib/utils";
import { Check, Languages } from 'lucide-react';
import { useTranslation } from 'react-i18next';

const languages = [
  { code: 'en', label: 'English' },
  { code: 'zh', label: '简体中文' },
  { code: 'fr', label: 'Français' },
  { code: 'es', label: 'Español' },
  { code: 'ja', label: '日本語' },
];

export function LanguageSelector() {
  const { i18n, t } = useTranslation();

  const handleLanguageChange = (value: string) => {
    i18n.changeLanguage(value);
  };

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          variant="ghost"
          size="icon"
          aria-label={t("a11y.select_language")}
          className="h-9 w-9"
        >
          <Languages className="h-[1.2rem] w-[1.2rem]" aria-hidden="true" />
          <span className="sr-only">{t("a11y.select_language")}</span>
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-44">
        {languages.map((lang) => {
          const isActive = i18n.language === lang.code || (i18n.language && i18n.language.startsWith(lang.code));

          return (
            <DropdownMenuItem
              key={lang.code}
              onSelect={() => handleLanguageChange(lang.code)}
              className={cn(
                "flex items-center gap-2",
                isActive && "bg-muted text-foreground focus:bg-muted"
              )}
            >
              <span className="flex-1 text-left">{lang.label}</span>
              {isActive && (
                <Check className="h-4 w-4 text-primary" aria-hidden="true" />
              )}
            </DropdownMenuItem>
          );
        })}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
