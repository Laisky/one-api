package model

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// openMemoryDBForClose opens a throwaway in-memory SQLite handle.
//
// Parameters:
//   - t: the running test, failed when the handle cannot be opened.
//
// Return values:
//   - *gorm.DB: a usable handle that the test is expected to close through CloseDB.
func openMemoryDBForClose(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	return db
}

// requirePoolClosed asserts that the pool behind a handle is no longer usable.
//
// Parameters:
//   - t: the running test.
//   - db: the handle whose pool must already be closed.
//
// Return values: none.
func requirePoolClosed(t *testing.T, db *gorm.DB) {
	t.Helper()

	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.Error(t, sqlDB.Ping(), "pool should be closed")
}

// withCloseDBGlobals swaps the package database globals for the duration of a
// test and restores them (including the idempotency bookkeeping) afterwards.
//
// Parameters:
//   - t: the running test; the restore is registered with t.Cleanup.
//   - primary: value to install into DB.
//   - logDB: value to install into LOG_DB.
//
// Return values: none.
func withCloseDBGlobals(t *testing.T, primary, logDB *gorm.DB) {
	t.Helper()

	closeDBMu.Lock()
	origDB, origLogDB := DB, LOG_DB
	origClosedPrimary, origClosedLog := closedPrimaryDB, closedLogDB
	DB, LOG_DB = primary, logDB
	closedPrimaryDB, closedLogDB = nil, nil
	closeDBMu.Unlock()

	t.Cleanup(func() {
		closeDBMu.Lock()
		defer closeDBMu.Unlock()
		DB, LOG_DB = origDB, origLogDB
		closedPrimaryDB, closedLogDB = origClosedPrimary, origClosedLog
	})
}

// TestCloseDBIsIdempotent verifies that repeated calls close both handles once
// and never turn into a confusing error on the later calls. main.go relies on
// this: the ordered shutdown sequence closes the database, and no other exit
// path may be able to report a failure by closing it again.
func TestCloseDBIsIdempotent(t *testing.T) {
	primary := openMemoryDBForClose(t)
	logDB := openMemoryDBForClose(t)
	withCloseDBGlobals(t, primary, logDB)

	require.NoError(t, CloseDB(), "first close must succeed")
	requirePoolClosed(t, primary)
	requirePoolClosed(t, logDB)

	closeDBMu.Lock()
	require.Same(t, primary, closedPrimaryDB, "the closed primary handle must be recorded")
	require.Same(t, logDB, closedLogDB, "the closed log handle must be recorded")
	closeDBMu.Unlock()

	require.NoError(t, CloseDB(), "second close must be a no-op")
	require.NoError(t, CloseDB(), "third close must be a no-op")
}

// TestCloseDBSkipsHandlesItAlreadyClosed proves the guard actually skips a
// recorded handle instead of relying on the pool tolerating a second Close: the
// handles here fail every close attempt, so a CloseDB that did not skip them
// would report an error.
func TestCloseDBSkipsHandlesItAlreadyClosed(t *testing.T) {
	brokenPrimary := &gorm.DB{Config: &gorm.Config{}}
	brokenLog := &gorm.DB{Config: &gorm.Config{}}
	require.Error(t, closeDB(brokenPrimary), "sanity: closing this handle must fail")
	require.Error(t, closeDB(brokenLog), "sanity: closing this handle must fail")

	withCloseDBGlobals(t, brokenPrimary, brokenLog)

	closeDBMu.Lock()
	closedPrimaryDB, closedLogDB = brokenPrimary, brokenLog
	closeDBMu.Unlock()

	require.NoError(t, CloseDB(), "already-closed handles must be skipped, not closed again")
	require.NoError(t, CloseDB())
}

// TestCloseDBRetriesFailedHandle verifies a genuine close failure is reported
// and is not recorded as a successful close. A later call retries because the
// resource may still be live.
func TestCloseDBRetriesFailedHandle(t *testing.T) {
	broken := &gorm.DB{Config: &gorm.Config{}}
	withCloseDBGlobals(t, broken, nil)

	require.Error(t, CloseDB(), "a failing close must be reported")
	require.Error(t, CloseDB(), "a handle that did not close successfully must remain retryable")
}

// TestCloseDBAttemptsPrimaryAfterLogDatabaseFailure verifies one failed pool
// does not prevent the independent pool from being closed during shutdown.
//
// Parameters:
//   - t: the running test.
//
// Return values: none.
func TestCloseDBAttemptsPrimaryAfterLogDatabaseFailure(t *testing.T) {
	primary := openMemoryDBForClose(t)
	brokenLog := &gorm.DB{Config: &gorm.Config{}}
	withCloseDBGlobals(t, primary, brokenLog)

	require.Error(t, CloseDB(), "the log database close failure must be reported")
	requirePoolClosed(t, primary)

	closeDBMu.Lock()
	require.Same(t, primary, closedPrimaryDB,
		"the primary handle must be closed even when the log handle fails")
	require.Nil(t, closedLogDB,
		"a handle whose Close failed must not be recorded as successfully closed")
	closeDBMu.Unlock()
}

// TestCloseDBIsIdempotentWithSharedHandle covers the single-database topology,
// where LOG_DB and DB are the same handle and must be closed exactly once.
func TestCloseDBIsIdempotentWithSharedHandle(t *testing.T) {
	shared := openMemoryDBForClose(t)
	withCloseDBGlobals(t, shared, shared)

	require.NoError(t, CloseDB())
	requirePoolClosed(t, shared)
	require.NoError(t, CloseDB())
}

// TestCloseDBIsIdempotentWithoutLogDatabase covers an InitDB-only caller that
// never initialized the log database.
func TestCloseDBIsIdempotentWithoutLogDatabase(t *testing.T) {
	primary := openMemoryDBForClose(t)
	withCloseDBGlobals(t, primary, nil)

	require.NoError(t, CloseDB())
	requirePoolClosed(t, primary)
	require.NoError(t, CloseDB())
}

// TestCloseDBWithoutAnyDatabase verifies CloseDB is safe before (or without) any
// database bootstrap.
func TestCloseDBWithoutAnyDatabase(t *testing.T) {
	withCloseDBGlobals(t, nil, nil)

	require.NoError(t, CloseDB())
	require.NoError(t, CloseDB())
}

// TestCloseDBClosesHandlesOpenedAfterAPreviousClose proves the idempotency guard
// tracks handles rather than latching a process-wide "already closed" flag, so a
// database opened again later is still closed.
func TestCloseDBClosesHandlesOpenedAfterAPreviousClose(t *testing.T) {
	first := openMemoryDBForClose(t)
	withCloseDBGlobals(t, first, nil)

	require.NoError(t, CloseDB())
	requirePoolClosed(t, first)

	second := openMemoryDBForClose(t)
	secondLog := openMemoryDBForClose(t)

	closeDBMu.Lock()
	DB, LOG_DB = second, secondLog
	closeDBMu.Unlock()

	require.NoError(t, CloseDB())
	requirePoolClosed(t, second)
	requirePoolClosed(t, secondLog)
}

// TestCloseDBConcurrentCallsAreSafe runs the shutdown close from several
// goroutines at once; under -race this pins the serialization of the worker
// joins and the handle bookkeeping.
func TestCloseDBConcurrentCallsAreSafe(t *testing.T) {
	primary := openMemoryDBForClose(t)
	logDB := openMemoryDBForClose(t)
	withCloseDBGlobals(t, primary, logDB)

	const callers = 16

	var (
		wg    sync.WaitGroup
		mu    sync.Mutex
		errs  []error
		start = make(chan struct{})
	)
	for range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			if err := CloseDB(); err != nil {
				mu.Lock()
				errs = append(errs, err)
				mu.Unlock()
			}
		}()
	}
	close(start)
	wg.Wait()

	require.Empty(t, errs, "concurrent closes must not report failures")
	requirePoolClosed(t, primary)
	requirePoolClosed(t, logDB)
}
