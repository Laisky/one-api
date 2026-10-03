// DuplicateTokenSource contains only settings accepted by the token creation API.
export interface DuplicateTokenSource {
  name: string;
  expired_time: number;
  remain_quota: number;
  unlimited_quota: boolean;
  models?: string | null;
  subnet?: string | null;
}

export const TOKEN_NAME_MAX_BYTES = 30;

// TokenDuplicateNameError identifies names that exceed the server's UTF-8 byte limit.
export class TokenDuplicateNameError extends Error {
  // constructor creates a validation error without including token data.
  constructor() {
    super('The duplicated token name exceeds the 30-byte limit.');
    this.name = 'TokenDuplicateNameError';
  }
}

/** nextDuplicateTokenName appends or increments a trailing decimal suffix without losing integer precision. */
export function nextDuplicateTokenName(name: string): string {
  const match = /-([0-9]+)$/.exec(name);
  // JavaScript's $ can also match before a final newline, which is not a suffix.
  if (!match || match.index + match[0].length !== name.length) return `${name}-1`;

  const digits = (match[1].replace(/^0+/, '') || '0').split('');
  let index = digits.length - 1;
  while (index >= 0 && digits[index] === '9') {
    digits[index--] = '0';
  }
  if (index < 0) digits.unshift('1');
  else digits[index] = String(Number(digits[index]) + 1);
  return `${name.slice(0, match.index)}-${digits.join('')}`;
}

/** buildDuplicateTokenPayload copies creation settings, excluding credentials, identities, status, and usage history. */
export function buildDuplicateTokenPayload(source: DuplicateTokenSource): DuplicateTokenSource {
  const name = nextDuplicateTokenName(source.name);
  if (new TextEncoder().encode(name).length > TOKEN_NAME_MAX_BYTES) {
    // Do not silently truncate or otherwise change the requested naming rule.
    throw new TokenDuplicateNameError();
  }
  return {
    name,
    expired_time: source.expired_time,
    remain_quota: source.remain_quota,
    unlimited_quota: source.unlimited_quota,
    models: source.models ?? null,
    subnet: source.subnet ?? null,
  };
}
