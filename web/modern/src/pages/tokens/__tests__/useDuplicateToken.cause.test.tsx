import { renderHook } from '@testing-library/react';
import { expect, it, vi } from 'vitest';

import { useDuplicateResource } from '@/hooks/useDuplicateResource';
import { api } from '@/lib/api';
import { TokenDuplicateNameError } from '../duplicateToken';
import { useDuplicateToken } from '../useDuplicateToken';

vi.mock('@/hooks/useDuplicateResource', () => ({ useDuplicateResource: vi.fn() }));
vi.mock('@/lib/api', () => ({ api: { get: vi.fn(), post: vi.fn() } }));

it('preserves the safe validation cause behind the localized error without retaining token data', async () => {
  const source = {
    name: '令'.repeat(10),
    key: 'synthetic-private-token',
    expired_time: -1,
    remain_quota: 100,
    unlimited_quota: false,
  };
  vi.mocked(api.get).mockResolvedValue({ data: { success: true, data: source } });
  renderHook(() => useDuplicateToken(vi.fn()));
  const { create } = vi.mocked(useDuplicateResource).mock.calls[0][0];
  const error = await create('synthetic-reference').catch((cause: unknown) => cause);

  expect(error).toBeInstanceOf(Error);
  const localized = error as Error;
  expect(localized.message).toBe('The new name exceeds 30 bytes. Shorten the source token name before duplicating it.');
  expect(localized.cause).toBeInstanceOf(TokenDuplicateNameError);
  expect((localized.cause as Error).message).toBe('The duplicated token name exceeds the 30-byte limit.');
  expect(Object.keys(localized.cause as Error)).toEqual(['name']);
  const retainedCause = localized.cause as Error;
  expect(Object.getOwnPropertyNames(retainedCause).sort()).toEqual(['message', 'name', 'stack']);
  const retainedText = [localized.message, localized.stack, retainedCause.message, retainedCause.stack].join('\n');
  expect(retainedText).not.toContain(source.key);
  expect(retainedText).not.toContain(source.name);
  expect(api.post).not.toHaveBeenCalled();
});
