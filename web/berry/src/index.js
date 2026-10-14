import { browserReporterReady } from './browser-sentry';
import { mountBrowserApp } from '../../legacy/browser-sentry-runtime.mjs';

// Settle bounded reporting readiness before loading application modules.
mountBrowserApp(
  browserReporterReady,
  (reporter) => import('./browser-app').then(({ mountApplication }) => mountApplication(reporter)),
  window
);
