import * as z from 'zod';

// Both endpoints count Unicode code points. Creation validates the unmodified password
// (dto.UserCreateRequest); updates validate strings.TrimSpace(password) (controller.UpdateUser).
export const USER_USERNAME_MAX_LENGTH = 30;
export const USER_PASSWORD_MIN_LENGTH = 8;
export const USER_CREATE_PASSWORD_MAX_LENGTH = 30;
export const USER_EDIT_PASSWORD_MAX_LENGTH = 20;
// common.Password2Hash uses bcrypt, which rejects inputs longer than 72 UTF-8 bytes.
export const USER_PASSWORD_MAX_BYTES = 72;

type PolicyTranslator = (key: string, defaultValue: string, options?: Record<string, unknown>) => string;

// Go strings.TrimSpace uses Unicode White_Space: unlike JavaScript trim, it includes
// NEXT LINE (U+0085) and keeps BOM (U+FEFF). Use the same set without modifying form data.
const GO_EDGE_WHITESPACE = /^[\t\n\v\f\r\u0020\u0085\u00a0\u1680\u2000-\u200a\u2028\u2029\u202f\u205f\u3000]+|[\t\n\v\f\r\u0020\u0085\u00a0\u1680\u2000-\u200a\u2028\u2029\u202f\u205f\u3000]+$/gu;

/** validatePasswordLength checks the endpoint-specific code point count and adds localized issues to ctx. */
function validatePasswordLength(value: string, isEdit: boolean, tr: PolicyTranslator, ctx: z.RefinementCtx): void {
  const password = isEdit ? value.replace(GO_EDGE_WHITESPACE, '') : value;
  const length = Array.from(password).length;
  const maxLength = isEdit ? USER_EDIT_PASSWORD_MAX_LENGTH : USER_CREATE_PASSWORD_MAX_LENGTH;
  if (length < USER_PASSWORD_MIN_LENGTH) {
    ctx.addIssue({
      code: 'custom',
      message: tr('validation.password_min', 'Password must be at least {{count}} characters', { count: USER_PASSWORD_MIN_LENGTH }),
    });
  } else if (length > maxLength) {
    ctx.addIssue({
      code: 'custom',
      message: tr('validation.password_max', 'Password must be at most {{count}} characters', { count: maxLength }),
    });
  } else if (new TextEncoder().encode(password).length > USER_PASSWORD_MAX_BYTES) {
    ctx.addIssue({
      code: 'custom',
      message: tr('validation.password_max_bytes', 'Password must be at most {{count}} bytes in UTF-8', { count: USER_PASSWORD_MAX_BYTES }),
    });
  }
}

/** createUserPasswordSchema takes the dialog translator and returns its required, untrimmed creation password schema. */
export function createUserPasswordSchema(tr: PolicyTranslator) {
  return z.string().superRefine((password, ctx) => validatePasswordLength(password, false, tr, ctx));
}

/**
 * adminUserPasswordSchema builds the create/edit page password validator from the operation and translator.
 * Truly empty edit input keeps the current password because the form omits it from the update payload.
 * Nonempty edit input must satisfy the backend policy after Go whitespace trimming; parsed data stays unchanged.
 */
export function adminUserPasswordSchema(isEdit: boolean, tr: PolicyTranslator) {
  return z
    .string()
    .optional()
    .superRefine((value, ctx) => {
      const password = value ?? '';
      if (password === '') {
        if (!isEdit) {
          ctx.addIssue({ code: 'custom', message: tr('validation.password_required', 'Password is required') });
        }
        return;
      }
      validatePasswordLength(password, isEdit, tr, ctx);
    });
}
