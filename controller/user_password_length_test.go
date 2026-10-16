package controller

import (
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/dto"
	"github.com/Laisky/one-api/model"
)

// TestUserPasswordLengthLimits verifies that the real request/model validators
// accept 8-30 characters, preserving the API minimum while extending the maximum.
func TestUserPasswordLengthLimits(t *testing.T) {
	for _, length := range []int{0, 7, 8, 9, 10, 20, 21, 29, 30, 31} {
		t.Run(strconv.Itoa(length), func(t *testing.T) {
			password := strings.Repeat("a", length)
			inputs := []struct {
				name  string
				value any
			}{
				{"register", &dto.UserRegisterRequest{Password: password}},
				{"admin_create", &dto.UserCreateRequest{Password: password}},
				{"self_update", &dto.UserSelfUpdateRequest{Password: password}},
				{"model", &model.User{Password: password}},
			}
			for _, input := range inputs {
				t.Run(input.name, func(t *testing.T) {
					err := common.Validate.Struct(input.value)
					if length >= 8 && length <= 30 {
						require.NoError(t, err)
					} else {
						require.Error(t, err)
					}
				})
			}
		})
	}
}

// TestUserPasswordMaximumDoesNotRelaxDisplayName checks that extending the
// password limit does not accidentally change the separate 20-character name cap.
func TestUserPasswordMaximumDoesNotRelaxDisplayName(t *testing.T) {
	password := "A1" + strings.Repeat("x", 28)
	name := strings.Repeat("n", 21)
	for _, input := range []any{
		&dto.UserRegisterRequest{Password: password, DisplayName: name},
		&dto.UserCreateRequest{Password: password, DisplayName: name},
		&dto.UserSelfUpdateRequest{Password: password, DisplayName: name},
		&model.User{Password: password, DisplayName: name},
	} {
		require.Error(t, common.Validate.Struct(input))
	}
}

// TestUserPasswordThirtyCharacterHashRoundTrip verifies that all 30 ASCII
// characters participate in the existing hash and comparison implementation.
func TestUserPasswordThirtyCharacterHashRoundTrip(t *testing.T) {
	password := "A1" + strings.Repeat("x", 28)
	require.Len(t, password, 30)
	hash, err := common.Password2Hash(password)
	require.NoError(t, err)
	require.True(t, common.ValidatePasswordAndHash(password, hash))
	require.False(t, common.ValidatePasswordAndHash(password[:20], hash))
	require.False(t, common.ValidatePasswordAndHash(password[:20]+"y"+password[21:], hash))
	require.False(t, common.ValidatePasswordAndHash(password[:29]+"y", hash))

	// A maximum-length expansion must not force a reset of shorter credentials.
	legacyPassword := "legacy12"
	legacyHash, err := common.Password2Hash(legacyPassword)
	require.NoError(t, err)
	require.True(t, common.ValidatePasswordAndHash(legacyPassword, legacyHash))
}
