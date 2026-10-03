package model

// Live-traversal semantics for the keyset log cursor (proposal
// docs/proposals/20260905_observability-data-tiering.md, W2.4 item 4 and the
// G2 "retention/concurrent inserts" case).
//
// The cursor walks live data, not a snapshot. The proposal states exactly what
// that does and does not promise, and these tests hold the implementation to
// the stated contract rather than to a stronger one it cannot keep:
//
//   - newer inserts before the anchor do not shift subsequent pages;
//   - deletes may shorten them;
//   - no row is ever visited twice;
//   - the traversal always terminates.

import (
	"context"
	"fmt"
	"math"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	glogger "gorm.io/gorm/logger"

	"github.com/Laisky/one-api/common"
)

// cursorTestDB installs an isolated log database for one test.
//
// These tests assert over the whole table rather than over a marked fixture, so
// they need a table nothing else writes to.
//
// Parameters:
//   - t: the test handle.
//
// Return values: none.
func cursorTestDB(t *testing.T) {
	t.Helper()

	// File-backed with WAL, matching how one-api actually runs SQLite: an
	// in-memory database would not exercise the concurrent-writer path.
	path := t.TempDir() + "/logs.db"
	db, err := gorm.Open(sqlite.Open(path+"?_busy_timeout=10000&_journal_mode=WAL&_synchronous=NORMAL"),
		&gorm.Config{Logger: glogger.Discard})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Log{}))

	prevLogDB, prevSQLite := LOG_DB, common.UsingSQLite.Load()
	LOG_DB = db
	common.UsingSQLite.Store(true)
	t.Cleanup(func() {
		LOG_DB = prevLogDB
		common.UsingSQLite.Store(prevSQLite)
	})
}

// seedCursorRows inserts consume rows at one row per second ending just before
// the supplied instant.
//
// Parameters:
//   - t: the test handle.
//   - count: how many rows to insert.
//   - endAt: the Unix second one past the newest row.
//   - prefix: content prefix identifying the batch.
//
// Return values:
//   - []string: the content values inserted, newest first.
func seedCursorRows(t *testing.T, count int, endAt int64, prefix string) []string {
	t.Helper()

	contents := make([]string, 0, count)
	for i := range count {
		content := fmt.Sprintf("%s-%04d", prefix, i)
		require.NoError(t, LOG_DB.Exec(
			"INSERT INTO logs (user_id, type, created_at, token_name, content) VALUES (?,?,?,?,?)",
			1, LogTypeConsume, endAt-int64(count-i), "prod", content).Error)
		contents = append(contents, content)
	}
	return contents
}

// walkCursor traverses every page, invoking between after each page.
//
// Parameters:
//   - t: the test handle.
//   - pageSize: rows per page.
//   - between: mutation to apply after each page; may be nil.
//
// Return values:
//   - []string: the content of every row visited, in traversal order.
func walkCursor(t *testing.T, pageSize int, between func(page int)) []string {
	t.Helper()
	return walkCursorFrom(t, pageSize, nil, between)
}

// walkCursorFrom traverses every page starting from an explicit anchor.
//
// Starting from a caller-supplied anchor is what makes a traversal that races
// concurrent writers deterministic. With a nil anchor the walk begins at
// "whatever is newest when the first query runs", so a row inserted between the
// test's setup and that first query legitimately belongs to the result -- and a
// test asserting the walk saw only pre-existing rows then fails for a reason
// that is not a defect.
//
// Parameters:
//   - t: the test handle.
//   - pageSize: rows per page.
//   - start: the anchor to begin after; nil begins at the newest row.
//   - between: mutation to apply after each page; may be nil.
//
// Return values:
//   - []string: the content of every visited row, in traversal order.
func walkCursorFrom(t *testing.T, pageSize int, start *LogCursorAnchor, between func(page int)) []string {
	t.Helper()

	scope := LogListScope{Kind: LogListScopeSelf, SubjectUserID: 1, PrincipalUserID: 1}
	filter := LogListFilter{}.Normalize(scope)

	anchor := start
	visited := make([]string, 0, 256)

	for page := 0; page < 500; page++ {
		got, err := FetchLogCursorPage(context.Background(), scope, filter, anchor, pageSize)
		require.NoError(t, err)

		for _, row := range got.Logs {
			visited = append(visited, row.Content)
		}
		if !got.HasMore || len(got.Logs) == 0 {
			return visited
		}

		last := got.Logs[len(got.Logs)-1]
		anchor = &LogCursorAnchor{CreatedAt: last.CreatedAt, ID: int64(last.Id)}
		if between != nil {
			between(page)
		}
	}

	t.Fatal("the traversal did not terminate")
	return nil
}

// TestCursorIgnoresRowsInsertedAheadOfTheAnchor proves a live insert newer than
// the current position neither appears mid-traversal nor shifts the pages that
// follow.
//
// Parameters:
//   - t: the test handle.
//
// Return values: none.
func TestCursorIgnoresRowsInsertedAheadOfTheAnchor(t *testing.T) {
	cursorTestDB(t)

	now := time.Now().UTC().Unix()
	original := seedCursorRows(t, 40, now, "original")

	inserted := 0
	visited := walkCursor(t, 7, func(page int) {
		// Traffic keeps arriving while the operator reads: each new row is
		// newer than every row already walked past.
		content := fmt.Sprintf("arrived-%02d", page)
		require.NoError(t, LOG_DB.Exec(
			"INSERT INTO logs (user_id, type, created_at, token_name, content) VALUES (?,?,?,?,?)",
			1, LogTypeConsume, now+int64(page)+1, "prod", content).Error)
		inserted++
	})

	require.Positive(t, inserted, "the test must actually have inserted during the walk")
	require.ElementsMatch(t, original, visited,
		"the traversal must yield exactly the rows that existed when it started")

	for _, content := range visited {
		require.NotContains(t, content, "arrived-",
			"a row inserted ahead of the anchor must not appear mid-traversal")
	}
	requireNoDuplicates(t, visited)
}

// TestCursorToleratesRetentionDeletesDuringTraversal proves retention running
// concurrently shortens the result without duplicating or stalling it.
//
// Parameters:
//   - t: the test handle.
//
// Return values: none.
func TestCursorToleratesRetentionDeletesDuringTraversal(t *testing.T) {
	cursorTestDB(t)

	now := time.Now().UTC().Unix()
	seedCursorRows(t, 60, now, "row")

	// Retention deletes from the oldest end, which is exactly where the
	// traversal is heading.
	deleted := map[string]bool{}
	visited := walkCursor(t, 6, func(page int) {
		content := fmt.Sprintf("row-%04d", page)
		require.NoError(t, LOG_DB.Exec("DELETE FROM logs WHERE content = ?", content).Error)
		deleted[content] = true
	})

	require.NotEmpty(t, deleted)
	requireNoDuplicates(t, visited)
	require.Less(t, len(visited), 60, "rows deleted before being reached are legitimately absent")

	// Whatever survived and was not deleted must still have been visited.
	var survivors []string
	require.NoError(t, LOG_DB.Raw("SELECT content FROM logs ORDER BY created_at DESC").Scan(&survivors).Error)
	seen := map[string]bool{}
	for _, content := range visited {
		seen[content] = true
	}
	for _, content := range survivors {
		if !seen[content] {
			// A survivor may legitimately be missed only if it was deleted and
			// re-created, which this test never does.
			t.Fatalf("row %q survived the traversal but was never visited", content)
		}
	}
}

// TestCursorNeverDuplicatesUnderConcurrentWriters walks the listing while
// writers insert continuously, which is the ordinary state of a busy gateway.
//
// Parameters:
//   - t: the test handle.
//
// Return values: none.
func TestCursorNeverDuplicatesUnderConcurrentWriters(t *testing.T) {
	cursorTestDB(t)

	now := time.Now().UTC().Unix()
	original := seedCursorRows(t, 80, now, "original")

	stop := make(chan struct{})
	var written atomic.Int64
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			default:
			}
			// SQLite serializes writers, so an error here is contention, not a
			// correctness signal; the traversal is what is under test.
			if err := LOG_DB.Exec(
				"INSERT INTO logs (user_id, type, created_at, token_name, content) VALUES (?,?,?,?,?)",
				1, LogTypeConsume, now+1+int64(i%10), "prod", fmt.Sprintf("concurrent-%05d", i)).Error; err == nil {
				written.Add(1)
			}
		}
	}()

	// Pin where the traversal starts. Every seeded row is older than `now` and
	// every concurrently written row is newer, so an anchor at `now` makes the
	// assertion below a statement about the cursor rather than about which of
	// two goroutines reached the database first.
	start := &LogCursorAnchor{CreatedAt: now, ID: math.MaxInt64}
	visited := walkCursorFrom(t, 9, start, nil)
	close(stop)
	wg.Wait()

	require.Positive(t, written.Load(), "writers must have been active during the walk")
	requireNoDuplicates(t, visited)
	require.Subset(t, original, visited,
		"the traversal must not return rows newer than where it started")
}

// requireNoDuplicates asserts a traversal visited each row at most once.
//
// Parameters:
//   - t: the test handle.
//   - visited: the traversal order.
//
// Return values: none.
func requireNoDuplicates(t *testing.T, visited []string) {
	t.Helper()

	seen := make(map[string]int, len(visited))
	for _, content := range visited {
		seen[content]++
	}
	for content, count := range seen {
		require.Equal(t, 1, count, "row %s was visited %d times", content, count)
	}
}
