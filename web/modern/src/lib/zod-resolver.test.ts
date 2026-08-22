import type { Resolver } from 'react-hook-form';
import { describe, expect, expectTypeOf, it } from 'vitest';
import { z } from 'zod';

import { zodResolver } from './zod-resolver';

const coercedSchema = z.object({ count: z.coerce.number() });

describe('zodResolver', () => {
  it('preserves coerced schema input and output types', () => {
    expectTypeOf(zodResolver(coercedSchema)).toEqualTypeOf<
      Resolver<z.input<typeof coercedSchema>, unknown, z.output<typeof coercedSchema>>
    >();
  });

  it('returns the coerced output at runtime', async () => {
    const result = await zodResolver(coercedSchema)({ count: '42' }, undefined, {
      criteriaMode: 'firstError',
      fields: {},
      names: ['count'],
      shouldUseNativeValidation: false,
    });

    expect(result).toEqual({ errors: {}, values: { count: 42 } });
  });
});
