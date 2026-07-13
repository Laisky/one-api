package model

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// containsChannelUUID reports whether the channel slice holds a row with the given uuid.
func containsChannelUUID(channels []*Channel, uuid string) bool {
	for _, c := range channels {
		if c.UUID == uuid {
			return true
		}
	}
	return false
}

// containsUserUUID reports whether the user slice holds a row with the given uuid.
func containsUserUUID(users []*User, uuid string) bool {
	for _, u := range users {
		if u.UUID == uuid {
			return true
		}
	}
	return false
}

// containsTokenUUID reports whether the token slice holds a row with the given uuid.
func containsTokenUUID(tokens []*Token, uuid string) bool {
	for _, tk := range tokens {
		if tk.UUID == uuid {
			return true
		}
	}
	return false
}

// containsRedemptionUUID reports whether the redemption slice holds a row with the given uuid.
func containsRedemptionUUID(redemptions []*Redemption, uuid string) bool {
	for _, r := range redemptions {
		if r.UUID == uuid {
			return true
		}
	}
	return false
}

// containsLogUUID reports whether the log slice holds a row with the given uuid.
func containsLogUUID(logs []*Log, uuid string) bool {
	for _, l := range logs {
		if l.UUID == uuid {
			return true
		}
	}
	return false
}

// TestSearchByUUID verifies that every list-page search accepts a canonical UUID as its
// keyword and returns the matching row, while user-scoped and provisional scopes are
// preserved so a UUID keyword can never leak rows outside the caller's scope.
//
// Parameters:
//   - t: active test handle.
//
// Return values: none.
func TestSearchByUUID(t *testing.T) {
	db := setupMigrationTestDB(t)
	originalDB := DB
	originalLOGDB := LOG_DB
	DB = db
	LOG_DB = db
	t.Cleanup(func() {
		DB = originalDB
		LOG_DB = originalLOGDB
	})

	require.NoError(t, migrateDB())

	// Two distinct owners let us assert cross-user isolation on scoped searches.
	userA := &User{Username: "search-uuid-user-a", Password: "hash-a", AccessToken: "search-uuid-access-a", AffCode: "search-uuid-aff-a"}
	require.NoError(t, DB.Create(userA).Error)
	userB := &User{Username: "search-uuid-user-b", Password: "hash-b", AccessToken: "search-uuid-access-b", AffCode: "search-uuid-aff-b"}
	require.NoError(t, DB.Create(userB).Error)

	channelA := &Channel{Type: 1, Name: "search-uuid-channel-a", Models: "gpt-4o", Config: "{}"}
	require.NoError(t, DB.Create(channelA).Error)

	tokenA := &Token{UserId: userA.Id, Key: "search-uuid-token-key-a", Name: "search-uuid-token-a"}
	require.NoError(t, DB.Create(tokenA).Error)
	tokenB := &Token{UserId: userB.Id, Key: "search-uuid-token-key-b", Name: "search-uuid-token-b"}
	require.NoError(t, DB.Create(tokenB).Error)

	redemptionA := &Redemption{UserId: userA.Id, Key: "search-uuid-redemption-a", Name: "search-uuid-redemption"}
	require.NoError(t, DB.Create(redemptionA).Error)

	logA := &Log{UserId: userA.Id, ChannelId: channelA.Id, Type: LogTypeConsume, TokenName: "search-uuid-token-a", Content: "consume entry for user a"}
	require.NoError(t, DB.Create(logA).Error)
	logB := &Log{UserId: userB.Id, ChannelId: channelA.Id, Type: LogTypeConsume, TokenName: "search-uuid-token-b", Content: "consume entry for user b"}
	require.NoError(t, DB.Create(logB).Error)
	provisionalLog := &Log{UserId: userA.Id, ChannelId: channelA.Id, Type: LogTypeProvisional, TokenName: "search-uuid-token-a", Content: "provisional entry for user a"}
	require.NoError(t, DB.Create(provisionalLog).Error)

	// Server-generated UUIDs must be present for the search to be meaningful.
	for _, uuid := range []string{userA.UUID, userB.UUID, channelA.UUID, tokenA.UUID, tokenB.UUID, redemptionA.UUID, logA.UUID, provisionalLog.UUID} {
		requireHyphenatedUUID(t, uuid)
	}

	t.Run("channels", func(t *testing.T) {
		channels, err := SearchChannels(channelA.UUID, "", "")
		require.NoError(t, err)
		require.True(t, containsChannelUUID(channels, channelA.UUID), "channel search by uuid must return the channel")

		// Name search must still work after adding the uuid clause.
		byName, err := SearchChannels("search-uuid-channel-a", "", "")
		require.NoError(t, err)
		require.True(t, containsChannelUUID(byName, channelA.UUID), "channel search by name must still work")
	})

	t.Run("normalization", func(t *testing.T) {
		// Stored UUIDs are lowercase; SQLite "=" is case-sensitive, so a padded,
		// upper-cased paste only matches if the keyword is trimmed and lowercased.
		padded := "  " + strings.ToUpper(channelA.UUID) + "  "
		channels, err := SearchChannels(padded, "", "")
		require.NoError(t, err)
		require.True(t, containsChannelUUID(channels, channelA.UUID), "channel search must normalize case/whitespace of a pasted uuid")

		users, err := SearchUsers("  "+strings.ToUpper(userA.UUID)+"  ", "", "")
		require.NoError(t, err)
		require.True(t, containsUserUUID(users, userA.UUID), "user search must normalize case/whitespace of a pasted uuid")

		tokens, _, err := SearchAllTokensForAdmin(strings.ToUpper(tokenA.UUID), 0, 100, "", "")
		require.NoError(t, err)
		require.True(t, containsTokenUUID(tokens, tokenA.UUID), "token search must normalize case of a pasted uuid")
	})

	t.Run("users", func(t *testing.T) {
		users, err := SearchUsers(userA.UUID, "", "")
		require.NoError(t, err)
		require.True(t, containsUserUUID(users, userA.UUID), "user search by uuid must return the user")
		require.False(t, containsUserUUID(users, userB.UUID), "user search by uuid must not return other users")
	})

	t.Run("admin_tokens", func(t *testing.T) {
		tokens, _, err := SearchAllTokensForAdmin(tokenA.UUID, 0, 100, "", "")
		require.NoError(t, err)
		require.True(t, containsTokenUUID(tokens, tokenA.UUID), "admin token search by uuid must return the token")
	})

	t.Run("user_tokens_scoped", func(t *testing.T) {
		owned, _, err := SearchUserTokens(userA.Id, tokenA.UUID, 0, 100, "", "")
		require.NoError(t, err)
		require.True(t, containsTokenUUID(owned, tokenA.UUID), "owner must find their token by uuid")

		// Security: a UUID keyword must never cross the user_id scope.
		foreign, total, err := SearchUserTokens(userB.Id, tokenA.UUID, 0, 100, "", "")
		require.NoError(t, err)
		require.Empty(t, foreign, "another user's token must not surface via uuid search")
		require.Zero(t, total, "cross-user uuid token search must report zero total")
	})

	t.Run("redemptions", func(t *testing.T) {
		redemptions, _, err := SearchRedemptions(redemptionA.UUID, 0, 100, "", "")
		require.NoError(t, err)
		require.True(t, containsRedemptionUUID(redemptions, redemptionA.UUID), "redemption search by uuid must return the redemption")
	})

	t.Run("all_logs", func(t *testing.T) {
		logs, _, err := SearchAllLogs(logA.UUID, 0, 100, "", "")
		require.NoError(t, err)
		require.True(t, containsLogUUID(logs, logA.UUID), "log search by uuid must return the log")

		// Security/scope: a UUID keyword must not defeat the provisional exclusion.
		provLogs, total, err := SearchAllLogs(provisionalLog.UUID, 0, 100, "", "")
		require.NoError(t, err)
		require.False(t, containsLogUUID(provLogs, provisionalLog.UUID), "provisional log must stay excluded even when matched by uuid")
		require.Zero(t, total, "provisional-only uuid log search must report zero total")
	})

	t.Run("user_logs_scoped", func(t *testing.T) {
		owned, _, err := SearchUserLogs(userA.Id, logA.UUID, 0, 100, "", "")
		require.NoError(t, err)
		require.True(t, containsLogUUID(owned, logA.UUID), "owner must find their log by uuid")

		// Security: a UUID keyword must never cross the user_id scope.
		foreign, total, err := SearchUserLogs(userB.Id, logA.UUID, 0, 100, "", "")
		require.NoError(t, err)
		require.Empty(t, foreign, "another user's log must not surface via uuid search")
		require.Zero(t, total, "cross-user uuid log search must report zero total")
	})
}
