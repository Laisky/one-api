package model

import (
	"context"
	"math"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// seedRetentionTracesAt inserts count trace rows whose created_at comes from a
// per-row function, so a fixture can mix distinct timestamps with tie groups.
//
// Parameters:
//   - t: the test, used to fail fast.
//   - prefix: trace id prefix, also used for cleanup and predicates.
//   - count: how many rows to insert.
//   - createdAt: maps the row index to its created_at in Unix milliseconds.
//
// Return values: none.
func seedRetentionTracesAt(t *testing.T, prefix string, count int, createdAt func(int) int64) {
	t.Helper()
	rows := make([]*Trace, 0, count)
	for i := range count {
		row, _, err := NewTraceRow(TraceRowInput{
			TraceId:   prefix + strconv.Itoa(i),
			URL:       "/api/test",
			Method:    "GET",
			CreatedAt: createdAt(i),
		})
		require.NoError(t, err)
		rows = append(rows, row)
	}
	written, err := InsertTraces(context.Background(), rows, 100)
	require.NoError(t, err)
	require.Equal(t, count, written)
}

// TestChunkedDeleteKeysetDeletesEveryEligibleRow proves the keyset watermark
// that bounds candidate location does not skip eligible rows.
//
// The fixture is built out of the two shapes a cursor can get wrong: strictly
// increasing timestamps, and a tie group LARGER than the batch size, where an
// exclusive cursor would jump over every tied row that did not fit in the
// chunk. Rows exactly at the cutoff and rows newer than it must survive, which
// is what proves the added range predicate did not widen the predicate.
func TestChunkedDeleteKeysetDeletesEveryEligibleRow(t *testing.T) {
	setupTestDatabase(t)
	cleanupTraces(t, "test-keyset-%")

	now := time.Now().UTC().UnixMilli()
	cutoff := now - 5_000

	seedRetentionTracesAt(t, "test-keyset-spread-", 20, func(i int) int64 {
		return cutoff - int64(20-i)*10
	})
	seedRetentionTracesAt(t, "test-keyset-tie-", 15, func(int) int64 { return cutoff - 1 })
	seedRetentionTracesAt(t, "test-keyset-boundary-", 3, func(int) int64 { return cutoff })
	seedRetentionTracesAt(t, "test-keyset-fresh-", 4, func(int) int64 { return now })

	const batchSize = 4
	stats, err := ChunkedDeleteWithStats(context.Background(), DB, ChunkedDeleteOptions{
		Table:       "traces",
		Where:       "created_at < ? AND trace_id LIKE ?",
		Args:        []any{cutoff, "test-keyset-%"},
		OrderColumn: "created_at",
		BatchSize:   batchSize,
	})
	require.NoError(t, err)

	require.Equal(t, int64(35), stats.Deleted)
	require.Zero(t, countRetentionTraces(t, "test-keyset-spread-%"))
	require.Zero(t, countRetentionTraces(t, "test-keyset-tie-%"),
		"a tie group larger than one batch must not be skipped by the cursor")
	require.Equal(t, int64(3), countRetentionTraces(t, "test-keyset-boundary-%"),
		"rows exactly at the cutoff are not eligible")
	require.Equal(t, int64(4), countRetentionTraces(t, "test-keyset-fresh-%"))

	require.Equal(t, 1, stats.Passes, "a drained sweep needs exactly one pass")
	require.Zero(t, stats.Backlog)
	require.Zero(t, stats.OldestEligibleAtMilli)

	// Candidate location is the property under test: after the opening chunk
	// (which deletes directly, having no watermark to seek past) the sweep read
	// exactly one cursor value per deleted row. Before the keyset walk every
	// chunk restarted at the beginning of the qualifying range, so locating the
	// same 35 rows in batches of 4 meant re-reading the expired prefix nine
	// times.
	require.Equal(t, 9, stats.Chunks)
	require.Equal(t, int64(35), stats.Candidates)
}

// TestChunkedDeleteRemovesOnlyLocatedIDs verifies a batch deletes the primary
// keys it probed, not every row sharing their timestamp range. The previous
// range DELETE could remove a concurrent row that was never part of the
// bounded candidate set.
func TestChunkedDeleteRemovesOnlyLocatedIDs(t *testing.T) {
	setupTestDatabase(t)
	cleanupTraces(t, "test-exact-locate-%")

	cutoff := time.Now().UTC().UnixMilli()
	seedRetentionTracesAt(t, "test-exact-locate-", 3, func(int) int64 { return cutoff - 1 })

	sweep := chunkedSweep{
		db: DB, table: "traces", where: "created_at < ? AND trace_id LIKE ?",
		args: []any{cutoff, "test-exact-locate-%"}, orderColumn: "created_at", batchSize: 2,
	}
	candidates, err := sweep.locateChunk(context.Background(), math.MinInt64)
	require.NoError(t, err)
	require.Len(t, candidates, 2)

	deleted, err := sweep.deleteLocated(context.Background(), candidates)
	require.NoError(t, err)
	require.Equal(t, int64(2), deleted)
	require.Equal(t, int64(1), countRetentionTraces(t, "test-exact-locate-%"),
		"the same-timestamp row outside the located ID set must remain")
}

// TestChunkedDeleteKeysetStatementsPerDialect verifies the bounded statements
// carry the cursor range on every engine, since the range predicate is what
// keeps the DELETE's own candidate search bounded.
func TestChunkedDeleteKeysetStatementsPerDialect(t *testing.T) {
	newSweep := func(db *gorm.DB) chunkedSweep {
		return chunkedSweep{
			db:          db,
			table:       "traces",
			where:       "created_at < ?",
			orderColumn: "created_at",
			batchSize:   500,
		}
	}

	for _, open := range []func(*testing.T) *gorm.DB{
		func(t *testing.T) *gorm.DB {
			db, err := gorm.Open(postgres.New(postgres.Config{DSN: "postgres://u:p@127.0.0.1:1/none", PreferSimpleProtocol: true}), &gorm.Config{DisableAutomaticPing: true})
			require.NoError(t, err)
			return db
		},
		func(t *testing.T) *gorm.DB {
			db, err := gorm.Open(mysql.New(mysql.Config{DSN: "u:p@tcp(127.0.0.1:1)/none", SkipInitializeWithVersion: true}), &gorm.Config{DisableAutomaticPing: true})
			require.NoError(t, err)
			return db
		},
		func(t *testing.T) *gorm.DB {
			db, err := gorm.Open(sqlite.Open(t.TempDir()+"/keyset.db"), &gorm.Config{})
			require.NoError(t, err)
			return db
		},
	} {
		db := open(t)
		require.Equal(t, "DELETE FROM traces WHERE id IN (?,?,?) AND (created_at < ?)", newSweep(db).deleteStatement(3))
	}
}

// TestRetentionCandidateLimitCapsSQLiteParameters verifies exact-ID deletion
// remains valid when an operator configures a batch larger than SQLite's
// placeholder limit. Range deletion used one predicate placeholder; exact IDs
// use one placeholder per candidate and must therefore be capped per dialect.
func TestRetentionCandidateLimitCapsSQLiteParameters(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/parameter-limit.db"), &gorm.Config{})
	require.NoError(t, err)

	limit := retentionCandidateLimit(db, 2)
	require.Equal(t, MaxStatementParametersSQLite-2, limit)

	requested := MaxStatementParametersSQLite * 2
	require.Equal(t, limit, min(requested, retentionCandidateLimit(db, 2)))
	statement := (chunkedSweep{table: "traces", where: "created_at < ? AND trace_id LIKE ?"}).deleteStatement(limit)
	require.Equal(t, MaxStatementParametersSQLite, strings.Count(statement, "?"))
}

// TestRetentionLocateUsesExistingOrderingIndex verifies exact-ID retention does
// not add id to ORDER BY. The schema indexes created_at and
// (last_accessed_at, created_at), not `(order_column, id)`; adding id would
// force a temporary sort of large timestamp tie groups and defeat bounded
// candidate location.
func TestRetentionLocateUsesExistingOrderingIndex(t *testing.T) {
	setupTestDatabase(t)
	cutoff := time.Now().UTC().UnixMilli()

	rows, err := DB.Raw("EXPLAIN QUERY PLAN SELECT id, created_at FROM traces WHERE created_at < ? AND created_at >= ? ORDER BY created_at ASC LIMIT 5", cutoff, math.MinInt64).Rows()
	require.NoError(t, err)
	defer rows.Close() //nolint:errcheck // read-only cursor

	var plan []string
	for rows.Next() {
		var id, parent, unused int
		var detail string
		require.NoError(t, rows.Scan(&id, &parent, &unused, &detail))
		plan = append(plan, detail)
	}
	require.NoError(t, rows.Err())
	joined := strings.Join(plan, " | ")
	require.Contains(t, joined, "SEARCH")
	require.NotContains(t, joined, "TEMP B-TREE")
}

// TestChunkedDeleteRejectsUnknownOrderColumn verifies the ordering column is
// allow-listed, because it is interpolated into SQL exactly like the table name.
func TestChunkedDeleteRejectsUnknownOrderColumn(t *testing.T) {
	setupTestDatabase(t)

	_, err := ChunkedDeleteWithStats(context.Background(), DB, ChunkedDeleteOptions{
		Table:       "traces",
		Where:       "1 = 1",
		OrderColumn: "created_at) --",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "not an allowed keyset column")

	_, err = ChunkedDeleteWithStats(context.Background(), DB, ChunkedDeleteOptions{
		Table:       "traces",
		Where:       "1 = 1",
		OrderColumn: "last_accessed_at",
	})
	require.Error(t, err, "a column that exists on another retention table is still not allowed here")
}

// TestChunkedDeleteBacklogAccounting verifies the keeping-up measures: how many
// eligible rows remain and how old the oldest one is.
func TestChunkedDeleteBacklogAccounting(t *testing.T) {
	setupTestDatabase(t)
	cleanupTraces(t, "test-backlog-%")

	now := time.Now().UTC().UnixMilli()
	cutoff := now - 5_000
	oldest := cutoff - 90_000

	seedRetentionTracesAt(t, "test-backlog-", 12, func(i int) int64 { return oldest + int64(i) })

	sweep := chunkedSweep{
		db:          DB,
		table:       "traces",
		where:       "created_at < ? AND trace_id LIKE ?",
		args:        []any{cutoff, "test-backlog-%"},
		orderColumn: "created_at",
		batchSize:   5,
	}

	backlog, truncated, oldestRemaining, err := sweep.measureBacklog(context.Background())
	require.NoError(t, err)
	require.Equal(t, int64(12), backlog)
	require.False(t, truncated)
	require.Equal(t, oldest, oldestRemaining)

	stats := ChunkedDeleteStats{OldestEligibleAtMilli: oldestRemaining}
	age := stats.OldestEligibleAge(time.UnixMilli(oldest + 3_600_000).UTC())
	require.Equal(t, time.Hour, age)

	deleted, err := ChunkedDelete(context.Background(), DB, ChunkedDeleteOptions{
		Table:       "traces",
		Where:       "created_at < ? AND trace_id LIKE ?",
		Args:        []any{cutoff, "test-backlog-%"},
		OrderColumn: "created_at",
		BatchSize:   5,
	})
	require.NoError(t, err)
	require.Equal(t, int64(12), deleted)

	backlog, truncated, oldestRemaining, err = sweep.measureBacklog(context.Background())
	require.NoError(t, err)
	require.Zero(t, backlog)
	require.False(t, truncated)
	require.Zero(t, oldestRemaining)
}

// TestChunkedDeleteReportsIncompleteBacklog verifies the compatibility API
// never reports success when eligible rows remain but the database did not
// delete them.
//
// Parameters:
//   - t: the running test.
//
// Return values: none.
func TestChunkedDeleteReportsIncompleteBacklog(t *testing.T) {
	setupTestDatabase(t)
	if dialectName(DB) != "sqlite" {
		t.Skip("the deterministic ignored-delete trigger is SQLite-specific")
	}
	cleanupTraces(t, "test-incomplete-retention-%")

	cutoff := time.Now().UTC().UnixMilli()
	seedRetentionTracesAt(t, "test-incomplete-retention-", 1, func(int) int64 { return cutoff - 1 })

	require.NoError(t, DB.Exec(
		"CREATE TRIGGER test_ignore_retention_delete BEFORE DELETE ON traces "+
			"WHEN OLD.trace_id LIKE 'test-incomplete-retention-%' BEGIN SELECT RAISE(IGNORE); END",
	).Error)
	t.Cleanup(func() {
		require.NoError(t, DB.Exec("DROP TRIGGER IF EXISTS test_ignore_retention_delete").Error)
		cleanupTraces(t, "test-incomplete-retention-%")
	})

	deleted, err := ChunkedDelete(context.Background(), DB, ChunkedDeleteOptions{
		Table:       "traces",
		Where:       "created_at < ? AND trace_id LIKE ?",
		Args:        []any{cutoff, "test-incomplete-retention-%"},
		OrderColumn: "created_at",
		BatchSize:   2,
	})
	require.Error(t, err, "eligible backlog must not be reported as a complete purge")
	require.Zero(t, deleted)
	require.Contains(t, err.Error(), "incomplete")
	require.Contains(t, err.Error(), "1 eligible rows remain")
}

// TestChunkedDeleteCatchesUpWithConcurrentInserts exercises W0.8's "retention
// must keep up with newly eligible rows under concurrent traffic".
//
// The arrivals are backdated BELOW the running walk's watermark, which is the
// only case a single keyset pass cannot see: the cursor has already moved past
// them. The sweep must notice the remaining backlog and walk again instead of
// returning on its first short chunk.
func TestChunkedDeleteCatchesUpWithConcurrentInserts(t *testing.T) {
	setupTestDatabase(t)
	cleanupTraces(t, "test-catchup-%")

	now := time.Now().UTC().UnixMilli()
	cutoff := now - 60_000

	const (
		seeded    = 120
		batchSize = 4
	)
	seedRetentionTracesAt(t, "test-catchup-seed-", seeded, func(i int) int64 {
		return cutoff - 10_000 + int64(i)
	})

	var (
		stats    ChunkedDeleteStats
		sweepErr error
		done     = make(chan struct{})
	)
	go func() {
		defer close(done)
		stats, sweepErr = ChunkedDeleteWithStats(context.Background(), DB, ChunkedDeleteOptions{
			Table:       "traces",
			Where:       "created_at < ? AND trace_id LIKE ?",
			Args:        []any{cutoff, "test-catchup-%"},
			OrderColumn: "created_at",
			BatchSize:   batchSize,
			Pause:       15 * time.Millisecond,
		})
	}()

	// Wait until two chunks committed. The first chunk deletes without a probe
	// and the second probes from the floor, so only from the third chunk on is
	// the cursor provably past the timestamps the arrivals will use.
	deadline := time.Now().Add(30 * time.Second)
	for countRetentionTraces(t, "test-catchup-seed-%") > seeded-2*batchSize {
		require.True(t, time.Now().Before(deadline), "sweep never made progress")
		time.Sleep(2 * time.Millisecond)
	}

	const arrivals = 12
	seedRetentionTracesAt(t, "test-catchup-late-", arrivals, func(i int) int64 {
		return cutoff - 500_000 + int64(i)
	})

	select {
	case <-done:
	case <-time.After(120 * time.Second):
		t.Fatal("concurrent sweep did not terminate")
	}

	require.NoError(t, sweepErr)
	require.Equal(t, int64(seeded+arrivals), stats.Deleted,
		"rows that became eligible below the watermark must still be removed")
	require.GreaterOrEqual(t, stats.Passes, 2,
		"a short chunk must not end the sweep while a backlog remains")
	require.Zero(t, stats.Backlog)
	require.Zero(t, countRetentionTraces(t, "test-catchup-%"))
}

// TestChunkedDeleteStatsMerge verifies the accounting a multi-pass sweep (such
// as the two async-task passes) reports as one logical run.
func TestChunkedDeleteStatsMerge(t *testing.T) {
	first := ChunkedDeleteStats{
		Deleted:               10,
		Chunks:                3,
		Passes:                2,
		Candidates:            12,
		Backlog:               4,
		OldestEligibleAtMilli: 1_000,
	}
	second := ChunkedDeleteStats{
		Deleted:               5,
		Chunks:                1,
		Passes:                1,
		Candidates:            5,
		Backlog:               1,
		BacklogTruncated:      true,
		OldestEligibleAtMilli: 500,
	}

	merged := first.Merge(second)
	require.Equal(t, int64(15), merged.Deleted)
	require.Equal(t, 4, merged.Chunks)
	require.Equal(t, 2, merged.Passes, "passes describe the harder pass, not the sum")
	require.Equal(t, int64(17), merged.Candidates)
	require.Equal(t, int64(5), merged.Backlog)
	require.True(t, merged.BacklogTruncated)
	require.Equal(t, int64(500), merged.OldestEligibleAtMilli, "the older row wins")

	empty := ChunkedDeleteStats{}
	require.Equal(t, int64(1_000), empty.Merge(first).OldestEligibleAtMilli)
	require.Equal(t, int64(1_000), first.Merge(empty).OldestEligibleAtMilli)
	require.Zero(t, empty.Merge(empty).OldestEligibleAtMilli)
	require.Zero(t, empty.OldestEligibleAge(time.Now().UTC()))
	require.Zero(t, ChunkedDeleteStats{OldestEligibleAtMilli: time.Now().UTC().Add(time.Hour).UnixMilli()}.
		OldestEligibleAge(time.Now().UTC()), "a future timestamp is not a negative age")
}

// TestRetentionTimestampUnitsAreDeclaredPerTable pins the one place the three
// retention tables disagree: logs.created_at stores epoch SECONDS (it is
// written by common/helper.GetTimestamp), while traces and async task bindings
// store milliseconds. The cursor itself compares raw int64s and does not care,
// but a reported "oldest eligible" age would be 1000x wrong if the unit were
// assumed.
func TestRetentionTimestampUnitsAreDeclaredPerTable(t *testing.T) {
	require.Equal(t, time.Second, retentionTables["logs"].orderColumnUnit)
	require.Equal(t, time.Millisecond, retentionTables["traces"].orderColumnUnit)
	require.Equal(t, time.Millisecond, retentionTables["async_task_bindings"].orderColumnUnit)

	seconds := time.Now().UTC().Unix()
	require.Equal(t, seconds*1000, normalizeRetentionTimestamp(seconds, time.Second))
	require.Equal(t, seconds*1000, normalizeRetentionTimestamp(seconds*1000, time.Millisecond))
	require.Zero(t, normalizeRetentionTimestamp(0, time.Second),
		"no eligible row must stay zero rather than becoming the epoch")

	stats := ChunkedDeleteStats{
		OldestEligibleAtMilli: normalizeRetentionTimestamp(seconds-3_600, time.Second),
	}
	require.Equal(t, time.Hour, stats.OldestEligibleAge(time.UnixMilli(seconds*1000).UTC()))
}
