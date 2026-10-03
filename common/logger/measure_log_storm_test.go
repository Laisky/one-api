package logger

// MEASUREMENT (not correctness) for W0.3 and W0.4 -- proposal
// docs/proposals/20260905_observability-data-tiering.md, Phase 0 / W0:
//
//	"Separate pressure-check cadence from slow retention sweeps. [...] At
//	 16 MB/s, 1 GB lasts about 62.5 seconds; an hourly guard cannot protect it."
//	"On exhausted headroom, use a bounded emergency policy [...] cap bytes
//	 written, suppress excess including repeated WARN/ERROR."
//
// The correctness assertions are in log_emergency_test.go and
// disk_pressure_test.go. This file reports the two numbers those tests do not:
// how many bytes a sustained error storm actually writes with the budget on and
// off, and what the fast guard loop costs per sample.

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/metrics"
)

// stormWindow is how long each storm arm runs. It must span several one-second
// budget windows so the measurement sees refills rather than a single bucket.
const stormWindow = 3 * time.Second

// countingSyncer is a zapcore.WriteSyncer that tallies what actually reached the
// writer, which is the quantity a full volume cares about.
type countingSyncer struct {
	// bytes is the total encoded bytes written.
	bytes atomic.Int64
	// lines is the number of encoded entries written.
	lines atomic.Int64
}

// Write implements io.Writer.
//
// Parameters:
//   - p: the encoded log line.
//
// Return values:
//   - int: len(p).
//   - error: always nil.
func (c *countingSyncer) Write(p []byte) (int, error) {
	c.bytes.Add(int64(len(p)))
	c.lines.Add(1)
	return len(p), nil
}

// Sync implements zapcore.WriteSyncer.
//
// Parameters: none.
//
// Return values:
//   - error: always nil.
func (c *countingSyncer) Sync() error { return nil }

// newStormLogger builds a logger with the production console encoder, the
// process's WARN-escalated level, and the emergency core installed.
//
// Parameters:
//   - gate: the gate the core consults.
//
// Return values:
//   - *zap.Logger: the logger under measurement.
//   - *countingSyncer: what reached the writer.
func newStormLogger(gate *emergencyGate) (*zap.Logger, *countingSyncer) {
	sink := &countingSyncer{}
	encoderCfg := zap.NewProductionEncoderConfig()
	encoderCfg.EncodeTime = zapcore.ISO8601TimeEncoder

	// zapcore.WarnLevel is exactly what escalateLogLevel raised the process to
	// on exhausted headroom: the pre-remediation containment, complete.
	base := zapcore.NewCore(zapcore.NewConsoleEncoder(encoderCfg), sink, zapcore.WarnLevel)
	return zap.New(&emergencyCore{Core: base, gate: gate}), sink
}

// newPlainStormLogger builds the same logger WITHOUT the emergency core, which
// is the pre-remediation write path.
//
// Parameters: none.
//
// Return values:
//   - *zap.Logger: the logger under measurement.
//   - *countingSyncer: what reached the writer.
func newPlainStormLogger() (*zap.Logger, *countingSyncer) {
	sink := &countingSyncer{}
	encoderCfg := zap.NewProductionEncoderConfig()
	encoderCfg.EncodeTime = zapcore.ISO8601TimeEncoder
	return zap.New(zapcore.NewCore(zapcore.NewConsoleEncoder(encoderCfg), sink, zapcore.WarnLevel)), sink
}

// TestMeasureErrorStormContainment reports the bytes a sustained WARN/ERROR
// storm writes, with the emergency budget engaged and with only the
// pre-remediation level escalation.
// requireMeasurementRun skips a measurement whose cost makes it unsuitable for
// the ordinary pre-merge suite.
//
// The repository convention is that long fixtures are opt-in (see
// ONEAPI_W24_PLANS in model/log_cursor_plan_test.go), so the scenario stays in
// the repository -- the proposal forbids ad-hoc one-off scripts -- without
// putting a multi-gigabyte allocation or a multi-second storm on every
// `go test ./...`.
//
// Parameters:
//   - t: the test to skip.
//
// Return values: none.
func requireMeasurementRun(t *testing.T) {
	t.Helper()
	if os.Getenv("ONEAPI_MEASURE") != "1" {
		t.Skip("set ONEAPI_MEASURE=1 to run this measurement")
	}
}

func TestMeasureErrorStormContainment(t *testing.T) {
	requireMeasurementRun(t)
	installFakeLogMetrics(t)

	prevBudget := config.LogEmergencyMaxBytesPerSec
	config.LogEmergencyMaxBytesPerSec = 1 << 20 // the shipped default, 1 MiB/s
	t.Cleanup(func() { config.LogEmergencyMaxBytesPerSec = prevBudget })

	arms := []struct {
		name   string
		engage bool
	}{
		{name: "before: escalateLogLevel only (level raised to WARN, no byte budget)", engage: false},
		{name: "after: bounded emergency policy (LOG_EMERGENCY_MAX_BYTES_PER_SEC=1048576)", engage: true},
	}

	for _, arm := range arms {
		gate := newEmergencyGate()
		if arm.engage {
			require.True(t, gate.engage(metrics.LogSuppressReasonDiskPressure))
		}
		logger, sink := newStormLogger(gate)

		var (
			wg       sync.WaitGroup
			attempts atomic.Int64
			deadline = time.Now().Add(stormWindow)
		)
		// Four producers, which is what a failing gateway looks like: several
		// request goroutines all logging the same upstream failure.
		for w := range 4 {
			wg.Add(1)
			go func(worker int) {
				defer wg.Done()
				for time.Now().Before(deadline) {
					for range 256 {
						logger.Error("upstream request failed",
							zap.String("channel", "channel-42"),
							zap.String("model", "gpt-4.1-mini"),
							zap.Int("worker", worker),
							zap.String("error", "dial tcp 10.0.0.1:443: connect: connection refused"))
						logger.Warn("channel suspended after repeated failures",
							zap.String("channel", "channel-42"),
							zap.Int("worker", worker))
						attempts.Add(2)
					}
				}
			}(w)
		}
		wg.Wait()
		elapsed := stormWindow

		bytes := sink.bytes.Load()
		lines := sink.lines.Load()
		summary := gate.release(metrics.LogSuppressReasonDiskPressure)

		t.Logf("arm=%q window=%s attempted_lines=%d written_lines=%d written_bytes=%d "+
			"bytes_per_second=%.0f suppressed_lines=%d",
			arm.name, elapsed, attempts.Load(), lines, bytes,
			float64(bytes)/elapsed.Seconds(), summary.Lines)
	}
}

// TestMeasureDiskGuardReactionTime reports the worst-case detection latency the
// cadence split changes, and the measured cost of one guard sample.
//
// The latency figures are ARITHMETIC over the configured intervals, not
// measurements: a 24-hour worst case cannot be observed in a test. The guard
// sample cost is measured.
func TestMeasureDiskGuardReactionTime(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "oneapi-2026-09-08.log"),
		make([]byte, 4<<20), 0o600))

	// A probe that always reports plenty of space, so the measurement times the
	// sampling work rather than a purge.
	prevProbe := freeDiskProbe
	freeDiskProbe = func(string) (uint64, error) { return 1 << 40, nil }
	t.Cleanup(func() { freeDiskProbe = prevProbe })

	installFakeLogMetrics(t)
	lg := Logger

	const samples = 2000
	start := time.Now()
	for range samples {
		checkDiskPressure(lg, dir, 1<<30, 4<<30)
	}
	perSample := time.Since(start) / samples

	t.Logf("measured: one disk-pressure sample costs %s (%d samples, one log file present)",
		perSample, samples)

	type cadence struct {
		profile  string
		sweepMin int
	}
	for _, c := range []cadence{{"standalone", 1440}, {"scaled", 60}, {"external", 60}} {
		before := time.Duration(c.sweepMin) * time.Minute
		after := time.Duration(config.LogDiskCheckIntervalSec) * time.Second
		t.Logf("profile=%s worst_case_detection_before=%s worst_case_detection_after=%s "+
			"improvement=%.0fx [ARITHMETIC from RETENTION_SWEEP_INTERVAL_MINUTES=%d "+
			"and LOG_DISK_CHECK_INTERVAL_SEC=%d]",
			c.profile, before, after, before.Seconds()/after.Seconds(),
			c.sweepMin, config.LogDiskCheckIntervalSec)
	}

	// The headroom arithmetic the proposal states, recomputed here so the report
	// carries it beside the cadence rather than as an unchecked assertion.
	for _, rate := range []int64{1 << 20, 16 << 20, 64 << 20} {
		reserve := int64(1) << 30
		t.Logf("write_rate=%s/s reserve=1GiB time_to_exhaustion=%.1fs "+
			"[ARITHMETIC] guard_samples_available_after=%.0f before_standalone=%.4f",
			byteRate(rate), float64(reserve)/float64(rate),
			float64(reserve)/float64(rate)/float64(config.LogDiskCheckIntervalSec),
			float64(reserve)/float64(rate)/(1440*60))
	}
}

// byteRate renders a byte-per-second figure for the report.
//
// Parameters:
//   - rate: bytes per second.
//
// Return values:
//   - string: a human-readable rate.
func byteRate(rate int64) string {
	switch {
	case rate >= 1<<20:
		return fmt.Sprintf("%dMiB", rate>>20)
	case rate >= 1<<10:
		return fmt.Sprintf("%dKiB", rate>>10)
	default:
		return fmt.Sprintf("%dB", rate)
	}
}

// BenchmarkMeasureEmergencyGateHotPath reports what the bounded policy costs a
// healthy process, which is the state it is in essentially always.
//
// Under RunParallel ns/op is reciprocal throughput, not per-call latency.
func BenchmarkMeasureEmergencyGateHotPath(b *testing.B) {
	arms := []struct {
		name string
		// core reports whether the emergency core is installed at all; false is
		// the pre-remediation logger.
		core bool
		// engaged reports whether the policy is active, which only happens on an
		// exhausted volume. Its cost is dominated by lines being DISCARDED
		// rather than encoded, so it is not comparable with the other two arms.
		engaged bool
	}{
		{name: "before_no_emergency_core", core: false},
		{name: "after_healthy", core: true},
		{name: "after_engaged", core: true, engaged: true},
	}

	for _, arm := range arms {
		b.Run(arm.name, func(b *testing.B) {
			prevBudget := config.LogEmergencyMaxBytesPerSec
			config.LogEmergencyMaxBytesPerSec = 1 << 20
			b.Cleanup(func() { config.LogEmergencyMaxBytesPerSec = prevBudget })

			gate := newEmergencyGate()
			if arm.engaged {
				gate.engage(metrics.LogSuppressReasonDiskPressure)
			}
			logger, _ := newStormLogger(gate)
			if !arm.core {
				logger, _ = newPlainStormLogger()
			}

			b.ReportAllocs()
			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					logger.Warn("channel suspended", zap.String("channel", "channel-42"))
				}
			})
		})
	}
}
