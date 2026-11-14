import { useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { useLocation } from 'react-router-dom';

/** DEFAULT_SITE_NAME is the brand used in titles before the backend status provides a system name. */
const DEFAULT_SITE_NAME = 'One API';

/**
 * ROUTE_TITLE_KEYS maps the first path segment of each routed page to the i18n key of its label.
 * The keys are the same ones the header navigation renders, so the tab title matches the menu.
 */
const ROUTE_TITLE_KEYS: Readonly<Record<string, string>> = {
  dashboard: 'common.dashboard',
  tokens: 'common.tokens',
  logs: 'common.logs',
  users: 'common.users',
  channels: 'common.channels',
  mcps: 'common.mcps',
  redemptions: 'common.redemptions',
  topup: 'common.topup',
  models: 'common.models',
  tools: 'common.tools',
  status: 'common.status',
  chat: 'common.playground',
  about: 'common.about',
  settings: 'common.settings',
  login: 'common.login',
  register: 'common.register',
  reset: 'auth.reset.title',
  user: 'auth.reset_confirm.title',
};

/** NOT_FOUND_TITLE_KEY labels any path that no route above recognizes. */
const NOT_FOUND_TITLE_KEY = 'notFound.message';

/**
 * initialDocumentTitle is the SEO title shipped in index.html. It is captured once when this module
 * loads (before any route changes it) and restored on the home page.
 */
const initialDocumentTitle = typeof document === 'undefined' ? '' : document.title;

/**
 * getPageTitleKey returns the i18n key that names the page at a pathname.
 *
 * @param pathname - The router pathname, such as "/tokens/edit/3".
 * @returns The label key for the first path segment, the not-found key for unknown segments, or null
 *   for the home page, whose index.html title is kept.
 */
export function getPageTitleKey(pathname: string): string | null {
  const segment = pathname.split('/').find((part) => part.length > 0);
  if (!segment) return null;
  return ROUTE_TITLE_KEYS[segment.toLowerCase()] ?? NOT_FOUND_TITLE_KEY;
}

/**
 * resolveSiteName picks the brand for the title suffix: the live system name, then the name persisted
 * by persistSystemStatus, then the default brand.
 *
 * @param systemName - The system name from the backend status, when already loaded.
 * @returns A non-empty site name.
 */
function resolveSiteName(systemName?: string): string {
  const live = systemName?.trim();
  if (live) return live;
  try {
    const cached = localStorage.getItem('system_name')?.trim();
    if (cached) return cached;
  } catch {
    // Storage can be unavailable (privacy modes); the default brand is still a valid title.
  }
  return DEFAULT_SITE_NAME;
}

/**
 * usePageTitle keeps document.title in sync with the current route during single-page navigation
 * (WCAG 2.4.2), using the "Page | Site" order so the unique part comes first. The home page keeps
 * the SEO title from index.html. Titles are re-translated when the language changes.
 *
 * @param systemName - The configured system name; falls back to the cached name or "One API".
 * @returns Nothing; the hook only updates document.title.
 */
export function usePageTitle(systemName?: string): void {
  const { pathname } = useLocation();
  const { t, i18n } = useTranslation();
  const language = i18n.language;

  useEffect(() => {
    const key = getPageTitleKey(pathname);
    if (!key) {
      if (initialDocumentTitle) document.title = initialDocumentTitle;
      return;
    }
    document.title = `${t(key)} | ${resolveSiteName(systemName)}`;
  }, [pathname, systemName, t, language]);
}
