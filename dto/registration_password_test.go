package dto

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestRegistrationPasswordSharedFixtures checks the same policy cases used by Modern.
func TestRegistrationPasswordSharedFixtures(t *testing.T) {
	data, err := os.ReadFile("../web/modern/src/lib/testdata/registration-passwords.json")
	require.NoError(t, err)
	var cases []struct {
		Name     string `json:"name"`
		Password string `json:"password"`
		Valid    bool   `json:"valid"`
	}
	require.NoError(t, json.Unmarshal(data, &cases))
	require.NotEmpty(t, cases)
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			require.Equal(t, tc.Valid, IsRegistrationPasswordValid(tc.Password))
		})
	}
}

// TestRegistrationPasswordASCII checks every ASCII byte, including punctuation and controls.
func TestRegistrationPasswordASCII(t *testing.T) {
	for code := 0; code < 128; code++ {
		password := "Abcdefg12" + string(rune(code))
		require.Equal(t, code >= 33 && code <= 126, IsRegistrationPasswordValid(password), "ASCII code %d", code)
	}
}
