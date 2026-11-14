import * as React from 'react';

type Theme = 'dark' | 'light' | 'system';

type ResolvedTheme = Exclude<Theme, 'system'>;

type ThemeProviderProps = {
  children: React.ReactNode;
  defaultTheme?: Theme;
  storageKey?: string;
};

type ThemeProviderState = {
  theme: Theme;
  setTheme: (theme: Theme) => void;
};

const initialState: ThemeProviderState = {
  theme: 'system',
  setTheme: () => null,
};

const ThemeProviderContext = React.createContext<ThemeProviderState>(initialState);

const DARK_SCHEME_QUERY = '(prefers-color-scheme: dark)';

// NOTE: index.html ships an inline boot script that mirrors readStoredTheme, resolveTheme and
// applyThemeToDocument before first paint. Keep both implementations in sync (storage format,
// html classes, color-scheme meta and theme-color media handling).

/**
 * isTheme reports whether a raw stored value is one of the supported theme preferences.
 * @param value - The raw value read from storage.
 * @returns True when the value is 'light', 'dark' or 'system'.
 */
function isTheme(value: unknown): value is Theme {
  return value === 'light' || value === 'dark' || value === 'system';
}

/**
 * readStoredTheme reads the persisted theme preference, tolerating blocked or unavailable storage.
 * @param storageKey - The localStorage key holding the raw preference string.
 * @param defaultTheme - The preference to use when nothing valid is stored.
 * @returns The stored preference when valid, otherwise defaultTheme.
 */
function readStoredTheme(storageKey: string, defaultTheme: Theme): Theme {
  let stored: string | null = null;
  try {
    stored = window.localStorage.getItem(storageKey);
  } catch {
    // Storage can be blocked (privacy modes, sandboxed frames); the default preference applies.
    stored = null;
  }
  return isTheme(stored) ? stored : defaultTheme;
}

/**
 * resolveTheme converts a theme preference into the concrete scheme that should be rendered.
 * @param theme - The user's preference, where 'system' follows the OS color scheme.
 * @returns 'dark' or 'light'.
 */
function resolveTheme(theme: Theme): ResolvedTheme {
  if (theme !== 'system') return theme;
  return window.matchMedia(DARK_SCHEME_QUERY).matches ? 'dark' : 'light';
}

/**
 * syncColorSchemeMeta points the color-scheme meta tag at the rendered scheme so browser UI that
 * renders before the stylesheet (canvas, scrollbars, form controls) matches the page.
 * @param resolved - The scheme currently applied to the document.
 * @returns Nothing; the meta tag is updated in place when present.
 */
function syncColorSchemeMeta(resolved: ResolvedTheme): void {
  const meta = document.querySelector('meta[name="color-scheme"]');
  meta?.setAttribute('content', resolved);
}

/**
 * syncThemeColorMeta makes the browser toolbar color follow the user's theme choice. index.html
 * declares one theme-color meta per scheme (tagged with data-scheme). For the 'system' preference
 * each meta keeps its prefers-color-scheme media query; for a manual choice the matching meta is
 * forced on with media="all" and the other is disabled with media="not all".
 * @param resolved - The scheme currently applied to the document.
 * @param preference - The user's preference ('light', 'dark' or 'system').
 * @returns Nothing; the meta tags are updated in place.
 */
function syncThemeColorMeta(resolved: ResolvedTheme, preference: Theme): void {
  document.querySelectorAll('meta[name="theme-color"][data-scheme]').forEach((meta) => {
    const scheme = meta.getAttribute('data-scheme');
    if (preference === 'system') {
      meta.setAttribute('media', `(prefers-color-scheme: ${scheme})`);
    } else {
      meta.setAttribute('media', scheme === resolved ? 'all' : 'not all');
    }
  });
}

/**
 * applyThemeToDocument applies a theme preference to the document: the html class consumed by the
 * Tailwind dark variant, the color-scheme meta and the theme-color metas.
 * @param preference - The user's preference ('light', 'dark' or 'system').
 * @returns Nothing; the document is updated in place.
 */
function applyThemeToDocument(preference: Theme): void {
  const resolved = resolveTheme(preference);
  const root = window.document.documentElement;
  root.classList.remove('light', 'dark');
  root.classList.add(resolved);
  syncColorSchemeMeta(resolved);
  syncThemeColorMeta(resolved, preference);
}

/**
 * ThemeProvider stores the user's theme preference and keeps the document's theme in sync with it,
 * following OS color-scheme changes while the preference is 'system'.
 * @param props.children - The subtree that can read and change the theme via useTheme.
 * @param props.defaultTheme - The preference used when nothing valid is stored; defaults to 'system'.
 * @param props.storageKey - The localStorage key that persists the preference as a raw string.
 * @returns The context provider wrapping children.
 */
export function ThemeProvider({ children, defaultTheme = 'system', storageKey = 'vite-ui-theme', ...props }: ThemeProviderProps) {
  const [theme, setTheme] = React.useState<Theme>(() => readStoredTheme(storageKey, defaultTheme));

  React.useEffect(() => {
    const applyTheme = () => applyThemeToDocument(theme);

    applyTheme();

    // Listen for OS theme changes when in "system" mode
    if (theme !== 'system') return;

    const mediaQuery = window.matchMedia(DARK_SCHEME_QUERY);
    const handleChange = () => applyTheme();

    mediaQuery.addEventListener('change', handleChange);
    return () => mediaQuery.removeEventListener('change', handleChange);
  }, [theme]);

  const value = {
    theme,
    setTheme: (theme: Theme) => {
      try {
        localStorage.setItem(storageKey, theme);
      } catch (error) {
        // The choice still applies for this session; it just cannot be persisted.
        console.warn(`ThemeProvider: unable to persist theme preference: ${String(error)}`);
      }
      setTheme(theme);
    },
  };

  return (
    <ThemeProviderContext.Provider {...props} value={value}>
      {children}
    </ThemeProviderContext.Provider>
  );
}

/**
 * useTheme returns the current theme preference and its setter from the nearest ThemeProvider.
 * @returns The ThemeProvider context value ({ theme, setTheme }).
 */
export const useTheme = () => {
  const context = React.useContext(ThemeProviderContext);

  if (context === undefined) throw new Error('useTheme must be used within a ThemeProvider');

  return context;
};
