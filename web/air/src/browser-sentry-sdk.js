import {
  captureException,
  globalHandlersIntegration,
  init,
  makeFetchTransport,
} from '@sentry/react';

// Named exports keep this lazily loaded adapter limited to browser error reporting.
export const errorsOnlySDK = {
  captureException,
  globalHandlersIntegration,
  init,
  makeFetchTransport,
};
