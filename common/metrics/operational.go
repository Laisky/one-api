package metrics

// Operational request and retention metrics (proposal
// docs/proposals/20260905_observability-data-tiering.md, Phase 3 / W3.3).
//
// W3.3 asks for operational views "from their actual operational sources", and
// the honest reading of that phrase is a constraint, not a wish list: a metric
// is added here only when this tree already computes the quantity somewhere
// real. Two of the quantities W3.3 names -- dashboard projection lag and
// projection backfill backlog -- have no source at all, because W2.2/W2.3 are
// not implemented. They are deliberately absent rather than reported as zero: a
// gauge that reads 0.0 for a pipeline that does not exist is worse than no
// gauge, since an operator cannot distinguish "healthy" from "not running".
//
// Everything here is SAMPLED-INDEPENDENT. TRACE_SAMPLE_RATE selects which
// traces are persisted; it must not decide which requests are counted, or the
// operational view would silently become a 5% view. The call sites therefore
// record outcomes BEFORE the sampling decision.
//
// None of this is billing truth. Quota and usage remain the authoritative
// financial record; these series are best-effort operational telemetry and may
// be lost on crash, drop, or exporter failure.

import "time"

// Request outcomes. These are the ONLY values that may be passed as the outcome
// label of RecordRequestOutcome and RecordTimeToFirstToken. Every value is a
// compile-time constant so the metric stays low-cardinality; never pass a path,
// model name, user id, channel id, token, or error message as a label.
//
// This vocabulary is owned by this package and is deliberately separate from
// tracing.FailureKind, whose documentation states that its values are not
// metric labels. Call sites map one onto the other explicitly.
const (
	// RequestOutcomeSuccess counts requests that completed with a non-error
	// status and reported no semantic failure.
	RequestOutcomeSuccess = "success"
	// RequestOutcomeClientError counts requests that ended with a 4xx status
	// and no semantic failure -- a caller mistake, not a gateway fault.
	RequestOutcomeClientError = "client_error"
	// RequestOutcomeServerError counts requests that ended with a 5xx status.
	RequestOutcomeServerError = "server_error"
	// RequestOutcomeUpstream counts requests that failed in the relay or
	// upstream, INCLUDING those whose client-visible status is 200 because the
	// stream had already flushed its headers. This is the outcome an HTTP
	// status histogram alone cannot see.
	RequestOutcomeUpstream = "upstream_error"
	// RequestOutcomeTimeout counts requests whose upstream or request deadline
	// expired.
	RequestOutcomeTimeout = "timeout"
	// RequestOutcomeCanceled counts requests whose client went away before
	// completion. These are not gateway faults and must stay separable from
	// server errors when alerting.
	RequestOutcomeCanceled = "canceled"
	// RequestOutcomePanic counts requests that panicked.
	RequestOutcomePanic = "panic"
)

// Retention sweep results. These are the ONLY values that may be passed as the
// result label of RecordRetentionSweep.
const (
	// RetentionResultCompleted counts a sweep that ran to completion.
	RetentionResultCompleted = "completed"
	// RetentionResultFailed counts a sweep that stopped on an error.
	RetentionResultFailed = "failed"
	// RetentionResultCanceled counts a sweep that stopped at a chunk boundary
	// because its context was cancelled, normally by shutdown. Work remained.
	RetentionResultCanceled = "canceled"
)

// RequestOutcomeRecorder is the OPTIONAL extension a recorder may implement to
// receive per-request operational outcomes.
//
// It is deliberately not part of MetricsRecorder: adding methods to that
// interface would stop any out-of-tree implementation from compiling, which the
// project's backward-compatibility rules forbid. Recorders that do not
// implement this interface are skipped.
type RequestOutcomeRecorder interface {
	// RecordRequestOutcome tallies one finished request and its total lifetime.
	RecordRequestOutcome(outcome string, durationMs float64)
	// RecordTimeToFirstToken observes the delay before a request produced its
	// first client byte. It is called only for requests that produced one.
	RecordTimeToFirstToken(outcome string, ttftMs float64)
}

// RetentionRecorder is the OPTIONAL extension a recorder may implement to
// receive retention-sweep throughput.
//
// It is separate from RequestOutcomeRecorder for the same reason that interface
// is separate from MetricsRecorder: adding a method to an existing exported
// interface breaks every out-of-tree implementation of it.
type RetentionRecorder interface {
	// RecordRetentionSweep tallies one finished retention sweep: how many rows
	// it removed from which target, how it ended, and how long it took.
	RecordRetentionSweep(target, result string, rows float64, durationMs float64)
}

// RecordRequestOutcome tallies one finished request under a bounded outcome
// label, independently of whether its trace was sampled.
//
// A recorder that does not implement RequestOutcomeRecorder is skipped, so an
// out-of-tree recorder keeps working without change.
//
// Parameters:
//   - outcome: one of the RequestOutcome* constants.
//   - duration: the request's total lifetime; a negative value is ignored.
//
// Return values: none.
func RecordRequestOutcome(outcome string, duration time.Duration) {
	if duration < 0 {
		return
	}
	if rr, ok := Recorder().(RequestOutcomeRecorder); ok {
		rr.RecordRequestOutcome(outcome, float64(duration.Nanoseconds())/1e6)
	}
}

// RecordRequestOutcomeMillis tallies one finished request whose lifetime is
// already expressed in milliseconds.
//
// The trace pipeline computes durations in milliseconds from recorded
// timestamps rather than from a time.Duration, and converting back and forth
// would lose the exact value the trace row carries.
//
// Parameters:
//   - outcome: one of the RequestOutcome* constants.
//   - durationMs: the request's total lifetime in milliseconds; negative is
//     ignored.
//
// Return values: none.
func RecordRequestOutcomeMillis(outcome string, durationMs int64) {
	if durationMs < 0 {
		return
	}
	if rr, ok := Recorder().(RequestOutcomeRecorder); ok {
		rr.RecordRequestOutcome(outcome, float64(durationMs))
	}
}

// RecordTimeToFirstToken observes how long a request took to produce its first
// client byte.
//
// Streaming relay lifetimes are dominated by the tail of the response, so a
// total-latency histogram cannot answer "how quickly did the caller start
// seeing output". This is the separate signal W1 required the sampler to
// consider and W3.3 requires the operational view to expose.
//
// Parameters:
//   - outcome: one of the RequestOutcome* constants.
//   - ttftMs: milliseconds from request receipt to first client byte; negative
//     is ignored.
//
// Return values: none.
func RecordTimeToFirstToken(outcome string, ttftMs int64) {
	if ttftMs < 0 {
		return
	}
	if rr, ok := Recorder().(RequestOutcomeRecorder); ok {
		rr.RecordTimeToFirstToken(outcome, float64(ttftMs))
	}
}

// RecordRetentionSweep tallies a finished retention sweep.
//
// Section 8.3 makes retention catch-up part of capacity acceptance: a sweep
// that cannot keep pace with newly eligible rows is a topology failure, and
// that can only be judged from measured throughput. This is the source.
//
// Parameters:
//   - target: the swept table or file set. It MUST come from a closed,
//     compile-time set (the retention targets), never from user input.
//   - result: one of the RetentionResult* constants.
//   - rows: how many rows or files the sweep removed.
//   - duration: how long the sweep took; a negative value is ignored.
//
// Return values: none.
func RecordRetentionSweep(target, result string, rows int64, duration time.Duration) {
	if duration < 0 {
		return
	}
	if rr, ok := Recorder().(RetentionRecorder); ok {
		rr.RecordRetentionSweep(target, result, float64(rows),
			float64(duration.Nanoseconds())/1e6)
	}
}
