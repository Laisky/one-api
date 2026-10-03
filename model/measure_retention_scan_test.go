package model

// MEASUREMENT (not correctness) for W0.8 -- proposal
// docs/proposals/20260905_observability-data-tiering.md, Phase 0 / W0:
//
//	"Also bound the work to locate candidates, not merely deleted rows;
//	 async-task predicates need their own plan."
//
// The correctness proof that the two sargable passes select exactly the row set
// the old CASE expression selected is TestAsyncTaskRetentionPredicateMatchesLegacy;
// the keyset walk's completeness is TestChunkedDeleteKeysetDeletesEveryEligibleRow.
// This file only measures how much work each shape performs.
//
// HOW THE "BEFORE" ARM IS RECONSTRUCTED
//
// Pre-remediation, CleanExpiredAsyncTaskBindings called ChunkedDelete with the
// CASE predicate, and ChunkedDelete's whole body was:
//
//	statement := boundedDeleteStatement(db, table, where, batchSize)
//	for { tx := db.Exec(statement, args...); ...; if tx.RowsAffected < batchSize { return } }
//
// legacyChunkedSweep below is that loop verbatim, built from the same
// boundedDeleteStatement the shipped code still uses, so the arms differ only in
// predicate shape and candidate location -- not in dialect handling.
//
// Run it with:
//
//	ONEAPI_BENCH_PG_DSN=... ONEAPI_BENCH_MYSQL_DSN=... \
//	go test ./model/ -run TestMeasureRetentionCandidateScan -v -timeout 60m
//
// Without the DSN variables it measures SQLite only, so an ordinary
// `go test ./...` never needs a container.

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	glogger "gorm.io/gorm/logger"

	"github.com/Laisky/one-api/common/benchdb"
	"github.com/Laisky/one-api/common/config"
)

// retentionMeasurementRows is the fixture size. It matches the order of
// magnitude of the 205000-row fixture the Phase 0/1 record used, so the two
// records are comparable.
const retentionMeasurementRows = 200000

// statementCountingLogger counts every statement a handle issues.
//
// gorm reports one Trace call per statement, so this is an exact statement
// count rather than a sample.
type statementCountingLogger struct {
	glogger.Interface

	// count is the number of statements observed since the last reset.
	count int
	// deleteRows is the total RowsAffected across observed DELETE statements.
	deleteRows int64
}

// Trace implements gorm's statement hook.
//
// Parameters:
//   - ctx: the statement context.
//   - begin: when the statement began.
//   - fc: lazily supplies SQL text and affected rows.
//   - err: the statement result.
//
// Return values: none.
func (l *statementCountingLogger) Trace(ctx context.Context, begin time.Time,
	fc func() (string, int64), err error) {
	sql, affected := fc()
	l.count++
	if strings.HasPrefix(strings.ToUpper(strings.TrimSpace(sql)), "DELETE") {
		l.deleteRows += affected
	}
}

// reset zeroes the counters between arms.
//
// Parameters: none.
//
// Return values: none.
func (l *statementCountingLogger) reset() { l.count, l.deleteRows = 0, 0 }

// legacyChunkedSweep is the pre-remediation ChunkedDelete loop.
//
// Parameters:
//   - ctx: cancellation scope.
//   - db: the handle owning the table.
//   - table: the physical table name.
//   - where: the predicate.
//   - args: the predicate arguments.
//   - batchSize: rows removed per statement.
//
// Return values:
//   - int64: rows removed.
//   - int: statements issued.
//   - error: the failure that stopped the sweep.
func legacyChunkedSweep(ctx context.Context, db *gorm.DB, table, where string,
	args []any, batchSize int) (int64, int, error) {
	statement := boundedDeleteStatement(db, table, where, batchSize)

	var (
		total  int64
		chunks int
	)
	for {
		if err := ctx.Err(); err != nil {
			return total, chunks, err
		}
		tx := db.WithContext(ctx).Exec(statement, args...)
		if tx.Error != nil {
			return total, chunks, tx.Error
		}
		chunks++
		total += tx.RowsAffected
		if tx.RowsAffected < int64(batchSize) {
			return total, chunks, nil
		}
	}
}

// rowsExaminedProbe reads an engine's rows-examined counter.
//
// Every engine reports this differently and SQLite does not report it at all,
// which is stated in the result rather than papered over.
type rowsExaminedProbe struct {
	// engine is the target engine.
	engine benchdb.Engine
	// db is the handle to probe. It must be pinned to a single connection for
	// MySQL, whose counters are per session.
	db *gorm.DB
	// supported reports whether the engine exposes a counter at all.
	supported bool
	// source names the counter, for the report.
	source string
}

// newRowsExaminedProbe builds the probe for an engine.
//
// Parameters:
//   - t: the test, used to fail fast.
//   - engine: the target engine.
//   - db: the handle to probe.
//
// Return values:
//   - *rowsExaminedProbe: the probe.
func newRowsExaminedProbe(t *testing.T, engine benchdb.Engine, db *gorm.DB) *rowsExaminedProbe {
	t.Helper()
	switch engine {
	case benchdb.EngineMySQL:
		return &rowsExaminedProbe{engine: engine, db: db, supported: true,
			source: "SHOW SESSION STATUS LIKE 'Handler_read%' (sum of all Handler_read_* counters)"}
	case benchdb.EnginePostgres:
		return &rowsExaminedProbe{engine: engine, db: db, supported: true,
			source: "pg_stat_user_tables.seq_tup_read + idx_tup_fetch, flushed with pg_stat_force_next_flush()"}
	default:
		return &rowsExaminedProbe{engine: engine, db: db, supported: false,
			source: "not exposed by SQLite; see the EXPLAIN QUERY PLAN section instead"}
	}
}

// read returns the current rows-examined counter.
//
// Parameters:
//   - t: the test, used to fail fast.
//
// Return values:
//   - int64: the counter value; 0 when the engine exposes none.
func (p *rowsExaminedProbe) read(t *testing.T) int64 {
	t.Helper()
	if !p.supported {
		return 0
	}

	switch p.engine {
	case benchdb.EngineMySQL:
		var rows []struct {
			VariableName string `gorm:"column:Variable_name"`
			Value        string `gorm:"column:Value"`
		}
		require.NoError(t, p.db.Raw("SHOW SESSION STATUS LIKE 'Handler_read%'").Scan(&rows).Error)
		var total int64
		for _, row := range rows {
			var v int64
			_, _ = fmt.Sscanf(row.Value, "%d", &v)
			total += v
		}
		return total
	case benchdb.EnginePostgres:
		require.NoError(t, p.db.Exec("SELECT pg_stat_force_next_flush()").Error)
		var total []int64
		require.NoError(t, p.db.Raw(
			"SELECT COALESCE(seq_tup_read,0) + COALESCE(idx_tup_fetch,0) FROM pg_stat_user_tables "+
				"WHERE relname = 'async_task_bindings'").Scan(&total).Error)
		if len(total) == 0 {
			return 0
		}
		return total[0]
	}
	return 0
}

// retentionMix describes one distribution of eligible rows in the fixture.
//
// The distribution is the whole measurement. A sweep whose eligible rows are the
// oldest rows -- correlated with the created_at index order -- is served well by
// any shape, because a scan in index order finds them immediately. The shape
// W0.8 exists for is the one an async-task table actually has in steady state:
// last_accessed_at is stamped on every poll, so a binding created yesterday and
// abandoned an hour later ages out while sitting late in created_at order, and
// eligibility is therefore uncorrelated with the column the old predicate could
// be scanned on.
type retentionMix struct {
	// name identifies the mix in the report.
	name string
	// eligiblePerThousand is how many rows in each thousand are eligible.
	eligiblePerThousand int
	// scattered spreads eligible rows uniformly through created_at order
	// instead of clustering them at the old end of it.
	scattered bool
}

// seedAsyncTaskRetentionCorpus writes the measurement fixture.
//
// Parameters:
//   - t: the test, used to fail fast.
//   - db: the handle to seed.
//   - cutoff: the retention cutoff, in Unix milliseconds.
//   - rows: how many rows to write.
//   - mix: the eligibility distribution.
//
// Return values:
//   - int: how many rows the retention predicate selects.
func seedAsyncTaskRetentionCorpus(t *testing.T, db *gorm.DB, cutoff int64, rows int,
	mix retentionMix) int {
	t.Helper()

	const perStatement = 1000
	expired := 0
	for start := 0; start < rows; start += perStatement {
		end := min(start+perStatement, rows)

		var (
			placeholders []string
			args         []any
		)
		for i := start; i < end; i++ {
			eligible := i%1000 < mix.eligiblePerThousand

			var createdAt, lastAccessed int64
			switch {
			case mix.scattered:
				// created_at spans one window for every row, so an index scan in
				// created_at order cannot reach the eligible rows early.
				createdAt = cutoff - int64(i%1_000_000) - 1
				if eligible {
					// Aged out by last access, whatever created_at says.
					lastAccessed = cutoff - int64(i%100_000) - 1
					expired++
				} else {
					lastAccessed = cutoff + 60_000 + int64(i%100_000)
				}
			case eligible:
				createdAt = cutoff - int64(i%1_000_000) - 1
				if i%3 == 0 {
					lastAccessed = 0 // never accessed: pass 2's row set
				} else {
					lastAccessed = cutoff - int64(i%500_000) - 1
				}
				expired++
			default:
				createdAt = cutoff + 60_000 + int64(i%100_000)
				lastAccessed = createdAt
			}

			placeholders = append(placeholders, "(?, ?, ?, ?, ?, ?, ?, ?)")
			args = append(args, fmt.Sprintf("measure-%08d", i), "video", 1+i%97, 2, 3,
				createdAt, createdAt, lastAccessed)
		}

		require.NoError(t, db.Exec(
			"INSERT INTO async_task_bindings "+
				"(task_id, task_type, user_id, channel_id, channel_type, created_at, updated_at, last_accessed_at) "+
				"VALUES "+strings.Join(placeholders, ","), args...).Error)
	}
	return expired
}

// analyzeRetentionTable refreshes planner statistics so a plan is taken against
// a table the engine actually understands.
//
// Parameters:
//   - t: the test, used to fail fast.
//   - engine: the target engine.
//   - db: the handle to analyze.
//
// Return values: none.
func analyzeRetentionTable(t *testing.T, engine benchdb.Engine, db *gorm.DB) {
	t.Helper()
	switch engine {
	case benchdb.EnginePostgres:
		require.NoError(t, db.Exec("VACUUM ANALYZE async_task_bindings").Error)
	case benchdb.EngineMySQL:
		require.NoError(t, db.Exec("ANALYZE TABLE async_task_bindings").Error)
	default:
		require.NoError(t, db.Exec("ANALYZE").Error)
	}
}

// explainRetentionPredicate renders the engine's plan for one predicate.
//
// Parameters:
//   - t: the test, used to fail fast.
//   - engine: the target engine.
//   - db: the handle to explain against.
//   - where: the predicate.
//   - cutoff: the predicate argument.
//
// Return values:
//   - string: the plan text, newline-joined.
func explainRetentionPredicate(t *testing.T, engine benchdb.Engine, db *gorm.DB,
	where string, cutoff int64) string {
	t.Helper()

	query := "SELECT last_accessed_at, created_at FROM async_task_bindings WHERE " + where +
		" ORDER BY created_at ASC LIMIT 5000"

	var prefix string
	switch engine {
	case benchdb.EnginePostgres:
		prefix = "EXPLAIN (ANALYZE, BUFFERS, COSTS OFF) "
	case benchdb.EngineMySQL:
		prefix = "EXPLAIN ANALYZE "
	default:
		prefix = "EXPLAIN QUERY PLAN "
	}

	rows, err := db.Raw(prefix+query, cutoff).Rows()
	require.NoError(t, err)
	defer rows.Close() //nolint:errcheck // read-only cursor

	cols, err := rows.Columns()
	require.NoError(t, err)

	var out []string
	for rows.Next() {
		cells := make([]any, len(cols))
		holders := make([]any, len(cols))
		for i := range cells {
			holders[i] = &cells[i]
		}
		require.NoError(t, rows.Scan(holders...))

		var parts []string
		for _, cell := range cells {
			switch v := cell.(type) {
			case nil:
			case []byte:
				parts = append(parts, string(v))
			case string:
				parts = append(parts, v)
			default:
				parts = append(parts, fmt.Sprintf("%v", v))
			}
		}
		out = append(out, strings.Join(parts, " | "))
	}
	require.NoError(t, rows.Err())
	return strings.Join(out, "\n")
}

// TestMeasureRetentionCandidateScan reports the work each async-task retention
// shape performs over an identical seeded table, for two eligibility
// distributions.
// requireMeasurementRun skips a measurement whose cost makes it unsuitable for
// the ordinary pre-merge suite.
//
// The repository convention is that long fixtures are opt-in (see
// ONEAPI_W24_PLANS in model/log_cursor_plan_test.go), so the scenario stays in
// the repository -- the proposal forbids ad-hoc one-off scripts -- without
// putting a multi-gigabyte allocation or a multi-second storm on every
// `go test ./...`.
//
// Parameters:
//   - t: the test to skip.
//
// Return values: none.
func requireMeasurementRun(t *testing.T) {
	t.Helper()
	if os.Getenv("ONEAPI_MEASURE") != "1" {
		t.Skip("set ONEAPI_MEASURE=1 to run this measurement")
	}
}

func TestMeasureRetentionCandidateScan(t *testing.T) {
	requireMeasurementRun(t)
	mixes := []retentionMix{
		{name: "backlog sweep (750/1000 eligible, eligibility correlated with created_at)",
			eligiblePerThousand: 750},
		{name: "steady state (100/1000 eligible, eligibility uncorrelated with created_at)",
			eligiblePerThousand: 100, scattered: true},
	}

	for _, target := range benchdb.Targets() {
		t.Run(string(target.Engine), func(t *testing.T) {
			db, restore := benchdb.Open(t, target)
			t.Cleanup(restore)

			// MySQL's Handler_read_* counters are per session and PostgreSQL's
			// plan stability benefits from a single backend, so the whole
			// measurement runs on one connection.
			sqlDB, err := db.DB()
			require.NoError(t, err)
			sqlDB.SetMaxOpenConns(1)
			sqlDB.SetMaxIdleConns(1)

			counter := &statementCountingLogger{Interface: glogger.Discard}
			db.Logger = counter

			prevDB := DB
			DB = db
			t.Cleanup(func() { DB = prevDB })

			// Pause 0 so wall time reflects work rather than the configured
			// yield; the production default is 10 ms per chunk.
			prevPause, prevBatch := config.RetentionDeletePauseMs, config.RetentionDeleteBatchSize
			config.RetentionDeletePauseMs = 0
			t.Cleanup(func() {
				config.RetentionDeletePauseMs, config.RetentionDeleteBatchSize = prevPause, prevBatch
			})

			const retentionDays = 7
			probe := newRowsExaminedProbe(t, target.Engine, db)
			t.Logf("engine=%s rows_examined_source=%q batch_size=%d",
				target.Engine, probe.source, config.RetentionDeleteBatchSize)

			for _, mix := range mixes {
				t.Run(mix.name, func(t *testing.T) {
					cutoff := time.Now().UTC().Add(-retentionDays * 24 * time.Hour).UnixMilli()

					reseed := func() int {
						require.NoError(t, db.Exec("DROP TABLE IF EXISTS async_task_bindings").Error)
						require.NoError(t, db.AutoMigrate(&AsyncTaskBinding{}))
						n := seedAsyncTaskRetentionCorpus(t, db, cutoff, retentionMeasurementRows, mix)
						analyzeRetentionTable(t, target.Engine, db)
						return n
					}

					expected := reseed()
					t.Logf("engine=%s mix=%q seeded_rows=%d eligible_rows=%d",
						target.Engine, mix.name, retentionMeasurementRows, expected)

					t.Logf("engine=%s mix=%q predicate=%q plan:\n%s", target.Engine, mix.name,
						asyncTaskLegacyRetentionPredicate,
						explainRetentionPredicate(t, target.Engine, db,
							asyncTaskLegacyRetentionPredicate, cutoff))
					t.Logf("engine=%s mix=%q predicate=%q plan:\n%s", target.Engine, mix.name,
						asyncTaskTouchedPredicate,
						explainRetentionPredicate(t, target.Engine, db,
							asyncTaskTouchedPredicate, cutoff))
					t.Logf("engine=%s mix=%q predicate=%q plan:\n%s", target.Engine, mix.name,
						asyncTaskUntouchedPredicate,
						explainRetentionPredicate(t, target.Engine, db,
							asyncTaskUntouchedPredicate, cutoff))

					// ---- arm: pre-remediation CASE predicate, no keyset ----
					counter.reset()
					examinedBefore := probe.read(t)
					start := time.Now()
					deleted, chunks, err := legacyChunkedSweep(context.Background(), db,
						"async_task_bindings", asyncTaskLegacyRetentionPredicate,
						[]any{cutoff}, config.RetentionDeleteBatchSize)
					legacyWall := time.Since(start)
					require.NoError(t, err)
					legacyExamined := probe.read(t) - examinedBefore
					legacyStatements := counter.count

					t.Logf("engine=%s mix=%q arm=%q deleted=%d chunks=%d statements=%d "+
						"rows_examined=%d wall=%s",
						target.Engine, mix.name, "before: CASE predicate, restart-from-start chunks",
						deleted, chunks, legacyStatements, legacyExamined, legacyWall)
					require.EqualValues(t, expected, deleted,
						"the before arm must remove every eligible row")

					// ---- arm: shipped two sargable passes with keyset watermark ----
					expected = reseed()
					counter.reset()
					examinedBefore = probe.read(t)
					start = time.Now()
					stats, err := CleanExpiredAsyncTaskBindingsStats(context.Background(), retentionDays)
					keysetWall := time.Since(start)
					require.NoError(t, err)
					keysetExamined := probe.read(t) - examinedBefore

					t.Logf("engine=%s mix=%q arm=%q deleted=%d chunks=%d passes=%d "+
						"candidates_located=%d backlog=%d statements=%d rows_examined=%d wall=%s",
						target.Engine, mix.name, "after: two sargable passes with keyset watermark",
						stats.Deleted, stats.Chunks, stats.Passes, stats.Candidates, stats.Backlog,
						counter.count, keysetExamined, keysetWall)
					require.EqualValues(t, expected, stats.Deleted,
						"the after arm must remove every eligible row")

					if probe.supported && legacyExamined > 0 && keysetExamined > 0 {
						t.Logf("engine=%s mix=%q rows_examined_ratio=%.2fx (before/after) wall_ratio=%.2fx",
							target.Engine, mix.name,
							float64(legacyExamined)/float64(keysetExamined),
							float64(legacyWall)/float64(keysetWall))
					}
				})
			}
		})
	}
}
