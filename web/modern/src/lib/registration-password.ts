/** RegistrationPasswordChecks describes the requirements for a new registration password. */
export interface RegistrationPasswordChecks {
  length: boolean;
  letter: boolean;
  number: boolean;
  ascii: boolean;
}

export const REGISTRATION_PASSWORD_MIN_LENGTH = 10;
// Keep these limits aligned with dto/registration_password.go. Both validators
// are tested against the same testdata/registration-passwords.json fixtures.
export const REGISTRATION_PASSWORD_MAX_LENGTH = 30;

/** getRegistrationPasswordChecks evaluates the supplied password without modifying it and returns each requirement's result. */
export function getRegistrationPasswordChecks(password: string): RegistrationPasswordChecks {
  return {
    length: password.length >= REGISTRATION_PASSWORD_MIN_LENGTH && password.length <= REGISTRATION_PASSWORD_MAX_LENGTH,
    letter: /[A-Za-z]/.test(password),
    number: /[0-9]/.test(password),
    // Allow every visible ASCII symbol; reject spaces, controls and Unicode.
    // A negated search also rejects trailing line terminators that $ can overlook.
    ascii: password.length > 0 && !/[^\x21-\x7e]/.test(password),
  };
}

/** isRegistrationPasswordValid checks the supplied password against the signup requirements and returns whether all are met. */
export function isRegistrationPasswordValid(password: string): boolean {
  return Object.values(getRegistrationPasswordChecks(password)).every(Boolean);
}
