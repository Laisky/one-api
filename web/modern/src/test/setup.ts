import '@testing-library/jest-dom';
import { vi } from 'vitest';
import enTranslations from '../i18n/locales/en';

// Mock react-i18next
vi.mock('react-i18next', async () => {
  const enTranslations = (await import('../i18n/locales/en')).default;

  const t = (key: string, arg2?: any, arg3?: any) => {
    // Handle overload: t(key, options) or t(key, defaultValue, options)
    let options = arg2;
    if (typeof arg2 === 'string') {
      options = arg3;
    }

    // Helper to traverse object by dot notation
    const getValue = (obj: any, path: string) => {
      return path.split('.').reduce((o, k) => (o || {})[k], obj);
    };

    let value = getValue(enTranslations, key);

    if (value === undefined) {
      // Fallback for arrays if returnObjects is true
      if (options?.returnObjects) {
        return ['Item 1', 'Item 2'];
      }
      // Fallback to default value if provided
      if (typeof arg2 === 'string') {
        value = arg2;
      } else {
        return key;
      }
    }

    // Handle interpolation if needed (simple version)
    if (options && typeof value === 'string') {
      Object.keys(options).forEach((k) => {
        if (k !== 'returnObjects' && k !== 'defaultValue') {
          value = value.replace(`{{${k}}}`, options[k]);
        }
      });
    }

    return value;
  };

  return {
    useTranslation: () => ({
      t,
      i18n: {
        changeLanguage: () => new Promise(() => {}),
        language: 'en',
      },
    }),
    initReactI18next: {
      type: '3rdParty',
      init: () => {},
    },
    Trans: ({ children }: { children: React.ReactNode }) => children,
  };
});

// Mock window.matchMedia
Object.defineProperty(window, 'matchMedia', {
  writable: true,
  value: vi.fn().mockImplementation((query) => ({
    matches: false,
    media: query,
    onchange: null,
    addListener: vi.fn(), // deprecated
    removeListener: vi.fn(), // deprecated
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
    dispatchEvent: vi.fn(),
  })),
});

// Mock ResizeObserver
global.ResizeObserver = vi.fn().mockImplementation(() => ({
  observe: vi.fn(),
  unobserve: vi.fn(),
  disconnect: vi.fn(),
}));

// Polyfill pointer capture APIs used by Radix UI under jsdom
if (!HTMLElement.prototype.hasPointerCapture) {
  // eslint-disable-next-line @typescript-eslint/no-empty-function
  HTMLElement.prototype.setPointerCapture = function () {};
  // eslint-disable-next-line @typescript-eslint/no-empty-function
  HTMLElement.prototype.releasePointerCapture = function () {};
  HTMLElement.prototype.hasPointerCapture = function () {
    return false;
  };
}

// Ensure PointerEvent exists for user-event and Radix
if (typeof window.PointerEvent === 'undefined') {
  class MockPointerEvent extends MouseEvent {
    constructor(type: string, props?: MouseEventInit) {
      super(type, props);
    }
  }
  // @ts-ignore assigning test-only PointerEvent polyfill for jsdom
  window.PointerEvent = MockPointerEvent as unknown as typeof PointerEvent;
}

// Polyfill scrollIntoView used by Radix when focusing items in portals
if (!Element.prototype.scrollIntoView) {
  // eslint-disable-next-line @typescript-eslint/no-empty-function
  Element.prototype.scrollIntoView = function () {};
}
