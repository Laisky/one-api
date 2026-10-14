// Keep startup styles eager: an optional font outage must not reject app loading.
import 'semantic-ui-css/semantic.min.css';
import './index.css';
import './mobile-override.css';
import 'react-toastify/dist/ReactToastify.css';

import { browserReporterReady } from './browser-sentry';
import { mountBrowserApp } from '../../legacy/browser-sentry-runtime.mjs';

// Settle bounded reporting readiness before loading application modules.
mountBrowserApp(
  browserReporterReady,
  (reporter) =>
    import('./browser-app').then(({ mountApplication }) =>
      mountApplication(reporter),
    ),
  window,
);
