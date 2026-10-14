import {
  captureException,
  captureReactException,
  init,
  reactErrorHandler,
} from '@sentry/react';

// Named exports keep this lazy adapter limited to browser error reporting.
export const errorsOnlySDK = {
  captureException,
  captureReactException,
  init,
  reactErrorHandler,
};
