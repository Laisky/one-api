import * as z from 'zod';

// Keep these limits aligned with the backend: usernames max 30 (model.User), passwords min 8, at most 30 on creation
// (dto.UserCreateRequest) and at most 20 on admin updates (controller.UpdateUser).
export const USER_USERNAME_MAX_LENGTH = 30;
export const USER_PASSWORD_MIN_LENGTH = 8;
export const USER_CREATE_PASSWORD_MAX_LENGTH = 30;
export const USER_EDIT_PASSWORD_MAX_LENGTH = 20;

type PolicyTranslator = (key: string, defaultValue: string, options?: Record<string, unknown>) => string;

/**
 * adminUserPasswordSchema builds the zod validator for the password field of the admin create/edit user page.
 * It takes whether an existing user is edited (an empty password then keeps the current one) and the page translator,
 * and returns a schema enforcing the backend length limits for that operation with localized messages.
 */
export function adminUserPasswordSchema(isEdit: boolean, tr: PolicyTranslator) {
  const maxLength = isEdit ? USER_EDIT_PASSWORD_MAX_LENGTH : USER_CREATE_PASSWORD_MAX_LENGTH;
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
      if (password.length < USER_PASSWORD_MIN_LENGTH) {
        ctx.addIssue({
          code: 'custom',
          message: tr('validation.password_min', 'Password must be at least {{count}} characters', { count: USER_PASSWORD_MIN_LENGTH }),
        });
      } else if (password.length > maxLength) {
        ctx.addIssue({
          code: 'custom',
          message: tr('validation.password_max', 'Password must be at most {{count}} characters', { count: maxLength }),
        });
      }
    });
}
