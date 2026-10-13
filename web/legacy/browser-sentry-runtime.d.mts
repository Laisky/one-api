import type { BrowserReporter } from './browser-sentry.mjs';
/** loadBrowserReporter loads bounded public runtime settings and resolves one immutable reporter per window. */
export function loadBrowserReporter(loadSDK: () => unknown | Promise<unknown>, release: string, target: unknown, theme: string, entryURL: string, timeoutMs?: number): Promise<BrowserReporter>;
