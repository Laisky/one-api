import i18n from "i18next";
import { initReactI18next } from "react-i18next";
import LanguageDetector from "i18next-browser-languagedetector";

import en from "./locales/en";
import zh from "./locales/zh";
import fr from "./locales/fr";
import es from "./locales/es";
import ja from "./locales/ja";

// Define the resources
const resources = {
  en: { translation: en },
  zh: { translation: zh },
  fr: { translation: fr },
  es: { translation: es },
  ja: { translation: ja },
};

i18n
  // Detect user language
  .use(LanguageDetector)
  // Pass the i18n instance to react-i18next
  .use(initReactI18next)
  // Init i18next
  .init({
    resources,
    fallbackLng: "en", // Default fallback
    debug: process.env.NODE_ENV === "development",

    interpolation: {
      escapeValue: false, // React already safes from xss
    },

    detection: {
      // Order and from where user language should be detected
      order: ["localStorage", "navigator"],
      // Keys or params to lookup language from
      lookupLocalStorage: "i18nextLng",
      // Cache user language on
      caches: ["localStorage"],
    },
  });

export default i18n;
