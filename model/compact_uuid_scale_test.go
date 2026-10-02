package model

// Scale and index-size qualification for compact UUID storage (AUTO-T25, AUTO-T26).
//
// These two tests are the only place the proposal's quantitative claims are checked against a real
// engine at a real row count. Everything drives the REAL coordinator against REAL PostgreSQL 17
// and MySQL 8.4 servers; every measurement comes from the engine's own catalog
// (pg_relation_size, mysql.innodb_index_stats) or from the Go runtime (runtime.ReadMemStats).
// Nothing is emulated.
//
// Gating: COMPACT_UUID_TEST_SCALE=1 plus the per-dialect DSN variables
// (COMPACT_UUID_TEST_POSTGRES_DSN, COMPACT_UUID_TEST_MYSQL_DSN), because these load 100k+ rows
// and take minutes; they also need `go test -timeout` above the fixture deadline. CI's no-skip
// guard is what stops the qualification workflow from going green by skipping them.
//
// Fixture tiers: COMPACT_UUID_TEST_SCALE_ROWS selects the row count (default 100,000; accepted
// range 1,000..1,000,000). The proposal's absolute deadline follows the tier: fixtures up to
// 100k rows carry the 60-minute deadline and anything larger the 1m tier's four hours. The 1m
// tier is therefore launched by configuration, not by editing this file.
//
// Scope: the AUTO-T25/T26 matrix also names three runs per dialect and all 27 index pairs. Only
// `users` is populated, so only its two pairs carry fixture-sized row counts, and each test run
// is one run per dialect. None of the rest is claimed.

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

const (
	// compactScaleDefaultRows is the fixture tier AUTO-T25 runs by default; the
	// COMPACT_UUID_TEST_SCALE_ROWS environment variable selects another tier without a rebuild.
	compactScaleDefaultRows = 100000
	// compactScaleMinRows and compactScaleMaxRows bound the accepted override range. A value
	// outside the range fails the run: silently clamping would qualify a fixture nobody asked
	// for and stamp its evidence with the wrong tier.
	compactScaleMinRows = 1000
	compactScaleMaxRows = 1000000
	// compactScaleDeadline is the proposal's absolute liveness deadline for fixtures up to
	// 100k rows, measured from the moment a healthy coordinator obtains ownership.
	compactScaleDeadline = 60 * time.Minute
	// compactScaleDeadlineLarge is the proposal's absolute deadline for the 1m tier; it applies
	// to every fixture above 100k rows because the proposal names no tier between them.
	compactScaleDeadlineLarge = 4 * time.Hour
	// compactScaleCycleTimeout bounds one coordinator cycle's context. It is far above every
	// bound the coordinator applies to itself (30s row budget, 30m DDL, 2h validation are the
	// production defaults) so the context never becomes the thing under test.
	compactScaleCycleTimeout = 15 * time.Minute
	// compactScaleHeapCeiling is the concrete migration heap ceiling this test enforces, and it is
	// defensible rather than arbitrary. No query may materialize more than
	// compactMaxMaterializedRows (1,000) rows and compactBatchRows in fact yields 200, so a batch
	// (id + <=36-byte text + 16-byte value) is ~20 KB and is garbage after each iteration. Peak
	// heap must therefore be dominated by fixed process overhead — test binary, gorm schema cache,
	// pgx buffers — and must not track fixture size at all. An implementation that materialized a
	// whole 100k-row target would instead hold ~15-25 MB per target and grow linearly with the
	// table, which is exactly the failure this bound exists to catch. 128 MiB is generous for the
	// fixed overhead yet unreachable by a bounded implementation at any fixture size.
	compactScaleHeapCeiling = 128 << 20
	// compactScaleHeapSampleInterval satisfies the proposal's "sample heap at least every 100
	// milliseconds during scale qualification" control.
	compactScaleHeapSampleInterval = 100 * time.Millisecond
	// compactScaleIndexRatioLimit is AUTO-T26's ceiling: each compact index, and their
	// aggregate, must be at most 70% of its exact text comparator's bytes.
	compactScaleIndexRatioLimit = 0.70
)

// compactScaleLimitPattern captures every "LIMIT <placeholder>" a statement carries.
//
// Both dialect placeholder shapes are matched because gorm rewrites Expr placeholders into
// dialect form while building Statement.SQL: by the time a callback observes a statement the
// text reads "LIMIT $2" on PostgreSQL and "LIMIT ?" on MySQL, never the source form. Matching
// anywhere rather than only at the end is also required: the bounded NULL-backlog probe puts
// its LIMIT inside a subquery.
var compactScaleLimitPattern = regexp.MustCompile(`(?i)\bLIMIT\s+(\$\d+|\?)`)

// compactScaleGate skips unless the scale gate is set. The DSN gates are per dialect and are
// applied by compactScaleTopology, so one absent engine never hides the other.
//
// Parameters:
//   - t: test handle used for skipping.
//
// Return values: none.
func compactScaleGate(t *testing.T) {
	t.Helper()
	if strings.TrimSpace(os.Getenv("COMPACT_UUID_TEST_SCALE")) != "1" {
		t.Skip("COMPACT_UUID_TEST_SCALE=1 is not set; these tests load 100k+ rows and take minutes")
	}
}

// compactScaleFixtureRows resolves this run's fixture size and its absolute deadline.
//
// COMPACT_UUID_TEST_SCALE_ROWS selects the tier; the deadline follows the proposal's absolute
// limits (section 12): up to 100k rows must reach ready within 60 minutes, anything larger
// within four hours. A malformed or out-of-range value fails the run rather than silently
// qualifying a different tier. The choice is logged deterministically so every piece of
// evidence a run produces is attributable to its tier.
//
// Parameters:
//   - t: test handle used for assertions and the tier log line.
//
// Return values:
//   - int: fixture row count.
//   - time.Duration: absolute readiness deadline for that row count.
func compactScaleFixtureRows(t *testing.T) (int, time.Duration) {
	t.Helper()
	rows := compactScaleDefaultRows
	if raw := strings.TrimSpace(os.Getenv("COMPACT_UUID_TEST_SCALE_ROWS")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		require.NoError(t, err, "COMPACT_UUID_TEST_SCALE_ROWS must be an integer")
		require.GreaterOrEqual(t, parsed, compactScaleMinRows,
			"COMPACT_UUID_TEST_SCALE_ROWS is below the accepted range %d..%d",
			compactScaleMinRows, compactScaleMaxRows)
		require.LessOrEqual(t, parsed, compactScaleMaxRows,
			"COMPACT_UUID_TEST_SCALE_ROWS is above the accepted range %d..%d",
			compactScaleMinRows, compactScaleMaxRows)
		rows = parsed
	}
	deadline := compactScaleDeadline
	if rows > compactScaleDefaultRows {
		deadline = compactScaleDeadlineLarge
	}
	t.Logf("AUTO-T25/T26 fixture tier: rows=%d deadline=%s "+
		"(COMPACT_UUID_TEST_SCALE_ROWS, default %d, accepted range %d..%d)",
		rows, deadline, compactScaleDefaultRows, compactScaleMinRows, compactScaleMaxRows)
	return rows, deadline
}

// compactScaleTopology builds the live unified topology for one dialect, skipping when that
// dialect's DSN is not configured. The skip is per dialect so a laptop with only one engine can
// still qualify it; CI's no-skip guard is what makes a skip fail qualification.
//
// Parameters:
//   - t: test handle used for skipping and assertions.
//   - dialect: live engine descriptor from compactLiveDialects.
//
// Return values:
//   - *gorm.DB: primary handle over a clean, migrated schema.
//   - *databaseTopology: constructed unified topology.
func compactScaleTopology(t *testing.T, dialect compactLiveDialect) (*gorm.DB, *databaseTopology) {
	t.Helper()
	db, topology, ok := newLiveCompactTopology(t, dialect, false)
	if !ok {
		t.Skipf("%s is not configured; the scale tiers are a deliberate opt-in (COMPACT_UUID_TEST_SCALE=1)", dialect.primaryEnv)
	}
	return db, topology
}

// compactScaleSeedUsers builds and verifies the fixture with one bulk INSERT ... SELECT.
//
// A bulk statement is required rather than convenient: 100k+ per-row round trips would make the
// fixture build dominate the measurement. The UUID text is synthesized in SQL with exactly the
// shape compactUUIDTextFor produces for the same index, so a failure is reproducible from Go.
// Even rows get a valid distinct inviter_uuid (the previous row's UUID) and odd rows an empty
// string, so both FK derivation paths — valid text to bytes, empty text to NULL — run at scale.
// Only the row generator differs per dialect (generate_series against PostgreSQL, a recursive
// CTE against MySQL); the verification below runs identically on both.
//
// Parameters:
//   - t: test handle used for assertions.
//   - db: live handle for the dialect.
//   - dialect: live engine descriptor.
//   - rows: number of users rows to insert.
//
// Return values:
//   - time.Duration: wall clock spent building the fixture, reported as context for the run.
func compactScaleSeedUsers(t *testing.T, db *gorm.DB, dialect compactLiveDialect,
	rows int) time.Duration {
	t.Helper()
	started := time.Now()

	if dialect.name == "mysql" {
		compactScaleSeedUsersMySQL(t, db, rows)
	} else {
		compactScaleSeedUsersPostgres(t, db, rows)
	}

	var count, distinct int64
	require.NoError(t, db.Raw("SELECT COUNT(*) FROM users").Scan(&count).Error)
	require.Equal(t, int64(rows), count, "the fixture must carry exactly %d rows", rows)
	require.NoError(t, db.Raw("SELECT COUNT(DISTINCT uuid) FROM users").Scan(&distinct).Error)
	require.Equal(t, int64(rows), distinct, "every fixture row needs a distinct owned uuid")

	// Prove the SQL-synthesized text really is the shape the Go codec accepts, on a row from each
	// FK path. A fixture that quietly produced malformed text would make the coordinator block
	// and the whole measurement meaningless.
	for _, id := range []int{1, 2, rows} {
		var uuid string
		require.NoError(t, db.Raw("SELECT uuid FROM users WHERE id = ?", id).Scan(&uuid).Error)
		require.Equal(t, compactUUIDTextFor(id), strings.TrimRight(uuid, " "),
			"fixture row %d must carry the canonical uuid text", id)
	}
	return time.Since(started)
}

// compactScaleSeedUsersPostgres bulk-inserts the fixture rows with generate_series.
//
// Parameters:
//   - t: test handle used for assertions.
//   - db: live PostgreSQL handle.
//   - rows: number of users rows to insert.
//
// Return values: none.
func compactScaleSeedUsersPostgres(t *testing.T, db *gorm.DB, rows int) {
	t.Helper()
	const insert = `
INSERT INTO users (id, uuid, username, password, access_token, aff_code, inviter_id, inviter_uuid,
                   role, status, quota, used_quota, request_count, created_at, updated_at)
SELECT n,
       '018f0000-0000-7000-8000-' || lpad(to_hex(n), 12, '0'),
       'scale-user-' || n,
       'x',
       lpad(to_hex(n), 32, '0'),
       'aff-' || n,
       CASE WHEN n % 2 = 0 THEN n - 1 ELSE 0 END,
       CASE WHEN n % 2 = 0
            THEN '018f0000-0000-7000-8000-' || lpad(to_hex(n - 1), 12, '0')
            ELSE '' END,
       1, 1, 0, 0, 0, 0, 0
FROM generate_series(1, ?) AS n`

	require.NoError(t, db.Exec(insert, rows).Error, "bulk-build the %d-row users fixture", rows)
}

// compactScaleInstallBoundsRecorder registers the recorder on a live handle's callbacks.
//
// Parameters:
//   - t: test handle used for assertions.
//   - db: live handle whose statements are observed.
//
// Return values:
//   - *compactScaleBoundsRecorder: the installed, initially disabled recorder.
func compactScaleInstallBoundsRecorder(t *testing.T, db *gorm.DB) *compactScaleBoundsRecorder {
	recorder := &compactScaleBoundsRecorder{}
	observe := func(tx *gorm.DB) { recorder.record(tx) }
	require.NoError(t, db.Callback().Raw().After("gorm:raw").Register("compact_scale:bounds", observe))
	require.NoError(t, db.Callback().Row().After("gorm:row").Register("compact_scale:bounds", observe))
	return recorder
}

// record captures one executed statement's bind count and LIMIT value.
//
// Parameters:
//   - tx: the gorm handle whose Statement has just executed.
//
// Return values: none.
func (recorder *compactScaleBoundsRecorder) record(tx *gorm.DB) {
	if !recorder.enabled.Load() || tx.Statement == nil {
		return
	}
	sql := tx.Statement.SQL.String()
	binds := int64(len(tx.Statement.Vars))
	recorder.statements.Add(1)
	compactScaleRaiseMax(&recorder.maxBinds, binds)

	// Resolve each LIMIT back to the bind it names, which is exactly the row ceiling the engine
	// was asked to materialize. PostgreSQL placeholders carry their ordinal ("$2"); MySQL
	// placeholders are positional, so the ordinal is the count of markers up to and including
	// this one — sound here because coordinator SQL is built from registry identifiers and
	// carries no string literal a stray '?' could hide in. Every integer width is accepted so a
	// statement binding an int64 limit cannot slip past unobserved.
	for _, match := range compactScaleLimitPattern.FindAllStringSubmatchIndex(sql, -1) {
		token := sql[match[2]:match[3]]
		ordinal := 0
		if token == "?" {
			ordinal = strings.Count(sql[:match[2]], "?") + 1
		} else if parsed, err := strconv.Atoi(token[1:]); err == nil {
			ordinal = parsed
		}
		if ordinal < 1 || int64(ordinal) > binds {
			continue
		}
		switch limit := tx.Statement.Vars[ordinal-1].(type) {
		case int:
			compactScaleRaiseMax(&recorder.maxLimit, int64(limit))
		case int32:
			compactScaleRaiseMax(&recorder.maxLimit, int64(limit))
		case int64:
			compactScaleRaiseMax(&recorder.maxLimit, limit)
		}
	}
}

// compactScaleRun is one measured migration run.
type compactScaleRun struct {
	// elapsed is the wall clock from first ownership acquisition to compactStateReady.
	elapsed time.Duration
	// cycles counts coordinator cycles executed.
	cycles int
	// peakHeap is the highest HeapAlloc observed during the run, in bytes.
	peakHeap uint64
	// heapSamples counts heap observations taken during the run.
	heapSamples int64
	// examined sums the rows every cycle materialized and classified.
	examined int
	// updated sums the shadows every cycle wrote.
	updated int
}

// compactScaleBacklog reports each populated target's remaining NULL shadow count.
//
// It exists for the deadline-miss diagnostic: "did not reach ready" is not actionable on its own,
// and which targets still hold NULL shadows is what separates a slow migration from a stuck one.
//
// Parameters:
//   - db: live PostgreSQL handle.
//   - topology: the topology under measurement.
//
// Return values:
//   - string: bounded, value-free per-target summary.
func compactScaleBacklog(db *gorm.DB, topology *databaseTopology) string {
	parts := []string{}
	for _, target := range compactTargetsForTopology(topology) {
		if target.table != "users" {
			continue
		}
		var remaining int64
		sql := "SELECT COUNT(*) FROM users WHERE " + target.compactColumn + " IS NULL"
		if err := db.Raw(sql).Scan(&remaining).Error; err != nil {
			parts = append(parts, target.id()+"=<unreadable>")
			continue
		}
		parts = append(parts, fmt.Sprintf("%s null_shadows=%d", target.id(), remaining))
	}
	return strings.Join(parts, "; ")
}

// compactScaleDriveToReady drives the real coordinator to ready, measured and heap-sampled.
//
// The clock starts the instant a healthy coordinator first obtains ownership, exactly where the
// proposal's liveness deadline starts. Ownership is taken and released once per cycle as the
// production worker does, not held across the whole migration.
//
// Parameters:
//   - t: test handle used for assertions.
//   - db: live handle, used only for the deadline-miss diagnostic.
//   - coordinator: the real coordinator under measurement.
//   - deadline: absolute wall-clock deadline for reaching ready.
//
// Return values:
//   - compactScaleRun: the measured run.
func compactScaleDriveToReady(t *testing.T, db *gorm.DB, coordinator *compactCoordinator,
	deadline time.Duration) compactScaleRun {
	t.Helper()

	sampler := compactScaleStartHeapSampler()
	defer sampler.stopSampling()
	// The heap measurement is independent of liveness, so it is reported from a cleanup: a run
	// that never reaches ready must still surface the peak it reached.
	t.Cleanup(func() {
		t.Logf("AUTO-T25 peak heap: %.2f MiB across %d samples every %s (ceiling %.0f MiB)",
			float64(sampler.peak.Load())/(1<<20), sampler.samples.Load(),
			compactScaleHeapSampleInterval, float64(compactScaleHeapCeiling)/(1<<20))
	})

	run := compactScaleRun{}
	started := time.Time{}
	lastReport := time.Now()

	for {
		result := compactScaleRunOneCycle(t, coordinator, &started)
		run.cycles++
		run.examined += result.examined
		run.updated += result.updated

		if result.state == compactStateReady {
			run.elapsed = time.Since(started)
			break
		}
		require.NotEqual(t, compactStateBlockedValidation, result.state,
			"compact migration blocked at scale: %s", result.reason)

		if time.Since(lastReport) >= 30*time.Second {
			t.Logf("scale progress: cycle=%d elapsed=%s state=%s examined=%d updated=%d",
				run.cycles, time.Since(started).Truncate(time.Second), result.state,
				run.examined, run.updated)
			lastReport = time.Now()
		}
		if time.Since(started) > deadline {
			sampler.stopSampling()
			t.Fatalf("compact migration did not reach ready within %s: cycles=%d state=%q "+
				"reason=%q examined=%d updated=%d; remaining backlog: %s",
				deadline, run.cycles, result.state, result.reason, run.examined, run.updated,
				compactScaleBacklog(db, coordinator.topology))
		}
	}

	sampler.stopSampling()
	run.peakHeap = uint64(sampler.peak.Load())
	run.heapSamples = sampler.samples.Load()
	return run
}

// compactScaleRunOneCycle acquires ownership, runs one real cycle, and releases ownership.
//
// Parameters:
//   - t: test handle used for assertions.
//   - coordinator: the real coordinator under measurement.
//   - started: set to now on the first acquisition; the deadline clock's origin.
//
// Return values:
//   - compactCycleResult: the cycle's result.
func compactScaleRunOneCycle(t *testing.T, coordinator *compactCoordinator,
	started *time.Time) compactCycleResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(withCompactLogger(context.Background()), compactScaleCycleTimeout)
	defer cancel()
	ownership, acquired, err := acquireCompactOwnership(ctx, coordinator.topology)
	require.NoError(t, err)
	require.True(t, acquired, "a single-worker scale run must always obtain ownership")
	defer ownership.release()
	if started.IsZero() {
		*started = time.Now()
	}
	result, err := runCompactCycle(ctx, coordinator, ownership)
	require.NoError(t, err)
	return result
}

// TestCompactUUIDScale is AUTO-T25: the fixture reaches ready inside its tier's absolute
// deadline on every configured dialect, with the materialization/bind bounds and the heap
// holding at scale.
//
// Scope: one run per configured dialect at the tier COMPACT_UUID_TEST_SCALE_ROWS selects
// (default 100k). The proposal's three-runs-per-dialect repetition is OUT OF SCOPE here and is
// not claimed.
//
// Parameters:
//   - t: test handle.
//
// Return values: none.
func TestCompactUUIDScale(t *testing.T) {
	compactScaleGate(t)
	rows, deadline := compactScaleFixtureRows(t)
	for _, dialect := range compactLiveDialects() {
		t.Run(dialect.name, func(t *testing.T) {
			runCompactScaleQualification(t, dialect, rows, deadline)
		})
	}
}

// runCompactScaleQualification runs the whole AUTO-T25 protocol against one live dialect.
//
// Parameters:
//   - t: test handle.
//   - dialect: live engine descriptor.
//   - rows: fixture row count for this run.
//   - deadline: absolute readiness deadline for that row count.
//
// Return values: none.
func runCompactScaleQualification(t *testing.T, dialect compactLiveDialect, rows int,
	deadline time.Duration) {
	db, topology := compactScaleTopology(t, dialect)

	seedFor := compactScaleSeedUsers(t, db, dialect, rows)
	t.Logf("AUTO-T25 %s fixture: %d users rows built in %s (one bulk INSERT ... SELECT)",
		dialect.name, rows, seedFor.Truncate(time.Millisecond))

	// The contractual bounds, asserted statically first so a violation is attributed to the
	// formula rather than to whatever the engine happened to receive.
	for _, target := range compactRegistry() {
		batch := compactBatchRows(target)
		require.LessOrEqual(t, batch, compactMaxMaterializedRows,
			"%s batch must respect the materialization ceiling", target.id())
		require.GreaterOrEqual(t, batch, 1, "%s batch must make progress", target.id())
		// A repair statement binds the derived value plus the three recheck predicates per row.
		require.LessOrEqual(t, batch*4+2, compactMaxBinds,
			"%s batch must respect the bind ceiling", target.id())
	}

	recorder := compactScaleInstallBoundsRecorder(t, db)
	recorder.enabled.Store(true)
	// The observed bounds are independent of liveness, so they are reported from a cleanup rather
	// than lost when the deadline is what fails.
	t.Cleanup(func() {
		t.Logf("AUTO-T25 observed bounds: max binds/statement=%d (ceiling %d); max LIMIT/query=%d "+
			"(ceiling %d); statements observed=%d", recorder.maxBinds.Load(), compactMaxBinds,
			recorder.maxLimit.Load(), compactMaxMaterializedRows, recorder.statements.Load())
	})
	run := compactScaleDriveToReady(t, db, newCompactCoordinator(topology), deadline)
	recorder.enabled.Store(false)

	t.Logf("AUTO-T25 %s ready time: %s for %d rows (deadline %s, %.1f%% of budget); cycles=%d "+
		"examined=%d written=%d", dialect.name, run.elapsed.Truncate(time.Millisecond), rows,
		deadline, 100*run.elapsed.Seconds()/deadline.Seconds(),
		run.cycles, run.examined, run.updated)
	require.Less(t, run.elapsed, deadline,
		"a %d-row fixture must reach ready inside the absolute deadline", rows)
	require.Greater(t, recorder.statements.Load(), int64(0),
		"the bounds recorder must have observed the coordinator's statements")
	require.LessOrEqual(t, recorder.maxBinds.Load(), int64(compactMaxBinds),
		"no statement may exceed the bind ceiling")
	require.Greater(t, recorder.maxLimit.Load(), int64(0),
		"the coordinator must issue bounded LIMIT reads")
	require.LessOrEqual(t, recorder.maxLimit.Load(), int64(compactMaxMaterializedRows),
		"no query may materialize more than the row ceiling")
	require.Greater(t, run.heapSamples, int64(0), "the heap must actually have been sampled")
	require.LessOrEqual(t, run.peakHeap, uint64(compactScaleHeapCeiling),
		"migration heap must stay bounded and independent of fixture size")

	// The migration is only meaningful if it actually derived the fixture's shadows.
	var nullShadows, fkShadows int64
	require.NoError(t, db.Raw("SELECT COUNT(*) FROM users WHERE uuid_compact IS NULL").Scan(&nullShadows).Error)
	require.Equal(t, int64(0), nullShadows, "every owned uuid must have derived its compact bytes")
	require.NoError(t, db.Raw(
		"SELECT COUNT(*) FROM users WHERE inviter_uuid_compact IS NOT NULL").Scan(&fkShadows).Error)
	require.Equal(t, int64(rows/2), fkShadows,
		"exactly the rows with valid inviter text may carry a compact fk value")

	requireLiveShadowMatches(t, db, dialect, 1, compactUUIDTextFor(1))
	requireLiveShadowMatches(t, db, dialect, rows, compactUUIDTextFor(rows))
}

// compactScaleIndexPair is one compact index and the exact text comparator it is measured against.
type compactScaleIndexPair struct {
	// label names the pair in reported output.
	label string
	// compactIndex is the compact index's name.
	compactIndex string
	// textIndex is the comparator index's name.
	textIndex string
}
