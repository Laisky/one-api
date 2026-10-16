import { describe, expect, it } from 'vitest';
import cases from './testdata/registration-passwords.json';
import { registrationPasswordTranslations } from '../i18n/locales/registration-password';
import { getRegistrationPasswordChecks, isRegistrationPasswordValid } from './registration-password';

describe('Modern registration password requirements', () => {
  it.each(cases)('validates the shared backend fixture $name', ({ password, valid }) => {
    expect(isRegistrationPasswordValid(password)).toBe(valid);
    expect(Object.values(getRegistrationPasswordChecks(password)).every(Boolean)).toBe(valid);
  });

  it('reports each requirement independently', () => {
    expect(getRegistrationPasswordChecks('short')).toEqual({ length: false, letter: true, number: false, ascii: true });
    expect(getRegistrationPasswordChecks('abcdefghij')).toEqual({ length: true, letter: true, number: false, ascii: true });
    expect(getRegistrationPasswordChecks('Abcdefg12é')).toEqual({ length: true, letter: true, number: true, ascii: false });
    expect(getRegistrationPasswordChecks('Abcdefgh12 ')).toEqual({ length: true, letter: true, number: true, ascii: false });
  });

  it('accepts every visible ASCII character and rejects spaces and every ASCII control character', () => {
    for (let code = 0; code < 128; code += 1) {
      const password = `Abcdefg12${String.fromCharCode(code)}`;
      expect(isRegistrationPasswordValid(password)).toBe(code >= 33 && code <= 126);
    }
  });

  it('keeps all five locales complete with matching interpolation placeholders', () => {
    const english = registrationPasswordTranslations.en;
    expect(Object.keys(registrationPasswordTranslations).sort()).toEqual(['en', 'es', 'fr', 'ja', 'zh']);
    for (const translations of Object.values(registrationPasswordTranslations)) {
      expect(Object.keys(translations).sort()).toEqual(Object.keys(english).sort());
      for (const key of Object.keys(english) as Array<keyof typeof english>) {
        expect(translations[key].trim()).not.toBe('');
        if (key === 'length' || key === 'invalid') {
          expect(translations[key]).toContain('30');
          expect(translations[key]).not.toContain('20');
        }
        expect(translations[key].match(/\{\{\w+\}\}/g)?.sort()).toEqual(english[key].match(/\{\{\w+\}\}/g)?.sort());
      }
    }
  });
});
