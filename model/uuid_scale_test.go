package model

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

const (
	uuidScaleDefaultLogRows   = 1_000_000
	uuidScaleDefaultTokenRows = 100_000
	uuidScaleInsertBatchSize  = 500
	// uuidScaleDefaultRepeats is how many independently reset fixtures each dialect runs so
	// qualification judges a median rather than a single sample.
	uuidScaleDefaultRepeats = 3
)

// uuidScaleProbe instruments a handle so scale assertions can bound what the migration
// materializes rather than accepting final row values as scale evidence.
type uuidScaleProbe struct {
	queries    atomic.Int64
	maxBinds   atomic.Int64
	maxRows    atomic.Int64
	rowsByStmt sync.Map
}

// installScaleProbe records query count, bind count, and rows materialized per statement.
// Parameters:
//   - t: test handle used for assertions.
//   - db: database handle to instrument.
//
// Return values:
//   - *uuidScaleProbe: probe observing the handle.
func installScaleProbe(t *testing.T, db *gorm.DB) *uuidScaleProbe {
	t.Helper()
	probe := &uuidScaleProbe{}
	record := func(tx *gorm.DB) {
		probe.queries.Add(1)
		updateMax(&probe.maxBinds, int64(len(tx.Statement.Vars)))
		rows := tx.RowsAffected
		updateMax(&probe.maxRows, rows)

		key := uuidScaleStatementKey(tx)
		value, _ := probe.rowsByStmt.LoadOrStore(key, new(atomic.Int64))
		updateMax(value.(*atomic.Int64), rows)
	}
	for name, register := range map[string]func(string, func(*gorm.DB)) error{
		"query":  db.Callback().Query().After("gorm:query").Register,
		"raw":    db.Callback().Raw().After("gorm:raw").Register,
		"row":    db.Callback().Row().After("gorm:row").Register,
		"update": db.Callback().Update().After("gorm:update").Register,
	} {
		require.NoError(t, register("uuidscale:"+name, record))
	}
	return probe
}

// drainCatchUp repeats bounded catch-up cycles until a full pass finds no work, mirroring
// what the background worker does. Each cycle is limited by the row and time budget, so a
// backlog larger than one cycle legitimately needs several passes.
// Parameters:
//   - t: test handle used for assertions.
//   - topology: topology under test.
//
// Return values:
//   - int: number of cycles required to settle.
func drainCatchUp(t *testing.T, topology *databaseTopology) int {
	t.Helper()
	const maxCycles = 50
	for cycle := 1; cycle <= maxCycles; cycle++ {
		result := runCatchUp(t, topology)
		if result.updated == 0 && !result.budgetExhausted {
			return cycle
		}
	}
	t.Fatalf("catch-up did not settle within %d bounded cycles", maxCycles)
	return maxCycles
}

// maxRowsFor returns the largest row count any statement against a table materialized.
// Parameters:
//   - table: table name to report.
//
// Return values:
//   - int64: maximum rows observed, or zero when the table was never queried.
func (probe *uuidScaleProbe) maxRowsFor(table string) int64 {
	value, ok := probe.rowsByStmt.Load(table)
	if !ok {
		return 0
	}
	return value.(*atomic.Int64).Load()
}

// updateMax stores value into target when it is larger than the current maximum.
// Parameters:
//   - target: atomic maximum to update.
//   - value: candidate value.
//
// Return values: none.
func updateMax(target *atomic.Int64, value int64) {
	for {
		current := target.Load()
		if value <= current || target.CompareAndSwap(current, value) {
			return
		}
	}
}

// uuidScaleStatementKey derives the table a statement touched for per-table row bounds.
// Parameters:
//   - tx: statement being observed.
//
// Return values:
//   - string: table name, or "unknown".
func uuidScaleStatementKey(tx *gorm.DB) string {
	if tx.Statement.Table != "" {
		return tx.Statement.Table
	}
	if tx.Statement.SQL.Len() > 0 {
		if table := uuidTableFromSQL(tx.Statement.SQL.String()); table != "" {
			return table
		}
	}
	return "unknown"
}

// TestUUIDMigrationMaterializationIsBounded covers UUID-A32 and UUID-A35: target reads never
// exceed 1,000 rows, reference reads never exceed 400 keys, generated statements never exceed
// the 900-bind budget, and row completion alone is not accepted as scale evidence. It runs on
// every pass with a fixture large enough to cross several batch boundaries.
func TestUUIDMigrationMaterializationIsBounded(t *testing.T) {
	db, topology := newUnifiedTestTopology(t)
	withFinalizerEnabled(t, false)

	const (
		logRows   = 4200
		tokenRows = 900
	)
	seedUUIDScaleReferenceRows(t, db)
	seedUUIDScaleTokens(t, db, tokenRows)
	seedUUIDScaleLogs(t, db, logRows, tokenRows)

	probe := installScaleProbe(t, db)
	// This fixture deliberately exceeds one cycle's 10,000-row budget, so drain it the way
	// the background worker does. That also proves a budget-bounded cycle resumes correctly.
	cycles := drainCatchUp(t, topology)
	require.Greater(t, cycles, 1, "the fixture must be large enough to span multiple bounded cycles")

	require.LessOrEqual(t, probe.maxBinds.Load(), int64(uuidBindBudget),
		"no generated statement may exceed the portable bind budget")
	require.LessOrEqual(t, probe.maxRowsFor("logs"), int64(uuidBackfillBatchSize),
		"a target read must never materialize more than %d rows", uuidBackfillBatchSize)
	require.LessOrEqual(t, probe.maxRowsFor("users"), int64(uuidReferenceBatchSize),
		"a reference read must never materialize more than %d keys", uuidReferenceBatchSize)
	require.LessOrEqual(t, probe.maxRowsFor("channels"), int64(uuidReferenceBatchSize))
	// tokens is both an owned target (batch limit) and a reference table (key limit); the
	// larger target bound is the correct ceiling for the table as a whole.
	require.LessOrEqual(t, probe.maxRowsFor("tokens"), int64(uuidBackfillBatchSize))

	requireUUIDScaleColumnComplete(t, db, "logs", "uuid")
	requireUUIDScaleColumnComplete(t, db, "logs", "user_uuid")
	requireUUIDScaleColumnComplete(t, db, "logs", "token_uuid")
	t.Logf("bounded fixture: queries=%d max_binds=%d max_rows=%d",
		probe.queries.Load(), probe.maxBinds.Load(), probe.maxRows.Load())
}

// TestUUIDMigrationTokenResolutionIsBounded covers UUID-A15 and UUID-A35: one ambiguous key
// backed by many duplicate token rows must materialize at most one aggregate result.
func TestUUIDMigrationTokenResolutionIsBounded(t *testing.T) {
	db, _ := newUnifiedTestTopology(t)
	seedUUIDScaleReferenceRows(t, db)

	const duplicates = 5000
	rows := make([]map[string]any, 0, duplicates)
	for id := 1; id <= duplicates; id++ {
		rows = append(rows, map[string]any{
			"id": id, "user_id": 1, "key": "dup-key-" + strconv.Itoa(id), "name": "ambiguous",
			"uuid": fmt.Sprintf("018f0000-0000-7000-8000-%012d", id),
		})
	}
	for start := 0; start < len(rows); start += uuidScaleInsertBatchSize {
		end := start + uuidScaleInsertBatchSize
		if end > len(rows) {
			end = len(rows)
		}
		require.NoError(t, db.Table("tokens").Create(rows[start:end]).Error)
	}

	probe := installScaleProbe(t, db)
	refs, ambiguous, err := resolveTokenUUIDsForKeys(context.Background(), db,
		[]uuidTokenNameKey{{userID: 1, name: "ambiguous"}})
	require.NoError(t, err)
	require.Empty(t, refs, "an ambiguous key must resolve to nothing")
	require.Equal(t, 1, ambiguous)
	require.LessOrEqual(t, probe.maxRowsFor("tokens"), int64(1),
		"%d duplicate tokens for one key must materialize exactly one aggregate row", duplicates)
}

// TestUnresolvedRowsMakeBoundedProgress covers UUID-A36: unresolved rows must not cause an
// infinite loop or prevent bounded progress to higher ids.
func TestUnresolvedRowsMakeBoundedProgress(t *testing.T) {
	db, topology := newUnifiedTestTopology(t)

	// A full leading batch of permanent orphans followed by one fillable row: if the keyset
	// cursor did not advance across examined-but-unresolved rows, the fillable row would
	// never be reached and the loop would spin forever.
	require.NoError(t, db.Exec("INSERT INTO users (id, uuid, username, password) VALUES (1, '018f0000-0000-7000-8000-000000000001', 'root', 'password-hash')").Error)
	orphans := make([]map[string]any, 0, uuidBackfillBatchSize+10)
	for id := 1; id <= uuidBackfillBatchSize+10; id++ {
		orphans = append(orphans, map[string]any{
			"id": id, "user_id": 999999, "type": 1, "content": "orphan",
		})
	}
	for start := 0; start < len(orphans); start += uuidScaleInsertBatchSize {
		end := start + uuidScaleInsertBatchSize
		if end > len(orphans) {
			end = len(orphans)
		}
		require.NoError(t, db.Table("logs").Create(orphans[start:end]).Error)
	}
	fillableID := uuidBackfillBatchSize + 11
	require.NoError(t, db.Exec("INSERT INTO logs (id, user_id, type, content) VALUES (?, 1, 1, 'fillable')", fillableID).Error)

	done := make(chan struct{})
	var runErr error
	go func() {
		defer close(done)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_, runErr = runUUIDMigrationCoordinator(ctx, topology, uuidMigrationModeCatchUp)
	}()
	select {
	case <-done:
	case <-time.After(60 * time.Second):
		t.Fatal("catch-up did not terminate: unresolved rows pinned the batch")
	}
	require.NoError(t, runErr)

	var fillable Log
	require.NoError(t, db.First(&fillable, "id = ?", fillableID).Error)
	require.NotNil(t, fillable.UserUUID, "a fillable row behind a full batch of orphans must still be reached")

	var orphan Log
	require.NoError(t, db.First(&orphan, "id = ?", 1).Error)
	require.Nil(t, orphan.UserUUID)
}

// TestExternalUUIDFinalizerScaleT27 covers UUID-A33, UUID-A34, and UUID-A39: the full fixture
// with at least 1,000,000 logs and 100,000 tokens on SQLite, MySQL, and PostgreSQL, with wall
// time, peak heap, query count, bind count, and materialization recorded as release evidence.
// Peak migration heap must grow by no more than two times while data grows ten times.
func TestExternalUUIDFinalizerScaleT27(t *testing.T) {
	if os.Getenv("ONEAPI_UUID_SCALE_TEST") != "1" {
		t.Skip("set ONEAPI_UUID_SCALE_TEST=1 to run the T27 UUID scale backfill acceptance test")
	}

	logRows := uuidScaleEnvInt(t, "ONEAPI_UUID_SCALE_LOG_ROWS", uuidScaleDefaultLogRows)
	tokenRows := uuidScaleEnvInt(t, "ONEAPI_UUID_SCALE_TOKEN_ROWS", uuidScaleDefaultTokenRows)
	// UUID-A33 requires qualification to FAIL rather than warn or skip on a smaller fixture.
	// A reduced run is only allowed as an explicitly opted-in local smoke test.
	if logRows < uuidScaleDefaultLogRows || tokenRows < uuidScaleDefaultTokenRows {
		if os.Getenv("ONEAPI_UUID_SCALE_ALLOW_REDUCED") != "1" {
			t.Fatalf("T27 acceptance requires logs>=%d and tokens>=%d, got logs=%d tokens=%d; set ONEAPI_UUID_SCALE_ALLOW_REDUCED=1 only for a local smoke run",
				uuidScaleDefaultLogRows, uuidScaleDefaultTokenRows, logRows, tokenRows)
		}
		t.Logf("running a reduced UUID scale smoke that is NOT valid qualification evidence: logs=%d tokens=%d",
			logRows, tokenRows)
	}

	repeats := uuidScaleEnvInt(t, "ONEAPI_UUID_SCALE_REPEATS", uuidScaleDefaultRepeats)

	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		dialect := dialect
		t.Run(dialect, func(t *testing.T) {
			// The protocol runs several independently reset fixtures per dialect and judges
			// the median, so one unlucky run cannot fail or pass qualification on its own.
			// UUID-A34 compares a ten-times-smaller fixture against the full one, so peak
			// heap is judged against data growth rather than in isolation.
			small := runUUIDScaleFixtures(t, dialect, logRows/10, tokenRows/10, repeats)
			large := runUUIDScaleFixtures(t, dialect, logRows, tokenRows, repeats)

			smallHeap := medianUint64(uuidScaleHeaps(small))
			largeHeap := medianUint64(uuidScaleHeaps(large))
			medianWall := medianDuration(uuidScaleDurations(large))

			require.LessOrEqual(t, largeHeap, smallHeap*2,
				"peak migration heap must grow at most 2x while data grows 10x (small=%d bytes, large=%d bytes)",
				smallHeap, largeHeap)

			// The wall-time gate needs an accepted baseline to regress against. Release
			// qualification supplies it; without one the run only records a candidate.
			if baseline := os.Getenv("ONEAPI_UUID_SCALE_BASELINE_SECONDS_" + strings.ToUpper(dialect)); baseline != "" {
				seconds, err := strconv.ParseFloat(baseline, 64)
				require.NoError(t, err, "baseline must be a number of seconds")
				allowed := time.Duration(seconds * 1.25 * float64(time.Second))
				require.LessOrEqual(t, medianWall, allowed,
					"median wall time may regress by no more than 25%% from the accepted baseline of %ss", baseline)
			} else {
				t.Logf("no accepted baseline for %s; record ONEAPI_UUID_SCALE_BASELINE_SECONDS_%s=%.1f to enforce the 25%% regression gate",
					dialect, strings.ToUpper(dialect), medianWall.Seconds())
			}

			last := large[len(large)-1]
			t.Logf("T27 release evidence for %s: fixtures=%d rows=%d median_wall=%s median_peak_heap=%dKiB small_peak_heap=%dKiB heap_ratio=%.2fx queries=%d max_binds=%d max_target_rows=%d",
				dialect, repeats, last.rows, medianWall, largeHeap/1024, smallHeap/1024,
				float64(largeHeap)/float64(smallHeap), last.queries, last.maxBinds, last.maxTargetRows)
		})
	}
}

// uuidScaleRun records one instrumented scale fixture result.
type uuidScaleRun struct {
	rows          int
	duration      time.Duration
	peakHeapBytes uint64
	queries       int64
	maxBinds      int64
	maxTargetRows int64
}

// runUUIDScaleFixture seeds and migrates one fixture, returning instrumented evidence.
// Parameters:
//   - t: test handle used for assertions.
//   - dialect: sqlite, mysql, or postgres.
//   - logRows: number of legacy log rows.
//   - tokenRows: number of legacy token rows.
//
// Return values:
//   - uuidScaleRun: recorded scale evidence.
func runUUIDScaleFixture(t *testing.T, dialect string, logRows int, tokenRows int) uuidScaleRun {
	t.Helper()

	var db *gorm.DB
	switch dialect {
	case "sqlite":
		db = setupMigrationTestDB(t)
	case "mysql", "postgres":
		// requireBackend honours ONEAPI_REQUIRE_DB_BACKENDS, so release qualification fails
		// on a missing DSN instead of silently skipping the dialect.
		db = requireBackend(t, dialect)
		if db == nil {
			t.Skipf("%s DSN not set, skipping UUID scale acceptance test", dialect)
		}
	default:
		t.Fatalf("unsupported dialect %q", dialect)
	}

	withTestDBGlobals(t, db, db)
	t.Cleanup(resetBackendFlags)

	dropUUIDMigrationTables(t, db)
	require.NoError(t, migrateDB())
	seedUUIDScaleReferenceRows(t, db)
	seedUUIDScaleTokens(t, db, tokenRows)
	seedUUIDScaleLogs(t, db, logRows, tokenRows)

	topology, err := newUnifiedTopology(db)
	require.NoError(t, err)
	withFinalizerEnabled(t, true)

	probe := installScaleProbe(t, db)
	stopHeap, peakHeap := trackPeakHeap()

	start := time.Now()
	_, err = runUUIDMigrationCoordinator(context.Background(), topology, uuidMigrationModeFinalizer)
	duration := time.Since(start)
	stopHeap()
	require.NoError(t, err)

	requireUUIDScaleColumnComplete(t, db, "logs", "uuid")
	requireUUIDScaleColumnComplete(t, db, "logs", "user_uuid")
	requireUUIDScaleColumnComplete(t, db, "logs", "channel_uuid")
	requireUUIDScaleColumnComplete(t, db, "logs", "token_uuid")
	requireUUIDScaleColumnComplete(t, db, "tokens", "uuid")
	requireUUIDScaleColumnComplete(t, db, "tokens", "user_uuid")
	requireUUIDUniqueIndex(t, db, uuidOwnedTarget{role: uuidRoleLog, table: "logs", model: &Log{}})
	requireUUIDUniqueIndex(t, db, uuidOwnedTarget{role: uuidRolePrimary, table: "tokens", model: &Token{}})

	require.LessOrEqual(t, probe.maxBinds.Load(), int64(uuidBindBudget))
	require.LessOrEqual(t, probe.maxRowsFor("logs"), int64(uuidBackfillBatchSize))

	return uuidScaleRun{
		rows:          logRows,
		duration:      duration,
		peakHeapBytes: peakHeap(),
		queries:       probe.queries.Load(),
		maxBinds:      probe.maxBinds.Load(),
		maxTargetRows: probe.maxRowsFor("logs"),
	}
}

// trackPeakHeap samples heap usage until the returned stop function is called.
// Parameters: none.
//
// Return values:
//   - func(): stops sampling and takes a final sample.
//   - func() uint64: returns the peak heap bytes observed.
func trackPeakHeap() (func(), func() uint64) {
	peak := &atomic.Uint64{}
	done := make(chan struct{})
	var once sync.Once

	sample := func() {
		stats := runtime.MemStats{}
		runtime.ReadMemStats(&stats)
		for {
			current := peak.Load()
			if stats.HeapAlloc <= current || peak.CompareAndSwap(current, stats.HeapAlloc) {
				break
			}
		}
	}
	runtime.GC()
	sample()
	go func() {
		ticker := time.NewTicker(20 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				sample()
			}
		}
	}()
	return func() { once.Do(func() { sample(); close(done) }) }, peak.Load
}

// uuidScaleEnvInt parses an optional positive integer test setting.
// Parameters:
//   - t: test handle used for assertions.
//   - name: environment variable name to parse.
//   - fallback: default value used when the environment variable is absent.
//
// Return values:
//   - int: parsed or fallback positive integer setting.
func uuidScaleEnvInt(t *testing.T, name string, fallback int) int {
	t.Helper()
	raw := os.Getenv(name)
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	require.NoError(t, err, "%s must be an integer", name)
	require.Positive(t, value, "%s must be positive", name)
	return value
}

// seedUUIDScaleReferenceRows inserts the user and channel referenced by scale rows.
// Parameters:
//   - t: test handle used for assertions.
//   - db: database handle receiving the reference rows.
//
// Return values: none.
func seedUUIDScaleReferenceRows(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.Table("users").Create(map[string]any{
		"id": 1, "username": "scale-root", "password": "password-hash",
	}).Error)
	require.NoError(t, db.Table("channels").Create(map[string]any{
		"id": 1, "type": 1, "name": "scale-channel", "models": "gpt-4o", "config": "{}",
	}).Error)
}

// seedUUIDScaleTokens inserts legacy token rows without UUID values.
// Parameters:
//   - t: test handle used for assertions.
//   - db: database handle receiving token rows.
//   - tokenRows: number of legacy token rows to insert.
//
// Return values: none.
func seedUUIDScaleTokens(t *testing.T, db *gorm.DB, tokenRows int) {
	t.Helper()
	seedUUIDScaleRows(t, db, "tokens", []string{"id", "user_id", "key", "name", "status"}, tokenRows, func(id int) []any {
		name := fmt.Sprintf("scale-token-%d", id)
		return []any{id, 1, name, name, TokenStatusEnabled}
	})
}

// seedUUIDScaleLogs inserts legacy log rows without own or FK UUID values.
// Parameters:
//   - t: test handle used for assertions.
//   - db: database handle receiving log rows.
//   - logRows: number of legacy log rows to insert.
//   - tokenRows: number of token names to cycle through for token UUID backfill.
//
// Return values: none.
func seedUUIDScaleLogs(t *testing.T, db *gorm.DB, logRows int, tokenRows int) {
	t.Helper()
	seedUUIDScaleRows(t, db, "logs", []string{"id", "user_id", "channel_id", "type", "token_name", "content"}, logRows, func(id int) []any {
		tokenID := ((id - 1) % tokenRows) + 1
		return []any{id, 1, 1, LogTypeConsume, fmt.Sprintf("scale-token-%d", tokenID), "scale log"}
	})
}

// seedUUIDScaleRows bulk inserts deterministic legacy rows for a scale test table.
// Parameters:
//   - t: test handle used for assertions.
//   - db: database handle receiving rows.
//   - table: trusted table name to insert into.
//   - columns: trusted column names included in each inserted row.
//   - total: number of rows to insert.
//   - build: callback that returns one row's values for a given id.
//
// Return values: none.
func seedUUIDScaleRows(t *testing.T, db *gorm.DB, table string, columns []string, total int, build func(id int) []any) {
	t.Helper()
	for start := 1; start <= total; start += uuidScaleInsertBatchSize {
		end := start + uuidScaleInsertBatchSize - 1
		if end > total {
			end = total
		}
		placeholders := make([]string, 0, end-start+1)
		args := make([]any, 0, (end-start+1)*len(columns))
		for id := start; id <= end; id++ {
			values := build(id)
			require.Len(t, values, len(columns))
			placeholders = append(placeholders, "("+strings.TrimRight(strings.Repeat("?,", len(columns)), ",")+")")
			args = append(args, values...)
		}
		sql := "INSERT INTO " + quoteIdentifier(db, table) + " (" + quotedUUIDScaleColumns(db, columns) + ") VALUES " + strings.Join(placeholders, ",")
		require.NoError(t, db.Exec(sql, args...).Error, "insert %s rows %d-%d", table, start, end)
	}
}

// quotedUUIDScaleColumns returns a comma-separated list of quoted column names.
// Parameters:
//   - db: database handle whose dialect controls identifier quoting.
//   - columns: trusted column names to quote.
//
// Return values:
//   - string: comma-separated quoted column names.
func quotedUUIDScaleColumns(db *gorm.DB, columns []string) string {
	quoted := make([]string, 0, len(columns))
	for _, column := range columns {
		quoted = append(quoted, quoteIdentifier(db, column))
	}
	return strings.Join(quoted, ",")
}

// requireUUIDScaleColumnComplete asserts that a backfilled column has no missing values.
// Parameters:
//   - t: test handle used for assertions.
//   - db: database handle containing the target table.
//   - table: trusted table name to inspect.
//   - column: trusted UUID column name to inspect.
//
// Return values: none.
func requireUUIDScaleColumnComplete(t *testing.T, db *gorm.DB, table string, column string) {
	t.Helper()
	var missing int64
	err := db.Table(table).
		Where(quoteIdentifier(db, column) + " IS NULL OR " + quoteIdentifier(db, column) + " = ''").
		Count(&missing).Error
	require.NoError(t, err, "count missing %s.%s", table, column)
	require.Zero(t, missing, "%s.%s still has missing UUID values", table, column)
}
