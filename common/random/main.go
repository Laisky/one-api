package random

import (
	"crypto/rand"
	"math/big"
	"strings"

	"github.com/Laisky/errors/v2"
	gutils "github.com/Laisky/go-utils/v6"
)

// GetUUID generates a UUID and returns it as a string without hyphens.
// It uses [github.com/google/uuid] for UUID generation.
//
// [github.com/google/uuid]: https://pkg.go.dev/github.com/google/uuid
func GetUUID() string {
	code := gutils.UUID7()
	code = strings.ReplaceAll(code, "-", "")
	return code
}

// GetUUIDWithHyphens generates a UUID string that preserves hyphens.
func GetUUIDWithHyphens() string {
	return gutils.UUID7()
}

const keyChars = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
const keyNumbers = "0123456789"

// GenerateKey creates a 48-character key consisting of 16 random characters
// followed by a modified UUID. This provides a unique, secure identifier
// suitable for authentication tokens or similar purposes.
func GenerateKey() string {
	key := make([]byte, 48)
	prefix := randomStringFromCharset(16, keyChars)
	copy(key[:16], prefix)
	uuid_ := GetUUID()
	for i := range 32 {
		c := uuid_[i]
		if i%2 == 0 && c >= 'a' && c <= 'z' {
			c = c - 'a' + 'A'
		}
		key[i+16] = c
	}
	return string(key)
}

// randomStringFromCharset generates a random string of the specified length using the provided charset.
func randomStringFromCharset(length int, charset string) string {
	key := make([]byte, length)
	for i := range length {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			panic(errors.Wrapf(err, "generating random string from charset %q and length %d", charset, length))
		}

		key[i] = charset[n.Int64()]
	}
	return string(key)
}

// GetRandomString generates a random string of the specified length
// using a mix of numbers and letters (both uppercase and lowercase).
// It uses [crypto/rand] for secure random number generation.
func GetRandomString(length int) string {
	return randomStringFromCharset(length, keyChars)
}

// GetRandomNumberString generates a random string of the specified length
// using only numeric characters (0-9). It uses [crypto/rand] for secure
// random number generation.
func GetRandomNumberString(length int) string {
	return randomStringFromCharset(length, keyNumbers)
}

// RandRange returns a random number between min and max (max is not included).
// If min == max, returns min. If min > max, panics.
func RandRange(min, max int) int {
	if min == max {
		return min
	}
	if min > max {
		panic(errors.Errorf("RandRange: min (%d) > max (%d)", min, max))
	}
	n, err := rand.Int(rand.Reader, big.NewInt(int64(max-min)))
	if err != nil {
		panic(errors.Wrapf(err, "generating random number between %d and %d", min, max))
	}
	return min + int(n.Int64())
}
