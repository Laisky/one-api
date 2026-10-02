package logger

// Tests for the bounded emergency logging policy (W0.4).

import (
	"strings"
	"sync"
	"testing"
	"time"

	errors "github.com/Laisky/errors/v2"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/metrics"
)

// recordedEntry is one entry a recordingCore accepted.
type recordedEntry struct {
	Level   zapcore.Level
	Message string
	Fields  []zapcore.Field
}

// recordingCore is a terminal core that keeps every entry it is asked to write
// and can be told to fail.
type recordingCore struct {
	zapcore.LevelEnabler

	mu       sync.Mutex
	entries  []recordedEntry
	bound    []zapcore.Field
	failWith error
	writes   int
}

// newRecordingCore builds a recording core enabled from debug upward.
//
// Parameters: none.
//
// Return values:
//   - *recordingCore: the core, ready to be wrapped.
func newRecordingCore() *recordingCore {
	return &recordingCore{LevelEnabler: zapcore.DebugLevel}
}

// With implements zapcore.Core. Derived cores share the recorder so a test sees
// every entry regardless of which logger emitted it.
//
// Parameters:
//   - fields: fields to bind.
//
// Return values:
//   - zapcore.Core: a core with the fields bound.
func (c *recordingCore) With(fields []zapcore.Field) zapcore.Core {
	c.mu.Lock()
	defer c.mu.Unlock()
	return &derivedRecordingCore{parent: c, bound: append(append([]zapcore.Field{}, c.bound...), fields...)}
}

// Check implements zapcore.Core by adding itself when the level is enabled.
//
// Parameters:
//   - entry: the entry being logged.
//   - ce: the checked entry accumulated so far.
//
// Return values:
//   - *zapcore.CheckedEntry: ce, with this core added when enabled.
func (c *recordingCore) Check(entry zapcore.Entry, ce *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	if c.Enabled(entry.Level) {
		return ce.AddCore(entry, c)
	}
	return ce
}

// Write implements zapcore.Core by recording the entry.
//
// Parameters:
//   - entry: the entry to write.
//   - fields: the entry's fields.
//
// Return values:
//   - error: the configured failure, or nil.
func (c *recordingCore) Write(entry zapcore.Entry, fields []zapcore.Field) error {
	return c.record(entry, nil, fields)
}

// record stores one entry and reports the configured outcome.
//
// Parameters:
//   - entry: the entry to write.
//   - bound: fields bound by a derived core.
//   - fields: the entry's own fields.
//
// Return values:
//   - error: the configured failure, or nil.
func (c *recordingCore) record(entry zapcore.Entry, bound, fields []zapcore.Field) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.writes++
	if c.failWith != nil {
		return c.failWith
	}
	all := append(append([]zapcore.Field{}, bound...), fields...)
	c.entries = append(c.entries, recordedEntry{Level: entry.Level, Message: entry.Message, Fields: all})
	return nil
}

// Sync implements zapcore.Core.
//
// Parameters: none.
//
// Return values:
//   - error: always nil.
func (c *recordingCore) Sync() error { return nil }

// Fields implements zapcore.Core (an addition in the Laisky zap fork).
//
// Parameters: none.
//
// Return values:
//   - []zapcore.Field: the bound fields.
func (c *recordingCore) Fields() []zapcore.Field {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.bound
}

// snapshot returns a copy of the recorded entries.
//
// Parameters: none.
//
// Return values:
//   - []recordedEntry: every entry accepted so far.
func (c *recordingCore) snapshot() []recordedEntry {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]recordedEntry{}, c.entries...)
}

// writeCount returns how many times Write was called, including failures.
//
// Parameters: none.
//
// Return values:
//   - int: the call count.
func (c *recordingCore) writeCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.writes
}

// fail makes every subsequent write return err.
//
// Parameters:
//   - err: the failure to report, or nil to stop failing.
//
// Return values: none.
func (c *recordingCore) fail(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.failWith = err
}

// derivedRecordingCore is what recordingCore.With returns, so bound fields are
// visible without splitting the recorded history across cores.
type derivedRecordingCore struct {
	parent *recordingCore
	bound  []zapcore.Field
}

// Enabled implements zapcore.Core.
//
// Parameters:
//   - level: the level being tested.
//
// Return values:
//   - bool: whether the parent enables the level.
func (c *derivedRecordingCore) Enabled(level zapcore.Level) bool { return c.parent.Enabled(level) }

// With implements zapcore.Core.
//
// Parameters:
//   - fields: fields to bind.
//
// Return values:
//   - zapcore.Core: a core with the fields bound.
func (c *derivedRecordingCore) With(fields []zapcore.Field) zapcore.Core {
	return &derivedRecordingCore{parent: c.parent, bound: append(append([]zapcore.Field{}, c.bound...), fields...)}
}

// Check implements zapcore.Core.
//
// Parameters:
//   - entry: the entry being logged.
//   - ce: the checked entry accumulated so far.
//
// Return values:
//   - *zapcore.CheckedEntry: ce, with this core added when enabled.
func (c *derivedRecordingCore) Check(entry zapcore.Entry, ce *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	if c.Enabled(entry.Level) {
		return ce.AddCore(entry, c)
	}
	return ce
}

// Write implements zapcore.Core.
//
// Parameters:
//   - entry: the entry to write.
//   - fields: the entry's fields.
//
// Return values:
//   - error: whatever the parent reports.
func (c *derivedRecordingCore) Write(entry zapcore.Entry, fields []zapcore.Field) error {
	return c.parent.record(entry, c.bound, fields)
}

// Sync implements zapcore.Core.
//
// Parameters: none.
//
// Return values:
//   - error: always nil.
func (c *derivedRecordingCore) Sync() error { return nil }

// Fields implements zapcore.Core (an addition in the Laisky zap fork).
//
// Parameters: none.
//
// Return values:
//   - []zapcore.Field: the bound fields.
func (c *derivedRecordingCore) Fields() []zapcore.Field { return c.bound }

// fakeLogMetrics captures the containment metrics the policy publishes.
type fakeLogMetrics struct {
	*metrics.NoOpRecorder

	mu       sync.Mutex
	lines    map[string]int
	bytes    map[string]int64
	pressure []float64
}

// installFakeLogMetrics installs a capturing recorder for the duration of a
// test.
//
// Parameters:
//   - t: the test handle; the previous recorder is restored on cleanup.
//
// Return values:
//   - *fakeLogMetrics: the installed recorder.
func installFakeLogMetrics(t *testing.T) *fakeLogMetrics {
	t.Helper()
	rec := &fakeLogMetrics{
		NoOpRecorder: &metrics.NoOpRecorder{},
		lines:        map[string]int{},
		bytes:        map[string]int64{},
	}
	metrics.SetRecorder(rec)
	t.Cleanup(func() { metrics.SetRecorder(nil) })
	return rec
}

// RecordLogSuppression implements metrics.LogPipelineRecorder.
//
// Parameters:
//   - reason: the bounded suppression reason.
//   - lines: how many lines were discarded.
//   - bytes: their estimated size.
//
// Return values: none.
func (f *fakeLogMetrics) RecordLogSuppression(reason string, lines int, bytes int64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lines[reason] += lines
	f.bytes[reason] += bytes
}

// UpdateLogDiskPressure implements metrics.LogPipelineRecorder.
//
// Parameters:
//   - active: 1 while degraded, 0 otherwise.
//
// Return values: none.
func (f *fakeLogMetrics) UpdateLogDiskPressure(active float64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pressure = append(f.pressure, active)
}

// suppressed reports the tally for one reason.
//
// Parameters:
//   - reason: the suppression reason to read.
//
// Return values:
//   - int: discarded lines.
//   - int64: their estimated bytes.
func (f *fakeLogMetrics) suppressed(reason string) (int, int64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.lines[reason], f.bytes[reason]
}

// pressureSeries returns every disk-pressure gauge value published so far.
//
// Parameters: none.
//
// Return values:
//   - []float64: the gauge values, in order.
func (f *fakeLogMetrics) pressureSeries() []float64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]float64{}, f.pressure...)
}

// newEmergencyHarness builds a logger whose core is bounded by a fresh gate on
// a frozen clock.
//
// Parameters:
//   - t: the test handle.
//   - budget: LOG_EMERGENCY_MAX_BYTES_PER_SEC for the duration of the test.
//
// Return values:
//   - *zap.Logger: the logger under test.
//   - *recordingCore: what actually reached the writer.
//   - *emergencyGate: the gate to engage and release.
//   - *time.Time: the frozen clock, advanced by assigning through it.
func newEmergencyHarness(t *testing.T, budget int) (*zap.Logger, *recordingCore, *emergencyGate, *time.Time) {
	t.Helper()

	prev := config.LogEmergencyMaxBytesPerSec
	config.LogEmergencyMaxBytesPerSec = budget
	t.Cleanup(func() { config.LogEmergencyMaxBytesPerSec = prev })

	clock := time.Date(2026, time.September, 5, 12, 0, 0, 0, time.UTC)
	gate := newEmergencyGate()
	gate.setClock(func() time.Time { return clock })

	base := newRecordingCore()
	logger := zap.New(&emergencyCore{Core: base, gate: gate})
	return logger, base, gate, &clock
}

// TestEmergencyGateInertWhenHealthy verifies the policy costs nothing and drops
// nothing while the disk is fine, which is the state the process is in
// essentially always.
func TestEmergencyGateInertWhenHealthy(t *testing.T) {
	rec := installFakeLogMetrics(t)
	logger, base, _, _ := newEmergencyHarness(t, 1)

	for range 50 {
		logger.Info("record log")
	}

	require.Len(t, base.snapshot(), 50, "nothing may be suppressed while healthy")
	lines, _ := rec.suppressed(metrics.LogSuppressReasonDiskPressure)
	require.Zero(t, lines)
}

// TestEmergencyGateBoundsEveryLevel is the guarantee escalateLogLevel could not
// give: once engaged, WARN and ERROR are bounded too, so an error storm cannot
// fill the volume the guard is trying to save.
func TestEmergencyGateBoundsEveryLevel(t *testing.T) {
	rec := installFakeLogMetrics(t)
	logger, base, gate, _ := newEmergencyHarness(t, 300)

	require.True(t, gate.engage(metrics.LogSuppressReasonDiskPressure))

	for range 100 {
		logger.Error("upstream failure")
	}
	for range 100 {
		logger.Warn("channel suspended")
	}
	for range 100 {
		logger.Info("record log")
	}

	written := base.snapshot()
	require.NotEmpty(t, written, "the budget admits some output rather than going silent")
	require.Less(t, len(written), 300, "warn and error must be bounded too, not only info")

	var admittedBytes int
	for _, e := range written {
		admittedBytes += encodedEntryBytes(zapcore.Entry{Level: e.Level, Message: e.Message}, e.Fields)
	}
	require.LessOrEqual(t, admittedBytes, 300, "no more than one second of budget may be admitted")

	// The gate flushes suppression accounting when the window rolls or when it
	// is released, so release before asserting.
	summary := gate.release(metrics.LogSuppressReasonDiskPressure)
	require.True(t, summary.Released)
	require.Equal(t, int64(300-len(written)), summary.Lines)

	lines, bytes := rec.suppressed(metrics.LogSuppressReasonDiskPressure)
	require.Equal(t, 300-len(written), lines)
	require.Positive(t, bytes)
}

// TestEmergencyGateRefillsEachSecond verifies the budget is per second rather
// than a one-off allowance, so a long emergency keeps a trickle of output.
func TestEmergencyGateRefillsEachSecond(t *testing.T) {
	installFakeLogMetrics(t)
	logger, base, gate, clock := newEmergencyHarness(t, 200)
	gate.setClock(func() time.Time { return *clock })

	gate.engage(metrics.LogSuppressReasonDiskPressure)

	for range 20 {
		logger.Info("record log")
	}
	first := len(base.snapshot())
	require.Positive(t, first)

	*clock = clock.Add(2 * time.Second)
	for range 20 {
		logger.Info("record log")
	}
	require.Greater(t, len(base.snapshot()), first, "the budget must refill on the next window")
}

// TestEmergencyWriterFailureIsCountedNotLogged verifies the recursion guard: a
// failing writer is tallied and the failure is NOT written back through the
// logger that just failed.
func TestEmergencyWriterFailureIsCountedNotLogged(t *testing.T) {
	rec := installFakeLogMetrics(t)
	logger, base, _, _ := newEmergencyHarness(t, 1<<20)

	base.fail(errors.New("no space left on device"))
	logger.Info("record log")

	require.Equal(t, 1, base.writeCount(), "a failed write must not be followed by a report through the same core")
	require.Empty(t, base.snapshot())

	lines, bytes := rec.suppressed(metrics.LogSuppressReasonWriterFailure)
	require.Equal(t, 1, lines)
	require.Positive(t, bytes)
}

// TestEmergencyWriterFailurePublishesTailWithoutSync verifies a burst that
// ends between logger Sync calls still publishes its final writer-failure
// metric. Production shutdown does not call Logger.Sync, so this cannot rely
// on the explicit flush path alone.
func TestEmergencyWriterFailurePublishesTailWithoutSync(t *testing.T) {
	rec := installFakeLogMetrics(t)
	logger, base, _, _ := newEmergencyHarness(t, 1<<20)
	base.fail(errors.New("no space left on device"))

	logger.Info("first failed line")
	logger.Info("second failed line")

	require.Eventually(t, func() bool {
		lines, _ := rec.suppressed(metrics.LogSuppressReasonWriterFailure)
		return lines == 2
	}, 2*time.Second, 10*time.Millisecond,
		"writer failure accounting must flush its tail without another log write or Sync")
}

// TestEmergencyWriterFailureFlushesTheTail verifies failures that arrive after
// the first one-second metric publication are not lost when the logger syncs.
func TestEmergencyWriterFailureFlushesTheTail(t *testing.T) {
	rec := installFakeLogMetrics(t)
	logger, base, gate, _ := newEmergencyHarness(t, 1<<20)
	base.fail(errors.New("no space left on device"))

	logger.Info("first failed line")
	logger.With(zap.String("request_url", strings.Repeat("x", 4096))).Info("second failed line")

	lines, _ := rec.suppressed(metrics.LogSuppressReasonWriterFailure)
	require.Equal(t, 1, lines, "the first failure is published immediately")
	require.NoError(t, logger.Sync())

	lines, _ = rec.suppressed(metrics.LogSuppressReasonWriterFailure)
	require.Equal(t, 2, lines, "Sync must publish the final writer-failure tail")
	require.False(t, gate.engaged.Load())
}

// TestEmergencyGateChargesBoundFieldsByEncodedSize verifies a logger derived
// with a large field cannot spend only the small per-field guess from the
// emergency byte budget.
func TestEmergencyGateChargesBoundFieldsByEncodedSize(t *testing.T) {
	installFakeLogMetrics(t)
	logger, base, gate, _ := newEmergencyHarness(t, 512)
	require.True(t, gate.engage(metrics.LogSuppressReasonDiskPressure))

	logger.With(zap.String("request_url", strings.Repeat("x", 4096))).Info("request completed")
	logger.With(zap.Binary("response_body", make([]byte, 4096))).Info("request completed")
	require.Empty(t, base.snapshot(), "a bound field larger than the byte budget must be suppressed")
}

// TestEncodedEntryBytesMatchesProductionConsoleEncoding verifies the byte
// budget measures the exact console representation the production logger emits,
// including bound fields and non-string fields.
func TestEncodedEntryBytesMatchesProductionConsoleEncoding(t *testing.T) {
	gate := newEmergencyGate()
	require.True(t, gate.engage(metrics.LogSuppressReasonDiskPressure))

	sink := &countingSyncer{}
	cfg := zap.NewProductionEncoderConfig()
	cfg.EncodeCaller = zapcore.ShortCallerEncoder
	cfg.EncodeLevel = zapcore.CapitalLevelEncoder
	cfg.EncodeTime = zapcore.RFC3339TimeEncoder
	core := &emergencyCore{
		Core: zapcore.NewCore(zapcore.NewConsoleEncoder(cfg), sink, zapcore.DebugLevel),
		gate: gate,
	}
	bound := core.With([]zapcore.Field{
		zap.String("request_url", strings.Repeat("x", 64)),
		zap.Binary("response_body", make([]byte, 64)),
	})
	entry := zapcore.Entry{Level: zapcore.InfoLevel, Time: time.Date(2026, time.September, 8, 12, 0, 0, 0, time.UTC), Message: "request completed"}
	fields := []zapcore.Field{zap.Int("status", 200)}
	expected := encodedEntryBytes(entry, append(bound.Fields(), fields...))

	require.NoError(t, bound.Write(entry, fields))
	require.Equal(t, int64(expected), sink.bytes.Load())
}

// TestEmergencyGatePublishesDiskPressure verifies the degraded state is
// observable, so an alert can fire on a logging pipeline that is dropping data.
func TestEmergencyGatePublishesDiskPressure(t *testing.T) {
	rec := installFakeLogMetrics(t)
	_, _, gate, _ := newEmergencyHarness(t, 1<<20)

	gate.engage(metrics.LogSuppressReasonDiskPressure)
	gate.engage(metrics.LogSuppressReasonActiveFileCap)
	require.Equal(t, []float64{1}, rec.pressureSeries(), "engaging twice must publish once")

	gate.release(metrics.LogSuppressReasonDiskPressure)
	require.Equal(t, []float64{1}, rec.pressureSeries(), "the second reason keeps the process degraded")

	gate.release(metrics.LogSuppressReasonActiveFileCap)
	require.Equal(t, []float64{1, 0}, rec.pressureSeries())
}

// TestEmergencyRecoverySummaryIsRateLimited verifies a flapping volume cannot
// turn the recovery report into its own log storm, while the suppressed totals
// are carried forward rather than lost.
func TestEmergencyRecoverySummaryIsRateLimited(t *testing.T) {
	installFakeLogMetrics(t)
	logger, _, gate, clock := newEmergencyHarness(t, 1)
	gate.setClock(func() time.Time { return *clock })

	gate.engage(metrics.LogSuppressReasonDiskPressure)
	for range 10 {
		logger.Info("record log")
	}
	first := gate.release(metrics.LogSuppressReasonDiskPressure)
	require.True(t, first.Report, "the first recovery must be reported")
	require.Equal(t, int64(10), first.Lines)

	gate.engage(metrics.LogSuppressReasonDiskPressure)
	for range 4 {
		logger.Info("record log")
	}
	*clock = clock.Add(time.Second)
	second := gate.release(metrics.LogSuppressReasonDiskPressure)
	require.True(t, second.Released)
	require.False(t, second.Report, "a second recovery inside the window must stay quiet")

	gate.engage(metrics.LogSuppressReasonDiskPressure)
	for range 3 {
		logger.Info("record log")
	}
	*clock = clock.Add(2 * emergencySummaryInterval)
	third := gate.release(metrics.LogSuppressReasonDiskPressure)
	require.True(t, third.Report)
	require.Equal(t, int64(7), third.Lines, "the unreported suppression must be carried forward, not lost")
}

// TestReportEmergencyRecoveryNamesTheGap verifies the summary explains the hole
// it left in the log, so an operator reading the file is not left guessing.
func TestReportEmergencyRecoveryNamesTheGap(t *testing.T) {
	base := newRecordingCore()
	lg := loggerOverCore(t, base)

	reportEmergencyRecovery(lg, emergencySummary{Released: true, Report: true, Lines: 42, Bytes: 4096},
		metrics.LogSuppressReasonDiskPressure)

	entries := base.snapshot()
	require.Len(t, entries, 1)
	require.Equal(t, zapcore.WarnLevel, entries[0].Level, "a bounded, expected condition is a warning, not a server fault")
	require.Contains(t, entries[0].Message, "bounded emergency policy")

	fields := map[string]any{}
	for _, f := range entries[0].Fields {
		fields[f.Key] = f.Integer
	}
	require.Equal(t, int64(42), fields["suppressed_lines"])
	require.Equal(t, int64(4096), fields["suppressed_bytes"])
}

// loggerOverCore builds a glog.Logger whose core is the supplied core, so a
// test can assert on what production code logs.
//
// Parameters:
//   - t: the test handle.
//   - core: the terminal core to install.
//
// Return values:
//   - glog.Logger: a logger writing only into core.
func loggerOverCore(t *testing.T, core zapcore.Core) glog.Logger {
	t.Helper()
	base, err := glog.NewConsoleWithName("emergency-test", glog.LevelDebug)
	require.NoError(t, err)
	return base.WithOptions(zap.WrapCore(func(zapcore.Core) zapcore.Core { return core }))
}

// TestEstimateEntryBytesTracksContent verifies the budget is charged by size
// rather than by line count, so one enormous entry cannot slip through a
// per-line allowance.
func TestEstimateEntryBytesTracksContent(t *testing.T) {
	small := estimateEntryBytes(zapcore.Entry{Message: "hi"}, nil)
	large := estimateEntryBytes(zapcore.Entry{Message: strings.Repeat("x", 4096)}, nil)
	require.Greater(t, large, small+4000)

	withField := estimateEntryBytes(zapcore.Entry{Message: "hi"},
		[]zapcore.Field{zap.String("payload", strings.Repeat("y", 512))})
	require.Greater(t, withField, small+500)

	withErr := estimateEntryBytes(zapcore.Entry{Message: "hi"},
		[]zapcore.Field{zap.Error(errors.New(strings.Repeat("z", 256)))})
	require.Greater(t, withErr, small+250)
}
