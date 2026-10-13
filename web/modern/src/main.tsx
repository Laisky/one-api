import { browserReporterReady } from './browser-sentry';
import { mountBrowserApp } from '../../legacy/browser-sentry-runtime.mjs';
import { createErrorBoundary, reactRootOptions } from '../../legacy/browser-sentry.mjs';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import React from 'react';
import ReactDOM from 'react-dom/client';
import App from './App';
import './i18n';
import './index.css';

// Runtime configuration is bounded and settles before React mounts; early module errors are not buffered.
mountBrowserApp(
  browserReporterReady,
  (browserReporter) => {
    const SentryBoundary = createErrorBoundary(React, browserReporter) as React.ComponentType<React.PropsWithChildren>;

    const qc = new QueryClient();

    ReactDOM.createRoot(document.getElementById('root')!, reactRootOptions(browserReporter)).render(
      <SentryBoundary>
        <React.StrictMode>
          <QueryClientProvider client={qc}>
            <App />
          </QueryClientProvider>
        </React.StrictMode>
      </SentryBoundary>
    );
  },
  window
);
