package model

// Tests for the shared normalized log-list filter (W2.4 step 1).
//
// The load-bearing property is equivalence: the shared predicate must select
// exactly the rows the legacy list helpers select, for every filter
// combination. If it does not, the additive cursor route shows a different
// history from the offset route it sits beside.

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// adminScope returns a site-wide administrative scope for tests.
//
// Parameters: none.
//
// Return values:
//   - LogListScope: the scope.
func adminScope() LogListScope {
	return LogListScope{Kind: LogListScopeAll, PrincipalUserID: 1, Role: RoleRootUser}
}

// selfScope returns a self scope for the given subject user.
//
// Parameters:
//   - userID: the subject user.
//
// Return values:
//   - LogListScope: the scope.
func selfScope(userID int) LogListScope {
	return LogListScope{Kind: LogListScopeSelf, SubjectUserID: userID, PrincipalUserID: userID, Role: RoleCommonUser}
}

// TestLogListFilterNormalizeIsIdempotent verifies a normalized filter digests
// identically on a second pass.
//
// Parameters:
//   - t: the test handle.
//
// Return values: none.
func TestLogListFilterNormalizeIsIdempotent(t *testing.T) {
	raw := LogListFilter{LogType: 2, ModelName: "  gpt-4.1 ", Username: " alice ", TokenName: "\tprod\n", ChannelID: 7}
	once := raw.Normalize(adminScope())
	twice := once.Normalize(adminScope())
	require.Equal(t, once, twice)
	require.Equal(t, "gpt-4.1", once.ModelName)
	require.Equal(t, "alice", once.Username)
	require.Equal(t, "prod", once.TokenName)
}

// TestLogListFilterNormalizeClearsAdminFieldsUnderSelfScope verifies the self
// route cannot be influenced by administrative filters.
//
// Parameters:
//   - t: the test handle.
//
// Return values: none.
func TestLogListFilterNormalizeClearsAdminFieldsUnderSelfScope(t *testing.T) {
	raw := LogListFilter{Username: "someone-else", ChannelID: 42, TokenName: "mine"}
	got := raw.Normalize(selfScope(9))
	require.Empty(t, got.Username, "the self route never filters by username")
	require.Zero(t, got.ChannelID, "the self route never filters by channel")
	require.Equal(t, "mine", got.TokenName, "token name remains a supported self filter")
}

// TestLogListFilterNormalizeClampsNegatives verifies out-of-range values become
// the documented "unset" zero rather than reaching SQL.
//
// Parameters:
//   - t: the test handle.
//
// Return values: none.
func TestLogListFilterNormalizeClampsNegatives(t *testing.T) {
	got := LogListFilter{LogType: -3, StartTimestamp: -1, EndTimestamp: -1, ChannelID: -5}.Normalize(adminScope())
	require.Equal(t, LogTypeUnknown, got.LogType)
	require.Zero(t, got.StartTimestamp)
	require.Zero(t, got.EndTimestamp)
	require.Zero(t, got.ChannelID)
}

// TestLogListFilterDigestIsSensitiveToEveryBoundField verifies each bound field
// changes the digest, so a cursor cannot be reused across a changed query.
//
// Parameters:
//   - t: the test handle.
//
// Return values: none.
func TestLogListFilterDigestIsSensitiveToEveryBoundField(t *testing.T) {
	base := LogListFilter{LogType: 2, StartTimestamp: 100, EndTimestamp: 200,
		ModelName: "m", Username: "u", TokenName: "t", ChannelID: 3}
	scope := adminScope()
	baseDigest := base.Digest(scope, "log.all")

	mutations := map[string]func(*LogListFilter){
		"log type":  func(f *LogListFilter) { f.LogType = 7 },
		"start":     func(f *LogListFilter) { f.StartTimestamp = 101 },
		"end":       func(f *LogListFilter) { f.EndTimestamp = 201 },
		"model":     func(f *LogListFilter) { f.ModelName = "m2" },
		"username":  func(f *LogListFilter) { f.Username = "u2" },
		"tokenName": func(f *LogListFilter) { f.TokenName = "t2" },
		"channel":   func(f *LogListFilter) { f.ChannelID = 4 },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			mutated := base
			mutate(&mutated)
			require.NotEqual(t, baseDigest, mutated.Digest(scope, "log.all"))
		})
	}

	require.NotEqual(t, baseDigest, base.Digest(scope, "log.self"), "endpoint must be bound")

	other := adminScope()
	other.PrincipalUserID = 99
	require.NotEqual(t, baseDigest, base.Digest(other, "log.all"), "principal must be bound")

	demoted := adminScope()
	demoted.Role = RoleCommonUser
	require.NotEqual(t, baseDigest, base.Digest(demoted, "log.all"), "role must be bound")

	subject := selfScope(5)
	require.NotEqual(t, base.Digest(subject, "log.self"), base.Digest(selfScope(6), "log.self"),
		"subject user must be bound so one user's cursor cannot address another's rows")
}

// TestLogListFilterDigestResistsFieldBoundaryAmbiguity verifies the canonical
// encoding is length-prefixed, so adjacent string fields cannot be shifted
// between each other without changing the digest.
//
// Parameters:
//   - t: the test handle.
//
// Return values: none.
func TestLogListFilterDigestResistsFieldBoundaryAmbiguity(t *testing.T) {
	scope := adminScope()
	a := LogListFilter{ModelName: "ab", Username: "c"}.Digest(scope, "log.all")
	b := LogListFilter{ModelName: "a", Username: "bc"}.Digest(scope, "log.all")
	require.NotEqual(t, a, b)
}

// TestLogListPredicateMatchesLegacyRowSelection is the equivalence test: for
// every filter combination, the shared predicate must select exactly the rows
// the legacy list helpers select.
//
// Parameters:
//   - t: the test handle.
//
// Return values: none.
func TestLogListPredicateMatchesLegacyRowSelection(t *testing.T) {
	setupTestDatabase(t)
	require.NoError(t, LOG_DB.Exec("DELETE FROM logs WHERE username LIKE 'pred-%'").Error)
	t.Cleanup(func() {
		require.NoError(t, LOG_DB.Exec("DELETE FROM logs WHERE username LIKE 'pred-%'").Error)
	})

	base := time.Now().UTC().Unix() - 10_000
	seed := []struct {
		user      int
		logType   int
		createdAt int64
		model     string
		username  string
		token     string
		channel   int
	}{
		{1, LogTypeConsume, base + 10, "gpt-4.1", "pred-alice", "prod", 3},
		{1, LogTypeConsume, base + 20, "gpt-4.1", "pred-alice", "dev", 4},
		{1, LogTypeTool, base + 30, "web_search", "pred-alice", "prod", 3},
		{2, LogTypeConsume, base + 40, "claude", "pred-bob", "prod", 5},
		{2, LogTypeProvisional, base + 50, "claude", "pred-bob", "prod", 5},
		{1, LogTypeConsume, base + 60, "gpt-4.1", "pred-alice", "prod", 3},
	}
	for i, s := range seed {
		require.NoError(t, LOG_DB.Exec(
			"INSERT INTO logs (user_id, type, created_at, model_name, username, token_name, channel_id, content) VALUES (?,?,?,?,?,?,?,?)",
			s.user, s.logType, s.createdAt, s.model, s.username, s.token, s.channel, fmt.Sprintf("pred-seed-%d", i),
		).Error)
	}

	cases := []struct {
		name   string
		scope  LogListScope
		filter LogListFilter
	}{
		{"admin no filter", adminScope(), LogListFilter{}},
		{"admin type consume", adminScope(), LogListFilter{LogType: LogTypeConsume}},
		{"admin type tool", adminScope(), LogListFilter{LogType: LogTypeTool}},
		{"admin explicit provisional", adminScope(), LogListFilter{LogType: LogTypeProvisional}},
		{"admin model", adminScope(), LogListFilter{ModelName: "gpt-4.1"}},
		{"admin username", adminScope(), LogListFilter{Username: "pred-alice"}},
		{"admin token", adminScope(), LogListFilter{TokenName: "prod"}},
		{"admin channel", adminScope(), LogListFilter{ChannelID: 3}},
		{"admin start only", adminScope(), LogListFilter{StartTimestamp: base + 30}},
		{"admin end only", adminScope(), LogListFilter{EndTimestamp: base + 30}},
		{"admin closed range", adminScope(), LogListFilter{StartTimestamp: base + 20, EndTimestamp: base + 40}},
		{"admin boundary equal", adminScope(), LogListFilter{StartTimestamp: base + 30, EndTimestamp: base + 30}},
		{"self user 1", selfScope(1), LogListFilter{}},
		{"self user 1 consume", selfScope(1), LogListFilter{LogType: LogTypeConsume}},
		{"self user 2", selfScope(2), LogListFilter{}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			filter := tc.filter.Normalize(tc.scope)

			var legacy []*Log
			var err error
			if tc.scope.Kind == LogListScopeAll {
				legacy, err = GetAllLogs(filter.LogType, filter.StartTimestamp, filter.EndTimestamp,
					filter.ModelName, filter.Username, filter.TokenName, 0, 1000, filter.ChannelID, "", "")
			} else {
				legacy, err = GetUserLogs(tc.scope.SubjectUserID, filter.LogType,
					filter.StartTimestamp, filter.EndTimestamp, filter.ModelName, filter.TokenName, 0, 1000, "", "")
			}
			require.NoError(t, err)

			where, args := BuildLogListPredicate(tc.scope, filter)
			var shared []int64
			require.NoError(t, LOG_DB.Raw(
				"SELECT id FROM logs WHERE "+where+" AND content LIKE 'pred-seed-%' ORDER BY id", args...).
				Scan(&shared).Error)

			legacyIDs := make([]int64, 0, len(legacy))
			for _, l := range legacy {
				if len(l.Content) >= 9 && l.Content[:9] == "pred-seed" {
					legacyIDs = append(legacyIDs, int64(l.Id))
				}
			}
			require.ElementsMatch(t, legacyIDs, shared,
				"the shared predicate must select exactly the rows the legacy list selects")
		})
	}
}
