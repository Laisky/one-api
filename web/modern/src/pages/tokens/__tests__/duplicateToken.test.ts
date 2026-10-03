import { describe, expect, it } from 'vitest';

import { buildDuplicateTokenPayload, nextDuplicateTokenName, TokenDuplicateNameError } from '../duplicateToken';
import { tokenDuplicateTranslations } from '@/i18n/locales/token-duplicate';

const source = {
  id: 7,
  uuid: 'source-uuid',
  user_id: 11,
  user_uuid: 'owner-uuid',
  key: 'source-secret-must-not-be-copied',
  name: 'production',
  status: 2,
  expired_time: 1800000000,
  remain_quota: 12500,
  unlimited_quota: false,
  models: 'model-a,model-b',
  subnet: '192.0.2.0/24,2001:db8::/32',
  used_quota: 7500,
  created_time: 100,
  accessed_time: 200,
  deleted_at: null,
};

describe('nextDuplicateTokenName', () => {
  it.each([
    ['production', 'production-1'],
    ['production-1', 'production-2'],
    ['production-9', 'production-10'],
    ['production-99', 'production-100'],
    ['production-0', 'production-1'],
    ['production-009', 'production-10'],
    ['production-000', 'production-1'],
    ['production-2-8', 'production-2-9'],
    ['production-8-extra', 'production-8-extra-1'],
    ['production_8', 'production_8-1'],
    ['production-', 'production--1'],
    ['production-1.5', 'production-1.5-1'],
    ['production-8 ', 'production-8 -1'],
    ['production-8\n', 'production-8\n-1'],
    ['production-１２', 'production-１２-1'],
    ['令牌-9', '令牌-10'],
    ['🔑', '🔑-1'],
    ['', '-1'],
    ['x-9007199254740992', 'x-9007199254740993'],
    ['x-99999999999999999999', 'x-100000000000000000000'],
  ])('copies %j as %j', (name, expected) => {
    expect(nextDuplicateTokenName(name)).toBe(expected);
  });
});

describe('buildDuplicateTokenPayload', () => {
  it('copies every creation setting but never credentials, identity, status, or usage history', () => {
    const frozen = Object.freeze({ ...source });
    expect(buildDuplicateTokenPayload(frozen)).toEqual({
      name: 'production-1',
      expired_time: source.expired_time,
      remain_quota: source.remain_quota,
      unlimited_quota: false,
      models: source.models,
      subnet: source.subnet,
    });
    expect(frozen).toEqual(source);
  });

  it('preserves unlimited quota, no expiration, and null restrictions', () => {
    expect(buildDuplicateTokenPayload({ ...source, unlimited_quota: true, expired_time: -1, models: null, subnet: null })).toEqual({
      name: 'production-1',
      expired_time: -1,
      remain_quota: source.remain_quota,
      unlimited_quota: true,
      models: null,
      subnet: null,
    });
  });

  it('normalizes missing optional restrictions without changing empty restrictions or zero quota', () => {
    const unrestricted = {
      name: source.name,
      expired_time: source.expired_time,
      remain_quota: source.remain_quota,
      unlimited_quota: source.unlimited_quota,
    };
    expect(buildDuplicateTokenPayload(unrestricted)).toMatchObject({ models: null, subnet: null });
    expect(buildDuplicateTokenPayload({ ...source, models: '', subnet: '', remain_quota: 0 })).toMatchObject({
      models: '',
      subnet: '',
      remain_quota: 0,
    });
  });

  it.each(['a'.repeat(28), '令'.repeat(9), '🔑'.repeat(7), `${'a'.repeat(28)}-1`])(
    'accepts a copied name within 30 UTF-8 bytes: %j',
    (name) => {
      expect(new TextEncoder().encode(buildDuplicateTokenPayload({ ...source, name }).name).length).toBeLessThanOrEqual(30);
    }
  );

  it.each(['a'.repeat(29), 'a'.repeat(30), '令'.repeat(10), '🔑'.repeat(8), `${'a'.repeat(28)}-9`])(
    'rejects overlong names instead of silently truncating: %j',
    (name) => {
      expect(() => buildDuplicateTokenPayload({ ...source, name })).toThrow(TokenDuplicateNameError);
    }
  );
});

describe('token duplicate translations', () => {
  it.each(Object.entries(tokenDuplicateTranslations))('has every message in %s', (_language, messages) => {
    expect(Object.keys(messages).sort()).toEqual(Object.keys(tokenDuplicateTranslations.en).sort());
    Object.values(messages).forEach((message) => expect(message.trim()).not.toBe(''));
    expect(messages.success).toContain('{{name}}');
  });
});
