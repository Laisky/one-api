package model

// Shared apparatus for the Section 12 compatibility workload
// (TestCompactUUIDCompatibilityWorkload in compact_uuid_workload_test.go).
//
// The workload is deterministic BY CONSTRUCTION rather than statistically: every operation is
// fully determined by (segment, worker, operation index), and the operation mix is a fixed
// repeating ten-operation template — three creates/updates, four exact reads, one search, one
// report, one cache read — which is exactly the proposal's 30/40/10/10/10 split. Determinism is
// what makes the migration-disabled baseline comparison exact: the baseline run replays the
// identical operation stream against a second database, so a held migration state may change
// timing but never semantics, and every category's outcome digest must match byte for byte.
//
// Segment 0 additionally begins with one bootstrap insert per (worker, owned table), so every
// later read, search, report, and cache operation has a row to target no matter where the
// template lands. The bootstrap rows are ordinary acknowledged creates: they count toward the
// totals and the final reconciliation, on top of the pure ten-operation template.
//
// Every symbol is prefixed compactWorkload so it can never collide with the fault suite, whose
// files this suite calls into but must not edit.

import (
	"context"
	"crypto/sha256"
	"fmt"
	"hash"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
	"gorm.io/gorm"
)

const (
	// compactWorkloadWorkers is Section 12's eight concurrent clients.
	compactWorkloadWorkers = 8
	// compactWorkloadSegments is one workload segment per held migration state.
	compactWorkloadSegments = 6
	// compactWorkloadTemplateSize is the repeating operation template's length. Positions 0-2
	// are creates/updates, 3-6 exact reads, 7 search, 8 report, and 9 cache: 30/40/10/10/10.
	compactWorkloadTemplateSize = 10
	// compactWorkloadMinOpsPerWorker is the per-worker floor per held state: 8 workers times
	// 130 operations is 1,040, above the proposal's 1,000 successful operations per state.
	compactWorkloadMinOpsPerWorker = 130
	// compactWorkloadHoldEnv optionally lengthens each held state, in whole seconds, so CI can
	// hold states longer without changing the code.
	compactWorkloadHoldEnv = "WORKLOAD_HOLD_SECONDS"
	// compactWorkloadDefaultHold is the default per-state hold. The proposal's own 60-second
	// hold belongs to AUTO-T09; this corpus asserts the operation count, not the wall clock.
	compactWorkloadDefaultHold = 3 * time.Second
	// compactWorkloadMinRate is the proposal's aggregate request-rate floor, in ops/second.
	compactWorkloadMinRate = 10.0
	// compactWorkloadFixtureRows is the recorded users fixture. It deliberately exceeds
	// config.MinCompactUUIDMaxRowsPerCycle so the held backfill provably cannot finish the
	// users fill inside one bounded cycle.
	compactWorkloadFixtureRows = 2500
	// compactWorkloadIDBase is the first row id the workload owns, far above the fixture.
	compactWorkloadIDBase = 2_000_000
	// compactWorkloadIDStride separates each worker's private id range within every table.
	compactWorkloadIDStride = 100_000
	// compactWorkloadUUIDBase is the first index of the private UUID vector space, disjoint
	// from the fixture's own vectors.
	compactWorkloadUUIDBase = 40_000_000
	// compactWorkloadUUIDStride separates each worker's private UUID vector space.
	compactWorkloadUUIDStride = 1_000_000
	// compactWorkloadBaseDSNEnv names the migration-disabled baseline database.
	compactWorkloadBaseDSNEnv = "COMPACT_UUID_TEST_POSTGRES_BASE_DSN"
)

// compactWorkloadCategory names one operation category of the ten-operation template.
type compactWorkloadCategory string

const (
	// compactWorkloadWrite is a create or update against a writer target.
	compactWorkloadWrite compactWorkloadCategory = "write"
	// compactWorkloadRead is an exact resolveIDByUUID read of an owned type.
	compactWorkloadRead compactWorkloadCategory = "read"
	// compactWorkloadSearch is the pasted-UUID keyword search path (WHERE uuid = ?).
	compactWorkloadSearch compactWorkloadCategory = "search"
	// compactWorkloadReport is an aggregate COUNT/GROUP BY report query.
	compactWorkloadReport compactWorkloadCategory = "report"
	// compactWorkloadCache is the real cache path with Redis disabled (SQL fallback).
	compactWorkloadCache compactWorkloadCategory = "cache"
)

// compactWorkloadCategories returns every category in one fixed comparison order.
// Parameters: none.
//
// Return values:
//   - []compactWorkloadCategory: the five categories, in deterministic order.
func compactWorkloadCategories() []compactWorkloadCategory {
	return []compactWorkloadCategory{compactWorkloadWrite, compactWorkloadRead,
		compactWorkloadSearch, compactWorkloadReport, compactWorkloadCache}
}

// compactWorkloadCategoryAt maps one operation index onto the repeating template.
// Parameters:
//   - k: zero-based operation index within a worker's segment.
//
// Return values:
//   - compactWorkloadCategory: the category the template fixes for this index.
func compactWorkloadCategoryAt(k int) compactWorkloadCategory {
	switch position := k % compactWorkloadTemplateSize; {
	case position < 3:
		return compactWorkloadWrite
	case position < 7:
		return compactWorkloadRead
	case position == 7:
		return compactWorkloadSearch
	case position == 8:
		return compactWorkloadReport
	default:
		return compactWorkloadCache
	}
}

// compactWorkloadTables returns every owned writer target, in registry order.
// Parameters: none.
//
// Return values:
//   - []string: the 12 owned-registry table names.
func compactWorkloadTables() []string {
	tables := make([]string, 0, len(uuidOwnedRegistry()))
	for _, owned := range uuidOwnedRegistry() {
		tables = append(tables, owned.table)
	}
	return tables
}

// compactWorkloadSearchTables returns the tables the search category rotates through.
// Parameters: none.
//
// Return values:
//   - []string: users, tokens, and channels, as Section 12 requires at least.
func compactWorkloadSearchTables() []string {
	return []string{"users", "tokens", "channels"}
}

// compactWorkloadSchedule fixes the deterministic shape shared by both runs.
type compactWorkloadSchedule struct {
	// holdFor is how long each held migration state lasts under the paced workload.
	holdFor time.Duration
	// opsPerWorker is the per-worker template operation count per segment.
	opsPerWorker int
	// pace is the per-operation sleep that stretches one segment across holdFor.
	pace time.Duration
}

// compactWorkloadScheduleFromEnv computes the schedule, honoring WORKLOAD_HOLD_SECONDS.
//
// The operation count scales with the hold so the aggregate rate can never fall below the
// proposal's 10 requests/second floor: 8 workers times max(130, 2*seconds) operations across
// holdFor seconds is at least 16 operations/second before any database latency is added.
// Parameters: none.
//
// Return values:
//   - compactWorkloadSchedule: the computed schedule.
//   - error: wrapped error when the environment value is not a positive integer.
func compactWorkloadScheduleFromEnv() (compactWorkloadSchedule, error) {
	holdFor := compactWorkloadDefaultHold
	if raw := strings.TrimSpace(os.Getenv(compactWorkloadHoldEnv)); raw != "" {
		seconds, err := strconv.Atoi(raw)
		if err != nil || seconds < 1 {
			return compactWorkloadSchedule{}, errors.Errorf(
				"%s must be a positive integer of seconds, got %q", compactWorkloadHoldEnv, raw)
		}
		holdFor = time.Duration(seconds) * time.Second
	}
	ops := compactWorkloadMinOpsPerWorker
	if scaled := int(holdFor/time.Second) * 2; scaled > ops {
		// Round up to a whole template so every segment ends on a template boundary.
		ops = ((scaled + compactWorkloadTemplateSize - 1) / compactWorkloadTemplateSize) *
			compactWorkloadTemplateSize
	}
	return compactWorkloadSchedule{holdFor: holdFor, opsPerWorker: ops,
		pace: holdFor / time.Duration(ops)}, nil
}

// compactWorkloadInsert writes one minimal-column row into an owned table, as a legacy client
// would: authoritative text only, never a compact column.
//
// Each statement carries exactly the columns the migrated schema requires (NOT NULL and unique
// keys, discovered from the model structs) plus the deterministic values the report category
// aggregates over (users.status, logs.type).
// Parameters:
//   - ctx: context bounding the statement.
//   - db: handle for the run's database.
//   - table: trusted owned-registry table name.
//   - id: private-range primary key.
//   - uuid: legacy UUID text to store.
//   - worker: worker index, embedded into unique columns.
//   - seq: per-(worker, table) insert sequence, embedded into unique columns.
//
// Return values:
//   - error: wrapped error when the insert fails or the table is unknown.
func compactWorkloadInsert(ctx context.Context, db *gorm.DB, table string,
	id int, uuid string, worker int, seq int) error {
	handle := db.WithContext(ctx)
	var err error
	switch table {
	case "users":
		err = handle.Exec("INSERT INTO users (id, username, password, uuid, status) VALUES (?, ?, 'x', ?, ?)",
			id, fmt.Sprintf("wk-user-%d-%d", worker, seq), uuid, 1+seq%2).Error
	case "tokens":
		err = handle.Exec("INSERT INTO tokens (id, user_id, key, name, uuid) VALUES (?, 1, ?, 'wk', ?)",
			id, fmt.Sprintf("wk-token-%d-%d", worker, seq), uuid).Error
	case "channels":
		err = handle.Exec("INSERT INTO channels (id, name, uuid) VALUES (?, ?, ?)",
			id, fmt.Sprintf("wk-channel-%d-%d", worker, seq), uuid).Error
	case "redemptions":
		err = handle.Exec("INSERT INTO redemptions (id, user_id, key, name, uuid) VALUES (?, 1, ?, 'wk', ?)",
			id, fmt.Sprintf("wk-red-%d-%d", worker, seq), uuid).Error
	case "token_transactions":
		err = handle.Exec("INSERT INTO token_transactions (id, uuid, transaction_id, token_id, user_id, status) "+
			"VALUES (?, ?, ?, ?, 1, 1)", id, uuid, fmt.Sprintf("wk-txn-%d-%d", worker, seq), id).Error
	case "user_request_costs":
		err = handle.Exec("INSERT INTO user_request_costs (id, uuid, request_id, quota) VALUES (?, ?, ?, 0)",
			id, uuid, fmt.Sprintf("wk-cost-%d-%d", worker, seq)).Error
	case "traces":
		err = handle.Exec("INSERT INTO traces (id, uuid, trace_id, url, method) VALUES (?, ?, ?, '/wk', 'GET')",
			id, uuid, fmt.Sprintf("wk-trace-%d-%d", worker, seq)).Error
	case "async_task_bindings":
		err = handle.Exec("INSERT INTO async_task_bindings (id, uuid, task_id, task_type, user_id, "+
			"channel_id, channel_type) VALUES (?, ?, ?, 'wk', 1, 1, 1)",
			id, uuid, fmt.Sprintf("wk-task-%d-%d", worker, seq)).Error
	case "mcp_servers":
		err = handle.Exec("INSERT INTO mcp_servers (id, uuid, name, base_url) VALUES (?, ?, ?, 'http://wk.invalid')",
			id, uuid, fmt.Sprintf("wk-mcp-%d-%d", worker, seq)).Error
	case "mcp_tools":
		err = handle.Exec("INSERT INTO mcp_tools (id, uuid, server_id, name) VALUES (?, ?, 1, ?)",
			id, uuid, fmt.Sprintf("wk-tool-%d-%d", worker, seq)).Error
	case "passkey_credentials":
		err = handle.Exec("INSERT INTO passkey_credentials (id, uuid, user_id, credential_name, "+
			"credential_id, public_key) VALUES (?, ?, 1, 'wk', ?, ?)",
			id, uuid, []byte(fmt.Sprintf("wk-cred-%d-%d", worker, seq)), []byte("wk-cose-key")).Error
	case "logs":
		err = handle.Exec("INSERT INTO logs (id, uuid, user_id, type) VALUES (?, ?, 1, ?)",
			id, uuid, seq%3).Error
	default:
		return errors.Errorf("unknown workload table %q", table)
	}
	return errors.Wrapf(err, "insert %s row %d", table, id)
}

// compactWorkloadWorker is one deterministic client. All of its state advances only through its
// own sequential operation stream, so a run against the migrated database and a run against the
// baseline database produce identical streams and identical expected outcomes.
type compactWorkloadWorker struct {
	// index is the worker's slot, deciding its private id and UUID spaces.
	index int
	// writeSeq, readSeq, searchSeq, and reportSeq rotate each category across its tables.
	writeSeq, readSeq, searchSeq, reportSeq int
	// uuidSeq draws the next private UUID vector.
	uuidSeq int
	// insertCount, writeCount, and updateCount are per-table operation counters.
	insertCount, writeCount, updateCount map[string]int
	// inserted lists this worker's row ids per table, in insertion order.
	inserted map[string][]int
	// current maps every acknowledged row to the text its last acknowledged write stored.
	current map[string]map[int]string
	// last is the most recently acknowledged row id per table.
	last map[string]int
	// digests accumulate one outcome stream per category.
	digests map[compactWorkloadCategory]hash.Hash
	// counts are per-category success counts.
	counts map[compactWorkloadCategory]int64
	// segWrites, segReads, segSearches, and segReports are per-segment coverage counters.
	segWrites, segReads, segSearches, segReports map[string]int
	// segCache counts this segment's cache operations.
	segCache int
}

// newCompactWorkloadWorker builds one worker with empty deterministic state.
// Parameters:
//   - index: worker slot.
//   - tables: the owned writer targets.
//
// Return values:
//   - *compactWorkloadWorker: the initialized worker.
func newCompactWorkloadWorker(index int, tables []string) *compactWorkloadWorker {
	worker := &compactWorkloadWorker{
		index:       index,
		insertCount: map[string]int{}, writeCount: map[string]int{}, updateCount: map[string]int{},
		inserted: map[string][]int{}, current: map[string]map[int]string{}, last: map[string]int{},
		digests: map[compactWorkloadCategory]hash.Hash{},
		counts:  map[compactWorkloadCategory]int64{},
	}
	for _, table := range tables {
		worker.current[table] = map[int]string{}
	}
	for _, category := range compactWorkloadCategories() {
		worker.digests[category] = sha256.New()
	}
	worker.resetCoverage()
	return worker
}

// resetCoverage clears the per-segment coverage counters.
// Parameters: none.
//
// Return values: none.
func (w *compactWorkloadWorker) resetCoverage() {
	w.segWrites, w.segReads = map[string]int{}, map[string]int{}
	w.segSearches, w.segReports = map[string]int{}, map[string]int{}
	w.segCache = 0
}

// record appends one operation outcome to a category digest and counts the success.
// Parameters:
//   - category: operation category.
//   - payload: deterministic outcome line.
//
// Return values: none.
func (w *compactWorkloadWorker) record(category compactWorkloadCategory, payload string) {
	fmt.Fprintln(w.digests[category], payload)
	w.counts[category]++
}

// nextUUID draws the next vector from this worker's private UUID space.
// Parameters: none.
//
// Return values:
//   - string: canonical UUID text unique across all workers, tables, and the fixture.
func (w *compactWorkloadWorker) nextUUID() string {
	index := compactWorkloadUUIDBase + w.index*compactWorkloadUUIDStride + w.uuidSeq
	w.uuidSeq++
	return compactUUIDTextFor(index)
}

// ackWrite records one acknowledged write in the worker's bookkeeping and outcome digest.
// Parameters:
//   - table: written table.
//   - id: written row id.
//   - text: the exact authoritative text the write stored.
//   - kind: "insert" or "update", part of the compared outcome.
//
// Return values: none.
func (w *compactWorkloadWorker) ackWrite(table string, id int, text string, kind string) {
	w.current[table][id] = text
	w.last[table] = id
	w.writeCount[table]++
	w.segWrites[table]++
	w.record(compactWorkloadWrite, fmt.Sprintf("W|%s|%s|%d|%s", kind, table, id, text))
}

// insertRow creates one new row in a table, from the worker's private spaces.
// Parameters:
//   - ctx: context bounding the statement.
//   - run: owning run.
//   - table: table to insert into.
//
// Return values:
//   - error: wrapped error when the insert fails.
func (w *compactWorkloadWorker) insertRow(ctx context.Context, run *compactWorkloadRun, table string) error {
	seq := w.insertCount[table]
	id := compactWorkloadIDBase + w.index*compactWorkloadIDStride + seq
	text := w.nextUUID()
	if err := compactWorkloadInsert(ctx, run.db, table, id, text, w.index, seq); err != nil {
		return err
	}
	w.insertCount[table] = seq + 1
	w.inserted[table] = append(w.inserted[table], id)
	w.ackWrite(table, id, text, "insert")
	return nil
}

// opWrite performs one create or update, alternating per table so both kinds cover every
// writer target.
// Parameters:
//   - ctx: context bounding the statement.
//   - run: owning run.
//
// Return values:
//   - error: wrapped error when the write fails.
func (w *compactWorkloadWorker) opWrite(ctx context.Context, run *compactWorkloadRun) error {
	table := run.tables[w.writeSeq%len(run.tables)]
	w.writeSeq++
	if w.writeCount[table]%2 == 0 {
		return w.insertRow(ctx, run, table)
	}
	rows := w.inserted[table]
	id := rows[w.updateCount[table]%len(rows)]
	w.updateCount[table]++
	text := w.nextUUID()
	if err := run.db.WithContext(ctx).Exec(
		"UPDATE "+table+" SET uuid = ? WHERE id = ?", text, id).Error; err != nil {
		return errors.Wrapf(err, "update %s row %d", table, id)
	}
	w.ackWrite(table, id, text, "update")
	return nil
}

// opRead resolves one acknowledged row through the production exact-lookup entry point.
// In unified mode the run's primary handle is the authoritative handle for both registry roles,
// so it serves every target including logs.
// Parameters:
//   - ctx: context bounding the lookup.
//   - run: owning run.
//
// Return values:
//   - error: wrapped error on a failed, stale, or wrong resolution.
func (w *compactWorkloadWorker) opRead(ctx context.Context, run *compactWorkloadRun) error {
	table := run.tables[w.readSeq%len(run.tables)]
	w.readSeq++
	id := w.last[table]
	text := w.current[table][id]
	resolved, err := resolveIDByUUID(ctx, run.db, run.targets[table], text)
	if err != nil {
		return errors.Wrapf(err, "resolve %s row %d", table, id)
	}
	if resolved != int64(id) {
		return errors.Errorf("stale read: %s uuid %s resolved to %d, want %d", table, text, resolved, id)
	}
	w.segReads[table]++
	w.record(compactWorkloadRead, fmt.Sprintf("R|%s|%d|%s", table, id, text))
	return nil
}

// opSearch performs the pasted-UUID keyword search (WHERE uuid = ?), the same clause the user
// search path applies to a keyword that parses as a UUID.
// Parameters:
//   - ctx: context bounding the query.
//   - run: owning run.
//
// Return values:
//   - error: wrapped error when the search fails or returns the wrong row set.
func (w *compactWorkloadWorker) opSearch(ctx context.Context, run *compactWorkloadRun) error {
	tables := compactWorkloadSearchTables()
	table := tables[w.searchSeq%len(tables)]
	w.searchSeq++
	id := w.last[table]
	text := w.current[table][id]
	rows := []struct {
		ID int `gorm:"column:id"`
	}{}
	if err := run.db.WithContext(ctx).Raw(
		"SELECT id FROM "+table+" WHERE uuid = ?", text).Scan(&rows).Error; err != nil {
		return errors.Wrapf(err, "search %s by uuid", table)
	}
	if len(rows) != 1 || rows[0].ID != id {
		return errors.Errorf("search %s for %s returned %d rows, want exactly row %d",
			table, text, len(rows), id)
	}
	w.segSearches[table]++
	w.record(compactWorkloadSearch, fmt.Sprintf("S|%s|%d|%d", table, len(rows), id))
	return nil
}

// opReport aggregates over this worker's own private id range, alternating users (GROUP BY
// status) and logs (GROUP BY type). The range restriction is what keeps a report deterministic
// under concurrency: no other worker ever writes into this worker's rows.
// Parameters:
//   - ctx: context bounding the query.
//   - run: owning run.
//
// Return values:
//   - error: wrapped error when the report fails or sees no rows.
func (w *compactWorkloadWorker) opReport(ctx context.Context, run *compactWorkloadRun) error {
	table, column := "users", "status"
	if w.reportSeq%2 == 1 {
		table, column = "logs", "type"
	}
	w.reportSeq++
	low := compactWorkloadIDBase + w.index*compactWorkloadIDStride
	high := low + compactWorkloadIDStride - 1
	rows := []struct {
		Grp   int   `gorm:"column:grp"`
		Total int64 `gorm:"column:total"`
	}{}
	if err := run.db.WithContext(ctx).Raw(
		"SELECT "+column+" AS grp, count(*) AS total FROM "+table+
			" WHERE id BETWEEN ? AND ? GROUP BY "+column+" ORDER BY "+column,
		low, high).Scan(&rows).Error; err != nil {
		return errors.Wrapf(err, "report on %s", table)
	}
	if len(rows) == 0 {
		return errors.Errorf("report on %s saw no rows in this worker's range", table)
	}
	payload := &strings.Builder{}
	fmt.Fprintf(payload, "P|%s", table)
	for _, row := range rows {
		fmt.Fprintf(payload, "|%d:%d", row.Grp, row.Total)
	}
	w.segReports[table]++
	w.record(compactWorkloadReport, payload.String())
	return nil
}

// opCache reads one of this worker's users rows through the real cache entry point. With Redis
// disabled, CacheGetUserById falls back to the SQL path against the package-global DB handle,
// which the run's setup pointed at this run's database.
// Parameters:
//   - ctx: context bounding the read.
//   - run: owning run.
//
// Return values:
//   - error: wrapped error when the cache read fails or returns the wrong payload.
func (w *compactWorkloadWorker) opCache(ctx context.Context, run *compactWorkloadRun) error {
	id := w.last["users"]
	expected := w.current["users"][id]
	user, err := CacheGetUserById(ctx, id)
	if err != nil {
		return errors.Wrapf(err, "cache read user %d", id)
	}
	if user.Id != id || strings.TrimRight(user.UUID, " ") != expected {
		return errors.Errorf("cache read user %d returned id %d uuid %q, want uuid %q",
			id, user.Id, user.UUID, expected)
	}
	w.segCache++
	w.record(compactWorkloadCache, fmt.Sprintf("C|%d|%s", id, expected))
	return nil
}

// step dispatches one template operation.
// Parameters:
//   - ctx: context bounding the operation.
//   - run: owning run.
//   - k: zero-based operation index within the segment.
//
// Return values:
//   - error: wrapped error from the operation.
func (w *compactWorkloadWorker) step(ctx context.Context, run *compactWorkloadRun, k int) error {
	switch compactWorkloadCategoryAt(k) {
	case compactWorkloadWrite:
		return w.opWrite(ctx, run)
	case compactWorkloadRead:
		return w.opRead(ctx, run)
	case compactWorkloadSearch:
		return w.opSearch(ctx, run)
	case compactWorkloadReport:
		return w.opReport(ctx, run)
	default:
		return w.opCache(ctx, run)
	}
}

// runSegment executes this worker's deterministic slice of one segment.
// Parameters:
//   - ctx: context bounding the segment.
//   - run: owning run.
//   - seedFirst: true on segment 0, to bootstrap one row per owned table first.
//
// Return values:
//   - error: wrapped error from the first failed operation.
func (w *compactWorkloadWorker) runSegment(ctx context.Context, run *compactWorkloadRun, seedFirst bool) error {
	if seedFirst {
		for _, table := range run.tables {
			if err := w.insertRow(ctx, run, table); err != nil {
				return errors.Wrapf(err, "worker %d bootstrap %s", w.index, table)
			}
			w.pause(run)
		}
	}
	for k := 0; k < run.schedule.opsPerWorker; k++ {
		if err := w.step(ctx, run, k); err != nil {
			return errors.Wrapf(err, "worker %d operation %d", w.index, k)
		}
		w.pause(run)
	}
	return nil
}

// pause applies the run's per-operation pacing, when any.
// Parameters:
//   - run: owning run.
//
// Return values: none.
func (w *compactWorkloadWorker) pause(run *compactWorkloadRun) {
	if run.pace > 0 {
		time.Sleep(run.pace)
	}
}
