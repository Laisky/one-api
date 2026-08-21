import { zodResolver as createZodResolver } from '@hookform/resolvers/zod';
import type { FieldValues, Resolver } from 'react-hook-form';

/**
 * Preserve Modern's existing single-shape React Hook Form contract while
 * allowing Zod 4 schemas to coerce and default values internally.
 */
export function zodResolver<TFieldValues extends FieldValues = any>(
  schema: any,
  schemaOptions?: any,
  resolverOptions?: any,
): Resolver<TFieldValues> {
  return createZodResolver(
    schema,
    schemaOptions,
    resolverOptions,
  ) as Resolver<TFieldValues>;
}
