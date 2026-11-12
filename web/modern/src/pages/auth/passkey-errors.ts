/**
 * PASSKEY_CANCELLATION_ERROR_NAMES lists the DOMException names a WebAuthn ceremony reports when the user dismissed
 * or timed out the browser prompt (NotAllowedError) or the ceremony was aborted programmatically (AbortError).
 * @simplewebauthn/browser keeps these names on its WebAuthnError wrapper (`name = cause.name`) while passing the
 * browser's own message through, so the message text cannot be used to detect a cancellation.
 */
const PASSKEY_CANCELLATION_ERROR_NAMES = new Set(['NotAllowedError', 'AbortError']);

/**
 * isPasskeyCeremonyCancelled reports whether a passkey ceremony failed only because it was cancelled.
 * It accepts the value thrown by startAuthentication or startRegistration and returns true when the error name is
 * NotAllowedError or AbortError, which callers should treat as a silent cancellation instead of showing an error.
 */
export function isPasskeyCeremonyCancelled(error: unknown): boolean {
  if (typeof error !== 'object' || error === null || !('name' in error)) {
    return false;
  }
  const { name } = error as { name?: unknown };
  return typeof name === 'string' && PASSKEY_CANCELLATION_ERROR_NAMES.has(name);
}
