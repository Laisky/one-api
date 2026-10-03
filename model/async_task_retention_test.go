package model

import (
	"context"
	"math"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// asyncTaskLegacyRetentionPredicate is the non-sargable expression the two
// sargable passes replaced. It is kept here, not in production code, so the
// equivalence tests below compare against the exact original text.
const asyncTaskLegacyRetentionPredicate = "CASE WHEN last_accessed_at > 0 THEN last_accessed_at ELSE created_at END < ?"

// asyncRetentionFixtureRow is one fixture binding.
type asyncRetentionFixtureRow struct {
	// taskID identifies the row in assertions.
	taskID string
	// createdAt is the creation timestamp in Unix milliseconds.
	createdAt int64
	// lastAccessedAt is the last-access timestamp; nil writes SQL NULL, which
	// is what an AutoMigrate that added the column leaves on existing rows.
	lastAccessedAt *int64
	// expired records whether the legacy CASE predicate selects this row, so
	// the fixture states the expectation independently of both predicates.
	expired bool
}

// seedAsyncRetentionFixture inserts the shared boundary fixture.
//
// Rows are written with raw SQL because the Go struct cannot express a NULL
// last_accessed_at, and NULL is one of the values the two predicates must agree
// on.
//
// Parameters:
//   - t: the test, used to fail fast.
//   - db: the handle to seed.
//   - cutoff: the retention cutoff the fixture is built around.
//   - safety: how far above the cutoff the "not expired" rows are pushed. Pass
//     0 to place them exactly on the boundary, which is the strictest test of a
//     predicate evaluated at a fixed cutoff. An end-to-end sweep recomputes its
//     own cutoff from time.Now a few milliseconds later, so it must pass a
//     margin larger than that drift or a boundary row's verdict becomes a race.
//
// Return values:
//   - []asyncRetentionFixtureRow: the seeded rows with their expected verdicts.
func seedAsyncRetentionFixture(t *testing.T, db *gorm.DB, cutoff, safety int64) []asyncRetentionFixtureRow {
	t.Helper()

	ms := func(v int64) *int64 { return &v }
	boundary := cutoff + safety
	fresh := boundary + 10_000

	rows := []asyncRetentionFixtureRow{
		// Touched rows are judged by last_accessed_at, whatever created_at says.
		{taskID: "touched-expired", createdAt: fresh, lastAccessedAt: ms(cutoff - 1), expired: true},
		{taskID: "touched-boundary", createdAt: cutoff - 100_000, lastAccessedAt: ms(boundary), expired: false},
		{taskID: "touched-fresh", createdAt: cutoff - 100_000, lastAccessedAt: ms(boundary + 1), expired: false},
		{taskID: "touched-ancient", createdAt: fresh, lastAccessedAt: ms(cutoff - 100_000), expired: true},
		{taskID: "touched-one", createdAt: fresh, lastAccessedAt: ms(1), expired: true},

		// Never-touched rows fall back to created_at.
		{taskID: "zero-expired", createdAt: cutoff - 1, lastAccessedAt: ms(0), expired: true},
		{taskID: "zero-boundary", createdAt: boundary, lastAccessedAt: ms(0), expired: false},
		{taskID: "zero-fresh", createdAt: fresh, lastAccessedAt: ms(0), expired: false},

		// A negative last access also takes the CASE ELSE branch.
		{taskID: "negative-expired", createdAt: cutoff - 1, lastAccessedAt: ms(-5), expired: true},
		{taskID: "negative-fresh", createdAt: fresh, lastAccessedAt: ms(-5), expired: false},

		// NULL is not > 0 either, so it takes the ELSE branch as well.
		{taskID: "null-expired", createdAt: cutoff - 1, lastAccessedAt: nil, expired: true},
		{taskID: "null-boundary", createdAt: boundary, lastAccessedAt: nil, expired: false},
		{taskID: "null-fresh", createdAt: fresh, lastAccessedAt: nil, expired: false},
	}

	for i, row := range rows {
		var lastAccessed any
		if row.lastAccessedAt != nil {
			lastAccessed = *row.lastAccessedAt
		}
		require.NoError(t, db.Exec(
			"INSERT INTO async_task_bindings "+
				"(task_id, task_type, user_id, channel_id, channel_type, created_at, updated_at, last_accessed_at) "+
				"VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
			row.taskID, "video", 1, 2, 3, row.createdAt, row.createdAt, lastAccessed,
		).Error, "seed fixture row %d", i)
	}

	return rows
}

// selectAsyncTaskIDs runs a predicate and returns the task ids it selects.
//
// Parameters:
//   - t: the test, used to fail fast.
//   - db: the handle to query.
//   - where: the predicate, with `?` placeholders.
//   - args: the predicate arguments.
//
// Return values:
//   - []string: matching task ids, ordered for comparison.
func selectAsyncTaskIDs(t *testing.T, db *gorm.DB, where string, args ...any) []string {
	t.Helper()
	var ids []string
	require.NoError(t, db.Raw(
		"SELECT task_id FROM async_task_bindings WHERE "+where+" ORDER BY task_id", args...,
	).Scan(&ids).Error)
	return ids
}

// TestAsyncTaskRetentionPredicateMatchesLegacy proves the sargable rewrite
// selects exactly the row set the old CASE expression selected.
//
// The fixture covers last_accessed_at = 0, > 0, negative and NULL, each with
// created_at above, below and exactly at the cutoff, and includes the two
// cross-cases the CASE exists for: an old row that was touched recently, and a
// new row that was touched long ago.
func TestAsyncTaskRetentionPredicateMatchesLegacy(t *testing.T) {
	db := setupAsyncTaskTestDB(t)

	cutoff := time.Now().UTC().UnixMilli()
	fixture := seedAsyncRetentionFixture(t, db, cutoff, 0)

	expected := make([]string, 0, len(fixture))
	for _, row := range fixture {
		if row.expired {
			expected = append(expected, row.taskID)
		}
	}
	require.Len(t, expected, 6, "the fixture must contain expired rows of every shape")

	legacy := selectAsyncTaskIDs(t, db, asyncTaskLegacyRetentionPredicate, cutoff)
	require.ElementsMatch(t, expected, legacy,
		"the fixture's own expectations must match the original expression")

	touched := selectAsyncTaskIDs(t, db, asyncTaskTouchedPredicate, cutoff)
	untouched := selectAsyncTaskIDs(t, db, asyncTaskUntouchedPredicate, cutoff)

	require.ElementsMatch(t, expected, append(append([]string{}, touched...), untouched...),
		"the two passes together must select exactly the legacy row set")
	require.Empty(t, intersectStrings(touched, untouched),
		"the passes must be disjoint, or rows would be located twice")

	// Also assert against the legacy result directly, so a fixture edit cannot
	// silently weaken the equivalence claim.
	union := selectAsyncTaskIDs(t, db,
		"("+asyncTaskTouchedPredicate+") OR ("+asyncTaskUntouchedPredicate+")", cutoff, cutoff)
	require.Equal(t, legacy, union)
}

// intersectStrings returns the values present in both slices.
//
// Parameters:
//   - a: the first slice.
//   - b: the second slice.
//
// Return values:
//   - []string: the shared values.
func intersectStrings(a, b []string) []string {
	seen := make(map[string]bool, len(a))
	for _, v := range a {
		seen[v] = true
	}
	var shared []string
	for _, v := range b {
		if seen[v] {
			shared = append(shared, v)
		}
	}
	return shared
}

// TestCleanExpiredAsyncTaskBindingsMatchesLegacySelection runs the real sweep
// over the same fixture and proves the surviving rows are exactly those the
// legacy predicate would have spared.
func TestCleanExpiredAsyncTaskBindingsMatchesLegacySelection(t *testing.T) {
	testDB := setupAsyncTaskTestDB(t)
	originalDB := DB
	DB = testDB
	defer func() { DB = originalDB }()

	const retentionDays = 7
	cutoff := time.Now().UTC().Add(-retentionDays * 24 * time.Hour).UnixMilli()

	// The sweep recomputes its cutoff from time.Now, milliseconds after this
	// fixture did, so the fixture keeps every surviving row a full minute above
	// the boundary; exact-boundary equivalence is proven at a fixed cutoff in
	// TestAsyncTaskRetentionPredicateMatchesLegacy.
	fixture := seedAsyncRetentionFixture(t, testDB, cutoff, 60_000)

	legacyExpired := selectAsyncTaskIDs(t, testDB, asyncTaskLegacyRetentionPredicate, cutoff)

	var survivorsBefore []string
	for _, row := range fixture {
		if !row.expired {
			survivorsBefore = append(survivorsBefore, row.taskID)
		}
	}

	stats, err := CleanExpiredAsyncTaskBindingsStats(context.Background(), retentionDays)
	require.NoError(t, err)
	require.Equal(t, int64(len(legacyExpired)), stats.Deleted)
	require.Zero(t, stats.Backlog, "a drained sweep must report no backlog")
	require.Positive(t, stats.Passes)

	remaining := selectAsyncTaskIDs(t, testDB, "1 = 1")
	require.ElementsMatch(t, survivorsBefore, remaining,
		"only rows the legacy predicate spared may survive")
}

// TestCleanExpiredAsyncTaskBindingsChunksBothPasses verifies both passes sweep
// in bounded chunks and that their statistics merge into one run.
func TestCleanExpiredAsyncTaskBindingsChunksBothPasses(t *testing.T) {
	testDB := setupAsyncTaskTestDB(t)
	originalDB := DB
	DB = testDB
	defer func() { DB = originalDB }()

	const retentionDays = 7
	cutoff := time.Now().UTC().Add(-retentionDays * 24 * time.Hour).UnixMilli()

	// Twelve touched rows sharing four distinct timestamps (a tie group per
	// timestamp) plus eight never-touched rows, so both passes run several
	// chunks and both meet ties.
	for i := range 12 {
		last := cutoff - int64(1+i%4)
		require.NoError(t, testDB.Exec(
			"INSERT INTO async_task_bindings (task_id, task_type, user_id, channel_id, channel_type, created_at, updated_at, last_accessed_at) "+
				"VALUES (?, 'video', 1, 2, 3, ?, ?, ?)",
			"touched-"+strconv.Itoa(i), cutoff+1_000, cutoff+1_000, last).Error)
	}
	for i := range 8 {
		created := cutoff - int64(1+i%2)
		var nullAccess any
		if i%2 == 0 {
			nullAccess = int64(0)
		}
		require.NoError(t, testDB.Exec(
			"INSERT INTO async_task_bindings (task_id, task_type, user_id, channel_id, channel_type, created_at, updated_at, last_accessed_at) "+
				"VALUES (?, 'video', 1, 2, 3, ?, ?, ?)",
			"untouched-"+strconv.Itoa(i), created, created, nullAccess).Error)
	}
	// One survivor of each kind.
	require.NoError(t, testDB.Exec(
		"INSERT INTO async_task_bindings (task_id, task_type, user_id, channel_id, channel_type, created_at, updated_at, last_accessed_at) "+
			"VALUES ('touched-alive', 'video', 1, 2, 3, ?, ?, ?)",
		cutoff-500_000, cutoff, cutoff+5_000).Error)
	require.NoError(t, testDB.Exec(
		"INSERT INTO async_task_bindings (task_id, task_type, user_id, channel_id, channel_type, created_at, updated_at, last_accessed_at) "+
			"VALUES ('untouched-alive', 'video', 1, 2, 3, ?, ?, NULL)",
		cutoff+5_000, cutoff+5_000).Error)

	touchedStats, err := ChunkedDeleteWithStats(context.Background(), DB, ChunkedDeleteOptions{
		Table:       "async_task_bindings",
		Where:       asyncTaskTouchedPredicate,
		Args:        []any{cutoff},
		OrderColumn: "last_accessed_at",
		BatchSize:   5,
	})
	require.NoError(t, err)
	require.Equal(t, int64(12), touchedStats.Deleted)
	require.Equal(t, 3, touchedStats.Chunks)
	require.Equal(t, int64(12), touchedStats.Candidates,
		"exact-ID deletion locates every deleted row once, including the opening chunk")

	untouchedStats, err := ChunkedDeleteWithStats(context.Background(), DB, ChunkedDeleteOptions{
		Table:       "async_task_bindings",
		Where:       asyncTaskUntouchedPredicate,
		Args:        []any{cutoff},
		OrderColumn: "created_at",
		BatchSize:   5,
	})
	require.NoError(t, err)
	require.Equal(t, int64(8), untouchedStats.Deleted)
	require.Equal(t, int64(8), untouchedStats.Candidates)

	merged := touchedStats.Merge(untouchedStats)
	require.Equal(t, int64(20), merged.Deleted)
	require.Zero(t, merged.Backlog)

	remaining := selectAsyncTaskIDs(t, testDB, "1 = 1")
	require.ElementsMatch(t, []string{"touched-alive", "untouched-alive"}, remaining)
}

// TestAsyncTaskRetentionPredicatesAreSargable is the execution-plan evidence for
// the rewrite: SQLite plans the legacy CASE as a table SCAN and both new
// predicates as index SEARCHes.
//
// This is the whole point of the change. A SCAN per chunk is what made the sweep
// quadratic in table size; a SEARCH seeks straight to the chunk's range.
func TestAsyncTaskRetentionPredicatesAreSargable(t *testing.T) {
	db := setupAsyncTaskTestDB(t)
	cutoff := time.Now().UTC().UnixMilli()
	seedAsyncRetentionFixture(t, db, cutoff, 0)

	plan := func(query string, args ...any) string {
		rows, err := db.Raw("EXPLAIN QUERY PLAN "+query, args...).Rows()
		require.NoError(t, err)
		defer rows.Close() //nolint:errcheck // read-only cursor

		var details []string
		for rows.Next() {
			var id, parent, notUsed int
			var detail string
			require.NoError(t, rows.Scan(&id, &parent, &notUsed, &detail))
			details = append(details, detail)
		}
		require.NoError(t, rows.Err())
		require.NotEmpty(t, details)
		return strings.Join(details, " | ")
	}

	floor := int64(math.MinInt64)

	legacyPlan := plan("SELECT id FROM async_task_bindings WHERE "+
		asyncTaskLegacyRetentionPredicate+" LIMIT 5", cutoff)
	require.Contains(t, legacyPlan, "SCAN",
		"the CASE predicate is not sargable; this is the behavior being replaced")
	require.NotContains(t, legacyPlan, "SEARCH")

	touchedPlan := plan("SELECT last_accessed_at FROM async_task_bindings WHERE ("+
		asyncTaskTouchedPredicate+") AND last_accessed_at >= ? ORDER BY last_accessed_at ASC LIMIT 5",
		cutoff, floor)
	require.Contains(t, touchedPlan, "SEARCH")
	require.Contains(t, touchedPlan, "idx_async_task_retention",
		"pass 1 must use the composite retention index")
	require.Contains(t, touchedPlan, "last_accessed_at<?",
		"the cutoff must bound the index range; a predicate the index cannot "+
			"evaluate would leave the cursor as the only bound and re-read the "+
			"whole column")

	untouchedPlan := plan("SELECT created_at FROM async_task_bindings WHERE ("+
		asyncTaskUntouchedPredicate+") AND created_at >= ? ORDER BY created_at ASC LIMIT 5",
		cutoff, floor)
	require.Contains(t, untouchedPlan, "SEARCH",
		"pass 2 must seek on its cursor column rather than scanning the table")
	require.Contains(t, untouchedPlan, "created_at<?",
		"the cutoff must bound pass 2's index range as well")
}

// TestAsyncTaskRetentionIndexExists verifies AutoMigrate actually creates the
// composite index the sweep plans against. GORM's multi-index field tags are
// easy to get subtly wrong, and a missing index would silently restore the
// quadratic behavior.
func TestAsyncTaskRetentionIndexExists(t *testing.T) {
	db := setupAsyncTaskTestDB(t)

	require.True(t, db.Migrator().HasIndex("async_task_bindings", "idx_async_task_retention"))

	var columns []string
	require.NoError(t, db.Raw("SELECT name FROM pragma_index_info('idx_async_task_retention')").
		Scan(&columns).Error)
	require.Equal(t, []string{"last_accessed_at", "created_at"}, columns,
		"last_accessed_at must lead, or pass 1 cannot range on it")
}

// TestCleanExpiredAsyncTaskBindingsIsDisabledForNonPositiveWindow keeps the
// disabled-window contract, which the stats rewrite must not change.
func TestCleanExpiredAsyncTaskBindingsIsDisabledForNonPositiveWindow(t *testing.T) {
	testDB := setupAsyncTaskTestDB(t)
	originalDB := DB
	DB = testDB
	defer func() { DB = originalDB }()

	seedAsyncRetentionFixture(t, testDB, time.Now().UTC().UnixMilli(), 0)

	stats, err := CleanExpiredAsyncTaskBindingsStats(context.Background(), 0)
	require.NoError(t, err)
	require.Zero(t, stats.Deleted)
	require.Zero(t, stats.Passes)

	deleted, err := CleanExpiredAsyncTaskBindings(-1)
	require.NoError(t, err)
	require.Zero(t, deleted)
	require.Len(t, selectAsyncTaskIDs(t, testDB, "1 = 1"), 13)
}
