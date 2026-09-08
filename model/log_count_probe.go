package model

// Bounded log counting (proposal
// docs/proposals/20260905_observability-data-tiering.md, W2.4 items 5 and 6).
//
// A count on the log list must never be allowed to become an unbounded scan,
// and it must never claim to know more than it does. The probe therefore bounds
// the number of MATCHING rows it will look at and reports what it actually
// established:
//
//	exact       - the true number of matching rows at the instant it ran
//	lower_bound - at least this many match; counting stopped at the bound
//	unavailable - the time budget ran out before either could be established
//
// The bound is on matching rows, NOT on rows examined. A selective predicate
// over deep history can still read a great deal of the table to prove that few
// rows match, which is why the time budget and the admission gate exist and why
// "unavailable" is an expected outcome rather than a failure.

import (
	"context"
	"strconv"
	"time"

	"github.com/Laisky/errors/v2"
	"gorm.io/gorm"
)

// LogCountQuality describes how much a reported count actually establishes.
type LogCountQuality string

const (
	// LogCountExact is the true number of matching rows when the count ran.
	LogCountExact LogCountQuality = "exact"
	// LogCountLowerBound means at least Value rows match.
	LogCountLowerBound LogCountQuality = "lower_bound"
	// LogCountUnavailable means the budget expired before anything was known.
	LogCountUnavailable LogCountQuality = "unavailable"
)

// LogCount is a count together with what it establishes and when.
type LogCount struct {
	// Value is the counted rows; nil when Quality is unavailable, so a missing
	// count can never be rendered as zero.
	Value *int64
	// Quality says what Value means.
	Quality LogCountQuality
	// AsOf is when the underlying count ran, in Unix seconds UTC.
	AsOf int64
	// Cached reports whether the value was reused rather than recomputed. A
	// cached exact count is exact as of AsOf, not as of now.
	Cached bool
}

// buildLogCountProbeStatement renders the bounded counting statement.
//
// One shape serves every engine: count the rows of a subquery that stops at the
// bound. The inner LIMIT is what makes the work bounded; it is interpolated as
// a validated integer because MySQL accepts a placeholder there only through
// the prepared-statement protocol.
//
// Parameters:
//   - db: the handle the statement will run on; its dialect selects the hint.
//   - where: the shared predicate fragment.
//   - bound: the maximum number of matching rows to establish.
//   - budget: the time budget, used for the MySQL optimizer hint.
//
// Return values:
//   - string: the SQL statement.
func buildLogCountProbeStatement(db *gorm.DB, where string, bound int, budget time.Duration) string {
	hint := ""
	if dialectName(db) == "mysql" {
		// A server-side ceiling slightly beyond the client deadline, so the
		// server gives up on its own rather than leaving a statement running
		// after the client has stopped waiting.
		ms := budget.Milliseconds() + 500
		hint = "/*+ MAX_EXECUTION_TIME(" + strconv.FormatInt(ms, 10) + ") */ "
	}

	return "SELECT " + hint + "count(*) FROM (SELECT 1 FROM logs WHERE " + where +
		" LIMIT " + strconv.Itoa(bound+1) + ") AS probe"
}

// ProbeLogCount counts matching log rows within a bounded budget.
//
// Parameters:
//   - ctx: the caller's scope; its cancellation is honored and distinguished
//     from budget exhaustion.
//   - scope: the effective authorization scope.
//   - filter: the NORMALIZED filter.
//   - bound: the maximum number of matching rows to establish exactly.
//   - budget: the time budget for the statement.
//
// Return values:
//   - LogCount: what was established.
//   - error: only a genuine query failure; an exhausted budget is reported as
//     an unavailable count, not an error.
func ProbeLogCount(ctx context.Context, scope LogListScope, filter LogListFilter, bound int, budget time.Duration) (LogCount, error) {
	if bound < 1 {
		bound = 1
	}

	where, args := BuildLogListPredicate(scope, filter)
	statement := buildLogCountProbeStatement(LOG_DB, where, bound, budget)

	probeCtx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()

	var counted int64
	err := LOG_DB.WithContext(probeCtx).Raw(statement, args...).Scan(&counted).Error
	now := time.Now().UTC().Unix()

	if err != nil {
		// Budget exhaustion is classified from the probe's own context rather
		// than from driver error text, which differs per engine. A cancelled
		// PARENT is a client disconnect and is reported as such.
		if ctx.Err() != nil {
			return LogCount{Quality: LogCountUnavailable, AsOf: now}, errors.Wrap(ctx.Err(), "probe log count")
		}
		if probeCtx.Err() == context.DeadlineExceeded {
			return LogCount{Quality: LogCountUnavailable, AsOf: now}, nil
		}
		return LogCount{Quality: LogCountUnavailable, AsOf: now}, errors.Wrap(err, "probe log count")
	}

	if counted > int64(bound) {
		value := int64(bound)
		return LogCount{Value: &value, Quality: LogCountLowerBound, AsOf: now}, nil
	}

	value := counted
	return LogCount{Value: &value, Quality: LogCountExact, AsOf: now}, nil
}
