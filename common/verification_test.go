package common

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestGenerateVerificationCodeLength tests that GenerateVerificationCode returns a code of the requested length.
func TestGenerateVerificationCodeLength(t *testing.T) {
	lengths := []int{0, 1, 4, 8, 16, 32}
	for _, length := range lengths {
		code := GenerateVerificationCode(length)
		if length == 0 {
			// Should return full UUID (length 32)
			require.Len(t, code, 32, "Expected code length 32 for length=0")
		} else {
			require.Len(t, code, length, "Expected code length %d", length)
		}
	}
}

// TestGenerateVerificationCodeUniqueness tests that GenerateVerificationCode generates unique codes.
func TestGenerateVerificationCodeUniqueness(t *testing.T) {
	codes := make(map[string]struct{})
	for range 100 {
		code := GenerateVerificationCode(8)
		_, exists := codes[code]
		require.False(t, exists, "Duplicate code generated: %s", code)
		codes[code] = struct{}{}
	}
}

// TestGenerateVerificationCodeZeroLength tests that length=0 returns a valid UUID.
func TestGenerateVerificationCodeZeroLength(t *testing.T) {
	code := GenerateVerificationCode(0)
	require.Len(t, code, 32, "Expected UUID length 32 for length=0")
}
