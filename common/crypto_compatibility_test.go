package common

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

// TestPasswordStorageCompatibility preserves bcrypt records across utility upgrades.
// It checks synthetic existing records, the current writer, and rejection controls.
func TestPasswordStorageCompatibility(t *testing.T) {
	const password = "consumer-fixture-only"
	// This synthetic record was generated independently using Python bcrypt at cost 10.
	const existing = "$2b$10$buyqFKq96t2LyPhnDVGPSeI1c4Xh9sT1C3U5yI.3bL2q54eL0PI6u"
	for _, hash := range []string{existing, strings.Replace(existing, "$2b$", "$2a$", 1)} {
		require.True(t, ValidatePasswordAndHash(password, hash))
		require.False(t, ValidatePasswordAndHash("incorrect-fixture-only", hash))
		require.False(t, ValidatePasswordAndHash("", hash))
	}
	require.False(t, ValidatePasswordAndHash(password, ""))
	require.False(t, ValidatePasswordAndHash(password, "malformed-hash"))
	hash, err := Password2Hash(password)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(hash, "$2a$"), "stored passwords must remain bcrypt")
	cost, err := bcrypt.Cost([]byte(hash))
	require.NoError(t, err)
	require.Equal(t, bcrypt.DefaultCost, cost)
	require.NoError(t, bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)),
		"the existing bcrypt reader must accept newly written records")
	require.True(t, ValidatePasswordAndHash(password, hash))
	require.False(t, ValidatePasswordAndHash("incorrect-fixture-only", hash))
	oversized, err := Password2Hash(strings.Repeat("x", 73))
	require.Error(t, err)
	require.Empty(t, oversized, "invalid input must not produce a stored record")
}
