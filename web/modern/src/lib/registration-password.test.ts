import { describe, expect, it } from 'vitest';
import { registrationPasswordTranslations } from '../i18n/locales/registration-password';
import { getRegistrationPasswordChecks, isRegistrationPasswordValid } from './registration-password';

describe('Modern registration password requirements', () => {
  const cases: Array<[string, boolean]> = [
    ['', false],
    ['Abcdefg12', false],
    ['Abcdefgh12', true],
    ['a1' + 'x'.repeat(18), true],
    ['a1' + 'x'.repeat(19), false],
    ['abcdefghij', false],
    ['1234567890', false],
    ['ABCDEFGHI1', true],
    ['abcdefghi1', true],
    ['  Abcdefg12!  ', true],
    ['Abcdefg12~', true],
    ['Abcdefg12é', false],
    ['Abcdefg12中', false],
    ['Abcdefg12😀', false],
    ['Abcdefghi１', false],
    ['Abcdefg12\u200b', false],
    ['Abcdefg12\u00a0', false],
    ['Abcdefg12\u007f', false],
    ['Abcdefg12\n', false],
    ['Abcdefg12\r', false],
    ['Abcdefg12\t', false],
    ['Abcdefg12\0', false],
  ];
  it.each(cases)('validates %j as %s without altering the input', (password, valid) => {
    expect(isRegistrationPasswordValid(password)).toBe(valid);
    expect(Object.values(getRegistrationPasswordChecks(password)).every(Boolean)).toBe(valid);
  });

  it('reports each requirement independently', () => {
    expect(getRegistrationPasswordChecks('short')).toEqual({ length: false, letter: true, number: false, ascii: true });
    expect(getRegistrationPasswordChecks('abcdefghij')).toEqual({ length: true, letter: true, number: false, ascii: true });
    expect(getRegistrationPasswordChecks('Abcdefg12é')).toEqual({ length: true, letter: true, number: true, ascii: false });
  });

  it('accepts every printable ASCII character and rejects every ASCII control character', () => {
    for (let code = 0; code < 128; code += 1) {
      const password = `Abcdefg12${String.fromCharCode(code)}`;
      expect(isRegistrationPasswordValid(password)).toBe(code >= 32 && code <= 126);
    }
  });

  it('keeps all five locales complete with matching interpolation placeholders', () => {
    const english = registrationPasswordTranslations.en;
    expect(Object.keys(registrationPasswordTranslations).sort()).toEqual(['en', 'es', 'fr', 'ja', 'zh']);
    for (const translations of Object.values(registrationPasswordTranslations)) {
      expect(Object.keys(translations).sort()).toEqual(Object.keys(english).sort());
      for (const key of Object.keys(english) as Array<keyof typeof english>) {
        expect(translations[key].trim()).not.toBe('');
        expect(translations[key].match(/\{\{\w+\}\}/g)?.sort()).toEqual(english[key].match(/\{\{\w+\}\}/g)?.sort());
      }
    }
  });
});
