package model

// Run, outcome, and reconciliation machinery for the section 12 compatibility workload
// (docs/proposals/20260715_compact-uuid-storage.md). The per-client operation machinery lives
// in compact_uuid_workload_harness_test.go; this file owns the run that drives the eight
// clients, the outcome it reduces to, acknowledged-write reconciliation, and the category-by-
// category comparison against the migration-disabled baseline. Split from the harness file
// only for the 600-line ceiling (proposal section 9.3).

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// compactWorkloadRun drives the eight deterministic clients against one database.
type compactWorkloadRun struct {
	// db is the run's database handle.
	db *gorm.DB
	// schedule is the deterministic shape shared with the paired run.
	schedule compactWorkloadSchedule
	// pace is the applied per-operation sleep: the schedule's for the held migrated run,
	// zero for the baseline replay, whose semantics do not depend on timing.
	pace time.Duration
	// tables are the owned writer targets.
	tables []string
	// targets are the owned lookup targets keyed by table.
	targets map[string]compactTarget
	// workers are the eight deterministic clients.
	workers []*compactWorkloadWorker
}

// newCompactWorkloadRun builds one run over a prepared database.
// Parameters:
//   - t: test handle used for assertions.
//   - db: prepared database handle.
//   - schedule: deterministic shape shared by both runs.
//   - paced: true to stretch each segment across the schedule's hold duration.
//
// Return values:
//   - *compactWorkloadRun: the initialized run.
func newCompactWorkloadRun(t *testing.T, db *gorm.DB, schedule compactWorkloadSchedule,
	paced bool) *compactWorkloadRun {
	t.Helper()
	require.Same(t, DB, db,
		"the cache category reads the package-global DB handle, so it must point at this run's database")
	run := &compactWorkloadRun{db: db, schedule: schedule,
		tables: compactWorkloadTables(), targets: map[string]compactTarget{}}
	require.Len(t, run.tables, 12, "the owned registry must carry every writer target")
	for _, table := range run.tables {
		target, err := compactLookupTarget(table)
		require.NoError(t, err, "owned lookup target for %s", table)
		run.targets[table] = target
	}
	if paced {
		run.pace = schedule.pace
	}
	for index := 0; index < compactWorkloadWorkers; index++ {
		run.workers = append(run.workers, newCompactWorkloadWorker(index, run.tables))
	}
	return run
}

// runSegment executes one segment concurrently across all eight clients and proves coverage.
// Parameters:
//   - t: test handle used for assertions.
//   - ctx: context bounding the segment.
//   - segment: zero-based segment index; segment 0 bootstraps one row per (worker, table).
//
// Return values:
//   - int64: successful operations executed in this segment.
//   - time.Duration: the segment's elapsed wall clock.
func (run *compactWorkloadRun) runSegment(t *testing.T, ctx context.Context, segment int) (int64, time.Duration) {
	t.Helper()
	for _, worker := range run.workers {
		worker.resetCoverage()
	}
	failures := make([]error, len(run.workers))
	started := time.Now()
	var wg sync.WaitGroup
	for slot, worker := range run.workers {
		wg.Add(1)
		go func(slot int, worker *compactWorkloadWorker) {
			defer wg.Done()
			failures[slot] = worker.runSegment(ctx, run, segment == 0)
		}(slot, worker)
	}
	wg.Wait()
	elapsed := time.Since(started)
	for slot, err := range failures {
		require.NoError(t, err, "worker %d must complete segment %d without any unexpected error", slot, segment)
	}
	run.assertCoverage(t, segment)
	ops := int64(len(run.workers)) * int64(run.schedule.opsPerWorker)
	if segment == 0 {
		ops += int64(len(run.workers) * len(run.tables))
	}
	return ops, elapsed
}

// assertCoverage proves one segment covered every writer target, every owned type, every search
// table, both report tables, and the cache path.
// Parameters:
//   - t: test handle used for assertions.
//   - segment: segment index, named in failures.
//
// Return values: none.
func (run *compactWorkloadRun) assertCoverage(t *testing.T, segment int) {
	t.Helper()
	writes, reads := map[string]int{}, map[string]int{}
	searches, reports, cache := map[string]int{}, map[string]int{}, 0
	for _, worker := range run.workers {
		for table, count := range worker.segWrites {
			writes[table] += count
		}
		for table, count := range worker.segReads {
			reads[table] += count
		}
		for table, count := range worker.segSearches {
			searches[table] += count
		}
		for table, count := range worker.segReports {
			reports[table] += count
		}
		cache += worker.segCache
	}
	for _, table := range run.tables {
		require.Positive(t, writes[table], "segment %d must write writer target %s", segment, table)
		require.Positive(t, reads[table], "segment %d must exact-read owned type %s", segment, table)
	}
	for _, table := range compactWorkloadSearchTables() {
		require.Positive(t, searches[table], "segment %d must search %s", segment, table)
	}
	for _, table := range []string{"users", "logs"} {
		require.Positive(t, reports[table], "segment %d must report over %s", segment, table)
	}
	require.Positive(t, cache, "segment %d must exercise the cache path", segment)
}

// compactWorkloadOutcome is one run's compared result: per-category success counts and outcome
// digests, the acknowledged-write ledger, and the full-table users ordering digest.
type compactWorkloadOutcome struct {
	// totalOps is the run's total successful operations.
	totalOps int64
	// counts are per-category success counts.
	counts map[compactWorkloadCategory]int64
	// digests are per-category outcome digests, combined across workers in slot order.
	digests map[compactWorkloadCategory]string
	// acked maps every table to its acknowledged rows and their exact final text.
	acked map[string]map[int]string
	// usersDigest is the sha256 of SELECT id, uuid FROM users ORDER BY id.
	usersDigest string
}

// finish reconciles every acknowledged write against the database and folds the run's outcome.
// Parameters:
//   - t: test handle used for assertions.
//
// Return values:
//   - compactWorkloadOutcome: the run's comparable outcome.
func (run *compactWorkloadRun) finish(t *testing.T) compactWorkloadOutcome {
	t.Helper()
	outcome := compactWorkloadOutcome{
		counts:  map[compactWorkloadCategory]int64{},
		digests: map[compactWorkloadCategory]string{},
		acked:   map[string]map[int]string{},
	}
	for _, category := range compactWorkloadCategories() {
		combined := sha256.New()
		for _, worker := range run.workers {
			outcome.counts[category] += worker.counts[category]
			combined.Write(worker.digests[category].Sum(nil))
		}
		outcome.totalOps += outcome.counts[category]
		outcome.digests[category] = hex.EncodeToString(combined.Sum(nil))
	}
	for _, table := range run.tables {
		acked := map[int]string{}
		for _, worker := range run.workers {
			for id, text := range worker.current[table] {
				acked[id] = text
			}
		}
		outcome.acked[table] = acked
		run.reconcile(t, table, acked)
	}
	outcome.usersDigest = run.usersOrderingDigest(t)
	return outcome
}

// reconcile proves every acknowledged write exists exactly once with its exact text, and that
// no unacknowledged row exists in the workload's private id range.
// Parameters:
//   - t: test handle used for assertions.
//   - table: table to reconcile.
//   - acked: the merged acknowledged-write ledger for the table.
//
// Return values: none.
func (run *compactWorkloadRun) reconcile(t *testing.T, table string, acked map[int]string) {
	t.Helper()
	rows := []struct {
		ID   int    `gorm:"column:id"`
		UUID string `gorm:"column:uuid"`
	}{}
	require.NoError(t, run.db.Raw("SELECT id, rtrim(uuid) AS uuid FROM "+table+
		" WHERE id >= ? ORDER BY id", compactWorkloadIDBase).Scan(&rows).Error)
	require.Len(t, rows, len(acked), "every acknowledged %s write must exist exactly once", table)
	for _, row := range rows {
		expected, ok := acked[row.ID]
		require.True(t, ok, "%s row %d exists but was never acknowledged", table, row.ID)
		require.Equal(t, expected, row.UUID, "%s row %d lost its acknowledged text", table, row.ID)
	}
}

// usersOrderingDigest fingerprints the ENTIRE users table — fixture plus workload rows — in
// primary-key order, which is Section 12's ordering comparison.
// Parameters:
//   - t: test handle used for assertions.
//
// Return values:
//   - string: hex sha256 over every row's id and authoritative text, in id order.
func (run *compactWorkloadRun) usersOrderingDigest(t *testing.T) string {
	t.Helper()
	rows := []struct {
		ID   int    `gorm:"column:id"`
		UUID string `gorm:"column:uuid"`
	}{}
	require.NoError(t, run.db.Raw("SELECT id, rtrim(uuid) AS uuid FROM users ORDER BY id").Scan(&rows).Error)
	digest := sha256.New()
	for _, row := range rows {
		fmt.Fprintf(digest, "%d:%s\n", row.ID, row.UUID)
	}
	return hex.EncodeToString(digest.Sum(nil))
}

// compareCompactWorkloadOutcomes asserts the migrated run's outcomes equal the baseline's:
// status (success counts), payload (per-category digests), row counts (inside the search and
// report digests), ordering (the users full-table digest), and acknowledged-write ledgers.
// Parameters:
//   - t: test handle used for assertions.
//   - migrated: outcome of the run held across migration states.
//   - baseline: outcome of the migration-disabled legacy run.
//
// Return values: none.
func compareCompactWorkloadOutcomes(t *testing.T, migrated compactWorkloadOutcome,
	baseline compactWorkloadOutcome) {
	t.Helper()
	require.Equal(t, baseline.totalOps, migrated.totalOps,
		"both runs must execute the identical deterministic operation stream")
	for _, category := range compactWorkloadCategories() {
		require.Equal(t, baseline.counts[category], migrated.counts[category],
			"category %s success counts must match the migration-disabled baseline", category)
		require.Equal(t, baseline.digests[category], migrated.digests[category],
			"category %s outcomes must match the migration-disabled baseline byte for byte", category)
	}
	require.Equal(t, baseline.usersDigest, migrated.usersDigest,
		"the full-table users ordering digest must match the migration-disabled baseline")
	require.Equal(t, baseline.acked, migrated.acked,
		"acknowledged writes must reconcile identically across both runs")
}
