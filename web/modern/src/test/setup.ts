import './jest-dom';
import { vi } from 'vitest';

// Mock react-i18next
vi.mock('react-i18next', async () => {
  const enTranslations = (await import('../i18n/locales/en')).default;

  /** t resolves a translation key, optional fallback and interpolation values for tests. */
  const t = (key: string, arg2?: any, arg3?: any) => {
    // Handle overload: t(key, options) or t(key, defaultValue, options)
    let options = arg2;
    if (typeof arg2 === 'string') {
      options = arg3;
    }

    /** getValue returns the nested value at a dot-separated path, or undefined when absent. */
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
      } else if (options?.defaultValue !== undefined) {
        value = options.defaultValue;
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
    /** useTranslation returns the deterministic translator and test language controls. */
    useTranslation: () => ({
      t,
      i18n: {
        /** changeLanguage leaves its promise pending because this fixture never switches locales. */
        changeLanguage: () => new Promise(() => {}),
        language: 'en',
      },
    }),
    initReactI18next: {
      type: '3rdParty',
      /** init accepts plugin initialization without registering a live i18n instance. */
      init: () => {},
    },
    /** Trans returns its children unchanged without loading translation infrastructure. */
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

/** MockResizeObserver provides a constructable no-layout observer for DOM-based tests. */
class MockResizeObserver implements ResizeObserver {
  /** constructor accepts the observer callback without scheduling resize notifications. */
  constructor(_callback: ResizeObserverCallback) {}

  /** observe accepts the target and options without observing layout, returning void. */
  observe(_target: Element, _options?: ResizeObserverOptions) {}

  /** unobserve accepts the target without retaining observations, returning void. */
  unobserve(_target: Element) {}

  /** disconnect has no retained observations to clear and returns void. */
  disconnect() {}
}

globalThis.ResizeObserver = MockResizeObserver;

// Polyfill pointer capture APIs used by Radix UI under jsdom
if (!HTMLElement.prototype.hasPointerCapture) {
  /** setPointerCapture is a no-op because jsdom does not dispatch native pointer capture. */
  // eslint-disable-next-line @typescript-eslint/no-empty-function
  HTMLElement.prototype.setPointerCapture = function () {};
  /** releasePointerCapture returns void without changing the no-capture fixture state. */
  // eslint-disable-next-line @typescript-eslint/no-empty-function
  HTMLElement.prototype.releasePointerCapture = function () {};
  /** hasPointerCapture returns false because this fixture never captures a pointer. */
  HTMLElement.prototype.hasPointerCapture = function () {
    return false;
  };
}

// Ensure PointerEvent exists for user-event and Radix
if (typeof window.PointerEvent === 'undefined') {
  /** MockPointerEvent supplies mouse-event behavior where PointerEvent is unavailable. */
  class MockPointerEvent extends MouseEvent {
    /** constructor creates the fallback event from its type and optional mouse properties. */
    constructor(type: string, props?: MouseEventInit) {
      super(type, props);
    }
  }
  window.PointerEvent = MockPointerEvent as unknown as typeof PointerEvent;
}

// Polyfill scrollIntoView used by Radix when focusing items in portals
if (!Element.prototype.scrollIntoView) {
  /** scrollIntoView returns void because jsdom does not perform layout or scrolling. */
  // eslint-disable-next-line @typescript-eslint/no-empty-function
  Element.prototype.scrollIntoView = function () {};
}
