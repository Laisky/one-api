package tracing

import (
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
