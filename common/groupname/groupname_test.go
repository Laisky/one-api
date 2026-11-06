package groupname

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestValidateSharedFixtures checks the group-name cases shared with the Modern frontend.
func TestValidateSharedFixtures(t *testing.T) {
	data, err := os.ReadFile("../../web/modern/src/lib/testdata/group-names.json")
	require.NoError(t, err)
	var cases []struct {
		Name  string `json:"name"`
		Group string `json:"group"`
		Valid bool   `json:"valid"`
	}
	require.NoError(t, json.Unmarshal(data, &cases))
	require.NotEmpty(t, cases)
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			require.Equal(t, tc.Valid, IsValid(tc.Group), "Validate(%q) = %v", tc.Group, Validate(tc.Group))
		})
	}
}

// TestValidateASCII checks every ASCII byte so the allowed symbol set cannot drift silently.
func TestValidateASCII(t *testing.T) {
	for code := range 128 {
		name := "vip" + string(rune(code))
		allowed := (code >= '0' && code <= '9') || (code >= 'A' && code <= 'Z') || (code >= 'a' && code <= 'z') ||
			code == '.' || code == '_' || code == '-'
		require.Equal(t, allowed, IsValid(name), "code %d", code)
	}
}

// TestValidateMessages checks that rejection reasons are specific enough for administrators.
func TestValidateMessages(t *testing.T) {
	require.ErrorContains(t, Validate(""), "cannot be empty")
	require.ErrorContains(t, Validate(" vip"), "whitespace")
	require.ErrorContains(t, Validate(strings.Repeat("a", MaxLength+1)), "cannot exceed 32 characters")
	require.ErrorContains(t, Validate("my group"), "may only contain")
}

// TestNormalizeTrimsWithoutChangingCase checks that normalization only removes surrounding whitespace.
func TestNormalizeTrimsWithoutChangingCase(t *testing.T) {
	require.Equal(t, "VIP", Normalize(" \tVIP\n"))
	require.NoError(t, Validate(Normalize("  team_a  ")))
}

// TestParseList checks trimming, empty-entry removal and legacy case-insensitive de-duplication.
func TestParseList(t *testing.T) {
	require.Nil(t, ParseList(""))
	require.Nil(t, ParseList(" , ,"))
	require.Equal(t, []string{"default", "vip"}, ParseList("default, vip"))
	require.Equal(t, []string{"Default", "VIP"}, ParseList(" Default,default,VIP,vip "))
	require.Equal(t, "default,vip", JoinList(ParseList(" default ,vip,")))
}
