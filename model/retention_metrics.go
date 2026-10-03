package model

// Retention sweep throughput metrics (proposal
// docs/proposals/20260905_observability-data-tiering.md, Phase 3 / W3.3, and
// section 8.3).
//
// Section 8.3 makes retention catch-up part of capacity acceptance: a sweep that
// cannot keep pace with newly eligible rows is a topology failure. The sweep
// logs already report the backlog, but a log line cannot be graphed or alerted
// on, so the same completion points also feed the operational metrics here.
//
// The result label is the part that is easy to get wrong. A sweep interrupted by
// shutdown stops at a chunk boundary with work remaining, and reporting that as
// `completed` would make a gateway that is permanently behind look healthy: every
// sweep would "finish" because every sweep was cut short. ChunkedDeleteWithStats
// reports cancellation as a wrapped context error precisely so this distinction
// survives to here.

import (
	"context"
	"time"

	"github.com/Laisky/errors/v2"

	"github.com/Laisky/one-api/common/metrics"
)

// retentionSweepResult classifies how a finished retention sweep ended.
//
// Parameters:
//   - err: the error the sweep returned, already wrapped by its caller; nil
//     means the sweep drained its eligible range.
//
// Return values:
//   - string: one of the metrics.RetentionResult* constants. A context
//     cancellation or deadline is `canceled` -- work remained -- and every other
//     error is `failed`.
func retentionSweepResult(err error) string {
	switch {
	case err == nil:
		return metrics.RetentionResultCompleted
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return metrics.RetentionResultCanceled
	default:
		return metrics.RetentionResultFailed
	}
}

// recordRetentionSweep reports one finished retention sweep to the operational
// metrics.
//
// The target label is checked against retentionTables rather than trusted. That
// map is already the allow-list deciding which table names may be interpolated
// into SQL, so reusing it keeps exactly one closed vocabulary for both the
// statement and the metric label, and a future target added to one is
// impossible to forget in the other. Every caller passes a compile-time
// constant, so the check is unreachable in practice; it exists so that no future
// caller can widen the label set by passing a computed name.
//
// Parameters:
//   - target: the swept table; must be a key of retentionTables, otherwise
//     nothing is recorded.
//   - stats: what the sweep did; only the deleted-row count is exported here,
//     because the backlog measures already have their own log fields and would
//     need a gauge per target rather than a throughput counter.
//   - err: the sweep's error, used only to classify the result.
//   - elapsed: how long the sweep took, measured around the sweep itself.
//
// Return values: none.
func recordRetentionSweep(target string, stats ChunkedDeleteStats, err error, elapsed time.Duration) {
	if _, allowed := retentionTables[target]; !allowed {
		return
	}
	metrics.RecordRetentionSweep(target, retentionSweepResult(err), stats.Deleted, elapsed)
}
