
import '@testing-library/jest-dom'
import { vi } from 'vitest'

// Mock window.matchMedia
Object.defineProperty(window, 'matchMedia', {
  writable: true,
  value: vi.fn().mockImplementation(query => ({
    matches: false,
    media: query,
    onchange: null,
    addListener: vi.fn(), // deprecated
    removeListener: vi.fn(), // deprecated
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
    dispatchEvent: vi.fn(),
  })),
})

// Mock ResizeObserver
global.ResizeObserver = vi.fn().mockImplementation(() => ({
  observe: vi.fn(),
  unobserve: vi.fn(),
  disconnect: vi.fn(),
}))

// Polyfill pointer capture APIs used by Radix UI under jsdom
if (!HTMLElement.prototype.hasPointerCapture) {
  // eslint-disable-next-line @typescript-eslint/no-empty-function
  HTMLElement.prototype.setPointerCapture = function () { }
  // eslint-disable-next-line @typescript-eslint/no-empty-function
  HTMLElement.prototype.releasePointerCapture = function () { }
  HTMLElement.prototype.hasPointerCapture = function () { return false }
}

// Ensure PointerEvent exists for user-event and Radix
if (typeof window.PointerEvent === 'undefined') {
  class MockPointerEvent extends MouseEvent {
    constructor(type: string, props?: MouseEventInit) {
      super(type, props)
    }
  }
  // @ts-ignore assigning test-only PointerEvent polyfill for jsdom
  window.PointerEvent = MockPointerEvent as unknown as typeof PointerEvent
}

// Polyfill scrollIntoView used by Radix when focusing items in portals
if (!Element.prototype.scrollIntoView) {
  // eslint-disable-next-line @typescript-eslint/no-empty-function
  Element.prototype.scrollIntoView = function () { }
}
