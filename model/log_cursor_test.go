package model

// Keyset traversal tests (W2.4 step 2).
//
// The load-bearing properties are that a full traversal reproduces
// `ORDER BY created_at DESC, id DESC` exactly, with no duplicate and no gap,
// and that it stays correct when many rows share one created_at second — the
// case the naive single-predicate keyset shape gets wrong.

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// seedCursorLogs inserts usage rows for a cursor traversal test.
//
// Parameters:
//   - t: the test handle.
//   - marker: a content prefix used to isolate and clean up the fixture.
//   - rows: the rows to insert as (userID, createdAt) pairs.
//
// Return values:
//   - []int64: the inserted row ids, in insertion order.
func seedCursorLogs(t *testing.T, marker string, rows [][2]int64) []int64 {
	t.Helper()
	for i, r := range rows {
		require.NoError(t, LOG_DB.Exec(
			"INSERT INTO logs (user_id, type, created_at, model_name, username, token_name, channel_id, content) VALUES (?,?,?,?,?,?,?,?)",
			int(r[0]), LogTypeConsume, r[1], "gpt-4.1", marker+"-user", "prod", 1,
			fmt.Sprintf("%s-%04d", marker, i),
		).Error)
	}
	var ids []int64
	require.NoError(t, LOG_DB.Raw(
		"SELECT id FROM logs WHERE content LIKE ? ORDER BY id", marker+"-%").Scan(&ids).Error)
	return ids
}

// expectedCursorOrder returns the fixture ids in the traversal's reference
// order, taken straight from the database.
//
// Parameters:
//   - t: the test handle.
//   - marker: the fixture content prefix.
//
// Return values:
//   - []int64: ids ordered by created_at DESC, id DESC.
func expectedCursorOrder(t *testing.T, marker string) []int64 {
	t.Helper()
	var ids []int64
	require.NoError(t, LOG_DB.Raw(
		"SELECT id FROM logs WHERE content LIKE ? AND type <> ? ORDER BY created_at DESC, id DESC",
		marker+"-%", LogTypeProvisional).Scan(&ids).Error)
	return ids
}

// traverseCursor walks every page and returns the ids in traversal order.
//
// Parameters:
//   - t: the test handle.
//   - scope: the authorization scope.
//   - filter: the normalized filter.
//   - pageSize: rows per page.
//
// Return values:
//   - []int64: ids in the order the traversal produced them.
//   - int: the number of pages fetched.
func traverseCursor(t *testing.T, scope LogListScope, filter LogListFilter, pageSize int) ([]int64, int) {
	t.Helper()
	var (
		got    []int64
		anchor *LogCursorAnchor
		pages  int
	)
	for {
		page, err := FetchLogCursorPage(context.Background(), scope, filter, anchor, pageSize)
		require.NoError(t, err)
		pages++
		for _, l := range page.Logs {
			got = append(got, int64(l.Id))
		}
		if !page.HasMore {
			break
		}
		require.NotEmpty(t, page.Logs, "a page reporting has_more must return rows")
		last := page.Last
		anchor = &last
		require.Less(t, pages, 1000, "traversal must terminate")
	}
	return got, pages
}

// TestLogCursorTraversalMatchesOrderedScan verifies a full traversal reproduces
// the reference ordering exactly at several page sizes.
//
// Parameters:
//   - t: the test handle.
//
// Return values: none.
func TestLogCursorTraversalMatchesOrderedScan(t *testing.T) {
	setupTestDatabase(t)
	const marker = "curtrav"
	require.NoError(t, LOG_DB.Exec("DELETE FROM logs WHERE content LIKE ?", marker+"-%").Error)
	t.Cleanup(func() {
		require.NoError(t, LOG_DB.Exec("DELETE FROM logs WHERE content LIKE ?", marker+"-%").Error)
	})

	base := time.Now().UTC().Unix() - 5_000
	rows := make([][2]int64, 0, 57)
	for i := range 57 {
		// Deliberate ties: three rows per second.
		rows = append(rows, [2]int64{1, base + int64(i/3)})
	}
	seedCursorLogs(t, marker, rows)

	want := expectedCursorOrder(t, marker)
	require.Len(t, want, 57)

	scope := LogListScope{Kind: LogListScopeAll, PrincipalUserID: 1, Role: RoleRootUser}
	filter := LogListFilter{TokenName: "prod"}.Normalize(scope)

	for _, pageSize := range []int{1, 2, 5, 10, 57, 100} {
		t.Run(fmt.Sprintf("page_size_%d", pageSize), func(t *testing.T) {
			got, pages := traverseCursor(t, scope, filter, pageSize)
			require.Equal(t, want, got, "traversal must equal ORDER BY created_at DESC, id DESC")
			require.LessOrEqual(t, pages, (len(want)/pageSize)+2)
		})
	}
}

// TestLogCursorTraversalWithHeavyTies verifies a traversal is exact when every
// row shares one created_at second, which is the case the naive keyset
// predicate handles incorrectly.
//
// Parameters:
//   - t: the test handle.
//
// Return values: none.
func TestLogCursorTraversalWithHeavyTies(t *testing.T) {
	setupTestDatabase(t)
	const marker = "curties"
	require.NoError(t, LOG_DB.Exec("DELETE FROM logs WHERE content LIKE ?", marker+"-%").Error)
	t.Cleanup(func() {
		require.NoError(t, LOG_DB.Exec("DELETE FROM logs WHERE content LIKE ?", marker+"-%").Error)
	})

	second := time.Now().UTC().Unix() - 900
	rows := make([][2]int64, 0, 250)
	for range 250 {
		rows = append(rows, [2]int64{1, second})
	}
	seedCursorLogs(t, marker, rows)

	want := expectedCursorOrder(t, marker)
	require.Len(t, want, 250)

	scope := LogListScope{Kind: LogListScopeAll, PrincipalUserID: 1, Role: RoleRootUser}
	filter := LogListFilter{TokenName: "prod"}.Normalize(scope)

	got, _ := traverseCursor(t, scope, filter, 7)
	require.Equal(t, want, got, "every row sharing one second must be visited exactly once")
	require.Len(t, got, 250)
}

// TestLogCursorRespectsScopeAndProvisionalExclusion verifies the page obeys the
// same authorization and provisional rules as the legacy list.
//
// Parameters:
//   - t: the test handle.
//
// Return values: none.
func TestLogCursorRespectsScopeAndProvisionalExclusion(t *testing.T) {
	setupTestDatabase(t)
	const marker = "curscope"
	require.NoError(t, LOG_DB.Exec("DELETE FROM logs WHERE content LIKE ?", marker+"-%").Error)
	t.Cleanup(func() {
		require.NoError(t, LOG_DB.Exec("DELETE FROM logs WHERE content LIKE ?", marker+"-%").Error)
	})

	base := time.Now().UTC().Unix() - 3_000
	for i, spec := range []struct {
		user      int
		logType   int
		createdAt int64
	}{
		{11, LogTypeConsume, base + 1},
		{11, LogTypeProvisional, base + 2},
		{12, LogTypeConsume, base + 3},
	} {
		require.NoError(t, LOG_DB.Exec(
			"INSERT INTO logs (user_id, type, created_at, token_name, content) VALUES (?,?,?,?,?)",
			spec.user, spec.logType, spec.createdAt, "prod", fmt.Sprintf("%s-%02d", marker, i)).Error)
	}

	filter := LogListFilter{TokenName: "prod"}

	selfScope11 := LogListScope{Kind: LogListScopeSelf, SubjectUserID: 11, PrincipalUserID: 11, Role: RoleCommonUser}
	page, err := FetchLogCursorPage(context.Background(), selfScope11, filter.Normalize(selfScope11), nil, 50)
	require.NoError(t, err)
	own := 0
	for _, l := range page.Logs {
		if len(l.Content) >= len(marker) && l.Content[:len(marker)] == marker {
			own++
			require.Equal(t, 11, l.UserId, "the self scope must never return another user's rows")
			require.NotEqual(t, LogTypeProvisional, l.Type, "provisional rows are excluded")
		}
	}
	require.Equal(t, 1, own)

	// An explicit provisional request must still return nothing: the exclusion
	// is unconditional, exactly as it is on the legacy route.
	provisional := LogListFilter{TokenName: "prod", LogType: LogTypeProvisional}
	page, err = FetchLogCursorPage(context.Background(), selfScope11, provisional.Normalize(selfScope11), nil, 50)
	require.NoError(t, err)
	for _, l := range page.Logs {
		require.NotContains(t, l.Content, marker, "type=6 must select no fixture row")
	}
	require.False(t, page.HasMore)
}

// TestLogCursorStatementShape verifies the emitted SQL is the two-arm UNION ALL
// with an inlined limit, and that the first page needs no anchor arm.
//
// Parameters:
//   - t: the test handle.
//
// Return values: none.
func TestLogCursorStatementShape(t *testing.T) {
	scope := LogListScope{Kind: LogListScopeAll, PrincipalUserID: 1, Role: RoleRootUser}
	filter := LogListFilter{LogType: LogTypeConsume}.Normalize(scope)

	first, firstArgs := buildLogCursorStatement(scope, filter, nil, 20)
	require.NotContains(t, first, "UNION ALL", "the first page needs no anchor arms")
	require.Contains(t, first, "LIMIT 21", "the probe row must be inlined, not bound")
	require.NotContains(t, first, "LIMIT ?", "MySQL rejects a placeholder limit without prepared statements")
	require.Contains(t, first, "created_at IS NOT NULL")
	require.Contains(t, first, "type <> ?")
	require.NotEmpty(t, firstArgs)

	next, nextArgs := buildLogCursorStatement(scope, filter, &LogCursorAnchor{CreatedAt: 100, ID: 7}, 20)
	require.Contains(t, next, "UNION ALL")
	require.Contains(t, next, "created_at = ? AND id < ?")
	require.Contains(t, next, "created_at < ?")
	require.Equal(t, 3, countOccurrences(next, "LIMIT 21"), "each arm and the outer query bound themselves")
	require.Contains(t, nextArgs, int64(100))
	require.Contains(t, nextArgs, int64(7))
}

// countOccurrences counts non-overlapping occurrences of sub in s.
//
// Parameters:
//   - s: the string to search.
//   - sub: the substring to count.
//
// Return values:
//   - int: the number of occurrences.
func countOccurrences(s, sub string) int {
	count := 0
	for i := 0; i+len(sub) <= len(s); {
		if s[i:i+len(sub)] == sub {
			count++
			i += len(sub)
			continue
		}
		i++
	}
	return count
}
