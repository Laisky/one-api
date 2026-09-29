import i18n from 'i18next';
import { initReactI18next } from 'react-i18next';
import LanguageDetector from 'i18next-browser-languagedetector';

import en from './locales/en';
import { mobileTableTranslations } from './locales/mobile-table';

type SupportedLanguage = 'en' | 'zh' | 'fr' | 'es' | 'ja';
type LocaleModule = { default: Record<string, unknown> };

const supportedLanguages: SupportedLanguage[] = ['en', 'zh', 'fr', 'es', 'ja'];

const localeLoaders: Record<Exclude<SupportedLanguage, 'en'>, () => Promise<LocaleModule>> = {
  zh: () => import('./locales/zh'),
  fr: () => import('./locales/fr'),
  es: () => import('./locales/es'),
  ja: () => import('./locales/ja'),
};

const loadedLanguages = new Set<SupportedLanguage>(['en']);

const normalizeLanguage = (lng?: string): SupportedLanguage => {
  const baseLanguage = lng?.split('-')[0] as SupportedLanguage | undefined;
  return baseLanguage && supportedLanguages.includes(baseLanguage) ? baseLanguage : 'en';
};

export const loadLanguageResources = async (lng?: string): Promise<SupportedLanguage> => {
  const language = normalizeLanguage(lng);
  if (loadedLanguages.has(language)) return language;
  if (language === 'en') return language;

  const locale = await localeLoaders[language]();
  i18n.addResourceBundle(language, 'translation', { ...locale.default, mobile_table: mobileTableTranslations[language] }, true, true);
  loadedLanguages.add(language);
  return language;
};

export const changeAppLanguage = async (lng: string) => {
  const language = await loadLanguageResources(lng);
  await i18n.changeLanguage(language);
};

const resources = {
  en: { translation: { ...en, mobile_table: mobileTableTranslations.en } },
};

i18n
  .use(LanguageDetector)
  .use(initReactI18next)
  .init({
    resources,
    fallbackLng: 'en',
    supportedLngs: supportedLanguages,
    partialBundledLanguages: true,
    debug: process.env.NODE_ENV === 'development',
    interpolation: { escapeValue: false },
    detection: {
      order: ['localStorage', 'navigator'],
      lookupLocalStorage: 'i18nextLng',
      caches: ['localStorage'],
    },
  });

// Sync the <html lang> attribute with the current i18next language.
const syncHtmlLang = (lng: string) => {
  document.documentElement.lang = lng;
};
syncHtmlLang(i18n.language);
i18n.on('languageChanged', (lng) => {
  syncHtmlLang(lng);
  const language = normalizeLanguage(lng);
  const alreadyLoaded = loadedLanguages.has(language);
  void loadLanguageResources(lng).then((language) => {
    if (!alreadyLoaded && normalizeLanguage(i18n.language) === language) void i18n.changeLanguage(language);
  });
});

void loadLanguageResources(i18n.language).then((language) => {
  if (language !== 'en') void i18n.changeLanguage(language);
});

export default i18n;
