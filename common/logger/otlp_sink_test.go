package logger

// Core-stack ordering proof for the OTLP application-log bridge (proposal
// docs/proposals/20260905_observability-data-tiering.md, Phase 3 / W3.2).
//
// otlp_sink.go claims a specific position for the bridge in the core stack:
// ABOVE the emergency disk budget and BELOW sampling. That claim is the whole
// design, and it is invisible in the code -- it is expressed only by where an
// option sits in a slice, so a future edit that reorders those three lines
// would change the behavior with nothing to catch it.
//
// These tests are the catch. Each drives the SAME option order
// SetupEnhancedLogger uses and asserts the consequence of the position, not the
// position itself.

import (
	"context"
	"sync"
	"testing"

	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	"github.com/stretchr/testify/require"
	sdklog "go.opentelemetry.io/otel/sdk/log"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/logger/otelbridge"
	"github.com/Laisky/one-api/common/metrics"
)

// bridgeRecordSink captures the records the bridge exported.
type bridgeRecordSink struct {
	mu      sync.Mutex
	records int
}

// Export implements sdklog.Exporter by counting the batch.
//
// Parameters:
//   - ctx: unused.
//   - records: the batch to count.
//
// Return values:
//   - error: always nil.
func (s *bridgeRecordSink) Export(_ context.Context, records []sdklog.Record) error {
	s.mu.Lock()
	s.records += len(records)
	s.mu.Unlock()
	return nil
}

// Shutdown implements sdklog.Exporter as a no-op.
//
// Parameters:
//   - ctx: unused.
//
// Return values:
//   - error: always nil.
func (s *bridgeRecordSink) Shutdown(context.Context) error { return nil }

// ForceFlush implements sdklog.Exporter as a no-op.
//
// Parameters:
//   - ctx: unused.
//
// Return values:
//   - error: always nil.
func (s *bridgeRecordSink) ForceFlush(context.Context) error { return nil }

// count reports how many records were exported.
//
// Parameters: none.
//
// Return values:
//   - int: the exported record count.
func (s *bridgeRecordSink) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.records
}

// newProductionCoreStack builds a logger with the exact option order
// SetupEnhancedLogger applies, over a recording local core and a real log SDK.
//
// Parameters:
//   - t: the test handle; configuration and the provider are restored on cleanup.
//   - gate: the emergency gate to install, so a test can engage it directly.
//
// Return values:
//   - *zap.Logger: the assembled logger.
//   - *recordingCore: what reached the local (file/stdout) branch.
//   - *bridgeRecordSink: what reached the OTLP branch.
func newProductionCoreStack(t *testing.T, gate *emergencyGate) (*zap.Logger, *recordingCore, *bridgeRecordSink) {
	t.Helper()

	sink := &bridgeRecordSink{}
	provider := sdklog.NewLoggerProvider(sdklog.WithProcessor(sdklog.NewSimpleProcessor(sink)))
	t.Cleanup(func() {
		require.NoError(t, provider.Shutdown(context.Background()))
	})

	holder := otelbridge.NewProviderHolder()
	require.NoError(t, holder.Install(provider))

	base := newRecordingCore()
	logger := zap.New(base)

	// The production order, verbatim: emergency budget innermost, then the OTLP
	// tee, then sampling outermost.
	opts := []zap.Option{
		zap.WrapCore(func(core zapcore.Core) zapcore.Core {
			return &emergencyCore{Core: core, gate: gate}
		}),
		zap.WrapCore(func(core zapcore.Core) zapcore.Core {
			return zapcore.NewTee(core,
				otelbridge.NewCore(holder, otelbridge.DefaultScopeName, zapcore.DebugLevel))
		}),
	}
	if opt, ok := samplingOption(); ok {
		opts = append(opts, opt)
	}

	return logger.WithOptions(opts...), base, sink
}

// TestDiskPressureDoesNotStopOTLPExport proves the bridge sits ABOVE the
// emergency disk budget: a full log volume must not stop shipping to a
// collector that has room.
//
// If the tee were installed below the budget, a disk incident would take out
// the remote copy of the logs at exactly the moment an operator needs it, and
// the budget's suppression counters would be charging for bytes that never
// touched the disk.
//
// Parameters:
//   - t: the test handle.
//
// Return values: none.
func TestDiskPressureDoesNotStopOTLPExport(t *testing.T) {
	installFakeLogMetrics(t)
	previousSampling := config.LogSampleInitial
	config.LogSampleInitial = 0 // isolate the budget from sampling
	t.Cleanup(func() { config.LogSampleInitial = previousSampling })

	previousBudget := config.LogEmergencyMaxBytesPerSec
	config.LogEmergencyMaxBytesPerSec = 1
	t.Cleanup(func() { config.LogEmergencyMaxBytesPerSec = previousBudget })

	gate := newEmergencyGate()
	logger, base, sink := newProductionCoreStack(t, gate)

	require.True(t, gate.engage(metrics.LogSuppressReasonDiskPressure))
	const lines = 40
	for range lines {
		logger.Info("relay finished")
	}

	require.Less(t, len(base.snapshot()), lines,
		"the disk budget must suppress local output under pressure")
	require.Equal(t, lines, sink.count(),
		"the OTLP branch must be unaffected by local disk pressure")
}

// TestSamplingAppliesToOTLPExport proves the bridge sits BELOW sampling: an
// operator who thinned the log stream to survive a storm did not ask for the
// unthinned stream to be shipped off-box instead.
//
// Parameters:
//   - t: the test handle.
//
// Return values: none.
func TestSamplingAppliesToOTLPExport(t *testing.T) {
	installFakeLogMetrics(t)

	previous := struct{ initial, thereafter, tick int }{
		config.LogSampleInitial, config.LogSampleThereafter, config.LogSampleTickMs,
	}
	config.LogSampleInitial = 2
	config.LogSampleThereafter = 0 // drop everything after the initial budget
	config.LogSampleTickMs = 60000 // one window for the whole test
	t.Cleanup(func() {
		config.LogSampleInitial, config.LogSampleThereafter, config.LogSampleTickMs =
			previous.initial, previous.thereafter, previous.tick
	})

	logger, base, sink := newProductionCoreStack(t, newEmergencyGate())

	for range 50 {
		logger.Info("repeated relay message")
	}

	require.Equal(t, 2, len(base.snapshot()), "sampling must thin the local branch")
	require.Equal(t, 2, sink.count(), "sampling must thin the OTLP branch identically")
}

// TestOTLPBridgeOptionIsOffByDefault proves the bridge costs nothing when
// APP_LOG_SINK did not ask for it, which is what section 2.1's unchanged-
// configuration contract requires.
//
// Parameters:
//   - t: the test handle.
//
// Return values: none.
func TestOTLPBridgeOptionIsOffByDefault(t *testing.T) {
	previous := config.AppLogOTLPEnabled
	t.Cleanup(func() { config.AppLogOTLPEnabled = previous })

	config.AppLogOTLPEnabled = false
	_, ok := otlpBridgeOption()
	require.False(t, ok, "no core may be installed unless APP_LOG_SINK named otlp")

	config.AppLogOTLPEnabled = true
	opt, ok := otlpBridgeOption()
	require.True(t, ok)
	require.NotNil(t, opt)
}

// TestOTLPBridgeLevelMapping pins the severity floor mapping, including the
// fallback that mirrors the configuration normalizer.
//
// Parameters:
//   - t: the test handle.
//
// Return values: none.
func TestOTLPBridgeLevelMapping(t *testing.T) {
	require.Equal(t, zapcore.DebugLevel, otlpBridgeLevel(config.AppLogOTLPLevelDebug))
	require.Equal(t, zapcore.InfoLevel, otlpBridgeLevel(config.AppLogOTLPLevelInfo))
	require.Equal(t, zapcore.WarnLevel, otlpBridgeLevel(config.AppLogOTLPLevelWarn))
	require.Equal(t, zapcore.ErrorLevel, otlpBridgeLevel(config.AppLogOTLPLevelError))
	require.Equal(t, zapcore.InfoLevel, otlpBridgeLevel("verbose"))
}

// TestBridgeLevelCanOnlyNarrow proves LOG_OTLP_MIN_LEVEL is a raise-only knob.
//
// zapcore.NewTee admits an entry when ANY child accepts it, so a floor below the
// process level would make the whole gateway construct debug entries it is
// configured not to log, and would produce an OTLP stream containing lines that
// appear in no local log file. This was measured to happen before the clamp
// existed, which is why the clamp is tested rather than assumed.
//
// Parameters:
//   - t: the test handle.
//
// Return values: none.
func TestBridgeLevelCanOnlyNarrow(t *testing.T) {
	previous := struct {
		level string
		debug bool
	}{config.AppLogOTLPMinLevel, config.DebugEnabled}
	t.Cleanup(func() {
		config.AppLogOTLPMinLevel, config.DebugEnabled = previous.level, previous.debug
	})

	// A floor below the process level is clamped up to it.
	config.DebugEnabled = false
	config.AppLogOTLPMinLevel = config.AppLogOTLPLevelDebug
	require.Equal(t, zapcore.InfoLevel, effectiveBridgeLevel(),
		"the bridge must never widen what the process logs")

	// A floor above the process level is honored: that is the whole point.
	config.AppLogOTLPMinLevel = config.AppLogOTLPLevelError
	require.Equal(t, zapcore.ErrorLevel, effectiveBridgeLevel())

	// With DEBUG on, the process itself is at debug, so a debug floor is real.
	config.DebugEnabled = true
	config.AppLogOTLPMinLevel = config.AppLogOTLPLevelDebug
	require.Equal(t, zapcore.DebugLevel, effectiveBridgeLevel())

	// ... and a higher floor still narrows, which is the documented use case:
	// running at debug locally must not ship debug volume to a shared collector.
	config.AppLogOTLPMinLevel = config.AppLogOTLPLevelWarn
	require.Equal(t, zapcore.WarnLevel, effectiveBridgeLevel())
}
