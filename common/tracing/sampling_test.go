package tracing

import (
	"math/rand/v2"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/config"
)

// withSamplingConfig installs sampling configuration for one test and restores
// the previous values afterwards.
//
// Parameters:
//   - t: the test, used to register cleanup.
//   - rate: TRACE_SAMPLE_RATE value.
//   - errors: TRACE_ALWAYS_SAMPLE_ERRORS value.
//   - slowMs: TRACE_ALWAYS_SAMPLE_SLOW_MS value.
//
// Return values: none.
func withSamplingConfig(t *testing.T, rate float64, errors bool, slowMs int) {
	t.Helper()
	prevRate, prevErrors, prevSlow := config.TraceSampleRate, config.TraceAlwaysSampleErrors, config.TraceAlwaysSampleSlowMs
	config.TraceSampleRate, config.TraceAlwaysSampleErrors, config.TraceAlwaysSampleSlowMs = rate, errors, slowMs
	t.Cleanup(func() {
		config.TraceSampleRate, config.TraceAlwaysSampleErrors, config.TraceAlwaysSampleSlowMs = prevRate, prevErrors, prevSlow
	})
}

// TestSampleDecisionRateBoundaries verifies the trivial rates short-circuit.
func TestSampleDecisionRateBoundaries(t *testing.T) {
	t.Run("rate 1 keeps everything", func(t *testing.T) {
		withSamplingConfig(t, 1.0, false, 0)
		require.True(t, SampleDecision(200, 1, false))
	})

	t.Run("rate 0 with no always-keep rule drops everything", func(t *testing.T) {
		withSamplingConfig(t, 0, false, 0)
		require.False(t, SampleDecision(200, 1, false))
		require.False(t, SampleDecision(500, 999999, false))
	})
}

// TestSampleDecisionAlwaysKeepRules verifies errors, slow requests, and forced
// traces survive an otherwise drop-everything sample rate. These rules are what
// make a 5% rate safe to run in production.
func TestSampleDecisionAlwaysKeepRules(t *testing.T) {
	t.Run("errors are kept", func(t *testing.T) {
		withSamplingConfig(t, 0, true, 0)
		require.True(t, SampleDecision(500, 1, false))
		require.True(t, SampleDecision(400, 1, false))
		require.False(t, SampleDecision(399, 1, false))
	})

	t.Run("errors are not kept when the rule is off", func(t *testing.T) {
		withSamplingConfig(t, 0, false, 0)
		require.False(t, SampleDecision(500, 1, false))
	})

	t.Run("slow requests are kept", func(t *testing.T) {
		withSamplingConfig(t, 0, false, 5000)
		require.True(t, SampleDecision(200, 5000, false))
		require.True(t, SampleDecision(200, 5001, false))
		require.False(t, SampleDecision(200, 4999, false))
	})

	t.Run("forced traces are kept", func(t *testing.T) {
		withSamplingConfig(t, 0, false, 0)
		require.True(t, SampleDecision(200, 1, true))
	})
}

// TestSampleDecisionProbabilistic verifies the drawn value is compared against
// the configured rate with the expected strictness.
func TestSampleDecisionProbabilistic(t *testing.T) {
	withSamplingConfig(t, 0.05, false, 0)

	restore := SetSampleDeciderForTest(func() float64 { return 0.049 })
	require.True(t, SampleDecision(200, 1, false))
	restore()

	restore = SetSampleDeciderForTest(func() float64 { return 0.05 })
	require.False(t, SampleDecision(200, 1, false), "the draw is compared with <, so an equal draw is dropped")
	restore()

	restore = SetSampleDeciderForTest(func() float64 { return 0.9 })
	require.False(t, SampleDecision(200, 1, false))
	restore()
}

// TestSetSampleDeciderForTestRestores verifies the test hook is reversible so a
// deterministic source never leaks into another test.
func TestSetSampleDeciderForTestRestores(t *testing.T) {
	restore := SetSampleDeciderForTest(func() float64 { return 0.5 })
	require.InDelta(t, 0.5, randomUnitFloat(), 1e-9)
	restore()
	require.Nil(t, sampleDecider.Load())
}

// TestSampleDecisionSemanticFailureSelection verifies selection is no longer
// HTTP-status-only.
//
// A streaming relay that fails after its headers are flushed keeps HTTP 200
// forever, so status-only selection dropped exactly the traces worth keeping.
func TestSampleDecisionSemanticFailureSelection(t *testing.T) {
	t.Run("a failed request that returned 200 is kept", func(t *testing.T) {
		withSamplingConfig(t, 0, true, 0)

		require.False(t, SampleDecisionFor(SampleInput{Status: 200}),
			"a successful 200 must still obey the sample rate")

		for _, kind := range []FailureKind{
			FailureUpstream, FailureTimeout, FailureClientCanceled, FailurePanic,
		} {
			require.True(t, SampleDecisionFor(SampleInput{Status: 200, Failure: kind}),
				"failure kind %q must be retained", kind)
		}
	})

	t.Run("semantic failures honour the always-sample-errors switch", func(t *testing.T) {
		withSamplingConfig(t, 0, false, 0)
		require.False(t, SampleDecisionFor(SampleInput{Status: 200, Failure: FailureUpstream}),
			"TRACE_ALWAYS_SAMPLE_ERRORS=false must disable semantic selection too")
	})

	t.Run("FailureNone is not a failure", func(t *testing.T) {
		withSamplingConfig(t, 0, true, 0)
		require.False(t, FailureNone.IsFailure())
		require.False(t, SampleDecisionFor(SampleInput{Status: 200, Failure: FailureNone}))
	})
}

// TestSlowRuleUsesTimeToFirstTokenNotStreamLifetime verifies the slow rule is
// evaluated against time-to-first-token, so a long-lived stream is not retained
// merely for staying open.
func TestSlowRuleUsesTimeToFirstTokenNotStreamLifetime(t *testing.T) {
	withSamplingConfig(t, 0, false, 5000)

	require.False(t, SampleDecisionFor(SampleInput{Status: 200, DurationMs: 600_000, TTFTMs: 120, TTFTKnown: true}),
		"a ten-minute stream that answered in 120ms is not a slow request")
	require.True(t, SampleDecisionFor(SampleInput{Status: 200, DurationMs: 600_000, TTFTMs: 6000, TTFTKnown: true}),
		"six seconds to the first token is a slow request")
	require.True(t, SampleDecisionFor(SampleInput{Status: 200, DurationMs: 600_000}),
		"with no first client byte the total lifetime is all there is")
}

// measureRetainedFraction runs the sampler over a synthetic population and
// returns the fraction it retained.
//
// Parameters:
//   - t: the test, used for helper bookkeeping.
//   - total: population size.
//   - build: builds the i-th request's sampling input.
//
// Return values:
//   - float64: retained count divided by total.
func measureRetainedFraction(t *testing.T, total int, build func(i int) SampleInput) float64 {
	t.Helper()

	// A seeded generator keeps the measurement deterministic: this test asserts
	// a formula, and a flaky sampler would make the assertion about scheduling.
	rng := rand.New(rand.NewPCG(0x5eed_1234, 0xf00d_5678))
	restore := SetSampleDeciderForTest(rng.Float64)
	t.Cleanup(restore)

	kept := 0
	for i := range total {
		if SampleDecisionFor(build(i)) {
			kept++
		}
	}
	return float64(kept) / float64(total)
}

// TestEffectiveRetainedFractionMatchesFormula is the G1 requirement from the
// proposal: measure the effective retained fraction rather than restating
// TRACE_SAMPLE_RATE.
//
// With base rate p and unconditional-rule fraction q, the effective selection is
// q + (1-q)*p. At p=0.05 and q=0.5 that is 52.5%, an order of magnitude above
// the 5% an operator would read off the configuration.
func TestEffectiveRetainedFractionMatchesFormula(t *testing.T) {
	const (
		p     = 0.05
		q     = 0.5
		total = 200_000
	)
	expected := q + (1-q)*p
	require.InDelta(t, 0.525, expected, 1e-9, "the formula must reproduce the proposal's worked example")

	t.Run("q from semantic failures", func(t *testing.T) {
		withSamplingConfig(t, p, true, 0)
		measured := measureRetainedFraction(t, total, func(i int) SampleInput {
			in := SampleInput{Status: 200, DurationMs: 10, TTFTMs: 5, TTFTKnown: true}
			if i%2 == 0 {
				in.Failure = FailureUpstream
			}
			return in
		})
		require.InDelta(t, expected, measured, 0.005)
		require.Greater(t, measured, 10*p,
			"the retained fraction is an order of magnitude above the configured rate")
	})

	t.Run("q from HTTP error status", func(t *testing.T) {
		withSamplingConfig(t, p, true, 0)
		measured := measureRetainedFraction(t, total, func(i int) SampleInput {
			in := SampleInput{Status: 200, DurationMs: 10, TTFTMs: 5, TTFTKnown: true}
			if i%2 == 0 {
				in.Status = 500
			}
			return in
		})
		require.InDelta(t, expected, measured, 0.005)
	})

	t.Run("q of zero collapses to the base rate", func(t *testing.T) {
		withSamplingConfig(t, p, true, 0)
		measured := measureRetainedFraction(t, total, func(int) SampleInput {
			return SampleInput{Status: 200, DurationMs: 10, TTFTMs: 5, TTFTKnown: true}
		})
		require.InDelta(t, p, measured, 0.005)
	})
}

// TestStreamingLifetimeDoesNotInflateEffectiveFraction measures the reason the
// slow rule uses time-to-first-token, on a realistic streaming population.
//
// Every request here is a ten-minute stream that answered in 120 ms. Judged by
// total lifetime, every one of them trips TRACE_ALWAYS_SAMPLE_SLOW_MS=5000, so
// q becomes 1 and a 5% configuration retains 100% of traces. Judged by
// time-to-first-token, q is 0 and the configuration means what it says.
func TestStreamingLifetimeDoesNotInflateEffectiveFraction(t *testing.T) {
	const (
		p     = 0.05
		total = 100_000
	)
	withSamplingConfig(t, p, false, 5000)

	byTTFT := measureRetainedFraction(t, total, func(int) SampleInput {
		return SampleInput{Status: 200, DurationMs: 600_000, TTFTMs: 120, TTFTKnown: true}
	})
	require.InDelta(t, p, byTTFT, 0.005)

	byLifetime := measureRetainedFraction(t, total, func(int) SampleInput {
		return SampleInput{Status: 200, DurationMs: 600_000}
	})
	require.InDelta(t, 1.0, byLifetime, 1e-9,
		"total streaming lifetime would retain everything at the same configuration")
}

// TestSampleDecisionDelegatesToSampleDecisionFor verifies the narrow form stays
// exactly the status-and-duration behavior its existing callers rely on.
func TestSampleDecisionDelegatesToSampleDecisionFor(t *testing.T) {
	withSamplingConfig(t, 0, true, 5000)

	require.True(t, SampleDecision(500, 1, false))
	require.True(t, SampleDecision(200, 5000, false))
	require.True(t, SampleDecision(200, 1, true))
	require.False(t, SampleDecision(200, 1, false))
}

// TestSlowRuleTreatsZeroTTFTAsKnown verifies an immediate first byte does not
// fall back to the full streaming lifetime merely because both values are zero.
func TestSlowRuleTreatsZeroTTFTAsKnown(t *testing.T) {
	withSamplingConfig(t, 0, false, 5000)

	require.False(t, SampleDecisionFor(SampleInput{
		Status: 200, DurationMs: 600_000, TTFTMs: 0, TTFTKnown: true,
	}), "a stream with an immediate first byte must not be selected as slow")
}
