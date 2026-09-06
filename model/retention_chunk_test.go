package model

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// seedRetentionTraces inserts count trace rows with the given created_at.
//
// Parameters:
//   - t: the test, used to fail fast.
//   - prefix: trace id prefix, also used for cleanup.
//   - count: how many rows to insert.
//   - createdAt: the created_at value in Unix milliseconds.
//
// Return values: none.
func seedRetentionTraces(t *testing.T, prefix string, count int, createdAt int64) {
	t.Helper()
	rows := make([]*Trace, 0, count)
	for i := range count {
		row, _, err := NewTraceRow(TraceRowInput{
			TraceId:   prefix + strconv.Itoa(i),
			URL:       "/api/test",
			Method:    "GET",
			CreatedAt: createdAt,
		})
		require.NoError(t, err)
		rows = append(rows, row)
	}
	written, err := InsertTraces(context.Background(), rows, 100)
	require.NoError(t, err)
	require.Equal(t, count, written)
}

// countRetentionTraces returns how many seeded rows remain.
//
// Parameters:
//   - t: the test, used to fail fast.
//   - pattern: a SQL LIKE pattern matched against trace_id.
//
// Return values:
//   - int64: the remaining row count.
func countRetentionTraces(t *testing.T, pattern string) int64 {
	t.Helper()
	var count int64
	require.NoError(t, DB.Model(&Trace{}).Where("trace_id LIKE ?", pattern).Count(&count).Error)
	return count
}

// TestChunkedDeleteRemovesEveryMatchingRow verifies a bounded sweep removes the
// same rows an unbounded DELETE would, across several chunks.
func TestChunkedDeleteRemovesEveryMatchingRow(t *testing.T) {
	setupTestDatabase(t)
	cleanupTraces(t, "test-chunk-old-%")
	cleanupTraces(t, "test-chunk-new-%")

	now := time.Now().UTC().UnixMilli()
	seedRetentionTraces(t, "test-chunk-old-", 25, now-10_000)
	seedRetentionTraces(t, "test-chunk-new-", 5, now)

	deleted, err := ChunkedDelete(context.Background(), DB, ChunkedDeleteOptions{
		Table:     "traces",
		Where:     "created_at < ? AND trace_id LIKE ?",
		Args:      []any{now - 5_000, "test-chunk-%"},
		BatchSize: 7,
	})
	require.NoError(t, err)
	require.Equal(t, int64(25), deleted)

	require.Zero(t, countRetentionTraces(t, "test-chunk-old-%"))
	require.Equal(t, int64(5), countRetentionTraces(t, "test-chunk-new-%"),
		"rows outside the predicate must survive")
}

// TestChunkedDeleteTerminatesOnExactMultiple verifies the loop ends when the
// row count is an exact multiple of the batch size, rather than spinning.
func TestChunkedDeleteTerminatesOnExactMultiple(t *testing.T) {
	setupTestDatabase(t)
	cleanupTraces(t, "test-chunk-exact-%")

	now := time.Now().UTC().UnixMilli()
	seedRetentionTraces(t, "test-chunk-exact-", 10, now-10_000)

	done := make(chan struct{})
	var deleted int64
	var err error
	go func() {
		defer close(done)
		deleted, err = ChunkedDelete(context.Background(), DB, ChunkedDeleteOptions{
			Table:     "traces",
			Where:     "trace_id LIKE ?",
			Args:      []any{"test-chunk-exact-%"},
			BatchSize: 5,
		})
	}()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("chunked delete did not terminate")
	}

	require.NoError(t, err)
	require.Equal(t, int64(10), deleted)
	require.Zero(t, countRetentionTraces(t, "test-chunk-exact-%"))
}

// TestChunkedDeleteStopsOnCancelledContext verifies a cancelled sweep REPORTS
// the cancellation rather than returning success for a partial delete.
//
// An operator-triggered purge must never be able to answer "done" when it
// stopped early; background sweepers recognise context.Canceled and treat it as
// ordinary shutdown.
func TestChunkedDeleteStopsOnCancelledContext(t *testing.T) {
	setupTestDatabase(t)
	cleanupTraces(t, "test-chunk-cancel-%")

	now := time.Now().UTC().UnixMilli()
	seedRetentionTraces(t, "test-chunk-cancel-", 10, now-10_000)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	deleted, err := ChunkedDelete(ctx, DB, ChunkedDeleteOptions{
		Table:     "traces",
		Where:     "trace_id LIKE ?",
		Args:      []any{"test-chunk-cancel-%"},
		BatchSize: 5,
	})
	require.Error(t, err, "a cancelled sweep must not be reported as a completed one")
	require.ErrorIs(t, err, context.Canceled)
	require.Zero(t, deleted)
	require.Equal(t, int64(10), countRetentionTraces(t, "test-chunk-cancel-%"))
}

// TestChunkedDeleteRejectsUnknownTable verifies the allow-list, which is what
// keeps the interpolated table name safe.
func TestChunkedDeleteRejectsUnknownTable(t *testing.T) {
	setupTestDatabase(t)

	_, err := ChunkedDelete(context.Background(), DB, ChunkedDeleteOptions{
		Table: "users; DROP TABLE users",
		Where: "1 = 1",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "not an allowed retention target")

	_, err = ChunkedDelete(context.Background(), DB, ChunkedDeleteOptions{Table: "users", Where: "1 = 1"})
	require.Error(t, err)
}

// TestChunkedDeleteRejectsNilHandle verifies the sweeper fails loudly rather
// than panicking when the database is not initialized.
func TestChunkedDeleteRejectsNilHandle(t *testing.T) {
	_, err := ChunkedDelete(context.Background(), nil, ChunkedDeleteOptions{Table: "traces", Where: "1 = 1"})
	require.Error(t, err)
}

// TestBoundedDeleteStatementPerDialect verifies each engine gets a form it
// actually supports: MySQL has native DELETE ... LIMIT, PostgreSQL has none and
// needs ctid, and the embedded SQLite is not built with
// SQLITE_ENABLE_UPDATE_DELETE_LIMIT so it needs rowid.
func TestBoundedDeleteStatementPerDialect(t *testing.T) {
	// The dialect must come from the handle, so each case builds a handle of the
	// engine under test rather than flipping a process-global flag. That is the
	// property the split SQL_DSN / LOG_SQL_DSN topology depends on.
	t.Run("postgres", func(t *testing.T) {
		db, err := gorm.Open(postgres.New(postgres.Config{DSN: "postgres://u:p@127.0.0.1:1/none", PreferSimpleProtocol: true}), &gorm.Config{DisableAutomaticPing: true})
		require.NoError(t, err)

		stmt := boundedDeleteStatement(db, "traces", "created_at < ?", 500)
		require.Equal(t,
			"DELETE FROM traces WHERE ctid IN (SELECT ctid FROM traces WHERE created_at < ? LIMIT 500)",
			stmt)
	})

	t.Run("mysql", func(t *testing.T) {
		db, err := gorm.Open(mysql.New(mysql.Config{DSN: "u:p@tcp(127.0.0.1:1)/none", SkipInitializeWithVersion: true}), &gorm.Config{DisableAutomaticPing: true})
		require.NoError(t, err)

		stmt := boundedDeleteStatement(db, "logs", "created_at < ?", 500)
		require.Equal(t, "DELETE FROM logs WHERE created_at < ? LIMIT 500", stmt)
	})

	t.Run("sqlite", func(t *testing.T) {
		db, err := gorm.Open(sqlite.Open(t.TempDir()+"/dialect.db"), &gorm.Config{})
		require.NoError(t, err)

		stmt := boundedDeleteStatement(db, "traces", "created_at < ?", 500)
		require.Equal(t,
			"DELETE FROM traces WHERE rowid IN (SELECT rowid FROM traces WHERE created_at < ? LIMIT 500)",
			stmt)
	})
}

// TestCleanExpiredTracesUsesChunkedDelete verifies the trace sweeper removes
// only expired rows when its window is smaller than the batch size and when it
// spans several chunks.
func TestCleanExpiredTracesUsesChunkedDelete(t *testing.T) {
	setupTestDatabase(t)
	cleanupTraces(t, "test-sweep-%")

	retentionDays := 30
	expired := time.Now().UTC().Add(-time.Duration(retentionDays+1) * 24 * time.Hour).UnixMilli()
	fresh := time.Now().UTC().Add(-time.Duration(retentionDays-1) * 24 * time.Hour).UnixMilli()

	seedRetentionTraces(t, "test-sweep-old-", 12, expired)
	seedRetentionTraces(t, "test-sweep-new-", 3, fresh)

	deleted, err := CleanExpiredTracesContext(context.Background(), retentionDays)
	require.NoError(t, err)
	require.GreaterOrEqual(t, deleted, int64(12))

	require.Zero(t, countRetentionTraces(t, "test-sweep-old-%"))
	require.Equal(t, int64(3), countRetentionTraces(t, "test-sweep-new-%"))
}
