import {
  captureException,
  globalHandlersIntegration,
  init,
  makeFetchTransport,
} from '@sentry/react';
import { createBrowserReporter } from '../../legacy/browser-sentry.mjs';

const errorsOnlySDK = {
  captureException,
  globalHandlersIntegration,
  init,
  makeFetchTransport,
};
export const browserReporter = createBrowserReporter(
  errorsOnlySDK,
  {
    VITE_SENTRY_ENABLED: import.meta.env.VITE_SENTRY_ENABLED,
    VITE_SENTRY_DSN: import.meta.env.VITE_SENTRY_DSN,
    VITE_SENTRY_ENVIRONMENT: import.meta.env.VITE_SENTRY_ENVIRONMENT,
    VITE_SENTRY_RELEASE: import.meta.env.VITE_SENTRY_RELEASE,
  },
  window,
  'air',
  import.meta.url,
);
