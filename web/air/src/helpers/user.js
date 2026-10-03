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
