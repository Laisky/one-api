package model

// Chunked retention deletes (proposal
// docs/proposals/20260905_observability-data-tiering.md, Phase 0 / W0.2).
//
// Every retention path in this project used to issue one unbounded
// `DELETE ... WHERE created_at < ?`. At the volumes this system targets that
// statement removes billions of rows in a single transaction: on PostgreSQL a
// multi-terabyte WAL burst plus table bloat, on MySQL a gap-locking transaction
// long enough to stall the gateway, on SQLite a write lock held for minutes.
//
// The fix is mechanical: delete in bounded chunks, pause between them so live
// traffic gets the database back, and stop when the caller's context is done.

import (
	"context"
	"strconv"
	"time"

	"github.com/Laisky/errors/v2"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common/config"
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
	// BatchSize bounds rows removed per statement; <= 0 uses the configured
	// default.
	BatchSize int
	// Pause is how long to wait between chunks; <= 0 disables the pause.
	Pause time.Duration
}

// retentionTables is the allow-list of tables the chunked sweeper may target.
// ChunkedDelete interpolates the table name into SQL, so it must never accept
// an arbitrary string.
var retentionTables = map[string]bool{
	"traces":              true,
	"logs":                true,
	"async_task_bindings": true,
}

// ChunkedDelete removes matching rows in bounded batches.
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
	if db == nil {
		return 0, errors.New("database handle is not initialized")
	}
	if !retentionTables[opts.Table] {
		return 0, errors.Errorf("table %q is not an allowed retention target", opts.Table)
	}

	batchSize := opts.BatchSize
	if batchSize <= 0 {
		batchSize = config.RetentionDeleteBatchSize
	}
	if batchSize <= 0 {
		batchSize = 1000
	}

	statement := boundedDeleteStatement(db, opts.Table, opts.Where, batchSize)

	var total int64
	for {
		if err := ctx.Err(); err != nil {
			// A cancelled sweep is reported, not swallowed. Background sweepers
			// treat context.Canceled as ordinary shutdown; an operator-triggered
			// purge must not be able to report success for a partial delete.
			return total, errors.Wrapf(err, "retention sweep of %s stopped after %d rows", opts.Table, total)
		}

		tx := db.WithContext(ctx).Exec(statement, opts.Args...)
		if tx.Error != nil {
			return total, errors.Wrapf(tx.Error, "delete expired rows from %s", opts.Table)
		}

		total += tx.RowsAffected
		if tx.RowsAffected < int64(batchSize) {
			// Short chunk: nothing left to remove.
			return total, nil
		}

		if opts.Pause > 0 {
			timer := time.NewTimer(opts.Pause)
			select {
			case <-ctx.Done():
				timer.Stop()
				return total, errors.Wrapf(ctx.Err(), "retention sweep of %s stopped after %d rows", opts.Table, total)
			case <-timer.C:
			}
		}
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
