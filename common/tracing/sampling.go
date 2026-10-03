package tracing

// Trace sampling (proposal docs/proposals/20260905_observability-data-tiering.md,
// Phase 1 / W1.4).
//
// The decision is made when the request ends rather than when it starts. That
// is the head-plus-tail hybrid the OpenTelemetry community recommends for
// cost-controlled pipelines, but it needs neither a decision_wait buffer nor a
// trace-id-aware load balancer here: one-api already holds the complete trace
// in one process, so status and duration are known for free at decision time.
//
// EFFECTIVE RETAINED FRACTION
//
// TRACE_SAMPLE_RATE is NOT the fraction of traces that are kept. With `p` the
// base rate and `q` the fraction of requests selected by an unconditional rule
// (errors, semantic failures, slow requests, forced traces), the effective
// selection is
//
//	q + (1 - q) * p
//
// before path exclusions and delivery failures. At p=0.05 and q=0.5 that is
// 52.5%, not 5% (proposal section 4, W1). TestEffectiveRetainedFractionMatchesFormula
// measures it against this formula rather than restating it.
//
// SLOW SELECTION AND STREAMING
//
// A streaming relay stays open for as long as the client reads, so its total
// lifetime says almost nothing about whether the request was slow: at
// TRACE_ALWAYS_SAMPLE_SLOW_MS=5000 every stream longer than five seconds would
// be retained unconditionally, driving `q` -- and therefore the effective
// fraction -- towards 1. The slow rule therefore evaluates time-to-first-token
// when it is known, and falls back to the total lifetime only when the request
// never produced a first client byte.

import (
	"math/rand/v2"
	"sync/atomic"

	"github.com/Laisky/one-api/common/config"
)

// sampleDecider draws the uniform random value used by probabilistic sampling.
// It is a package variable so tests can make the sampler deterministic.
var sampleDecider atomic.Pointer[func() float64]

// FailureKind names a semantic request failure that the HTTP status code alone
// does not express.
//
// It exists because a relay failure can still return HTTP 200: once a streaming
// response has flushed its headers the status is fixed at 200, and a later
// upstream error, timeout or disconnect can no longer change it. Selecting on
// status alone therefore drops exactly the traces an operator needs.
//
// The values are a small closed vocabulary, but they are NOT metric labels and
// must never be used as one; they stay inside the sampling decision and the
// request span.
type FailureKind string

const (
	// FailureNone means no semantic failure was reported.
	FailureNone FailureKind = ""
	// FailureUpstream is a relay or upstream error, including one reported
	// after a streamed 200 response.
	FailureUpstream FailureKind = "upstream"
	// FailureTimeout is an upstream or request deadline that expired.
	FailureTimeout FailureKind = "timeout"
	// FailureClientCanceled is the client going away before completion.
	FailureClientCanceled FailureKind = "client_canceled"
	// FailurePanic is a panic observed by the tracing middleware.
	FailurePanic FailureKind = "panic"
)

// IsFailure reports whether the kind denotes an actual failure.
//
// Parameters: none.
//
// Return values:
//   - bool: false only for FailureNone.
func (k FailureKind) IsFailure() bool { return k != FailureNone }

// SampleInput carries everything the completion-time sampler considers.
//
// It is a struct rather than a parameter list so a new signal can be added
// without breaking the call sites that already pass the existing ones.
type SampleInput struct {
	// Status is the final HTTP status code the client received; 0 when none
	// was recorded.
	Status int
	// DurationMs is the total request lifetime in milliseconds, which for a
	// stream is how long the client kept reading.
	DurationMs int64
	// TTFTMs is the time from request receipt to the first client byte, in
	// milliseconds.
	TTFTMs int64
	// TTFTKnown reports whether the request produced a first client byte. It
	// distinguishes an immediate response (0 ms) from no response at all. A
	// positive TTFTMs remains treated as known for compatibility with callers
	// compiled before this field was added.
	TTFTKnown bool
	// Forced marks a trace pinned by ForceTraceSample.
	Forced bool
	// Failure is the semantic failure reported for the request, if any.
	Failure FailureKind
}

// slowMetricMs returns the duration the slow rule is evaluated against.
//
// Parameters: none.
//
// Return values:
//   - int64: time-to-first-token when it is known, otherwise the total
//     lifetime, so a long-lived stream is not retained merely for being long.
func (in SampleInput) slowMetricMs() int64 {
	if in.TTFTKnown || in.TTFTMs > 0 {
		return in.TTFTMs
	}
	return in.DurationMs
}

// SampleDecision reports whether a completed trace should be persisted, using
// HTTP status and total duration only.
//
// It is the narrow form kept for callers that have no semantic failure signal;
// SampleDecisionFor is the full form.
//
// Parameters:
//   - status: the final HTTP status code; 0 when none was recorded.
//   - durationMs: total request duration in milliseconds.
//   - forced: whether the caller pinned this trace for retention.
//
// Return values:
//   - bool: true when the trace must be handed to the sinks.
func SampleDecision(status int, durationMs int64, forced bool) bool {
	return SampleDecisionFor(SampleInput{Status: status, DurationMs: durationMs, Forced: forced})
}

// SampleDecisionFor reports whether a completed trace should be persisted.
//
// Order of evaluation, first match wins:
//  1. a rate of 1 keeps everything, a rate of 0 with no always-keep rule drops
//     everything;
//  2. explicitly forced traces are kept;
//  3. error responses AND semantically failed requests are kept when
//     TRACE_ALWAYS_SAMPLE_ERRORS is set, so a relay failure that still returned
//     HTTP 200 is retained;
//  4. slow responses are kept when TRACE_ALWAYS_SAMPLE_SLOW_MS is set, measured
//     against time-to-first-token rather than total streaming lifetime;
//  5. otherwise the configured probability decides.
//
// Rules 2 to 4 are the unconditional `q` of the effective-fraction formula
// documented at the top of this file.
//
// Parameters:
//   - in: the completed request's sampling signals.
//
// Return values:
//   - bool: true when the trace must be handed to the sinks.
func SampleDecisionFor(in SampleInput) bool {
	if config.TraceSampleRate >= 1 {
		return true
	}
	if in.Forced {
		return true
	}
	if config.TraceAlwaysSampleErrors && (in.Status >= 400 || in.Failure.IsFailure()) {
		return true
	}
	if config.TraceAlwaysSampleSlowMs > 0 && in.slowMetricMs() >= int64(config.TraceAlwaysSampleSlowMs) {
		return true
	}
	if config.TraceSampleRate <= 0 {
		return false
	}
	return randomUnitFloat() < config.TraceSampleRate
}

// randomUnitFloat returns a uniform value in [0, 1).
//
// Parameters: none.
//
// Return values:
//   - float64: the drawn value, from the test override when one is installed.
func randomUnitFloat() float64 {
	if fn := sampleDecider.Load(); fn != nil {
		return (*fn)()
	}
	return rand.Float64()
}

// SetSampleDeciderForTest installs a deterministic source for the sampler and
// returns a function restoring the previous source.
//
// Parameters:
//   - fn: the replacement source; nil restores the default randomness.
//
// Return values:
//   - func(): restores the source that was installed before this call.
func SetSampleDeciderForTest(fn func() float64) func() {
	prev := sampleDecider.Load()
	if fn == nil {
		sampleDecider.Store(nil)
	} else {
		sampleDecider.Store(&fn)
	}
	return func() { sampleDecider.Store(prev) }
}
