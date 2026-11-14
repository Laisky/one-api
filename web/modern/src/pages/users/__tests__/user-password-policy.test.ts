import { describe, expect, it } from 'vitest';
import cases from './testdata/admin-user-passwords.json';
import { createUserPasswordSchema, adminUserPasswordSchema } from '../user-password-policy';

/** translate interpolates counts in default policy messages for the tests. */
const translate = (_key: string, fallback: string, options?: Record<string, unknown>) =>
  fallback.replace('{{count}}', String(options?.count));

// CreateUser validates the original string with go-playground/validator (rune counts).
// UpdateUser validates strings.TrimSpace(password) with utf8.RuneCountInString.
// Both then hash using bcrypt, which rejects more than 72 UTF-8 bytes.
describe('admin password policy matches backend Unicode and whitespace semantics', () => {
  it.each(cases)('$name', ({ password, createValid, editValid }) => {
    expect(createUserPasswordSchema(translate).safeParse(password).success).toBe(createValid);
    expect(adminUserPasswordSchema(false, translate).safeParse(password).success).toBe(createValid);
    expect(adminUserPasswordSchema(true, translate).safeParse(password).success).toBe(editValid);
  });

  it('allows only truly empty or omitted edit input to keep the current password', () => {
    for (const password of ['', undefined]) {
      expect(adminUserPasswordSchema(true, translate).safeParse(password).success).toBe(true);
      expect(adminUserPasswordSchema(false, translate).safeParse(password).success).toBe(false);
    }
  });

  it('preserves the submitted string because the update endpoint owns normalization', () => {
    const password = '\u0085  ' + '😀'.repeat(8) + '  \u3000';
    expect(adminUserPasswordSchema(true, translate).parse(password)).toBe(password);
    expect(adminUserPasswordSchema(false, translate).parse(' abc123 ')).toBe(' abc123 ');
  });
});
