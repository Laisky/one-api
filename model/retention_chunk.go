package model

// Chunked retention deletes (proposal
// docs/proposals/20260905_observability-data-tiering.md, Phase 0 / W0.2 and
// W0.8).
//
// Every retention path in this project used to issue one unbounded
// `DELETE ... WHERE created_at < ?`. At the volumes this system targets that
// statement removes billions of rows in a single transaction: on PostgreSQL a
// multi-terabyte WAL burst plus table bloat, on MySQL a gap-locking transaction
// long enough to stall the gateway, on SQLite a write lock held for minutes.
//
// The fix is mechanical: delete in bounded chunks, pause between them so live
// traffic gets the database back, and stop when the caller's context is done.
//
// W0.8 adds three requirements on top of that:
//
//  1. Bound the work to LOCATE candidates, not merely the rows deleted. A
//     `LIMIT` on the DELETE caps what is removed; it does not cap what the
//     engine reads to find those rows. Each chunk restarted at the beginning of
//     the qualifying range, so a sweep over N eligible rows in batches of B did
//     O(N^2/B) index work in the best case and re-read the same dead tuples on
//     PostgreSQL until the next vacuum. This file now walks the ordering column
//     with a keyset watermark: every chunk seeks straight to where the previous
//     chunk stopped.
//  2. Give async-task predicates their own plan (see async_task_retention.go).
//  3. Keep up with newly eligible rows under concurrent traffic. A single
//     keyset pass ends when its scan reaches the end of the eligible range as
//     it looked at scan time; rows that became eligible BELOW the watermark
//     while the pass was running would otherwise wait for the next sweep
//     interval, which can be a day. Each sweep therefore measures the remaining
//     backlog with a bounded probe and runs another pass while a backlog
//     remains and the previous pass still made progress.

import (
	"context"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common/config"
)

const (
	// retentionBacklogProbeRows caps the bounded backlog probe.
	//
	// The backlog counter answers one operational question -- is retention
	// keeping up? -- and an exact `COUNT(*)` over an unswept table is exactly
	// the unbounded query this whole work package exists to remove (section 3
	// of the proposal: "never discover this limit using an unbounded exact
	// count"). The probe therefore counts at most this many rows and reports
	// BacklogTruncated when it hits the cap: "at least 100000 eligible rows
	// remain" is all an operator needs to know that the sweep is behind.
	//
	// The value mirrors the LOG_COUNT_EXACT_MAX_ROWS budget named in the
	// proposal. It is a constant, not a configuration key, because W0.8 does
	// not authorize new configuration surface.
	retentionBacklogProbeRows = 100000

	// retentionMaxSweepPasses bounds how many times one sweep restarts its
	// keyset walk after finding a remaining backlog.
	//
	// Restarting is what lets a sweep catch rows that became eligible below the
	// watermark while it was running. Restarting without a bound would let a
	// sweep run forever against a workload that produces eligible rows faster
	// than the sweeper deletes them, holding database time indefinitely instead
	// of yielding until the next interval. Eight passes is far more than any
	// healthy deployment needs (a healthy sweep ends after one) and is reached
	// only when arrivals genuinely outpace deletion, which is a capacity
	// problem the reported backlog is meant to surface.
	retentionMaxSweepPasses = 8
)

// ChunkedDeleteOptions describes one bounded retention sweep.
type ChunkedDeleteOptions struct {
	// Table is the physical table name, used to build the dialect-specific
	// bounded DELETE. It must be a compile-time constant from this package,
	// never a caller-supplied value: it is interpolated into SQL.
	Table string
	// Where is the row-selection predicate, with `?` placeholders.
	Where string
	// Args are the predicate arguments.
	Args []any
	// OrderColumn is the indexed 64-bit column the predicate is anchored on,
	// used as the keyset cursor and as the "oldest eligible row" measure. It
	// must be listed in the target table's keyset allow-list; an empty value
	// selects the table's default. Every retention ordering column in this
	// schema is an epoch timestamp stored as a bigint, which is why the cursor
	// type is int64; the unit differs per table and is recorded in
	// retentionTables.
	OrderColumn string
	// BatchSize bounds rows removed per statement; <= 0 uses the configured
	// default.
	BatchSize int
	// Pause is how long to wait between chunks; <= 0 disables the pause.
	Pause time.Duration
}

// ChunkedDeleteStats reports what one sweep did and what it left behind.
//
// The backlog fields are the W0.8 "is retention keeping up?" accounting: they
// are measured after the final pass, so a sweep that drained the table reports
// Backlog 0, and a sweep that could not keep up reports what remains and how
// old the oldest eligible row is.
type ChunkedDeleteStats struct {
	// Deleted is the total rows removed across every chunk and pass.
	Deleted int64
	// Chunks is how many bounded DELETE statements ran.
	Chunks int
	// Passes is how many keyset walks ran; more than one means the first walk
	// finished with rows still eligible.
	Passes int
	// Candidates is how many rows the keyset probes located. It is the bounded
	// candidate-location work the sweep actually performed.
	Candidates int64
	// Backlog is how many eligible rows remained after the final pass, counted
	// by a probe bounded at retentionBacklogProbeRows.
	Backlog int64
	// BacklogTruncated reports that the probe hit its cap, so Backlog is a
	// lower bound rather than an exact count.
	BacklogTruncated bool
	// OldestEligibleAtMilli is the timestamp of the oldest row still eligible
	// after the sweep, normalized to Unix milliseconds whatever unit the
	// ordering column stores; 0 when nothing remains or when the sweep had no
	// ordering column.
	OldestEligibleAtMilli int64
}

// OldestEligibleAge reports how far behind the sweep is.
//
// Parameters:
//   - now: the reference instant, normally time.Now().UTC().
//
// Return values:
//   - time.Duration: age of the oldest still-eligible row, or 0 when the sweep
//     left nothing eligible behind (or ran without an ordering column).
func (s ChunkedDeleteStats) OldestEligibleAge(now time.Time) time.Duration {
	if s.OldestEligibleAtMilli <= 0 {
		return 0
	}
	age := now.UTC().UnixMilli() - s.OldestEligibleAtMilli
	if age <= 0 {
		return 0
	}
	return time.Duration(age) * time.Millisecond
}

// retentionTarget describes one allow-listed retention table.
type retentionTarget struct {
	// defaultOrderColumn is the keyset cursor used when the caller does not
	// name one. It must be an indexed bigint column.
	defaultOrderColumn string
	// orderColumnUnit is the time unit the ordering column stores. The cursor
	// itself is unit-agnostic -- it only ever compares raw int64s -- but the
	// reported oldest-eligible timestamp is normalized with this so a caller
	// can read ChunkedDeleteStats the same way for every table.
	orderColumnUnit time.Duration
	// keysetColumns is the set of columns a caller may order by. Ordering
	// columns are interpolated into SQL exactly like the table name, so they
	// are allow-listed exactly like the table name.
	keysetColumns map[string]bool
}

// retentionTables is the allow-list of tables the chunked sweeper may target.
// ChunkedDelete interpolates the table name and the ordering column into SQL,
// so neither may ever come from an arbitrary string.
var retentionTables = map[string]retentionTarget{
	// traces.created_at carries its own index and stores epoch milliseconds.
	"traces": {
		defaultOrderColumn: "created_at",
		orderColumnUnit:    time.Millisecond,
		keysetColumns:      map[string]bool{"created_at": true},
	},
	// logs.created_at leads the idx_created_at_type composite index, so a
	// keyset seek on it is an index range scan. This default is what gives the
	// operator purge in DeleteOldLogContext bounded candidate location without
	// that call site having to opt in.
	//
	// It stores epoch SECONDS (common/helper.GetTimestamp), unlike every other
	// retention table.
	"logs": {
		defaultOrderColumn: "created_at",
		orderColumnUnit:    time.Second,
		keysetColumns:      map[string]bool{"created_at": true},
	},
	// async_task_bindings is swept as two sargable passes with different
	// cursors; see async_task_retention.go.
	"async_task_bindings": {
		defaultOrderColumn: "created_at",
		orderColumnUnit:    time.Millisecond,
		keysetColumns:      map[string]bool{"created_at": true, "last_accessed_at": true},
	},
}

// ChunkedDelete removes matching rows in bounded batches.
//
// It is the compatibility wrapper over ChunkedDeleteWithStats for callers that
// only need the deleted count.
//
// Parameters:
//   - ctx: cancellation scope; a cancelled context stops the sweep cleanly
//     between chunks and returns what was already removed.
//   - db: the handle owning the target table.
//   - opts: the sweep description.
//
// Return values:
//   - int64: total rows removed across all chunks.
//   - error: wrapped failure from the chunk that could not complete, or a
//     configuration error when the table is not on the allow-list.
func ChunkedDelete(ctx context.Context, db *gorm.DB, opts ChunkedDeleteOptions) (int64, error) {
	stats, err := ChunkedDeleteWithStats(ctx, db, opts)
	return stats.Deleted, err
}

// ChunkedDeleteWithStats removes matching rows in bounded batches and reports
// whether the sweep kept up.
//
// Parameters:
//   - ctx: cancellation scope; a cancelled context stops the sweep cleanly
//     between chunks and returns what was already removed.
//   - db: the handle owning the target table; its dialect decides the SQL, so a
//     split SQL_DSN / LOG_SQL_DSN topology stays correct.
//   - opts: the sweep description.
//
// Return values:
//   - ChunkedDeleteStats: rows removed, work performed, and the remaining
//     backlog measured after the final pass.
//   - error: wrapped failure from the chunk that could not complete, or a
//     configuration error when the table or ordering column is not allow-listed.
func ChunkedDeleteWithStats(ctx context.Context, db *gorm.DB, opts ChunkedDeleteOptions) (ChunkedDeleteStats, error) {
	var stats ChunkedDeleteStats

	if db == nil {
		return stats, errors.New("database handle is not initialized")
	}
	target, allowed := retentionTables[opts.Table]
	if !allowed {
		return stats, errors.Errorf("table %q is not an allowed retention target", opts.Table)
	}

	orderColumn := opts.OrderColumn
	if orderColumn == "" {
		orderColumn = target.defaultOrderColumn
	}
	if orderColumn != "" && !target.keysetColumns[orderColumn] {
		return stats, errors.Errorf("column %q is not an allowed keyset column for table %s",
			orderColumn, opts.Table)
	}

	batchSize := opts.BatchSize
	if batchSize <= 0 {
		batchSize = config.RetentionDeleteBatchSize
	}
	if batchSize <= 0 {
		batchSize = 1000
	}
	batchSize = min(batchSize, retentionCandidateLimit(db, len(opts.Args)))

	sweep := chunkedSweep{
		db:          db,
		table:       opts.Table,
		where:       opts.Where,
		args:        opts.Args,
		orderColumn: orderColumn,
		batchSize:   batchSize,
		pause:       opts.Pause,
	}

	for pass := 1; pass <= retentionMaxSweepPasses; pass++ {
		passDeleted, err := sweep.run(ctx, &stats)
		stats.Deleted += passDeleted
		stats.Passes = pass
		if err != nil {
			// The backlog probe is deliberately skipped on failure: the sweep
			// context is usually already cancelled, so probing would only
			// replace a precise error with a confusing one.
			return stats, err
		}

		backlog, truncated, oldest, err := sweep.measureBacklog(ctx)
		if err != nil {
			return stats, err
		}
		stats.Backlog, stats.BacklogTruncated = backlog, truncated
		stats.OldestEligibleAtMilli = normalizeRetentionTimestamp(oldest, target.orderColumnUnit)

		if backlog == 0 {
			return stats, nil
		}
		if passDeleted == 0 || orderColumn == "" {
			// backlog == 0: nothing eligible remains, the normal exit.
			// passDeleted == 0: a backlog exists that this sweep cannot remove
			//   (rows locked by another transaction, for example). Retrying
			//   inside the same sweep would spin; the reported backlog is the
			//   signal.
			// orderColumn == "": without a cursor a pass already re-scans from
			//   the start of the range every chunk, so it cannot have skipped
			//   rows that a second pass would find.
			return stats, errors.Errorf("retention sweep of %s incomplete: %d eligible rows remain", opts.Table, backlog)
		}

		if opts.Pause > 0 {
			if err := waitBetweenRetentionChunks(ctx, opts.Pause, opts.Table, stats.Deleted); err != nil {
				return stats, err
			}
		}
	}

	return stats, errors.Errorf("retention sweep of %s incomplete after %d passes: at least %d eligible rows remain",
		opts.Table, retentionMaxSweepPasses, stats.Backlog)
}

// retentionCandidateLimit returns the largest exact-ID batch that fits one
// DELETE statement on db after the retention predicate's placeholders.
//
// Parameters:
//   - db: the database whose placeholder ceiling applies.
//   - predicateArgs: placeholders consumed by the eligibility predicate.
//
// Return values:
//   - int: candidate IDs permitted in one DELETE; always at least one.
func retentionCandidateLimit(db *gorm.DB, predicateArgs int) int {
	limit := MaxStatementParametersSQLite
	if db != nil && dialectName(db) != "sqlite" {
		limit = MaxStatementParametersDefault
	}
	if predicateArgs >= limit {
		return 1
	}
	return max(1, limit-predicateArgs)
}

// chunkedSweep holds the resolved parameters of one sweep.
type chunkedSweep struct {
	db          *gorm.DB
	table       string
	where       string
	args        []any
	orderColumn string
	batchSize   int
	pause       time.Duration
}

// run performs one keyset walk over the eligible range.
//
// The walk locates at most batchSize candidates with an index seek ordered by
// the existing indexed ordering column, then deletes exactly those primary keys
// while rechecking eligibility. The seek is what makes the sweep linear: chunk
// k starts where chunk k-1 stopped instead of re-reading the expired prefix,
// and exact IDs prevent concurrent same-timestamp rows from being deleted merely
// because they fall inside a value range.
//
// A keyset watermark was chosen over a scan budget because every retention
// predicate in this schema is a range over an indexed bigint -- traces and logs
// on created_at, async task bindings on last_accessed_at / created_at after the
// rewrite in async_task_retention.go -- so the ordering column IS the predicate
// column and `LIMIT batchSize` on the seek bounds rows read, not just rows
// returned. A scan budget (reading a capped number of rows and stopping early)
// would be the tool for a predicate that no index can serve; the correct fix
// for those is an index, which is what W0.8 required for async tasks. A
// per-statement timeout was deliberately not added: it needs a configured
// value to be either safe or useful, W0.8 authorizes no new configuration key,
// and the caller's context already bounds total sweep time.
//
// The cursor is inclusive (`>=`) on the existing indexed ordering value.
// Timestamp ties are common and existing indexes intentionally do not include
// id; an `(order,id)` cursor would force a sort of a large tie group. Exact-ID
// deletion makes repeated probes of the current tie safe: each removes up to
// batchSize rows, then the next probe sees only the remaining tied rows.
//
// Parameters:
//   - ctx: cancellation scope; a cancelled walk reports what it removed.
//   - stats: accumulator for chunk and candidate counters.
//
// Return values:
//   - int64: rows removed by this pass.
//   - error: wrapped failure, including a wrapped context error on cancellation.
func (s chunkedSweep) run(ctx context.Context, stats *ChunkedDeleteStats) (int64, error) {
	var total int64
	watermarkOrder := int64(math.MinInt64)

	for {
		if err := ctx.Err(); err != nil {
			// A cancelled sweep is reported, not swallowed. Background sweepers
			// treat context.Canceled as ordinary shutdown; an operator-triggered
			// purge must not be able to report success for a partial delete.
			return total, errors.Wrapf(err, "retention sweep of %s stopped after %d rows", s.table, total)
		}

		candidates, err := s.locateChunk(ctx, watermarkOrder)
		if err != nil {
			return total, err
		}
		if len(candidates) == 0 {
			return total, nil
		}
		stats.Candidates += int64(len(candidates))
		deleted, err := s.deleteLocated(ctx, candidates)
		if err != nil {
			return total, err
		}
		total += deleted
		stats.Chunks++
		last := candidates[len(candidates)-1]
		if last.ordering == watermarkOrder && deleted == 0 {
			return total, nil
		}
		watermarkOrder = last.ordering
		if len(candidates) < s.batchSize {
			return total, nil
		}

		if s.pause > 0 {
			if err := waitBetweenRetentionChunks(ctx, s.pause, s.table, total); err != nil {
				return total, err
			}
		}
	}
}

// deleteStatement builds a DELETE for exactly one located candidate set.
//
// Candidate selection is bounded by the keyset query; this statement does not
// rediscover candidates by a timestamp range. It rechecks the original
// predicate, so a row changed after it was located is preserved.
//
// Parameters:
//   - candidateCount: number of located primary keys to bind.
//
// Return values:
//   - string: the SQL statement, with one placeholder per primary key followed
//     by the predicate placeholders.
func (s chunkedSweep) deleteStatement(candidateCount int) string {
	placeholders := strings.TrimSuffix(strings.Repeat("?,", candidateCount), ",")
	return "DELETE FROM " + s.table + " WHERE id IN (" + placeholders + ") AND (" + s.where + ")"
}

// locateChunk finds the next bounded group of candidate rows.
//
// Parameters:
//   - ctx: cancellation scope.
//   - watermarkOrder: inclusive last ordering value already visited.
//
// Return values:
//   - []retentionCandidate: exact candidate primary keys and ordering values;
//     fewer than the batch size means the eligible range is exhausted.
//   - error: wrapped failure from the probe.
type retentionCandidate struct {
	id       int64
	ordering int64
}

// locateChunk locates exact primary keys using the existing ordering index.
func (s chunkedSweep) locateChunk(ctx context.Context, watermarkOrder int64) ([]retentionCandidate, error) {
	query := "SELECT id, " + s.orderColumn + " FROM " + s.table +
		" WHERE (" + s.where + ") AND " + s.orderColumn + " >= ?" +
		" ORDER BY " + s.orderColumn + " ASC LIMIT " + strconv.Itoa(s.batchSize)

	args := append(append(make([]any, 0, len(s.args)+1), s.args...), watermarkOrder)

	rows, err := s.db.WithContext(ctx).Raw(query, args...).Rows()
	if err != nil {
		return nil, errors.Wrapf(err, "locate expired rows in %s", s.table)
	}
	defer rows.Close() //nolint:errcheck // read-only cursor

	candidates := make([]retentionCandidate, 0, s.batchSize)
	for rows.Next() {
		var candidate retentionCandidate
		if err := rows.Scan(&candidate.id, &candidate.ordering); err != nil {
			return nil, errors.Wrapf(err, "scan retention cursor of %s", s.table)
		}
		candidates = append(candidates, candidate)
	}
	if err := rows.Err(); err != nil {
		return nil, errors.Wrapf(err, "locate expired rows in %s", s.table)
	}
	return candidates, nil
}

// deleteLocated deletes exactly the primary keys selected by locateChunk while
// rechecking the eligibility predicate against concurrent updates.
func (s chunkedSweep) deleteLocated(ctx context.Context, candidates []retentionCandidate) (int64, error) {
	if len(candidates) == 0 {
		return 0, nil
	}
	args := make([]any, 0, len(candidates)+len(s.args))
	for _, candidate := range candidates {
		args = append(args, candidate.id)
	}
	args = append(args, s.args...)
	tx := s.db.WithContext(ctx).Exec(s.deleteStatement(len(candidates)), args...)
	if tx.Error != nil {
		return 0, errors.Wrapf(tx.Error, "delete expired rows from %s", s.table)
	}
	return tx.RowsAffected, nil
}

// measureBacklog reports what the sweep left behind.
//
// Both probes are bounded: the count stops at retentionBacklogProbeRows, and
// the oldest-row probe is a single index seek. Neither may become the unbounded
// query the sweep exists to avoid.
//
// Parameters:
//   - ctx: cancellation scope.
//
// Return values:
//   - int64: eligible rows remaining, capped at retentionBacklogProbeRows.
//   - bool: true when the count hit that cap and is therefore a lower bound.
//   - int64: ordering value of the oldest remaining eligible row, 0 when none
//     remains or the sweep has no ordering column.
//   - error: wrapped failure from either probe.
func (s chunkedSweep) measureBacklog(ctx context.Context) (int64, bool, int64, error) {
	probe := "SELECT COUNT(*) FROM (SELECT 1 FROM " + s.table +
		" WHERE " + s.where + " LIMIT " + strconv.Itoa(retentionBacklogProbeRows) +
		") AS retention_backlog_probe"

	var backlog int64
	if err := s.db.WithContext(ctx).Raw(probe, s.args...).Scan(&backlog).Error; err != nil {
		return 0, false, 0, errors.Wrapf(err, "measure retention backlog of %s", s.table)
	}
	if backlog == 0 || s.orderColumn == "" {
		return backlog, backlog >= retentionBacklogProbeRows, 0, nil
	}

	oldestQuery := "SELECT " + s.orderColumn + " FROM " + s.table +
		" WHERE " + s.where + " ORDER BY " + s.orderColumn + " ASC LIMIT 1"

	var oldest []int64
	if err := s.db.WithContext(ctx).Raw(oldestQuery, s.args...).Scan(&oldest).Error; err != nil {
		return backlog, backlog >= retentionBacklogProbeRows, 0,
			errors.Wrapf(err, "measure oldest eligible row of %s", s.table)
	}
	if len(oldest) == 0 {
		return backlog, backlog >= retentionBacklogProbeRows, 0, nil
	}
	return backlog, backlog >= retentionBacklogProbeRows, oldest[0], nil
}

// waitBetweenRetentionChunks yields the database to live traffic.
//
// Parameters:
//   - ctx: cancellation scope; a cancelled wait aborts the sweep.
//   - pause: how long to wait.
//   - table: the swept table, for the error message.
//   - deleted: rows removed so far, for the error message.
//
// Return values:
//   - error: wrapped context error when the wait was cancelled, nil otherwise.
func waitBetweenRetentionChunks(ctx context.Context, pause time.Duration, table string, deleted int64) error {
	timer := time.NewTimer(pause)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return errors.Wrapf(ctx.Err(), "retention sweep of %s stopped after %d rows", table, deleted)
	case <-timer.C:
		return nil
	}
}

// boundedDeleteStatement builds a dialect-appropriate row-limited DELETE.
//
// The three engines disagree about how to bound a DELETE:
//
//   - MySQL supports `DELETE ... LIMIT n` natively, and forbids a subquery that
//     names the target table, so the native form is the only option.
//   - PostgreSQL has no DELETE ... LIMIT; rows are selected by physical
//     location (ctid), the cheapest bounded selection that assumes nothing
//     about the primary key.
//   - SQLite only supports DELETE ... LIMIT when compiled with
//     SQLITE_ENABLE_UPDATE_DELETE_LIMIT, which the embedded driver is not, so
//     it uses the same subquery shape keyed on rowid.
//
// The dialect is read from the HANDLE, not from the process-global
// common.UsingPostgreSQL / UsingMySQL flags. Those flags are set as a side
// effect of opening any handle, so a deployment that splits SQL_DSN and
// LOG_SQL_DSN across engines (a SQLite primary with a MySQL log database, for
// example) leaves them describing only whichever handle was opened last. Using
// them here would emit MySQL syntax against SQLite and make retention fail
// forever on one of the two databases.
//
// Parameters:
//   - db: the handle the statement will run on; its dialector names the engine.
//   - table: an allow-listed physical table name.
//   - where: the row-selection predicate.
//   - batchSize: maximum rows per statement.
//
// Return values:
//   - string: the SQL statement, with the predicate's placeholders preserved.
func boundedDeleteStatement(db *gorm.DB, table, where string, batchSize int) string {
	limit := strconv.Itoa(batchSize)

	switch dialectName(db) {
	case "postgres":
		return "DELETE FROM " + table +
			" WHERE ctid IN (SELECT ctid FROM " + table +
			" WHERE " + where + " LIMIT " + limit + ")"
	case "mysql":
		return "DELETE FROM " + table + " WHERE " + where + " LIMIT " + limit
	default:
		return "DELETE FROM " + table +
			" WHERE rowid IN (SELECT rowid FROM " + table +
			" WHERE " + where + " LIMIT " + limit + ")"
	}
}

// Merge combines the statistics of two sweeps that together form one logical
// retention run, such as the two sargable passes over async_task_bindings.
//
// Backlogs add, because the passes cover disjoint row sets. The oldest eligible
// timestamp is the older (smaller) of the two, because that is the age an
// operator cares about. Passes take the maximum rather than the sum: it
// describes how hard the harder pass had to work, not how many statements ran.
//
// Parameters:
//   - other: the statistics to fold into the receiver.
//
// Return values:
//   - ChunkedDeleteStats: the combined statistics.
func (s ChunkedDeleteStats) Merge(other ChunkedDeleteStats) ChunkedDeleteStats {
	merged := ChunkedDeleteStats{
		Deleted:          s.Deleted + other.Deleted,
		Chunks:           s.Chunks + other.Chunks,
		Candidates:       s.Candidates + other.Candidates,
		Backlog:          s.Backlog + other.Backlog,
		BacklogTruncated: s.BacklogTruncated || other.BacklogTruncated,
		Passes:           max(s.Passes, other.Passes),
	}

	switch {
	case s.OldestEligibleAtMilli == 0:
		merged.OldestEligibleAtMilli = other.OldestEligibleAtMilli
	case other.OldestEligibleAtMilli == 0:
		merged.OldestEligibleAtMilli = s.OldestEligibleAtMilli
	default:
		merged.OldestEligibleAtMilli = min(s.OldestEligibleAtMilli, other.OldestEligibleAtMilli)
	}

	return merged
}

// normalizeRetentionTimestamp converts an ordering-column value to milliseconds.
//
// Parameters:
//   - value: the raw column value; 0 means "nothing eligible".
//   - unit: the unit the column stores, from retentionTables.
//
// Return values:
//   - int64: the value in Unix milliseconds, or 0 when there was none.
func normalizeRetentionTimestamp(value int64, unit time.Duration) int64 {
	if value == 0 || unit < time.Millisecond {
		return value
	}
	return value * int64(unit/time.Millisecond)
}
