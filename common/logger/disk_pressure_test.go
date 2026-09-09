package logger

// Tests for the fast disk-pressure guard and its hysteresis (W0.3, W0.4).

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	errors "github.com/Laisky/errors/v2"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/metrics"
)

// stubFreeDisk replaces the free-space probe for the duration of a test.
//
// Hysteresis is only testable this way: the entry floor, the recovery floor and
// the band between them are distinguished by driving free space to specific
// values, which no test can do to a real filesystem.
//
// Parameters:
//   - t: the test handle; the real probe is restored on cleanup.
//   - free: a pointer whose value is reported as the available bytes.
//
// Return values: none.
func stubFreeDisk(t *testing.T, free *uint64) {
	t.Helper()
	prev := freeDiskProbe
	freeDiskProbe = func(string) (uint64, error) { return *free, nil }
	t.Cleanup(func() { freeDiskProbe = prev })
}

// resetDiskGuardState returns the process-wide guard state to idle, before and
// after a test, so no case inherits an emergency another case engaged.
//
// Parameters:
//   - t: the test handle.
//
// Return values: none.
func resetDiskGuardState(t *testing.T) {
	t.Helper()
	reset := func() {
		diskEmergency.reset()
		belowFloorReport.reset()
		activeFileReport.reset()
		levelEscalated.Store(false)
	}
	level := Logger.Level()
	reset()
	t.Cleanup(func() {
		reset()
		require.NoError(t, Logger.ChangeLevel(level))
	})
}

// TestFreeDiskFloorHysteresis is the flap fix: the pre-W0.4 guard entered and
// left on the SAME threshold, so free space oscillating by a single byte around
// the floor toggled the process log level and the emergency policy on every
// observation -- now every five seconds instead of every 24 hours, which would
// have made the flapping continuous.
func TestFreeDiskFloorHysteresis(t *testing.T) {
	if !diskFreeSupported() {
		t.Skip("free disk inspection is not supported on this platform")
	}

	resetDiskGuardState(t)
	installFakeLogMetrics(t)

	prevMargin := config.LogDiskRecoveryMarginPct
	config.LogDiskRecoveryMarginPct = 20
	t.Cleanup(func() { config.LogDiskRecoveryMarginPct = prevMargin })

	const floor = int64(1000)
	require.Equal(t, int64(1200), diskRecoveryFloor(floor))

	dir := t.TempDir()
	free := uint64(900)
	stubFreeDisk(t, &free)

	// Below the floor: engage.
	_, err := enforceFreeDiskFloor(Logger, dir, floor)
	require.NoError(t, err)
	require.True(t, diskEmergency.engaged.Load(), "free space under the floor must engage the bounded policy")
	require.True(t, levelEscalated.Load())

	// Inside the hysteresis band: stay engaged. Recovering here is what made
	// the old guard flap.
	free = 1100
	_, err = enforceFreeDiskFloor(Logger, dir, floor)
	require.NoError(t, err)
	require.True(t, diskEmergency.engaged.Load(), "the band must not release the policy")
	require.True(t, levelEscalated.Load())

	// Above the recovery floor: release.
	free = 1250
	_, err = enforceFreeDiskFloor(Logger, dir, floor)
	require.NoError(t, err)
	require.False(t, diskEmergency.engaged.Load())
	require.False(t, levelEscalated.Load())

	// Back inside the band from above: still healthy, because entry requires
	// crossing the lower threshold.
	free = 1100
	_, err = enforceFreeDiskFloor(Logger, dir, floor)
	require.NoError(t, err)
	require.False(t, diskEmergency.engaged.Load(), "the band must not engage the policy either")
}

// TestFreeDiskFloorPurgeTargetsRecoveryFloor verifies deletion aims at the
// recovery threshold, not the entry floor: stopping at the floor would leave the
// process one byte away from re-entering the emergency it just left.
func TestFreeDiskFloorPurgeTargetsRecoveryFloor(t *testing.T) {
	if !diskFreeSupported() {
		t.Skip("free disk inspection is not supported on this platform")
	}

	resetDiskGuardState(t)
	installFakeLogMetrics(t)

	prevMargin := config.LogDiskRecoveryMarginPct
	config.LogDiskRecoveryMarginPct = 20
	t.Cleanup(func() { config.LogDiskRecoveryMarginPct = prevMargin })

	dir := t.TempDir()
	writeLogFile(t, dir, "oneapi-20260101.log", 10, 72*time.Hour)
	writeLogFile(t, dir, "oneapi-20260102.log", 10, 48*time.Hour)
	writeLogFile(t, dir, "oneapi-20260103.log", 10, time.Hour)

	free := uint64(900)
	prev := freeDiskProbe
	freeDiskProbe = func(string) (uint64, error) {
		// Each deletion recovers 150 bytes: 1050 clears the 1000 floor but not
		// the 1200 recovery threshold.
		free += 150
		return free - 150, nil
	}
	t.Cleanup(func() { freeDiskProbe = prev })

	deleted, err := enforceFreeDiskFloor(Logger, dir, 1000)
	require.NoError(t, err)
	require.Equal(t, 2, deleted, "purging must continue past the entry floor up to the recovery floor")
	require.False(t, diskEmergency.engaged.Load(), "reaching the recovery floor releases the policy")
}

// TestFreeDiskProbeFailureIsWrapped verifies the guard never returns a bare
// error to its caller.
func TestFreeDiskProbeFailureIsWrapped(t *testing.T) {
	if !diskFreeSupported() {
		t.Skip("free disk inspection is not supported on this platform")
	}

	resetDiskGuardState(t)

	prev := freeDiskProbe
	freeDiskProbe = func(string) (uint64, error) { return 0, errors.New("statfs refused") }
	t.Cleanup(func() { freeDiskProbe = prev })

	_, err := enforceFreeDiskFloor(Logger, t.TempDir(), 1000)
	require.Error(t, err)
	require.Contains(t, err.Error(), "read free disk space")
	require.Contains(t, err.Error(), "statfs refused")
}

// TestActiveFileCeilingEngagesWhenRotationCannotAct verifies the guard bounds
// output instead of silently pretending an unrotatable file is capped. This is
// the ONLY_ONE_LOG_FILE case: there is no rotation sink, so nothing can cut the
// file, and the honest response is to cap what is written to it.
func TestActiveFileCeilingEngagesWhenRotationCannotAct(t *testing.T) {
	resetDiskGuardState(t)
	rec := installFakeLogMetrics(t)

	dir := t.TempDir()
	active := writeLogFile(t, dir, "oneapi.log", 4096, time.Minute)

	require.NoError(t, enforceActiveFileCeiling(Logger, dir, 1024))
	require.True(t, diskEmergency.engaged.Load(), "an over-ceiling active file must engage the bounded policy")
	require.Equal(t, []float64{1}, rec.pressureSeries())

	// Shrinking the file below the ceiling releases it again.
	require.NoError(t, os.WriteFile(active, make([]byte, 16), 0o600))
	require.NoError(t, enforceActiveFileCeiling(Logger, dir, 1024))
	require.False(t, diskEmergency.engaged.Load())
	require.Equal(t, []float64{1, 0}, rec.pressureSeries())
}

// TestActiveFileCeilingDisabled verifies a zero ceiling makes the check inert.
func TestActiveFileCeilingDisabled(t *testing.T) {
	resetDiskGuardState(t)
	installFakeLogMetrics(t)

	dir := t.TempDir()
	writeLogFile(t, dir, "oneapi.log", 4096, time.Minute)

	require.NoError(t, enforceActiveFileCeiling(Logger, dir, 0))
	require.False(t, diskEmergency.engaged.Load())
}

// TestEnforceSizeCeilingCountsTheActiveFile verifies the directory budget
// accounts for the file the logger holds open. Excluding it would report a
// directory as compliant while the one file nothing can delete grows past the
// budget on its own.
func TestEnforceSizeCeilingCountsTheActiveFile(t *testing.T) {
	dir := t.TempDir()
	rotated := writeLogFile(t, dir, "oneapi-20260101.log", 100, 72*time.Hour)
	active := writeLogFile(t, dir, "oneapi-20260103.log", 400, time.Hour)

	// The rotated file alone is inside a 200-byte budget; only counting the
	// active file's 400 bytes makes the directory non-compliant.
	deleted, err := enforceSizeCeiling(Logger, dir, 200)
	require.NoError(t, err)
	require.Equal(t, 1, deleted, "the active file's bytes must push the directory over its budget")
	require.NoFileExists(t, rotated)
	require.FileExists(t, active, "the file the logger holds open is never deleted")
}

// withPressureConfig pins the configuration the guard reads and restores it.
//
// Parameters:
//   - t: the test handle.
//   - minFreeMB: LOG_MIN_FREE_DISK_MB.
//   - maxActiveMB: LOG_MAX_ACTIVE_FILE_SIZE_MB.
//   - maxTotalMB: LOG_MAX_TOTAL_SIZE_MB.
//
// Return values: none.
func withPressureConfig(t *testing.T, minFreeMB, maxActiveMB, maxTotalMB int) {
	t.Helper()
	prevFree, prevActive, prevTotal := config.LogMinFreeDiskMB, config.LogMaxActiveFileSizeMB, config.LogMaxTotalSizeMB
	prevInterval, prevSink := config.LogDiskCheckIntervalSec, config.AppLogSink
	config.LogMinFreeDiskMB = minFreeMB
	config.LogMaxActiveFileSizeMB = maxActiveMB
	config.LogMaxTotalSizeMB = maxTotalMB
	config.LogDiskCheckIntervalSec = 1
	config.AppLogSink = config.AppLogSinkFile
	t.Cleanup(func() {
		config.LogMinFreeDiskMB, config.LogMaxActiveFileSizeMB, config.LogMaxTotalSizeMB = prevFree, prevActive, prevTotal
		config.LogDiskCheckIntervalSec, config.AppLogSink = prevInterval, prevSink
	})
}

// TestPressureGuardDoesNotStartWhenUnconfigured verifies an explicit opt-out:
// disabling both the free-space floor and active-file ceiling starts no worker.
func TestPressureGuardDoesNotStartWhenUnconfigured(t *testing.T) {
	resetDiskGuardState(t)
	installFakeLogMetrics(t)

	before := diskPressureChecks.Load()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	startDiskPressureGuard(ctx, Logger, t.TempDir(), 0, 0)

	require.Equal(t, before, diskPressureChecks.Load(),
		"no floor and no active-file ceiling means no pressure loop")
}

// TestSingleLogFileIgnoresTheProfileActiveFileCap verifies the compatibility
// case where ONLY_ONE_LOG_FILE predates size rotation. A profile-provided cap
// cannot rotate that file, so treating it as a pressure-guard cap would put an
// existing deployment into permanent emergency throttling.
func TestSingleLogFileIgnoresTheProfileActiveFileCap(t *testing.T) {
	resetDiskGuardState(t)
	installFakeLogMetrics(t)

	prevOnly, prevCap, prevFloor, prevTotal :=
		config.OnlyOneLogFile, config.LogMaxActiveFileSizeMB, config.LogMinFreeDiskMB, config.LogMaxTotalSizeMB
	config.OnlyOneLogFile = true
	config.LogMaxActiveFileSizeMB = 1
	config.LogMinFreeDiskMB = 0
	config.LogMaxTotalSizeMB = 0
	t.Cleanup(func() {
		config.OnlyOneLogFile, config.LogMaxActiveFileSizeMB, config.LogMinFreeDiskMB, config.LogMaxTotalSizeMB =
			prevOnly, prevCap, prevFloor, prevTotal
	})

	dir := t.TempDir()
	writeLogFile(t, dir, "oneapi.log", 4096, time.Minute)
	before := diskPressureChecks.Load()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	StartLogRetentionCleaner(ctx, 0, dir)

	require.Equal(t, before, diskPressureChecks.Load(),
		"a profile-provided cap must be inert when the single-file mode cannot rotate")
	require.False(t, diskEmergency.engaged.Load(),
		"legacy single-file mode must not enter permanent emergency throttling")
}

// TestPressureGuardSamplesFasterThanTheRetentionSweep verifies the split: the
// guard runs on LOG_DISK_CHECK_INTERVAL_SEC, not on the retention cadence that
// would have sampled a 62-second reserve once a day.
func TestPressureGuardSamplesFasterThanTheRetentionSweep(t *testing.T) {
	if !diskFreeSupported() {
		t.Skip("free disk inspection is not supported on this platform")
	}

	resetDiskGuardState(t)
	installFakeLogMetrics(t)
	withPressureConfig(t, 1, 0, 0)

	free := uint64(1 << 40)
	stubFreeDisk(t, &free)

	require.Greater(t, config.RetentionSweepInterval(), 10*time.Second,
		"the sweep cadence must be far slower than the guard for this test to mean anything")

	before := diskPressureChecks.Load()
	ctx, cancel := context.WithCancel(context.Background())
	startDiskPressureGuard(ctx, Logger, t.TempDir(), config.LogMinFreeDiskBytes(), 0)

	require.Equal(t, before+1, diskPressureChecks.Load(), "the guard checks once synchronously at startup")

	require.Eventually(t, func() bool {
		return diskPressureChecks.Load() >= before+3
	}, 6*time.Second, 100*time.Millisecond, "the guard must tick at the disk-check interval")

	cancel()
	WaitForLogRetentionCleanerForTests()
}

// TestPressureGuardStopsOnContextCancel verifies both the clean shutdown and
// that shutdown does not leave the process-wide log level or the disk-pressure
// gauge stuck in the degraded state the guard owns.
func TestPressureGuardStopsOnContextCancel(t *testing.T) {
	if !diskFreeSupported() {
		t.Skip("free disk inspection is not supported on this platform")
	}

	resetDiskGuardState(t)
	rec := installFakeLogMetrics(t)
	withPressureConfig(t, 1, 0, 0)

	free := uint64(0)
	stubFreeDisk(t, &free)

	ctx, cancel := context.WithCancel(context.Background())
	startDiskPressureGuard(ctx, Logger, t.TempDir(), 1<<20, 0)
	require.True(t, diskEmergency.engaged.Load(), "a floor no filesystem meets engages the policy")

	cancel()
	WaitForLogRetentionCleanerForTests()

	require.False(t, diskEmergency.engaged.Load(), "shutdown must not leave the process degraded")
	require.False(t, levelEscalated.Load())
	series := rec.pressureSeries()
	require.Equal(t, float64(0), series[len(series)-1])
}

// TestRetentionSweepNoLongerRunsTheDiskGuard verifies the two loops are truly
// independent: an age-only configuration starts the slow sweep and no pressure
// loop, which is what makes the guard's cadence a separate decision.
func TestRetentionSweepNoLongerRunsTheDiskGuard(t *testing.T) {
	resetDiskGuardState(t)
	installFakeLogMetrics(t)
	withPressureConfig(t, 0, 0, 0)

	dir := t.TempDir()
	stale := writeLogFile(t, dir, "oneapi-20260101.log", 100, 96*time.Hour)

	before := diskPressureChecks.Load()
	ctx, cancel := context.WithCancel(context.Background())
	StartLogRetentionCleaner(ctx, 1, dir)

	require.NoFileExists(t, stale, "the slow sweep still expires by age")
	require.Equal(t, before, diskPressureChecks.Load(), "an age-only configuration starts no pressure loop")

	cancel()
	WaitForLogRetentionCleanerForTests()
}

// TestStartLogRetentionCleanerStartsOnlyThePressureGuard verifies the mirror
// case: a floor-only configuration starts the fast loop and no expiry sweep.
func TestStartLogRetentionCleanerStartsOnlyThePressureGuard(t *testing.T) {
	if !diskFreeSupported() {
		t.Skip("free disk inspection is not supported on this platform")
	}

	resetDiskGuardState(t)
	installFakeLogMetrics(t)
	withPressureConfig(t, 1, 0, 0)

	free := uint64(1 << 40)
	stubFreeDisk(t, &free)

	dir := t.TempDir()
	old := writeLogFile(t, dir, "oneapi-20260101.log", 100, 96*time.Hour)

	before := diskPressureChecks.Load()
	ctx, cancel := context.WithCancel(context.Background())
	StartLogRetentionCleaner(ctx, 0, dir)

	require.Equal(t, before+1, diskPressureChecks.Load(), "the floor starts the pressure loop")
	require.FileExists(t, old, "no age limit is configured, so nothing expires")

	cancel()
	WaitForLogRetentionCleanerForTests()
}

// TestCheckDiskPressureReportsFailuresAsWarnings verifies an operational
// failure of the guard is a warning, not an ERROR page: the process is healthy,
// the directory momentarily is not, and the loop retries on the next tick.
func TestCheckDiskPressureReportsFailuresAsWarnings(t *testing.T) {
	if !diskFreeSupported() {
		t.Skip("free disk inspection is not supported on this platform")
	}

	resetDiskGuardState(t)
	installFakeLogMetrics(t)

	prev := freeDiskProbe
	freeDiskProbe = func(string) (uint64, error) { return 0, errors.New("volume unmounted") }
	t.Cleanup(func() { freeDiskProbe = prev })

	base := newRecordingCore()
	lg := loggerOverCore(t, base)

	checkDiskPressure(lg, filepath.Join(t.TempDir(), "gone"), 1<<20, 0)

	entries := base.snapshot()
	require.NotEmpty(t, entries)
	for _, e := range entries {
		require.Equal(t, glog.LevelWarn.String(), e.Level.String(),
			"a transient directory failure must not page as a server fault")
	}
}

// TestDiskPressureRepublishesAnAlreadyActiveState verifies a guard that starts
// before monitoring does not leave the disk-pressure gauge permanently at zero.
func TestDiskPressureRepublishesAnAlreadyActiveState(t *testing.T) {
	resetDiskGuardState(t)
	require.True(t, diskEmergency.engage(metrics.LogSuppressReasonDiskPressure))

	rec := installFakeLogMetrics(t)
	checkDiskPressure(Logger, t.TempDir(), 0, 0)

	require.Equal(t, []float64{1}, rec.pressureSeries(),
		"each observation must publish an emergency that predates metric setup")
}

// TestReasonConstantsStayBounded guards the metric label cardinality contract:
// only compile-time constants may reach RecordLogSuppression.
func TestReasonConstantsStayBounded(t *testing.T) {
	require.Equal(t, "disk_pressure", metrics.LogSuppressReasonDiskPressure)
	require.Equal(t, "active_file_cap", metrics.LogSuppressReasonActiveFileCap)
	require.Equal(t, "writer_failure", metrics.LogSuppressReasonWriterFailure)
}
