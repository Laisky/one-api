import { describe, expect, it } from 'vitest';
import { shouldHighlightTokenQuota, type Token } from '../TokensPage.impl';

const makeToken = (overrides: Partial<Token> = {}): Token => ({
  id: 1,
  name: 'Test token',
  key: 'sk-test',
  status: 1,
  remain_quota: 0,
  unlimited_quota: false,
  used_quota: 0,
  created_time: 0,
  accessed_time: 0,
  expired_time: -1,
  models: undefined,
  subnet: undefined,
  ...overrides,
});

describe('shouldHighlightTokenQuota', () => {
  it('returns false when user quota is unavailable', () => {
    const token = makeToken({ remain_quota: 500 });
    expect(shouldHighlightTokenQuota(token, null)).toBe(false);
  });

  it('returns false when user quota is unlimited', () => {
    const token = makeToken({ remain_quota: 500 });
    expect(shouldHighlightTokenQuota(token, -1)).toBe(false);
  });

  it('returns true when token quota exceeds user quota', () => {
    const token = makeToken({ remain_quota: 600 });
    expect(shouldHighlightTokenQuota(token, 500)).toBe(true);
  });

  it('returns false when token quota does not exceed user quota', () => {
    const token = makeToken({ remain_quota: 400 });
    expect(shouldHighlightTokenQuota(token, 500)).toBe(false);
  });

  it('returns true for unlimited tokens when user quota is finite', () => {
    const token = makeToken({ unlimited_quota: true, remain_quota: -1 });
    expect(shouldHighlightTokenQuota(token, 500)).toBe(true);
  });
});
