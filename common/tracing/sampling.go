package tracing

// Trace sampling (proposal docs/proposals/20260905_observability-data-tiering.md,
// Phase 1 / W1.4).
//
// The decision is made when the request ends rather than when it starts. That
// is the head-plus-tail hybrid the OpenTelemetry community recommends for
// cost-controlled pipelines, but it needs neither a decision_wait buffer nor a
// trace-id-aware load balancer here: one-api already holds the complete trace
// in one process, so status and duration are known for free at decision time.

import (
	"math/rand/v2"
	"sync/atomic"

	"github.com/Laisky/one-api/common/config"
)

// sampleDecider draws the uniform random value used by probabilistic sampling.
// It is a package variable so tests can make the sampler deterministic.
var sampleDecider atomic.Pointer[func() float64]

// SampleDecision reports whether a completed trace should be persisted.
//
// Order of evaluation, first match wins:
//  1. a rate of 1 keeps everything, a rate of 0 with no always-keep rule drops
//     everything;
//  2. explicitly forced traces are kept;
//  3. error responses are kept when TRACE_ALWAYS_SAMPLE_ERRORS is set;
//  4. slow responses are kept when TRACE_ALWAYS_SAMPLE_SLOW_MS is set;
//  5. otherwise the configured probability decides.
//
// Parameters:
//   - status: the final HTTP status code; 0 when none was recorded.
//   - durationMs: total request duration in milliseconds.
//   - forced: whether the caller pinned this trace for retention.
//
// Return values:
//   - bool: true when the trace must be handed to the sinks.
func SampleDecision(status int, durationMs int64, forced bool) bool {
	if config.TraceSampleRate >= 1 {
		return true
	}
	if forced {
		return true
	}
	if config.TraceAlwaysSampleErrors && status >= 400 {
		return true
	}
	if config.TraceAlwaysSampleSlowMs > 0 && durationMs >= int64(config.TraceAlwaysSampleSlowMs) {
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
