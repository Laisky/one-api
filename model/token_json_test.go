package model

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/config"
)

// These tests moved from asserting json.Marshal(Token) (the retired
// Token.MarshalJSON whitelist) to asserting json.Marshal(tok.ToResponse()), the
// explicit boundary DTO that replaced it. The external contract (prefixed key,
// UUID identifiers, no legacy int id/user_id) is unchanged.

func TestTokenToResponse_DefaultPrefix(t *testing.T) {
	// backup and restore
	old := config.TokenKeyPrefix
	config.TokenKeyPrefix = "sk-"
	defer func() { config.TokenKeyPrefix = old }()

	tok := Token{Id: 1, UserId: 2, Key: "abcdef"}
	b, err := json.Marshal(tok.ToResponse())
	require.NoError(t, err, "marshal error")
	got := string(b)
	require.True(t, containsJSONPair(got, `"key":"sk-abcdef"`), "expected key with sk- prefix, got: %s", got)
	require.NotContains(t, got, `"id"`, "token S2 strict-out must omit legacy int id")
	require.NotContains(t, got, `"user_id"`, "token S2 strict-out must omit legacy int user_id")
}

func TestTokenToResponse_CustomPrefix(t *testing.T) {
	old := config.TokenKeyPrefix
	config.TokenKeyPrefix = "custom-"
	defer func() { config.TokenKeyPrefix = old }()

	tok := Token{Id: 1, UserId: 2, Key: "abcdef"}
	b, err := json.Marshal(tok.ToResponse())
	require.NoError(t, err, "marshal error")
	got := string(b)
	require.True(t, containsJSONPair(got, `"key":"custom-abcdef"`), "expected key with custom- prefix, got: %s", got)
}

func TestTokenToResponse_StripsLegacyPrefix(t *testing.T) {
	old := config.TokenKeyPrefix
	config.TokenKeyPrefix = "sk-"
	defer func() { config.TokenKeyPrefix = old }()

	tok := Token{Id: 1, UserId: 2, Key: "sk-abcdef"}
	b, err := json.Marshal(tok.ToResponse())
	require.NoError(t, err, "marshal error")
	got := string(b)
	require.True(t, containsJSONPair(got, `"key":"sk-abcdef"`), "expected single sk- prefix, got: %s", got)
}

// TestTokenToResponse_EmitsExternalUUIDs verifies token S2 responses keep UUID identifiers.
func TestTokenToResponse_EmitsExternalUUIDs(t *testing.T) {
	userUUID := "018f0000-0000-7000-8000-000000000001"
	tok := Token{
		Id:       1,
		UUID:     "018f0000-0000-7000-8000-000000000002",
		UserId:   2,
		UserUUID: &userUUID,
		Key:      "abcdef",
	}
	b, err := json.Marshal(tok.ToResponse())
	require.NoError(t, err, "marshal error")
	got := string(b)
	require.True(t, containsJSONPair(got, `"uuid":"018f0000-0000-7000-8000-000000000002"`), "expected token uuid, got: %s", got)
	require.True(t, containsJSONPair(got, `"user_uuid":"018f0000-0000-7000-8000-000000000001"`), "expected token user uuid, got: %s", got)
	require.NotContains(t, got, `"id"`, "token S2 strict-out must omit legacy int id")
	require.NotContains(t, got, `"user_id"`, "token S2 strict-out must omit legacy int user_id")
}

// TestTokenRollbackDrillT29 verifies token S2 output can revert to S1 int+UUID output and re-apply cleanly.
func TestTokenRollbackDrillT29(t *testing.T) {
	userUUID := "018f0000-0000-7000-8000-000000000001"
	tok := Token{
		Id:       11,
		UUID:     "018f0000-0000-7000-8000-000000000002",
		UserId:   22,
		UserUUID: &userUUID,
		Key:      "abcdef",
		Name:     "pilot-token",
	}

	rollbackJSON, err := json.Marshal(tokenRollbackDTOFromToken(tok))
	require.NoError(t, err)
	var legacyReader struct {
		ID     int `json:"id"`
		UserID int `json:"user_id"`
	}
	require.NoError(t, json.Unmarshal(rollbackJSON, &legacyReader))
	require.Equal(t, 11, legacyReader.ID)
	require.Equal(t, 22, legacyReader.UserID)
	require.True(t, containsJSONPair(string(rollbackJSON), `"uuid":"018f0000-0000-7000-8000-000000000002"`))
	require.True(t, containsJSONPair(string(rollbackJSON), `"user_uuid":"018f0000-0000-7000-8000-000000000001"`))

	s2JSON, err := json.Marshal(tok.ToResponse())
	require.NoError(t, err)
	require.NotContains(t, string(s2JSON), `"id"`, "re-applied S2 output must omit legacy int id")
	require.NotContains(t, string(s2JSON), `"user_id"`, "re-applied S2 output must omit legacy int user_id")

	rollbackAgainJSON, err := json.Marshal(tokenRollbackDTOFromToken(tok))
	require.NoError(t, err)
	require.JSONEq(t, string(rollbackJSON), string(rollbackAgainJSON))
}

type tokenRollbackDTO struct {
	ID             int     `json:"id"`
	UUID           string  `json:"uuid"`
	UserID         int     `json:"user_id"`
	UserUUID       *string `json:"user_uuid"`
	Key            string  `json:"key"`
	Status         int     `json:"status"`
	Name           string  `json:"name"`
	CreatedTime    int64   `json:"created_time"`
	AccessedTime   int64   `json:"accessed_time"`
	ExpiredTime    int64   `json:"expired_time"`
	RemainQuota    int64   `json:"remain_quota"`
	UnlimitedQuota bool    `json:"unlimited_quota"`
	UsedQuota      int64   `json:"used_quota"`
	CreatedAt      int64   `json:"created_at"`
	UpdatedAt      int64   `json:"updated_at"`
	Models         *string `json:"models"`
	Subnet         *string `json:"subnet"`
}

// tokenRollbackDTOFromToken builds a pilot-resource S1 response with legacy int and UUID keys.
func tokenRollbackDTOFromToken(tok Token) tokenRollbackDTO {
	return tokenRollbackDTO{
		ID:             tok.Id,
		UUID:           tok.UUID,
		UserID:         tok.UserId,
		UserUUID:       tok.UserUUID,
		Key:            tok.Key,
		Status:         tok.Status,
		Name:           tok.Name,
		CreatedTime:    tok.CreatedTime,
		AccessedTime:   tok.AccessedTime,
		ExpiredTime:    tok.ExpiredTime,
		RemainQuota:    tok.RemainQuota,
		UnlimitedQuota: tok.UnlimitedQuota,
		UsedQuota:      tok.UsedQuota,
		CreatedAt:      tok.CreatedAt,
		UpdatedAt:      tok.UpdatedAt,
		Models:         tok.Models,
		Subnet:         tok.Subnet,
	}
}

// containsJSONPair is a tiny helper to avoid pulling extra deps
func containsJSONPair(s, pair string) bool {
	return len(s) >= len(pair) && (stringContains(s, pair))
}

func stringContains(s, sub string) bool {
	return (len(sub) == 0) || (len(s) >= len(sub) && indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	// simple search to avoid importing strings for a single use in tests
outer:
	for i := 0; i+len(sub) <= len(s); i++ {
		for j := 0; j < len(sub); j++ {
			if s[i+j] != sub[j] {
				continue outer
			}
		}
		return i
	}
	return -1
}
