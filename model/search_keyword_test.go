package model

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// stripUUIDHyphens strips the hyphens from a canonical UUID, producing the 32-hex form an
// operator gets when a UI or log line renders a UUID without separators.
//
// Parameters:
//   - uuid: canonical hyphenated UUID.
//
// Return values:
//   - string: the same UUID without hyphens.
func stripUUIDHyphens(uuid string) string {
	return strings.ReplaceAll(uuid, "-", "")
}

// TestCanonicalUUIDKeyword pins the single matching rule shared by every list-page search:
// which keywords are treated as UUIDs and what canonical form they are compared against.
//
// Parameters:
//   - t: active test handle.
//
// Return values: none.
func TestCanonicalUUIDKeyword(t *testing.T) {
	const canonical = "0197f3a1-9c2b-7d4e-8f10-2b3c4d5e6f70"

	cases := []struct {
		name    string
		keyword string
		want    string
		wantOK  bool
	}{
		{name: "canonical", keyword: canonical, want: canonical, wantOK: true},
		{name: "uppercase", keyword: strings.ToUpper(canonical), want: canonical, wantOK: true},
		{name: "padded", keyword: "  " + canonical + "\n", want: canonical, wantOK: true},
		{name: "compact", keyword: stripUUIDHyphens(canonical), want: canonical, wantOK: true},
		{name: "compact_uppercase_padded", keyword: " " + strings.ToUpper(stripUUIDHyphens(canonical)) + " ", want: canonical, wantOK: true},
		{name: "empty", keyword: "", wantOK: false},
		{name: "whitespace_only", keyword: "   ", wantOK: false},
		{name: "garbage", keyword: "not-a-uuid", wantOK: false},
		{name: "name_like_prefix", keyword: "prod-channel", wantOK: false},
		{name: "non_hex_same_length", keyword: "0197f3a1-9c2b-7d4e-8f10-2b3c4d5e6fzz", wantOK: false},
		{name: "hyphens_misplaced", keyword: "0197f3a19-c2b-7d4e-8f10-2b3c4d5e6f70", wantOK: false},
		{name: "too_short", keyword: "0197f3a1", wantOK: false},
		{name: "compact_with_extra_digit", keyword: stripUUIDHyphens(canonical) + "a", wantOK: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := canonicalUUIDKeyword(tc.keyword)
			require.Equal(t, tc.wantOK, ok)
			if tc.wantOK {
				require.Equal(t, tc.want, got)
			} else {
				require.Empty(t, got)
			}
		})
	}
}

// containsMCPServerUUID reports whether the server slice holds a row with the given uuid.
func containsMCPServerUUID(servers []*MCPServer, uuid string) bool {
	for _, s := range servers {
		if s.UUID == uuid {
			return true
		}
	}
	return false
}

// containsMCPToolUUID reports whether the tool slice holds a row with the given uuid.
func containsMCPToolUUID(tools []*MCPTool, uuid string) bool {
	for _, tool := range tools {
		if tool.UUID == uuid {
			return true
		}
	}
	return false
}

// containsTokenTransactionUUID reports whether the transaction slice holds a row with the given uuid.
func containsTokenTransactionUUID(txns []*TokenTransaction, uuid string) bool {
	for _, txn := range txns {
		if txn.UUID == uuid {
			return true
		}
	}
	return false
}

// searchKeywordFixture holds the rows shared by the keyword-search integration subtests.
type searchKeywordFixture struct {
	userA      *User
	userB      *User
	channelA   *Channel
	tokenA     *Token
	tokenB     *Token
	redemption *Redemption
	logA       *Log
	logB       *Log
	mcpServer  *MCPServer
	mcpToolA   *MCPTool
	txnA       *TokenTransaction
}

// setupSearchKeywordFixture points the package DB handles at a migrated in-memory
// database and seeds two owners plus one row of every searchable entity.
//
// Parameters:
//   - t: active test handle; cleanup restores the original handles.
//
// Return values:
//   - *searchKeywordFixture: the seeded rows, with server-generated UUIDs populated.
func setupSearchKeywordFixture(t *testing.T) *searchKeywordFixture {
	t.Helper()

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

	f := &searchKeywordFixture{}

	f.userA = &User{Username: "kw-user-a", Password: "hash-a", AccessToken: "kw-access-a", AffCode: "kw-aff-a"}
	require.NoError(t, DB.Create(f.userA).Error)
	f.userB = &User{Username: "kw-user-b", Password: "hash-b", AccessToken: "kw-access-b", AffCode: "kw-aff-b"}
	require.NoError(t, DB.Create(f.userB).Error)

	f.channelA = &Channel{Type: 1, Name: "kw-channel-a", Models: "gpt-4o", Config: "{}"}
	require.NoError(t, DB.Create(f.channelA).Error)

	// FK uuid columns are denormalised by the write paths (and the backfill); seed them
	// explicitly so the FK search arms are exercised against realistic rows.
	f.tokenA = &Token{UserId: f.userA.Id, UserUUID: &f.userA.UUID, Key: "kw-token-key-a", Name: "kw-token-a"}
	require.NoError(t, DB.Create(f.tokenA).Error)
	f.tokenB = &Token{UserId: f.userB.Id, UserUUID: &f.userB.UUID, Key: "kw-token-key-b", Name: "kw-token-b"}
	require.NoError(t, DB.Create(f.tokenB).Error)

	f.redemption = &Redemption{UserId: f.userA.Id, UserUUID: &f.userA.UUID, Key: "kw-redemption-a", Name: "kw-redemption"}
	require.NoError(t, DB.Create(f.redemption).Error)

	f.logA = &Log{
		UserId: f.userA.Id, UserUUID: &f.userA.UUID,
		ChannelId: f.channelA.Id, ChannelUUID: &f.channelA.UUID,
		TokenUUID: &f.tokenA.UUID,
		Type:      LogTypeConsume, TokenName: "kw-token-a", Content: "consume entry for user a",
	}
	require.NoError(t, DB.Create(f.logA).Error)
	f.logB = &Log{
		UserId: f.userB.Id, UserUUID: &f.userB.UUID,
		ChannelId: f.channelA.Id, ChannelUUID: &f.channelA.UUID,
		TokenUUID: &f.tokenB.UUID,
		Type:      LogTypeConsume, TokenName: "kw-token-b", Content: "consume entry for user b",
	}
	require.NoError(t, DB.Create(f.logB).Error)

	f.mcpServer = &MCPServer{
		Name:     "kw-mcp-server",
		BaseURL:  "https://mcp.example.com/kw",
		Protocol: "streamable_http",
		AuthType: "none",
		Status:   MCPServerStatusEnabled,
	}
	require.NoError(t, DB.Create(f.mcpServer).Error)
	otherServer := &MCPServer{
		Name:     "kw-other-server",
		BaseURL:  "https://other.example.com/kw",
		Protocol: "sse",
		AuthType: "bearer",
		Status:   MCPServerStatusEnabled,
	}
	require.NoError(t, DB.Create(otherServer).Error)

	f.mcpToolA = &MCPTool{
		ServerId:   f.mcpServer.Id,
		ServerUUID: &f.mcpServer.UUID,
		Name:       "kw-tool-a",
		Status:     1,
	}
	require.NoError(t, DB.Create(f.mcpToolA).Error)

	f.txnA = &TokenTransaction{
		TransactionID: "kw-txn-a",
		TokenId:       f.tokenA.Id,
		TokenUUID:     &f.tokenA.UUID,
		UserId:        f.userA.Id,
		UserUUID:      &f.userA.UUID,
		Status:        TokenTransactionStatusPending,
		PreQuota:      100,
	}
	require.NoError(t, DB.Create(f.txnA).Error)

	for _, uuid := range []string{
		f.userA.UUID, f.userB.UUID, f.channelA.UUID, f.tokenA.UUID, f.tokenB.UUID,
		f.redemption.UUID, f.logA.UUID, f.logB.UUID, f.mcpServer.UUID, f.mcpToolA.UUID, f.txnA.UUID,
	} {
		requireHyphenatedUUID(t, uuid)
	}
	return f
}

// TestSearchKeywordUUIDForms verifies that every list-page search accepts a pasted UUID in
// canonical, uppercase, and compact (hyphen-stripped) form, that an empty keyword still
// returns the unfiltered page, and that a garbage keyword filters everything out.
//
// Parameters:
//   - t: active test handle.
//
// Return values: none.
func TestSearchKeywordUUIDForms(t *testing.T) {
	f := setupSearchKeywordFixture(t)
	const garbage = "zzz-no-such-keyword"

	t.Run("channels", func(t *testing.T) {
		for name, keyword := range map[string]string{
			"canonical": f.channelA.UUID,
			"uppercase": strings.ToUpper(f.channelA.UUID),
			"compact":   stripUUIDHyphens(f.channelA.UUID),
		} {
			t.Run(name, func(t *testing.T) {
				channels, err := SearchChannels(keyword, "", "")
				require.NoError(t, err)
				require.True(t, containsChannelUUID(channels, f.channelA.UUID))
			})
		}

		byName, err := SearchChannels("kw-channel-a", "", "")
		require.NoError(t, err)
		require.True(t, containsChannelUUID(byName, f.channelA.UUID), "non-uuid keyword must keep the name prefix behaviour")

		none, err := SearchChannels(garbage, "", "")
		require.NoError(t, err)
		require.Empty(t, none, "garbage keyword must not match any channel")

		all, err := SearchChannels("", "", "")
		require.NoError(t, err)
		require.True(t, containsChannelUUID(all, f.channelA.UUID), "empty keyword must return the unfiltered page")
	})

	t.Run("users", func(t *testing.T) {
		for name, keyword := range map[string]string{
			"canonical": f.userA.UUID,
			"uppercase": strings.ToUpper(f.userA.UUID),
			"compact":   stripUUIDHyphens(f.userA.UUID),
		} {
			t.Run(name, func(t *testing.T) {
				users, err := SearchUsers(keyword, "", "")
				require.NoError(t, err)
				require.True(t, containsUserUUID(users, f.userA.UUID))
				require.False(t, containsUserUUID(users, f.userB.UUID), "a uuid keyword must match exactly one user")
			})
		}

		none, err := SearchUsers(garbage, "", "")
		require.NoError(t, err)
		require.Empty(t, none, "garbage keyword must not match any user")

		all, err := SearchUsers("", "", "")
		require.NoError(t, err)
		require.True(t, containsUserUUID(all, f.userA.UUID), "empty keyword must return the unfiltered page")
		require.True(t, containsUserUUID(all, f.userB.UUID), "empty keyword must return the unfiltered page")
	})

	t.Run("admin_tokens", func(t *testing.T) {
		for name, keyword := range map[string]string{
			"canonical": f.tokenA.UUID,
			"uppercase": strings.ToUpper(f.tokenA.UUID),
			"compact":   stripUUIDHyphens(f.tokenA.UUID),
		} {
			t.Run(name, func(t *testing.T) {
				tokens, total, err := SearchAllTokensForAdmin(keyword, 0, 100, "", "")
				require.NoError(t, err)
				require.True(t, containsTokenUUID(tokens, f.tokenA.UUID))
				require.EqualValues(t, 1, total, "total must agree with the returned rows")
			})
		}

		// FK arm: pasting a USER uuid lists that user's tokens, and only theirs.
		owned, total, err := SearchAllTokensForAdmin(f.userA.UUID, 0, 100, "", "")
		require.NoError(t, err)
		require.True(t, containsTokenUUID(owned, f.tokenA.UUID), "user uuid must list that user's tokens")
		require.False(t, containsTokenUUID(owned, f.tokenB.UUID), "user uuid must not list another user's tokens")
		require.EqualValues(t, 1, total)

		none, total, err := SearchAllTokensForAdmin(garbage, 0, 100, "", "")
		require.NoError(t, err)
		require.Empty(t, none)
		require.Zero(t, total)

		all, total, err := SearchAllTokensForAdmin("", 0, 100, "", "")
		require.NoError(t, err)
		require.True(t, containsTokenUUID(all, f.tokenA.UUID))
		require.True(t, containsTokenUUID(all, f.tokenB.UUID))
		require.EqualValues(t, 2, total)
	})

	t.Run("user_tokens_scoped", func(t *testing.T) {
		owned, total, err := SearchUserTokens(f.userA.Id, stripUUIDHyphens(f.tokenA.UUID), 0, 100, "", "")
		require.NoError(t, err)
		require.True(t, containsTokenUUID(owned, f.tokenA.UUID), "owner must find their token by a compact uuid")
		require.EqualValues(t, 1, total)

		// Security: neither the entity uuid nor the FK user_uuid arm may cross user_id.
		foreign, total, err := SearchUserTokens(f.userB.Id, f.tokenA.UUID, 0, 100, "", "")
		require.NoError(t, err)
		require.Empty(t, foreign, "another user's token must not surface via uuid search")
		require.Zero(t, total)

		foreignOwner, total, err := SearchUserTokens(f.userB.Id, f.userA.UUID, 0, 100, "", "")
		require.NoError(t, err)
		require.Empty(t, foreignOwner, "another user's uuid must not surface their tokens")
		require.Zero(t, total)
	})

	t.Run("redemptions", func(t *testing.T) {
		for name, keyword := range map[string]string{
			"canonical": f.redemption.UUID,
			"uppercase": strings.ToUpper(f.redemption.UUID),
			"compact":   stripUUIDHyphens(f.redemption.UUID),
			"owner_fk":  f.userA.UUID,
		} {
			t.Run(name, func(t *testing.T) {
				redemptions, total, err := SearchRedemptions(keyword, 0, 100, "", "")
				require.NoError(t, err)
				require.True(t, containsRedemptionUUID(redemptions, f.redemption.UUID))
				require.EqualValues(t, 1, total)
			})
		}

		none, total, err := SearchRedemptions(garbage, 0, 100, "", "")
		require.NoError(t, err)
		require.Empty(t, none)
		require.Zero(t, total)

		all, total, err := SearchRedemptions("", 0, 100, "", "")
		require.NoError(t, err)
		require.True(t, containsRedemptionUUID(all, f.redemption.UUID))
		require.EqualValues(t, 1, total)
	})

	t.Run("all_logs", func(t *testing.T) {
		for name, keyword := range map[string]string{
			"canonical":  f.logA.UUID,
			"uppercase":  strings.ToUpper(f.logA.UUID),
			"compact":    stripUUIDHyphens(f.logA.UUID),
			"user_fk":    f.userA.UUID,
			"token_fk":   f.tokenA.UUID,
			"compact_fk": stripUUIDHyphens(f.userA.UUID),
		} {
			t.Run(name, func(t *testing.T) {
				logs, total, err := SearchAllLogs(keyword, 0, 100, "", "")
				require.NoError(t, err)
				require.True(t, containsLogUUID(logs, f.logA.UUID))
				require.False(t, containsLogUUID(logs, f.logB.UUID), "another owner's log must not match")
				require.EqualValues(t, 1, total)
			})
		}

		// The channel FK is shared by both logs, so it must return both.
		byChannel, total, err := SearchAllLogs(f.channelA.UUID, 0, 100, "", "")
		require.NoError(t, err)
		require.True(t, containsLogUUID(byChannel, f.logA.UUID))
		require.True(t, containsLogUUID(byChannel, f.logB.UUID))
		require.EqualValues(t, 2, total)

		none, total, err := SearchAllLogs(garbage, 0, 100, "", "")
		require.NoError(t, err)
		require.Empty(t, none)
		require.Zero(t, total)

		all, total, err := SearchAllLogs("", 0, 100, "", "")
		require.NoError(t, err)
		require.True(t, containsLogUUID(all, f.logA.UUID))
		require.EqualValues(t, 2, total)

		byContent, _, err := SearchAllLogs("consume entry for user a", 0, 100, "", "")
		require.NoError(t, err)
		require.True(t, containsLogUUID(byContent, f.logA.UUID), "non-uuid keyword must keep the content LIKE behaviour")
	})

	t.Run("user_logs_scoped", func(t *testing.T) {
		owned, total, err := SearchUserLogs(f.userA.Id, stripUUIDHyphens(f.logA.UUID), 0, 100, "", "")
		require.NoError(t, err)
		require.True(t, containsLogUUID(owned, f.logA.UUID))
		require.EqualValues(t, 1, total)

		foreign, total, err := SearchUserLogs(f.userB.Id, f.logA.UUID, 0, 100, "", "")
		require.NoError(t, err)
		require.Empty(t, foreign, "another user's log must not surface via uuid search")
		require.Zero(t, total)

		// The channel FK matches both logs globally, but the user scope must still hold.
		scoped, total, err := SearchUserLogs(f.userB.Id, f.channelA.UUID, 0, 100, "", "")
		require.NoError(t, err)
		require.False(t, containsLogUUID(scoped, f.logA.UUID), "channel uuid must not cross the user scope")
		require.True(t, containsLogUUID(scoped, f.logB.UUID))
		require.EqualValues(t, 1, total)
	})

	t.Run("mcp_servers", func(t *testing.T) {
		for name, keyword := range map[string]string{
			"canonical": f.mcpServer.UUID,
			"uppercase": strings.ToUpper(f.mcpServer.UUID),
			"compact":   stripUUIDHyphens(f.mcpServer.UUID),
			"name":      "kw-mcp-server",
			"base_url":  "mcp.example.com",
		} {
			t.Run(name, func(t *testing.T) {
				servers, err := ListMCPServers(keyword, 0, 100, "", "")
				require.NoError(t, err)
				require.True(t, containsMCPServerUUID(servers, f.mcpServer.UUID))
				require.Len(t, servers, 1)

				total, err := CountMCPServers(keyword)
				require.NoError(t, err)
				require.EqualValues(t, len(servers), total, "total must agree with the returned rows")
			})
		}

		none, err := ListMCPServers(garbage, 0, 100, "", "")
		require.NoError(t, err)
		require.Empty(t, none)
		total, err := CountMCPServers(garbage)
		require.NoError(t, err)
		require.Zero(t, total)

		all, err := ListMCPServers("", 0, 100, "", "")
		require.NoError(t, err)
		require.Len(t, all, 2, "empty keyword must return the unfiltered page")
		total, err = CountMCPServers("")
		require.NoError(t, err)
		require.EqualValues(t, 2, total)

		// Whitespace-only keywords must behave like no keyword at all.
		blank, err := ListMCPServers("   ", 0, 100, "", "")
		require.NoError(t, err)
		require.Len(t, blank, 2)
	})

	t.Run("mcp_tools", func(t *testing.T) {
		for name, keyword := range map[string]string{
			"canonical":  f.mcpToolA.UUID,
			"uppercase":  strings.ToUpper(f.mcpToolA.UUID),
			"compact":    stripUUIDHyphens(f.mcpToolA.UUID),
			"server_fk":  f.mcpServer.UUID,
			"name":       "kw-tool-a",
			"compact_fk": stripUUIDHyphens(f.mcpServer.UUID),
		} {
			t.Run(name, func(t *testing.T) {
				tools, err := SearchMCPTools(0, nil, keyword, 0, 100, "", "")
				require.NoError(t, err)
				require.True(t, containsMCPToolUUID(tools, f.mcpToolA.UUID))

				total, err := CountSearchedMCPTools(0, nil, keyword)
				require.NoError(t, err)
				require.EqualValues(t, len(tools), total, "total must agree with the returned rows")
			})
		}

		none, err := SearchMCPTools(0, nil, garbage, 0, 100, "", "")
		require.NoError(t, err)
		require.Empty(t, none)

		all, err := SearchMCPTools(0, nil, "", 0, 100, "", "")
		require.NoError(t, err)
		require.True(t, containsMCPToolUUID(all, f.mcpToolA.UUID), "empty keyword must return the unfiltered page")

		legacy, err := ListMCPTools(0, nil, 0, 100, "", "")
		require.NoError(t, err)
		require.Len(t, legacy, len(all), "the keyword-free wrapper must stay equivalent")
	})

	t.Run("token_transactions", func(t *testing.T) {
		ctx := context.Background()
		for name, keyword := range map[string]string{
			"canonical": f.txnA.UUID,
			"uppercase": strings.ToUpper(f.txnA.UUID),
			"compact":   stripUUIDHyphens(f.txnA.UUID),
			"token_fk":  f.tokenA.UUID,
			"user_fk":   f.userA.UUID,
		} {
			t.Run(name, func(t *testing.T) {
				txns, err := SearchTokenTransactionsByTokenID(ctx, f.tokenA.Id, keyword, 0, 100)
				require.NoError(t, err)
				require.True(t, containsTokenTransactionUUID(txns, f.txnA.UUID))

				total, err := CountSearchedTokenTransactionsByTokenID(ctx, f.tokenA.Id, keyword)
				require.NoError(t, err)
				require.EqualValues(t, len(txns), total, "total must agree with the returned rows")
			})
		}

		// Security: the token_id scope always ANDs, so another token's id yields nothing
		// even when the keyword names a transaction that really exists.
		foreign, err := SearchTokenTransactionsByTokenID(ctx, f.tokenB.Id, f.txnA.UUID, 0, 100)
		require.NoError(t, err)
		require.Empty(t, foreign, "a transaction must not surface outside its token scope")

		// A non-uuid keyword deliberately applies no filter: transactions expose no
		// human-readable name, so this preserves the endpoint's previous behaviour.
		unfiltered, err := SearchTokenTransactionsByTokenID(ctx, f.tokenA.Id, garbage, 0, 100)
		require.NoError(t, err)
		require.True(t, containsTokenTransactionUUID(unfiltered, f.txnA.UUID))

		all, err := SearchTokenTransactionsByTokenID(ctx, f.tokenA.Id, "", 0, 100)
		require.NoError(t, err)
		require.True(t, containsTokenTransactionUUID(all, f.txnA.UUID))
		total, err := CountSearchedTokenTransactionsByTokenID(ctx, f.tokenA.Id, "")
		require.NoError(t, err)
		require.EqualValues(t, len(all), total)
	})
}

// TestSearchKeywordEmptyUUIDColumnsNeverMatchEmptyKeyword guards the PostgreSQL char(36)
// bpchar hazard: trailing-space-insensitive comparison would let `uuid = ”` match every
// row whose uuid has not been backfilled. Rejecting non-UUID keywords in one shared helper
// is what prevents that, so an empty or garbage keyword must never be routed to a uuid
// equality arm at all.
//
// Parameters:
//   - t: active test handle.
//
// Return values: none.
func TestSearchKeywordEmptyUUIDColumnsNeverMatchEmptyKeyword(t *testing.T) {
	f := setupSearchKeywordFixture(t)

	// Simulate an un-backfilled row: empty own uuid and NULL FK uuid.
	require.NoError(t, DB.Model(&Token{}).Where("id = ?", f.tokenB.Id).
		Updates(map[string]any{"uuid": "", "user_uuid": nil}).Error)

	for _, keyword := range []string{"", "   ", "not-a-uuid"} {
		tokens, _, err := SearchAllTokensForAdmin(keyword, 0, 100, "", "")
		require.NoError(t, err)
		for _, tk := range tokens {
			require.NotEmpty(t, tk.Name, "matched rows must come from the name arm, never a uuid arm")
		}
	}

	// The blank uuid must not be reachable by any keyword form.
	blank, total, err := SearchAllTokensForAdmin("00000000-0000-0000-0000-000000000000", 0, 100, "", "")
	require.NoError(t, err)
	require.Empty(t, blank)
	require.Zero(t, total)
}
