import type { BrowserReporter } from './browser-sentry.mjs';
/** loadBrowserReporter loads bounded public runtime settings and resolves one immutable reporter per window. */
export function loadBrowserReporter(loadSDK: () => unknown | Promise<unknown>, release: string, target: unknown, theme: string, entryURL: string, timeoutMs?: number): Promise<BrowserReporter>;
/** mountBrowserApp mounts once after readiness and redispatches original failures through the native browser error path. */
export function mountBrowserApp(readiness: Promise<import('./browser-sentry.mjs').BrowserReporter>, mount: (reporter: import('./browser-sentry.mjs').BrowserReporter) => unknown, target: unknown): Promise<unknown>;
