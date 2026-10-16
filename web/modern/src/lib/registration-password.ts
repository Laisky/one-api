/** RegistrationPasswordChecks describes the requirements for a new Modern registration password. */
export interface RegistrationPasswordChecks {
  length: boolean;
  letter: boolean;
  number: boolean;
  ascii: boolean;
}

export const REGISTRATION_PASSWORD_MIN_LENGTH = 10;
// The registration API and user password validators accept up to 30 characters.
// Keep this limit visible rather than silently truncating a generated password.
export const REGISTRATION_PASSWORD_MAX_LENGTH = 30;

/** getRegistrationPasswordChecks evaluates the supplied password without modifying it and returns each requirement's result. */
export function getRegistrationPasswordChecks(password: string): RegistrationPasswordChecks {
  return {
    length: password.length >= REGISTRATION_PASSWORD_MIN_LENGTH && password.length <= REGISTRATION_PASSWORD_MAX_LENGTH,
    letter: /[A-Za-z]/.test(password),
    number: /[0-9]/.test(password),
    // Spaces and punctuation are allowed; control characters and Unicode are not.
    ascii: password.length > 0 && !/[^\x20-\x7e]/.test(password),
  };
}

/** isRegistrationPasswordValid checks the supplied password against the Modern signup requirements and returns whether all are met. */
export function isRegistrationPasswordValid(password: string): boolean {
  return Object.values(getRegistrationPasswordChecks(password)).every(Boolean);
}
