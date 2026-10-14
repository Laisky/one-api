/** BrowserReporter exposes enabled state and a failure-safe browser exception capture method. */
export interface BrowserReporter {
  enabled: boolean;
  /** capture reports an exception without throwing into application code. */
  capture(error: unknown, info?: { componentStack?: string | null }): void;
}
/** createBrowserReporter validates public configuration and initializes at most one reporter per window. */
export function createBrowserReporter(
  sdk: unknown,
  settings: unknown,
  target: unknown,
  theme: string,
  entryURL: string,
  release: string
): BrowserReporter;
/** reactRootOptions returns enabled React19 root callbacks while preserving disabled defaults. */
export function reactRootOptions(reporter: BrowserReporter): {
  onUncaughtError?: (error: unknown, info?: { componentStack?: string | null }) => void;
  onCaughtError?: (error: unknown, info?: { componentStack?: string | null; errorBoundary?: unknown }) => void;
  onRecoverableError?: (error: unknown, info?: { componentStack?: string | null }) => void;
};
/** createErrorBoundary returns a render-error boundary or the disabled transparent wrapper. */
export function createErrorBoundary(React: unknown, reporter: BrowserReporter): unknown;
