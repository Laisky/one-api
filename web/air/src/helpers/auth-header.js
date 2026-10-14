import { markPrivateError } from '../../../legacy/browser-sentry-privacy.mjs';

/** authHeader reads saved user data and returns the existing bearer header when available. */
export function authHeader() {
  // return authorization header with jwt token
  const savedUser = localStorage.getItem('user');
  let user;
  try {
    user = JSON.parse(savedUser);
  } catch (error) {
    throw markPrivateError(error, 'Saved user data could not be parsed.');
  }

  if (user && user.token) {
    return { Authorization: 'Bearer ' + user.token };
  } else {
    return {};
  }
}
