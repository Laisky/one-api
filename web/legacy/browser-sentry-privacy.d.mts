/** PrivacyConfig contains validated immutable public deployment identifiers supplied by the reporter. */
export interface PrivacyConfig {
  site?: string;
  theme: string;
  release: string;
  environment: string;
}
/** PrivacyHint provides the original exception and mutable SDK attachment list to the privacy hook. */
export interface PrivacyHint {
  originalException?: unknown;
  attachments?: unknown[];
}
/** BreadcrumbHint provides SDK console input and optional browser DOM context. */
export interface BreadcrumbHint {
  input?: unknown[];
  event?: Event;
  target?: Element;
}
/** markPrivateError annotates an error without changing the object, UI message or stack. */
export function markPrivateError<T>(error: T, summary: string, expected?: boolean): T;
/** privateResponseError retains UI response text while marking expected private telemetry content. */
export function privateResponseError(message: string, summary: string): Error;
/** isExpectedRequestError checks an annotated error or bounded linked cause. */
export function isExpectedRequestError(error: unknown): boolean;
/** sanitizeURL preserves source paths while removing credentials, queries and fragments. */
export function sanitizeURL(value: string): string;
/** sanitizeText preserves diagnostic text while masking recognizable credential values. */
export function sanitizeText(value: string): string;
/** sanitizeBreadcrumb returns sanitized SDK breadcrumb data, or null for private browser input. */
export function sanitizeBreadcrumb<T extends object>(breadcrumb: T, hint?: BreadcrumbHint): T | null;
/** sanitizeEvent returns sanitized SDK event data with authoritative deployment identifiers, or null on failure. */
export function sanitizeEvent<T extends object>(event: T, config: PrivacyConfig, hint?: PrivacyHint): T | null;
