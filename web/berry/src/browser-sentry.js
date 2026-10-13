import { loadBrowserReporter } from '../../legacy/browser-sentry-runtime.mjs';

export const browserReporterReady = loadBrowserReporter(
  async () => (await import('./browser-sentry-sdk')).errorsOnlySDK,
  __ONEAPI_BROWSER_RELEASE__,
  window,
  'berry',
  import.meta.url
);
