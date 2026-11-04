package metrics

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// installRecorder replaces the process-wide recorder for the duration of a test
// and restores the previous one on cleanup.
//
// Parameters:
//   - t: the running test.
//   - recorder: the recorder to install.
//
// Return values: none.
func installRecorder(t *testing.T, recorder MetricsRecorder) {
	t.Helper()
	previous := Recorder()
	SetRecorder(recorder)
	t.Cleanup(func() { SetRecorder(previous) })
}

// TestRecordChannelHealthCheckReachesRecorder proves the helper forwards every
// outcome to an installed recorder that implements ChannelHealthRecorder, so a
// fail-closed sweep discard actually shows up in the exported metrics.
func TestRecordChannelHealthCheckReachesRecorder(t *testing.T) {
	spy := newCountingRecorder()
	installRecorder(t, spy)

	outcomes := []string{
		ChannelHealthCheckApplied,
		ChannelHealthCheckStatusChanged,
		ChannelHealthCheckTransitionRefused,
		ChannelHealthCheckOptedOut,
		ChannelHealthCheckConfigChanged,
		ChannelHealthCheckApplyFailed,
		ChannelHealthCheckUnreadable,
	}
	for _, outcome := range outcomes {
		RecordChannelHealthCheck(outcome)
	}
	RecordChannelHealthCheck(ChannelHealthCheckApplyFailed)

	for _, outcome := range outcomes {
		want := 1
		if outcome == ChannelHealthCheckApplyFailed {
			want = 2
		}
		require.Equal(t, want, spy.channelHealth[outcome],
			"outcome %s must reach the installed recorder", outcome)
	}
	require.Len(t, spy.channelHealth, len(outcomes),
		"only the recorded outcomes may appear")
}

// TestRecordChannelHealthCheckIsNoOpForNoOpRecorder proves that with metrics
// disabled the helper neither panics nor leaks a sample to a recorder that was
// installed earlier.
func TestRecordChannelHealthCheckIsNoOpForNoOpRecorder(t *testing.T) {
	spy := newCountingRecorder()
	installRecorder(t, spy)
	RecordChannelHealthCheck(ChannelHealthCheckOptedOut)

	installRecorder(t, &NoOpRecorder{})
	require.NotPanics(t, func() {
		RecordChannelHealthCheck(ChannelHealthCheckOptedOut)
		RecordChannelHealthCheck(ChannelHealthCheckUnreadable)
	})

	require.Equal(t, map[string]int{ChannelHealthCheckOptedOut: 1}, spy.channelHealth,
		"a sample recorded while NoOpRecorder is installed must go nowhere")
}
