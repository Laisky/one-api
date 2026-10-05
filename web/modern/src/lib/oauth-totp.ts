import { api, isSafeInternalPath } from '@/lib/api';
import type { useAuthStore } from '@/lib/stores/auth';

/** OAuthLoginUser is the user payload accepted by the auth store's login action. */
export type OAuthLoginUser = Parameters<ReturnType<typeof useAuthStore.getState>['login']>[0];

/** OAUTH_TOTP_CODE_PATTERN matches exactly six ASCII digits, the only TOTP code shape the backend accepts. */
export const OAUTH_TOTP_CODE_PATTERN = /^\d{6}$/;

/** OAUTH_TOTP_REQUIRED_MESSAGE is the message the backend returns when a login needs a second factor. */
export const OAUTH_TOTP_REQUIRED_MESSAGE = 'totp_required';

/** OAUTH_TOTP_ENDPOINT completes a pending OAuth or WeChat login after the user proves TOTP possession. */
export const OAUTH_TOTP_ENDPOINT = '/api/oauth/totp';

/**
 * OAuthTotpVerifyResult describes the outcome of submitting a TOTP code for a pending OAuth login.
 * `success` carries the authenticated user, `invalid` means the challenge is still valid and the user may retry,
 * `rate_limited` means the backend throttled the attempt, `expired` means the pending challenge is gone and the
 * user must start over, and `error` covers transport or unexpected failures.
 */
export type OAuthTotpVerifyResult =
  | { kind: 'success'; user: OAuthLoginUser }
  | { kind: 'invalid'; message: string }
  | { kind: 'rate_limited'; message: string }
  | { kind: 'expired'; message: string }
  | { kind: 'error'; message: string };

/**
 * readResponseFlag reports whether the object `data` carries `key` set to a truthy boolean-like value.
 * It accepts `true`, the string `'true'`, and the number `1`, and returns false for anything else.
 */
const readResponseFlag = (data: unknown, key: string): boolean => {
  if (!data || typeof data !== 'object') return false;
  const value = (data as Record<string, unknown>)[key];
  return value === true || value === 'true' || value === 1;
};

/**
 * readResponseMessage returns the trimmed string `message` field of an API envelope, or an empty string when absent.
 */
const readResponseMessage = (body: unknown): string => {
  if (!body || typeof body !== 'object') return '';
  const message = (body as Record<string, unknown>).message;
  return typeof message === 'string' ? message.trim() : '';
};

/**
 * isTotpRequiredResponse reports whether an OAuth or WeChat login callback body is the password-login
 * `totp_required` challenge. It takes the parsed JSON body and returns true only for a failed envelope whose
 * message is `totp_required` or whose `data.totp_required` flag is set, so callers must stop retrying the
 * already-consumed callback and prompt for a TOTP code instead.
 */
export function isTotpRequiredResponse(body: unknown): boolean {
  if (!body || typeof body !== 'object') return false;
  const envelope = body as Record<string, unknown>;
  if (envelope.success !== false) return false;
  return readResponseMessage(body).toLowerCase() === OAUTH_TOTP_REQUIRED_MESSAGE || readResponseFlag(envelope.data, 'totp_required');
}

/**
 * isTotpExpiredResponse reports whether a POST /api/oauth/totp body says the pending challenge expired or never
 * started. It takes the parsed JSON body and returns true only for a failed envelope with `data.totp_expired` set.
 */
export function isTotpExpiredResponse(body: unknown): boolean {
  if (!body || typeof body !== 'object') return false;
  const envelope = body as Record<string, unknown>;
  return envelope.success === false && readResponseFlag(envelope.data, 'totp_expired');
}

/**
 * classifyTotpBody maps a POST /api/oauth/totp JSON body and its HTTP status to a verification result.
 * It takes the parsed body (possibly undefined) and the HTTP status (0 when unknown) and returns the result kind
 * together with the backend message, which may be empty when the backend did not send one.
 */
const classifyTotpBody = (body: unknown, status: number): OAuthTotpVerifyResult => {
  const message = readResponseMessage(body);
  if (status === 429) return { kind: 'rate_limited', message };
  if (isTotpExpiredResponse(body)) return { kind: 'expired', message };
  if (status >= 200 && status < 300 && body && typeof body === 'object' && (body as Record<string, unknown>).success === true) {
    return { kind: 'success', user: (body as Record<string, unknown>).data as OAuthLoginUser };
  }
  if (status >= 200 && status < 300) return { kind: 'invalid', message };
  return { kind: 'error', message };
};

/**
 * verifyOAuthTotpCode submits `code` to POST /api/oauth/totp to finish a pending OAuth or WeChat login.
 * It takes the six-digit code and returns a classified result; it never throws, because HTTP failures such as
 * 429 are surfaced by axios as rejections and are folded into the result here.
 */
export async function verifyOAuthTotpCode(code: string): Promise<OAuthTotpVerifyResult> {
  try {
    const response = await api.post(OAUTH_TOTP_ENDPOINT, { totp_code: code });
    return classifyTotpBody(response?.data, typeof response?.status === 'number' ? response.status : 200);
  } catch (error) {
    const httpResponse = (error as { response?: { status?: number; data?: unknown } } | null)?.response;
    if (httpResponse && typeof httpResponse.status === 'number') {
      return classifyTotpBody(httpResponse.data, httpResponse.status);
    }
    const message = error instanceof Error ? error.message : '';
    console.error(`OAuth TOTP verification request failed: ${message || 'unknown error'}`);
    return { kind: 'error', message };
  }
}

/**
 * resolveOAuthStateRedirect extracts the `redirect_to` target embedded in an OAuth `state` value.
 * It takes the raw state (possibly null) and returns the decoded path when it is a same-origin internal path
 * that does not point back at /login; otherwise it returns `fallback`, which defaults to `/`.
 */
export function resolveOAuthStateRedirect(state: string | null | undefined, fallback = '/'): string {
  if (!state || !state.includes('redirect_to=')) return fallback;
  const redirectTo = state.split('redirect_to=')[1];
  if (!redirectTo) return fallback;
  try {
    const decodedPath = decodeURIComponent(redirectTo);
    if (isSafeInternalPath(decodedPath) && !decodedPath.startsWith('/login')) {
      return decodedPath;
    }
  } catch (error) {
    console.error(`Invalid redirect_to parameter: ${error instanceof Error ? error.message : String(error)}`);
  }
  return fallback;
}
