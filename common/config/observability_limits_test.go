package config

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestLogDiskRecoveryFloorBytesNeverInvertsHysteresis pins the one property the
// disk guard depends on: the recovery threshold is never BELOW the entry floor.
//
// The naive form of this calculation, `floor + floor*pct/100`, overflows int64
// well before the floor itself does. A wrapped product is negative, so the
// function would return a recovery threshold under the entry floor -- and the
// guard, which enters at `free < floor` and leaves at `free >= recovery`, would
// then leave immediately on every check. Hysteresis would be silently disabled
// in exactly the direction no test of ordinary values can see, because for any
// realistic floor the arithmetic is correct.
func TestLogDiskRecoveryFloorBytesNeverInvertsHysteresis(t *testing.T) {
	original := LogDiskRecoveryMarginPct
	t.Cleanup(func() { LogDiskRecoveryMarginPct = original })

	// Sweep every power of two rather than hand-picking floors. Which values
	// overflow depends on the margin in a way that is not obvious: at 20% only
	// 2^59 and 2^61 invert, while 2^60 and 2^62 wrap to a harmless result. A
	// hand-picked list is how this test first passed against the naive
	// arithmetic it was written to catch.
	floors := []int64{0, 1, 1024, math.MaxInt64 / 2, math.MaxInt64 - 1, math.MaxInt64}
	for shift := 0; shift < 63; shift++ {
		floors = append(floors, int64(1)<<uint(shift))
	}

	for _, pct := range []int{1, 5, 20, 50, 100, 300} {
		LogDiskRecoveryMarginPct = pct
		for _, floor := range floors {
			require.GreaterOrEqual(t, LogDiskRecoveryFloorBytes(floor), floor,
				"recovery threshold must never fall below the entry floor (floor=%d, margin=%d%%)",
				floor, pct)
		}
	}
}

// TestLogDiskRecoveryFloorBytesAppliesTheMargin verifies the ordinary case still
// produces the documented band, so the overflow guard above cannot pass by
// making the function a no-op.
func TestLogDiskRecoveryFloorBytesAppliesTheMargin(t *testing.T) {
	original := LogDiskRecoveryMarginPct
	t.Cleanup(func() { LogDiskRecoveryMarginPct = original })

	LogDiskRecoveryMarginPct = 20
	require.Equal(t, int64(1200), LogDiskRecoveryFloorBytes(1000),
		"a 20%% margin over a 1000-byte floor recovers at 1200")

	LogDiskRecoveryMarginPct = 0
	require.Equal(t, int64(1000), LogDiskRecoveryFloorBytes(1000),
		"a zero margin disables hysteresis and recovers at the floor")

	LogDiskRecoveryMarginPct = 20
	require.Zero(t, LogDiskRecoveryFloorBytes(0),
		"a disabled floor has no recovery threshold")
}

// TestBoundedResourceDefaultsAreEnabled pins the policy that separates a bound
// from a behavior opt-in.
//
// Settings that change what a user observes -- sampling, batched writes, file
// deletion, dashboard staleness -- default to the legacy behavior and must be
// opted into. Settings whose ABSENCE is a defect default to enabled, because
// the deployment running defaults is the one least able to discover the defect.
// A future change that flips one of these back to "unlimited" fails here.
func TestBoundedResourceDefaultsAreEnabled(t *testing.T) {
	require.Positive(t, TraceMaxActiveRecorders,
		"an unbounded active recorder set is an out-of-memory kill, not a safe default")
	require.Positive(t, TraceMaxRecordBytes,
		"per-record size must be bounded so the memory model is provable")
	require.Positive(t, TraceMaxExternalCalls,
		"external-call growth must be bounded for the whole request lifetime")
	require.Positive(t, TraceBatchMaxBytes,
		"the flush-local buffer must be bounded in bytes, not only in rows")
	require.Positive(t, LogMaxActiveFileSizeMB,
		"an unbounded active log file is the W0.2 defect")
	require.Positive(t, LogDiskCheckIntervalSec,
		"the disk guard must sample on its own cadence, not the retention sweep")
	require.Positive(t, LogEmergencyMaxBytesPerSec,
		"the emergency policy needs a byte budget; level escalation alone bounds nothing")
}
