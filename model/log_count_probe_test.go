package model

// Tests for bounded log counting (W2.4 step 5).

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// TestProbeLogCountQualityAtBoundary verifies the probe reports exact below the
// bound and a lower bound at or above it.
//
// Parameters:
//   - t: the test handle.
//
// Return values: none.
func TestProbeLogCountQualityAtBoundary(t *testing.T) {
	setupTestDatabase(t)
	const marker = "cntbound"
	require.NoError(t, LOG_DB.Exec("DELETE FROM logs WHERE content LIKE ?", marker+"-%").Error)
	t.Cleanup(func() {
		require.NoError(t, LOG_DB.Exec("DELETE FROM logs WHERE content LIKE ?", marker+"-%").Error)
	})

	base := time.Now().UTC().Unix() - 4_000
	const rows = 12
	for i := range rows {
		require.NoError(t, LOG_DB.Exec(
			"INSERT INTO logs (user_id, type, created_at, token_name, content) VALUES (?,?,?,?,?)",
			1, LogTypeConsume, base+int64(i), "cntprobe", fmt.Sprintf("%s-%02d", marker, i)).Error)
	}

	scope := LogListScope{Kind: LogListScopeAll, PrincipalUserID: 1, Role: RoleRootUser}
	filter := LogListFilter{TokenName: "cntprobe"}.Normalize(scope)

	for _, tc := range []struct {
		bound       int
		wantQuality LogCountQuality
		wantValue   int64
	}{
		{rows - 1, LogCountLowerBound, rows - 1},
		{rows, LogCountExact, rows},
		{rows + 1, LogCountExact, rows},
		{rows + 2, LogCountExact, rows},
	} {
		t.Run(fmt.Sprintf("bound_%d", tc.bound), func(t *testing.T) {
			got, err := ProbeLogCount(context.Background(), scope, filter, tc.bound, 5*time.Second)
			require.NoError(t, err)
			require.Equal(t, tc.wantQuality, got.Quality)
			require.NotNil(t, got.Value)
			require.Equal(t, tc.wantValue, *got.Value)
			require.Positive(t, got.AsOf)
			require.False(t, got.Cached)
		})
	}
}

// TestProbeLogCountMatchesLegacyCount verifies the probe agrees with the legacy
// exact count whenever it reports exact.
//
// Parameters:
//   - t: the test handle.
//
// Return values: none.
func TestProbeLogCountMatchesLegacyCount(t *testing.T) {
	setupTestDatabase(t)
	const marker = "cntlegacy"
	require.NoError(t, LOG_DB.Exec("DELETE FROM logs WHERE content LIKE ?", marker+"-%").Error)
	t.Cleanup(func() {
		require.NoError(t, LOG_DB.Exec("DELETE FROM logs WHERE content LIKE ?", marker+"-%").Error)
	})

	base := time.Now().UTC().Unix() - 2_000
	for i := range 7 {
		logType := LogTypeConsume
		if i%3 == 0 {
			logType = LogTypeProvisional
		}
		require.NoError(t, LOG_DB.Exec(
			"INSERT INTO logs (user_id, type, created_at, token_name, content) VALUES (?,?,?,?,?)",
			5, logType, base+int64(i), "cntlegacy", fmt.Sprintf("%s-%02d", marker, i)).Error)
	}

	scope := LogListScope{Kind: LogListScopeSelf, SubjectUserID: 5, PrincipalUserID: 5, Role: RoleCommonUser}
	filter := LogListFilter{TokenName: "cntlegacy"}.Normalize(scope)

	legacy, err := GetUserLogsCount(5, filter.LogType, filter.StartTimestamp, filter.EndTimestamp,
		filter.ModelName, filter.TokenName)
	require.NoError(t, err)

	probed, err := ProbeLogCount(context.Background(), scope, filter, 1000, 5*time.Second)
	require.NoError(t, err)
	require.Equal(t, LogCountExact, probed.Quality)
	require.NotNil(t, probed.Value)
	require.Equal(t, legacy, *probed.Value,
		"an exact probe must agree with the legacy count, provisional exclusion included")
}

// TestProbeLogCountReportsUnavailableOnBudgetExhaustion verifies an expired
// budget yields an unavailable count rather than an error, and that a cancelled
// caller is reported as an error instead.
//
// Parameters:
//   - t: the test handle.
//
// Return values: none.
func TestProbeLogCountReportsUnavailableOnBudgetExhaustion(t *testing.T) {
	setupTestDatabase(t)

	scope := LogListScope{Kind: LogListScopeAll, PrincipalUserID: 1, Role: RoleRootUser}
	filter := LogListFilter{}.Normalize(scope)

	// A budget that cannot be met.
	got, err := ProbeLogCount(context.Background(), scope, filter, 1000, time.Nanosecond)
	require.NoError(t, err, "an exhausted budget is an outcome, not an error")
	require.Equal(t, LogCountUnavailable, got.Quality)
	require.Nil(t, got.Value, "an unavailable count must have no value so it cannot render as zero")

	// A cancelled caller is distinguishable from an exhausted budget.
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = ProbeLogCount(cancelled, scope, filter, 1000, 5*time.Second)
	require.Error(t, err, "a cancelled caller is a client disconnect, not a count outcome")
}

// TestLogCountProbeStatementIsBounded verifies the statement carries the inner
// limit on every dialect and never binds it as a placeholder.
//
// Parameters:
//   - t: the test handle.
//
// Return values: none.
func TestLogCountProbeStatementIsBounded(t *testing.T) {
	scope := LogListScope{Kind: LogListScopeAll, PrincipalUserID: 1, Role: RoleRootUser}
	where, _ := BuildLogListPredicate(scope, LogListFilter{}.Normalize(scope))

	stmt := buildLogCountProbeStatement(LOG_DB, where, 500, 3*time.Second)
	require.Contains(t, stmt, "LIMIT 501", "the bound must be inlined")
	require.NotContains(t, stmt, "LIMIT ?")
	require.Contains(t, stmt, "AS probe", "MySQL requires a derived-table alias")

	my, err := gorm.Open(mysql.New(mysql.Config{DSN: "u:p@tcp(127.0.0.1:1)/none", SkipInitializeWithVersion: true}), &gorm.Config{DisableAutomaticPing: true})
	require.NoError(t, err)
	mysqlStmt := buildLogCountProbeStatement(my, where, 500, 3*time.Second)
	require.Contains(t, mysqlStmt, "MAX_EXECUTION_TIME(3500)",
		"MySQL gets a server-side ceiling just beyond the client deadline")
	require.NotContains(t, stmt, "MAX_EXECUTION_TIME",
		"the hint must not leak onto engines that reject it")
}
