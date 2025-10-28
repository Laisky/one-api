package common

import "golang.org/x/crypto/bcrypt"

// Password2Hash converts the provided plaintext password into a bcrypt hash using the default cost.
// It returns the hashed password string and any error emitted by the bcrypt library.
func Password2Hash(password string) (string, error) {
	passwordBytes := []byte(password)
	hashedPassword, err := bcrypt.GenerateFromPassword(passwordBytes, bcrypt.DefaultCost)
	return string(hashedPassword), err
}

// ValidatePasswordAndHash checks whether the plaintext password matches the supplied bcrypt hash.
// It returns true when the hash corresponds to the password, otherwise false.
func ValidatePasswordAndHash(password string, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}
