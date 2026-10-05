/**
 * normalizeUser backfills the legacy `id` field from the UUID carried by the
 * UUID-only management API so components that still key on `user.id` keep
 * working. It is the single normalisation point for every payload that ends up
 * in the user context (login, OAuth callbacks, /api/user/self refreshes).
 *
 * Parameters:
 *   - user: object|null|undefined, a user DTO as returned by the backend.
 *
 * Return value: the same user extended with `uuid` and `id` when a UUID is
 * available; the input untouched otherwise.
 */
export const normalizeUser = (user) => {
  if (!user) return user;
  const uuid = user.uuid || user.user_uuid;
  return uuid ? { ...user, uuid, id: uuid } : user;
};

/**
 * isTotpRequired reports whether a login response (password, OAuth, or
 * WeChat) asks the user for a TOTP code before the session is created.
 *
 * Parameters:
 *   - message: string|undefined, the `message` field of the login response.
 *   - data: object|null|undefined, the `data` field of the login response.
 *
 * Return value: true when the server answered with the `totp_required`
 * challenge, false otherwise.
 */
export const isTotpRequired = (message, data) => message === 'totp_required' || data?.totp_required === true;
