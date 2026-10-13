import { browserReporter } from './browser-sentry';
import { createErrorBoundary, reactRootOptions } from '../../legacy/browser-sentry.mjs';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import React from 'react';
import ReactDOM from 'react-dom/client';
import App from './App';
import './i18n';
import './index.css';

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
