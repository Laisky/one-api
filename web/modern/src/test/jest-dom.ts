import * as matchers from '@testing-library/jest-dom/matchers'
import type { TestingLibraryMatchers } from '@testing-library/jest-dom/matchers'
import { expect } from 'vitest'

// Vitest 5 extensions use Matchers<R, T>. The jest-dom/vitest entry point
// still augments the legacy Assertion interface with incompatible generics.
// Register the same runtime matchers without disabling strict library checks.
declare module 'vitest' {
  // eslint-disable-next-line @typescript-eslint/no-empty-object-type
  interface Matchers<R, T> extends TestingLibraryMatchers<() => T, R> {}
}

expect.extend(matchers)
