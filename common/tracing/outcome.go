package tracing

// Per-request trace outcome bookkeeping (proposal
// docs/proposals/20260905_observability-data-tiering.md, Phase 1 / W1,
// "Outcome" and "Metrics").
//
// Two things live here that do not belong in the recorder: the exclusion tally,
// which must be counted for requests that never get a recorder at all, and the
// semantic failure a request reports, which the recorder cannot hold because a
// request may be excluded or denied admission before one exists.

import (
	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/Laisky/one-api/common/metrics"
	"github.com/Laisky/one-api/model"
)

const (
	// excludedCountedKey marks a request whose exclusion has already been
	// counted, so the start and end hooks together produce exactly one
	// `excluded` metric sample per request rather than one per hook.
	excludedCountedKey = "one_api.tracing.excluded_counted"
	// failureKindKey holds the FailureKind reported for a request.
	failureKindKey = "one_api.tracing.failure_kind"
)

// noteRequestExcluded counts a request that tracing deliberately skipped.
//
// Path exclusions and TRACE_SINK=none used to return from every hook with no
// metric at all, which made a configured saving indistinguishable from a
// capacity problem: an operator saw neither a sampled_out nor a dropped sample,
// just an absent trace. The tally is deduplicated per request because
// RecordTraceStart and recordTraceEnd both reach it.
//
// Parameters:
//   - c: the gin context of the excluded request; nil or a request-less context
//     is not counted, since no request was actually excluded.
//
// Return values: none.
func noteRequestExcluded(c *gin.Context) {
	if c == nil || c.Request == nil {
		return
	}
	if _, counted := c.Get(excludedCountedKey); counted {
		return
	}
	c.Set(excludedCountedKey, true)
	metrics.RecordTraceOutcome(metrics.TraceOutcomeExcluded, 1)
}

// RecordTraceFailure reports a semantic request failure for sampling.
//
// A relay failure that happens after a streaming response has flushed its
// headers can no longer change the HTTP status: the client already received
// 200, and c.Writer.Status() will keep reporting 200. Recording the failure
// separately is what lets the always-sample-errors rule retain such a trace,
// while the persisted status stays truthful about what the client received.
//
// The first reported failure wins, so a late client disconnect does not mask the
// upstream error that caused it.
//
// Parameters:
//   - c: the gin context of the request; nil or a request-less context is a
//     no-op.
//   - kind: the failure kind; FailureNone is a no-op.
//
// Return values: none.
func RecordTraceFailure(c *gin.Context, kind FailureKind) {
	if c == nil || c.Request == nil || !kind.IsFailure() {
		return
	}
	if existing := TraceFailure(c); existing.IsFailure() {
		return
	}
	c.Set(failureKindKey, kind)

	// Mirror onto the request span so the OTLP path carries the signal too; the
	// `traces` table has no column for it.
	if span := spanFromGin(c); span.IsRecording() {
		span.SetAttributes(attribute.String("one_api.failure", string(kind)))
		span.SetStatus(codes.Error, "")
	}
}

// TraceFailure returns the semantic failure reported for a request.
//
// Parameters:
//   - c: the gin context of the request; nil yields FailureNone.
//
// Return values:
//   - FailureKind: the reported failure, or FailureNone when none was reported.
func TraceFailure(c *gin.Context) FailureKind {
	if c == nil {
		return FailureNone
	}
	v, ok := c.Get(failureKindKey)
	if !ok {
		return FailureNone
	}
	kind, _ := v.(FailureKind)
	return kind
}

// timeToFirstTokenMs computes how long a request took to produce its first
// client byte and whether that byte exists.
//
// Parameters:
//   - in: the finalized row input returned by Recorder.Finish.
//
// Return values:
//   - int64: milliseconds from request receipt to the first client response.
//   - bool: true when the request produced a first client response.
func timeToFirstTokenMs(in model.TraceRowInput) (int64, bool) {
	if in.Timestamps == nil || in.Timestamps.FirstClientResponse == nil {
		return 0, false
	}
	start := in.CreatedAt
	if in.Timestamps.RequestReceived != nil {
		start = *in.Timestamps.RequestReceived
	}
	ttft := *in.Timestamps.FirstClientResponse - start
	if ttft < 0 {
		return 0, false
	}
	return ttft, true
}

// requestOutcome maps a finished request onto the bounded operational outcome
// vocabulary owned by common/metrics.
//
// FailureKind is deliberately NOT passed through as a label. Its documentation
// in sampling.go states that its values are not metric labels: they are a
// sampling vocabulary, free to grow whenever a new signal helps the sampler,
// and a metric label set must instead be closed and stable. The translation is
// therefore explicit and total -- every input produces one of the
// metrics.RequestOutcome* constants, including a FailureKind this function does
// not recognize.
//
// PRECEDENCE, first match wins:
//
//  1. panic          -- FailurePanic
//  2. timeout        -- FailureTimeout
//  3. canceled       -- FailureClientCanceled
//  4. upstream_error -- any other reported semantic failure, INCLUDING a
//     request whose client-visible status is 200 because the
//     stream had already flushed its headers
//  5. server_error   -- status >= 500 with no semantic failure
//  6. client_error   -- status in [400, 500) with no semantic failure
//  7. success        -- everything else
//
// The semantic failure outranks the status on purpose. A failed stream's status
// is pinned at 200 and a proxied upstream error commonly arrives as 502; both
// descriptions are less actionable than "the relay failed", and an outcome
// series that could not separate those from real successes would be exactly the
// blind spot W3.3 exists to remove.
//
// Within the failures the order is by actionability rather than by frequency.
// A panic outranks a timeout because a panicking handler often trips its
// deadline on the way out, and the panic is the fact worth alerting on. A
// timeout outranks a client cancellation because a client that gives up on a
// stalled upstream reports the disconnect it observed, not the cause. A client
// cancellation outranks an upstream error so that callers hanging up cannot
// inflate the gateway's own error rate.
//
// Parameters:
//   - status: the client-visible HTTP status code; a value below 400 (including
//     0, meaning none was recorded) is a success unless a failure was reported.
//   - failure: the semantic failure reported through RecordTraceFailure, if any.
//
// Return values:
//   - string: one of the metrics.RequestOutcome* constants.
func requestOutcome(status int, failure FailureKind) string {
	switch failure {
	case FailurePanic:
		return metrics.RequestOutcomePanic
	case FailureTimeout:
		return metrics.RequestOutcomeTimeout
	case FailureClientCanceled:
		return metrics.RequestOutcomeCanceled
	case FailureUpstream:
		return metrics.RequestOutcomeUpstream
	case FailureNone:
		// Fall through to the status-derived outcomes below.
	default:
		// A FailureKind added to sampling.go without a case here is still a
		// failure, and reporting it as a success would be the one wrong answer.
		// upstream_error is the conservative bucket: it says the relay failed
		// without claiming to know how.
		if failure.IsFailure() {
			return metrics.RequestOutcomeUpstream
		}
	}

	switch {
	case status >= 500:
		return metrics.RequestOutcomeServerError
	case status >= 400:
		return metrics.RequestOutcomeClientError
	default:
		return metrics.RequestOutcomeSuccess
	}
}

// recordRequestOutcome emits the operational counters for one finished request.
//
// SAMPLING INDEPENDENCE. This is called from recordTraceEnd BEFORE the sampling
// decision, so TRACE_SAMPLE_RATE never reduces it. The rate selects which traces
// are PERSISTED; letting it also select which requests are COUNTED would turn
// the operational view into a 5% view of the gateway at the default rate, and an
// operator reading a request-rate or error-rate panel would have no way to tell.
//
// EXACTLY ONCE. The caller reaches this only when Recorder.Finish returned its
// single-shot ok flag, so a panicking handler or a doubly registered middleware
// produces one sample per request rather than two.
//
// EXCLUDED REQUESTS ARE NOT COUNTED, DELIBERATELY. A request on an excluded path
// (TRACE_EXCLUDED_PATH_PREFIXES), a request under TRACE_SINK=none, and a request
// denied recorder admission (TRACE_MAX_ACTIVE_RECORDERS) all return from
// recordTraceEnd before this point, so they contribute no outcome and no
// latency. That is the honest reading rather than a gap: those requests have no
// MEASURED lifetime, because the recorder that stamps request_received is
// exactly what was skipped, and recording a fabricated 0 ms would corrupt the
// latency histogram it lands in. The skip is not silent -- noteRequestExcluded
// counts the exclusion and NewRecorder counts the admission drop, both through
// the trace-pipeline outcome series -- so an operator can always tell "not
// measured" from "measured and healthy". The practical consequence to know is
// that TRACE_SINK=none turns off the per-request operational metrics too.
//
// ORDERING AT STARTUP. main.go installs the real recorder in monitor.InitMonitoring
// well after tracing.InitSinks, so anything recorded in between reaches the
// no-op recorder. For per-request metrics that window closes before the HTTP
// server accepts its first request, so nothing measurable is lost.
//
// Parameters:
//   - status: the client-visible HTTP status code.
//   - failure: the semantic failure reported for the request, if any.
//   - durationMs: the request's total lifetime in milliseconds.
//   - ttftMs: milliseconds to the first client byte; meaningful only when
//     ttftKnown is true.
//   - ttftKnown: whether the request produced a first client byte at all.
//
// Return values: none.
func recordRequestOutcome(status int, failure FailureKind, durationMs, ttftMs int64, ttftKnown bool) {
	outcome := requestOutcome(status, failure)
	metrics.RecordRequestOutcomeMillis(outcome, durationMs)
	if !ttftKnown {
		// A request that never produced a client byte has no time-to-first-token.
		// Recording 0 would put "never answered" in the fastest bucket of the
		// histogram, which is the opposite of the truth.
		return
	}
	metrics.RecordTimeToFirstToken(outcome, ttftMs)
}
