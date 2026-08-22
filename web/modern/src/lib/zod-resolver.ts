import { zodResolver as createZodResolver } from '@hookform/resolvers/zod';

/**
 * zodResolver creates a React Hook Form resolver from the supplied Zod schema,
 * optional schema parsing settings, and optional resolver settings, returning
 * a resolver that preserves the schema's distinct input and output types.
 */
export const zodResolver: typeof createZodResolver = createZodResolver;
