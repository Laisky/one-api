package dto

// RegistrationPasswordMinLength is the product minimum for new password signups.
const RegistrationPasswordMinLength = 10

// RegistrationPasswordMaxLength is the product maximum for new password signups.
const RegistrationPasswordMaxLength = 30

// RegistrationPasswordError is public guidance and never includes submitted credentials.
const RegistrationPasswordError = "Use 10-30 characters, including at least one ASCII letter and one number. ASCII punctuation is allowed; spaces, other whitespace, control characters and non-ASCII characters are not allowed."

// IsRegistrationPasswordValid checks the supplied new signup password and returns
// whether it meets the product policy. It does not trim, normalize, log or hash
// the value. Do not use it to validate existing credentials during login: an
// older password may legitimately contain spaces or non-ASCII characters.
func IsRegistrationPasswordValid(password string) bool {
	if len(password) < RegistrationPasswordMinLength || len(password) > RegistrationPasswordMaxLength {
		return false
	}
	hasLetter, hasNumber := false, false
	for i := 0; i < len(password); i++ {
		ch := password[i]
		// All visible ASCII punctuation is supported, including quotes and
		// backslashes. This is a product character policy, not injection defense.
		if ch < 0x21 || ch > 0x7e {
			return false
		}
		hasLetter = hasLetter || (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z')
		hasNumber = hasNumber || (ch >= '0' && ch <= '9')
	}
	return hasLetter && hasNumber
}
