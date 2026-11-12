import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { act, render, screen } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { ThemeProvider, useTheme } from '@/components/theme-provider';

const STORAGE_KEY = 'one-api-theme';
const DARK_QUERY = '(prefers-color-scheme: dark)';

type DocumentThemeState = {
  classes: string[];
  colorScheme: string | null;
  lightThemeColorMedia: string | null;
  darkThemeColorMedia: string | null;
};

/** loadBootScript extracts the classic inline theme boot script from index.html and returns its source. */
function loadBootScript(): string {
  const html = readFileSync(resolve(__dirname, '../../../index.html'), 'utf8');
  // Parse the document instead of regex-matching tags; classic inline scripts have neither src nor type.
  const parsed = new DOMParser().parseFromString(html, 'text/html');
  const scripts = [...parsed.querySelectorAll('script:not([src]):not([type])')].map((script) => script.textContent ?? '');
  const boot = scripts.filter((source) => source.includes(STORAGE_KEY));
  if (boot.length !== 1) throw new Error(`expected exactly one theme boot script, found ${boot.length}`);
  return boot[0];
}

/** resetDocument restores the theme-related <head> metas from index.html and clears the html theme classes. */
function resetDocument(): void {
  document.documentElement.className = '';
  document.head.innerHTML = [
    '<meta name="color-scheme" content="light dark" />',
    '<meta name="theme-color" content="hsl(192, 40%, 24%)" media="(prefers-color-scheme: light)" data-scheme="light" />',
    '<meta name="theme-color" content="hsl(186, 38%, 52%)" media="(prefers-color-scheme: dark)" data-scheme="dark" />',
  ].join('');
}

/** readDocumentState captures the theme-relevant DOM state and returns it for comparison. */
function readDocumentState(): DocumentThemeState {
  return {
    classes: [...document.documentElement.classList].filter((name) => name === 'light' || name === 'dark'),
    colorScheme: document.querySelector('meta[name="color-scheme"]')?.getAttribute('content') ?? null,
    lightThemeColorMedia: document.querySelector('meta[name="theme-color"][data-scheme="light"]')?.getAttribute('media') ?? null,
    darkThemeColorMedia: document.querySelector('meta[name="theme-color"][data-scheme="dark"]')?.getAttribute('media') ?? null,
  };
}

/**
 * mockSystemScheme replaces matchMedia with a controllable OS color-scheme preference.
 * @param initialDark - Whether the OS prefers a dark scheme initially.
 * @returns A controller whose setDark changes the preference and notifies change listeners.
 */
function mockSystemScheme(initialDark: boolean) {
  const listeners = new Set<() => void>();
  let dark = initialDark;
  window.matchMedia = vi.fn((query: string) => ({
    get matches() {
      return query === DARK_QUERY ? dark : false;
    },
    media: query,
    onchange: null,
    addEventListener: (_type: string, listener: () => void) => listeners.add(listener),
    removeEventListener: (_type: string, listener: () => void) => listeners.delete(listener),
    addListener: vi.fn(),
    removeListener: vi.fn(),
    dispatchEvent: vi.fn(),
  })) as unknown as typeof window.matchMedia;
  return {
    /** setDark updates the mocked OS preference and fires the registered change listeners. */
    setDark(next: boolean) {
      dark = next;
      listeners.forEach((listener) => listener());
    },
    /** listenerCount returns how many change listeners are currently registered. */
    listenerCount: () => listeners.size,
  };
}

/** ThemeButtons exposes the current preference and buttons that call setTheme for each preference. */
function ThemeButtons() {
  const { theme, setTheme } = useTheme();
  return (
    <div>
      <span data-testid="theme">{theme}</span>
      <button onClick={() => setTheme('light')}>light</button>
      <button onClick={() => setTheme('dark')}>dark</button>
      <button onClick={() => setTheme('system')}>system</button>
    </div>
  );
}

/** renderProvider mounts ThemeProvider with the App's storage key and default preference. */
function renderProvider() {
  return render(
    <ThemeProvider defaultTheme="system" storageKey={STORAGE_KEY}>
      <ThemeButtons />
    </ThemeProvider>
  );
}

const originalMatchMedia = window.matchMedia;

describe('ThemeProvider document sync', () => {
  beforeEach(() => {
    localStorage.clear();
    resetDocument();
  });

  afterEach(() => {
    window.matchMedia = originalMatchMedia;
    vi.restoreAllMocks();
    localStorage.clear();
    resetDocument();
  });

  it('applies a manual dark choice to the html class, color-scheme meta and theme-color metas', () => {
    mockSystemScheme(false);
    localStorage.setItem(STORAGE_KEY, 'dark');
    renderProvider();

    expect(readDocumentState()).toEqual({
      classes: ['dark'],
      colorScheme: 'dark',
      lightThemeColorMedia: 'not all',
      darkThemeColorMedia: 'all',
    });
  });

  it('applies a manual light choice even when the OS prefers dark', () => {
    mockSystemScheme(true);
    localStorage.setItem(STORAGE_KEY, 'light');
    renderProvider();

    expect(readDocumentState()).toEqual({
      classes: ['light'],
      colorScheme: 'light',
      lightThemeColorMedia: 'all',
      darkThemeColorMedia: 'not all',
    });
  });

  it('restores the prefers-color-scheme media queries for the system preference and follows OS changes', () => {
    const system = mockSystemScheme(true);
    renderProvider();

    expect(readDocumentState()).toEqual({
      classes: ['dark'],
      colorScheme: 'dark',
      lightThemeColorMedia: '(prefers-color-scheme: light)',
      darkThemeColorMedia: '(prefers-color-scheme: dark)',
    });

    act(() => system.setDark(false));
    expect(readDocumentState()).toEqual({
      classes: ['light'],
      colorScheme: 'light',
      lightThemeColorMedia: '(prefers-color-scheme: light)',
      darkThemeColorMedia: '(prefers-color-scheme: dark)',
    });
  });

  it('switches between manual and system preferences, persisting each choice', () => {
    const system = mockSystemScheme(false);
    renderProvider();

    act(() => screen.getByText('dark').click());
    expect(localStorage.getItem(STORAGE_KEY)).toBe('dark');
    expect(screen.getByTestId('theme').textContent).toBe('dark');
    expect(readDocumentState()).toEqual({
      classes: ['dark'],
      colorScheme: 'dark',
      lightThemeColorMedia: 'not all',
      darkThemeColorMedia: 'all',
    });
    // A manual choice stops following the OS preference.
    expect(system.listenerCount()).toBe(0);

    act(() => screen.getByText('system').click());
    expect(localStorage.getItem(STORAGE_KEY)).toBe('system');
    expect(readDocumentState()).toEqual({
      classes: ['light'],
      colorScheme: 'light',
      lightThemeColorMedia: '(prefers-color-scheme: light)',
      darkThemeColorMedia: '(prefers-color-scheme: dark)',
    });
    expect(system.listenerCount()).toBe(1);
  });

  it('falls back to the default preference for unknown stored values instead of adding them as classes', () => {
    mockSystemScheme(false);
    localStorage.setItem(STORAGE_KEY, 'sepia');
    renderProvider();

    expect(screen.getByTestId('theme').textContent).toBe('system');
    expect(document.documentElement.classList.contains('sepia')).toBe(false);
    expect(readDocumentState().classes).toEqual(['light']);
  });

  it('keeps working when storage is blocked', () => {
    mockSystemScheme(true);
    vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
      throw new Error('SecurityError');
    });
    const setItem = vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
      throw new Error('SecurityError');
    });
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {});
    renderProvider();

    expect(readDocumentState().classes).toEqual(['dark']);
    act(() => screen.getByText('light').click());
    expect(setItem).toHaveBeenCalled();
    expect(readDocumentState().classes).toEqual(['light']);
    expect(warn).toHaveBeenCalledWith(expect.stringContaining('unable to persist theme preference'));
  });
});

describe('index.html theme boot script', () => {
  const bootScript = loadBootScript();
  /** runBootScript executes the extracted inline script the same way the browser parser would. */
  const runBootScript = () => new Function(bootScript)();

  beforeEach(() => {
    localStorage.clear();
    resetDocument();
  });

  afterEach(() => {
    window.matchMedia = originalMatchMedia;
    vi.restoreAllMocks();
    localStorage.clear();
    resetDocument();
  });

  const storedValues: Array<string | null> = [null, 'light', 'dark', 'system', 'sepia'];
  for (const stored of storedValues) {
    for (const osDark of [false, true]) {
      it(`matches ThemeProvider for stored=${String(stored)} osDark=${osDark}`, () => {
        mockSystemScheme(osDark);
        if (stored !== null) localStorage.setItem(STORAGE_KEY, stored);

        runBootScript();
        const booted = readDocumentState();

        resetDocument();
        const { unmount } = renderProvider();
        const provided = readDocumentState();
        unmount();

        expect(booted).toEqual(provided);
        const expectedResolved = stored === 'light' || stored === 'dark' ? stored : osDark ? 'dark' : 'light';
        expect(booted.classes).toEqual([expectedResolved]);
        expect(booted.colorScheme).toBe(expectedResolved);
      });
    }
  }

  it('falls back to the system scheme without throwing when storage is blocked', () => {
    mockSystemScheme(true);
    vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
      throw new Error('SecurityError');
    });
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {});

    expect(runBootScript).not.toThrow();
    expect(readDocumentState()).toEqual({
      classes: ['dark'],
      colorScheme: 'dark',
      lightThemeColorMedia: '(prefers-color-scheme: light)',
      darkThemeColorMedia: '(prefers-color-scheme: dark)',
    });
    expect(warn).not.toHaveBeenCalled();
  });

  it('stays ES5-compatible so it can run before any bundle or polyfill', () => {
    const code = bootScript.replace(/^\s*\/\/.*$/gm, '');
    expect(code).toContain('function ()');
    expect(code).not.toMatch(/\b(?:const|let|class)\s|=>|`|\?\.|\?\?/);
  });
});
