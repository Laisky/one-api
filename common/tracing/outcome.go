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
