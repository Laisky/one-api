package model

// Batched trace persistence (proposal 20260905_observability-data-tiering.md,
// Phase 1 / W1.3).
//
// The pre-proposal write path issued one INSERT plus five read-modify-write
// UPDATE pairs plus one status UPDATE per request, all on the request
// goroutine. This file provides the two primitives the new path needs instead:
// a pure row builder that a caller can run without touching the database, and a
// multi-row insert that an asynchronous writer calls once per batch.

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/Laisky/errors/v2"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/utils"
)

// TraceRowInput carries the finalized, in-memory state of one request's trace.
// It exists so callers outside this package can build a persistable row without
// knowing the storage layout or the sanitization rules.
type TraceRowInput struct {
	// TraceId is the per-request identifier from gin-middlewares.
	TraceId string
	// URL is the raw request URL; it is sanitized and length-bounded here.
	URL string
	// Method is the HTTP method.
	Method string
	// BodySize is the request body size in bytes.
	BodySize int64
	// Status is the final HTTP status code.
	Status int
	// CreatedAt is the request start time in Unix milliseconds.
	CreatedAt int64
	// Timestamps holds the accumulated lifecycle timestamps and external calls.
	Timestamps *TraceTimestamps
}

// traceTimestampColumns lists the W1.5 projection columns, in the order the
// schema declares them.
var traceTimestampColumns = []string{
	"ts_request_received",
	"ts_request_forwarded",
	"ts_first_upstream_response",
	"ts_first_client_response",
	"ts_upstream_completed",
	"ts_request_completed",
}

// traceColumnProbeInterval bounds how often the schema is re-probed while the
// columns are still missing.
const traceColumnProbeInterval = time.Minute

var (
	traceColumnsMu      sync.Mutex
	traceColumnsPresent bool
	traceColumnsChecked time.Time
)

// traceTimestampColumnsAvailable reports whether the traces table carries the
// per-timestamp columns, caching the answer.
//
// Only the master node runs AutoMigrate (see model.initPrimaryDatabase), so a
// NODE_TYPE=slave process upgraded ahead of its master sees a traces table that
// still lacks these columns. Naming them in an INSERT there would fail every
// trace write for the whole rolling upgrade. Probing instead lets such a node
// degrade to the pre-change column set, which still carries the complete
// timestamp document in the JSON column, and pick the columns up automatically
// once the master migrates.
//
// Parameters:
//   - db: the handle to probe.
//
// Return values:
//   - bool: true when the columns exist and may be written.
func traceTimestampColumnsAvailable(db *gorm.DB) bool {
	if db == nil {
		return false
	}

	traceColumnsMu.Lock()
	defer traceColumnsMu.Unlock()

	if traceColumnsPresent {
		return true
	}
	if !traceColumnsChecked.IsZero() && time.Since(traceColumnsChecked) < traceColumnProbeInterval {
		return false
	}

	traceColumnsChecked = time.Now()
	traceColumnsPresent = true
	for _, column := range traceTimestampColumns {
		if !db.Migrator().HasColumn(&Trace{}, column) {
			traceColumnsPresent = false
			break
		}
	}
	return traceColumnsPresent
}

// ResetTraceColumnProbe clears the cached schema probe.
//
// Parameters: none.
//
// Return values: none.
func ResetTraceColumnProbe() {
	traceColumnsMu.Lock()
	defer traceColumnsMu.Unlock()
	traceColumnsPresent = false
	traceColumnsChecked = time.Time{}
}

// traceWriteSession returns the session to insert trace rows through, omitting
// the per-timestamp columns when the schema does not have them yet.
//
// Parameters:
//   - db: the handle to write through.
//
// Return values:
//   - *gorm.DB: the session to call Create on.
func traceWriteSession(db *gorm.DB) *gorm.DB {
	if traceTimestampColumnsAvailable(db) {
		return db
	}
	return db.Omit(traceTimestampColumns...)
}

// NewTraceRow builds a persistable trace row from finalized request state.
//
// It applies the same URL sanitization and length bound as the synchronous
// CreateTrace path, serializes the timestamp document, and projects that
// document onto the per-timestamp columns.
//
// Parameters:
//   - in: the finalized trace state; Timestamps may be nil.
//
// Return values:
//   - *Trace: the row ready for insertion.
//   - bool: whether the URL was truncated, so the caller can warn once.
//   - error: wrapped failure when the timestamp document cannot be serialized.
func NewTraceRow(in TraceRowInput) (*Trace, bool, error) {
	urlToStore, truncated := SanitizeTraceURL(in.URL)

	timestamps := in.Timestamps
	if timestamps == nil {
		timestamps = &TraceTimestamps{}
	}

	timestampsJSON, err := json.Marshal(timestamps)
	if err != nil {
		return nil, truncated, errors.Wrapf(err, "failed to marshal trace timestamps for trace_id: %s", in.TraceId)
	}

	row := &Trace{
		TraceId:    in.TraceId,
		URL:        urlToStore,
		Method:     in.Method,
		BodySize:   in.BodySize,
		Status:     in.Status,
		Timestamps: string(timestampsJSON),
		CreatedAt:  in.CreatedAt,
	}
	row.applyTimestampColumns(timestamps)

	return row, truncated, nil
}

// InsertTraces writes finalized trace rows using multi-row INSERT statements.
//
// A batch that fails as a whole is retried row by row. This matters because the
// `trace_id` column is unique and a single duplicate — which a client retrying
// with a fixed traceparent can produce — would otherwise discard every other
// trace in the same batch. Duplicates are counted as skipped, not as failures,
// mirroring the best-effort contract of the synchronous CreateTrace path.
//
// Parameters:
//   - ctx: cancellation scope for the write; trace writes intentionally survive
//     request cancellation via traceDBWithContext.
//   - rows: the rows to insert; an empty slice is a no-op.
//   - batchSize: maximum rows per INSERT statement; values below 1 become 1.
//
// Return values:
//   - int: number of rows durably written.
//   - error: wrapped failure describing how many rows could not be written.
func InsertTraces(ctx context.Context, rows []*Trace, batchSize int) (int, error) {
	if len(rows) == 0 {
		return 0, nil
	}
	if batchSize < 1 {
		batchSize = 1
	}

	db := traceDBWithContext(ctx)
	if db == nil {
		return 0, errors.New("trace database handle is not initialized")
	}
	writer := traceWriteSession(db)

	written := 0
	var failed int
	var firstErr error

	for start := 0; start < len(rows); start += batchSize {
		end := min(start+batchSize, len(rows))
		chunk := rows[start:end]

		err := runWithSQLiteBusyRetryForDB(ctx, writer, func() error {
			return errors.WithStack(writer.Create(chunk).Error)
		})
		if err == nil {
			written += len(chunk)
			continue
		}

		// Only retry row by row when the failure is plausibly row-level. A
		// batch-wide failure -- the database is down, the pool is exhausted, the
		// schema is wrong -- would otherwise turn every 500-row batch into 501
		// doomed statements against an already struggling database.
		if !IsDuplicateTraceKeyError(err) {
			failed += len(chunk)
			if firstErr == nil {
				firstErr = err
			}
			continue
		}

		// A duplicate trace id in the batch: insert the rest individually so one
		// retried request cannot discard the other 499 traces.
		for _, row := range chunk {
			if err := runWithSQLiteBusyRetryForDB(ctx, writer, func() error {
				return errors.WithStack(writer.Create(row).Error)
			}); err != nil {
				if IsDuplicateTraceKeyError(err) {
					continue
				}
				failed++
				if firstErr == nil {
					firstErr = err
				}
				continue
			}
			written++
		}
	}

	if firstErr != nil {
		return written, errors.Wrapf(firstErr, "failed to insert %d of %d trace rows", failed, len(rows))
	}
	return written, nil
}

// SanitizeTraceURL applies the storage sanitization and length bound used for
// every persisted or exported trace URL.
//
// URL.String() keeps the raw query bytes, so a request line such as
// "GET /?a=\xc0" arrives as invalid UTF-8; MySQL/PostgreSQL text columns and
// OTLP span attributes both reject that. Sensitive query parameters are
// redacted before the value is ever stored or exported.
//
// Parameters:
//   - raw: the request URL as received.
//
// Return values:
//   - string: the sanitized, length-bounded URL.
//   - bool: whether the value had to be truncated.
func SanitizeTraceURL(raw string) (string, bool) {
	return enforceTraceURLLimit(utils.ToValidUTF8(common.SanitizeURLForLogging(raw)))
}

// IsDuplicateTraceKeyError reports whether err is a unique-constraint violation
// on the traces table.
//
// gorm.ErrDuplicatedKey alone is not enough here: translating driver errors into
// gorm sentinels requires gorm.Config.TranslateError, which this project does
// not enable, so the sentinel never matches in practice. The driver text is
// therefore checked as well, using the same signal vocabulary the controller
// layer already relies on.
//
// Parameters:
//   - err: the error returned by an insert.
//
// Return values:
//   - bool: true when the insert failed because the trace id already exists.
func IsDuplicateTraceKeyError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	msg := strings.ToLower(err.Error())
	for _, signal := range []string{
		"duplicate",
		"unique constraint",
		"unique violation",
		"sqlstate 23505",
		"error 1062",
	} {
		if strings.Contains(msg, signal) {
			return true
		}
	}
	return false
}

// Per-statement placeholder ceilings. SQLITE_MAX_VARIABLE_NUMBER defaults to
// 32766 since SQLite 3.32, which is what the bundled driver ships; MySQL and
// PostgreSQL both cap a statement at 65535 placeholders.
const (
	// MaxStatementParametersSQLite is SQLite's per-statement placeholder ceiling.
	MaxStatementParametersSQLite = 32766
	// MaxStatementParametersDefault is the per-statement placeholder ceiling for
	// MySQL and PostgreSQL.
	MaxStatementParametersDefault = 65535
)

// TraceMaxRowsPerStatement returns how many trace rows one multi-row INSERT may
// carry without exceeding the trace database's parameter ceiling.
//
// The ceiling is read from the DIALECT OF THE HANDLE THAT ACTUALLY WRITES
// TRACES, not from a process-global engine flag. Those agree today because
// traces live on the primary handle, but a deployment that moved `traces` to
// LOG_DB would silently get the wrong ceiling from a global describing the
// primary database -- and the failure mode is not a slow write, it is every
// trace INSERT failing to prepare. Reading the owning handle costs nothing and
// removes the trap.
//
// Parameters:
//   - parametersPerRow: how many placeholders one row binds.
//
// Return values:
//   - int: the maximum rows per statement; always at least 1. When the trace
//     handle is not initialized yet it returns the SMALLER ceiling, because a
//     batch that is too small merely costs an extra statement while one that is
//     too large fails outright.
func TraceMaxRowsPerStatement(parametersPerRow int) int {
	if parametersPerRow < 1 {
		parametersPerRow = 1
	}

	limit := MaxStatementParametersSQLite
	if db := traceDBWithContext(nil); db != nil {
		if dialectName(db) != "sqlite" {
			limit = MaxStatementParametersDefault
		}
	}

	if rows := limit / parametersPerRow; rows >= 1 {
		return rows
	}
	return 1
}
