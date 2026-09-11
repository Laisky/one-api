package logger

// Bounded emergency logging policy (proposal
// docs/proposals/20260905_observability-data-tiering.md, Phase 0 / W0.4).
//
// The pre-existing response to an exhausted log volume was escalateLogLevel:
// raise the process level to warn and hope. That is not a bound. WARN and ERROR
// stay fully enabled, and an error storm is exactly what accompanies a failing
// gateway, so the volume still fills -- only more slowly, and with the INFO
// context that would have explained the incident removed first.
//
// This file adds the bound the escalation lacks: while the emergency is
// engaged, application logging admits at most LOG_EMERGENCY_MAX_BYTES_PER_SEC
// across ALL levels and counts what it discards. Three properties matter.
//
//  1. Suppression is observable. A loss nothing counts is indistinguishable
//     from a logger that silently died, so every discarded line is tallied
//     through metrics.RecordLogSuppression under a bounded reason label, and
//     metrics.UpdateLogDiskPressure publishes whether the process is degraded.
//  2. Recovery is reported. Leaving the emergency emits a rate-limited summary
//     naming how many lines and bytes were dropped, so the gap in the log has
//     an explanation inside the log itself.
//  3. A failing writer is never logged through the failing logger. That is how
//     a process spins: the write fails, the failure is logged, that write
//     fails. Writer failures are counted and swallowed instead.
//
// Billing SQL is outside this policy. Nothing here touches the database write
// path; only application log output is suppressed.

import (
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"

	"github.com/Laisky/errors/v2"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/metrics"
)

const (
	// emergencyWindow is the token-bucket refill period. The budget is
	// expressed per second, so the window is one second and the bucket is
	// refilled wholesale rather than continuously: a log line is small relative
	// to the budget, and a coarse window keeps the hot path to one comparison.
	emergencyWindow = time.Second

	// emergencySummaryInterval rate-limits the recovery summary. Free space
	// oscillating around the floor would otherwise emit one summary per
	// LOG_DISK_CHECK_INTERVAL_SEC, which is itself log volume produced by a
	// guard whose purpose is to produce less of it.
	emergencySummaryInterval = time.Minute

	// emergencyEntryOverheadBytes approximates the fixed part of an encoded log
	// line: timestamp, level, caller, separators and the trailing newline.
	emergencyEntryOverheadBytes = 64

	// emergencyFieldGuessBytes approximates one encoded field whose rendered
	// length is not knowable without encoding it.
	emergencyFieldGuessBytes = 24
)

// diskEmergency is the process-wide bounded emergency policy.
//
// It is a package-level singleton because the zap core that consults it is
// installed once, into the global logger, and every derived logger shares it:
// a per-logger budget would let each subsystem spend the whole allowance.
var diskEmergency = newEmergencyGate()

// emergencySummary reports what one exit from emergency mode discarded.
type emergencySummary struct {
	// Released is true when this call actually left emergency mode, rather
	// than finding it already inactive.
	Released bool
	// Report is true when the rate limiter permits emitting a summary now.
	Report bool
	// Lines is how many log lines were discarded since the last reported
	// summary.
	Lines int64
	// Bytes is the estimated size of those lines.
	Bytes int64
}

// emergencyGate is the token bucket and accounting behind the bounded policy.
//
// engaged is an atomic mirror of the mutex-protected reason flags so the
// logging hot path costs one atomic load when nothing is wrong, which is the
// overwhelmingly common case.
type emergencyGate struct {
	mu sync.Mutex

	// diskPressure is set while free space is below the configured floor.
	diskPressure bool
	// activeFileCap is set while the active log file is over its ceiling and
	// the writer cannot rotate it away.
	activeFileCap bool

	windowStart time.Time
	windowBytes int64

	// pendingLines and pendingBytes accumulate suppression between metric
	// flushes. Emitting one metric observation per discarded line would make
	// the accounting cost scale with the storm it is measuring.
	pendingLines int64
	pendingBytes int64

	// totalLines and totalBytes accumulate across an entire emergency, and are
	// reset only when a recovery summary actually reports them.
	totalLines int64
	totalBytes int64

	lastSummaryAt time.Time

	// failureWindowStart, failureLines and failureBytes aggregate writer
	// failures on the same one-second cadence.
	failureWindowStart time.Time
	failureLines       int64
	failureBytes       int64
	// failureFlushTimer publishes a window's final writer failures even when no
	// later log entry arrives and the process never explicitly Syncs the logger.
	failureFlushTimer *time.Timer

	engaged atomic.Bool
	now     func() time.Time
}

// newEmergencyGate builds an idle gate driven by the wall clock.
//
// Parameters: none.
//
// Return values:
//   - *emergencyGate: a gate with no reason engaged.
func newEmergencyGate() *emergencyGate {
	return &emergencyGate{now: func() time.Time { return time.Now().UTC() }}
}

// engage turns the bounded policy on for one reason.
//
// Parameters:
//   - reason: one of the metrics.LogSuppressReason* constants that this gate
//     can be engaged for.
//
// Return values:
//   - bool: true when this call moved the process from healthy to degraded, so
//     the caller emits its one-time report.
func (g *emergencyGate) engage(reason string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()

	before := g.diskPressure || g.activeFileCap
	switch reason {
	case metrics.LogSuppressReasonDiskPressure:
		g.diskPressure = true
	case metrics.LogSuppressReasonActiveFileCap:
		g.activeFileCap = true
	default:
		return false
	}

	if before {
		return false
	}

	g.windowStart = g.now()
	g.windowBytes = 0
	g.engaged.Store(true)
	metrics.UpdateLogDiskPressure(true)
	return true
}

// release turns the bounded policy off for one reason and reports what the
// emergency discarded.
//
// Parameters:
//   - reason: the reason to clear; other reasons keep the gate engaged.
//
// Return values:
//   - emergencySummary: whether the gate actually left emergency mode, whether
//     a summary may be emitted now, and the suppression totals.
func (g *emergencyGate) release(reason string) emergencySummary {
	g.mu.Lock()
	defer g.mu.Unlock()

	switch reason {
	case metrics.LogSuppressReasonDiskPressure, metrics.LogSuppressReasonActiveFileCap:
	default:
		return emergencySummary{}
	}

	// Flush BEFORE clearing the flag. reasonLocked reads those flags to label
	// the suppression, so publishing after the clear would attribute every
	// discarded line to whichever reason happened to remain set -- or to the
	// fallback reason when none did.
	g.flushLocked()

	if reason == metrics.LogSuppressReasonDiskPressure {
		g.diskPressure = false
	} else {
		g.activeFileCap = false
	}

	if g.diskPressure || g.activeFileCap {
		return emergencySummary{}
	}
	if !g.engaged.Swap(false) {
		return emergencySummary{}
	}

	metrics.UpdateLogDiskPressure(false)

	summary := emergencySummary{Released: true, Lines: g.totalLines, Bytes: g.totalBytes}
	now := g.now()
	if g.totalLines > 0 && (g.lastSummaryAt.IsZero() || now.Sub(g.lastSummaryAt) >= emergencySummaryInterval) {
		summary.Report = true
		g.lastSummaryAt = now
		g.totalLines = 0
		g.totalBytes = 0
	}

	return summary
}

// admit asks whether one log line fits in the current byte budget.
//
// Parameters:
//   - size: the estimated encoded size of the line.
//
// Return values:
//   - bool: true when the line may be written; false when it was discarded and
//     counted.
func (g *emergencyGate) admit(size int) bool {
	if !g.engaged.Load() {
		return true
	}

	g.mu.Lock()
	defer g.mu.Unlock()

	if !g.diskPressure && !g.activeFileCap {
		return true
	}

	now := g.now()
	if now.Sub(g.windowStart) >= emergencyWindow {
		g.flushLocked()
		g.windowStart = now
		g.windowBytes = 0
	}

	budget := int64(config.LogEmergencyMaxBytesPerSec)
	if budget <= 0 || g.windowBytes+int64(size) > budget {
		g.pendingLines++
		g.pendingBytes += int64(size)
		return false
	}

	g.windowBytes += int64(size)
	return true
}

// flushLocked publishes accumulated suppression to the metrics recorder.
//
// The caller must hold g.mu.
//
// Parameters: none.
//
// Return values: none.
func (g *emergencyGate) flushLocked() {
	if g.pendingLines <= 0 {
		return
	}

	metrics.RecordLogSuppression(g.reasonLocked(), int(g.pendingLines), g.pendingBytes)
	g.totalLines += g.pendingLines
	g.totalBytes += g.pendingBytes
	g.pendingLines = 0
	g.pendingBytes = 0
}

// reasonLocked names the dominant reason the gate is engaged.
//
// Disk pressure outranks the active-file ceiling: when both hold, the volume
// itself is the emergency and the file ceiling is a symptom.
//
// The caller must hold g.mu.
//
// Parameters: none.
//
// Return values:
//   - string: a metrics.LogSuppressReason* constant.
func (g *emergencyGate) reasonLocked() string {
	if g.diskPressure {
		return metrics.LogSuppressReasonDiskPressure
	}
	return metrics.LogSuppressReasonActiveFileCap
}

// recordWriterFailure counts a log line the underlying writer refused.
//
// It never logs. Reporting a logging failure through the logger that just
// failed is how a process spins, and when the cause is a full volume the report
// is also the thing making it worse.
//
// Parameters:
//   - size: the estimated size of the line that was lost.
//
// Return values: none.
func (g *emergencyGate) recordWriterFailure(size int) {
	g.mu.Lock()

	g.failureLines++
	g.failureBytes += int64(size)

	now := g.now()
	if !g.failureWindowStart.IsZero() && now.Sub(g.failureWindowStart) < emergencyWindow {
		g.mu.Unlock()
		return
	}

	lines, bytes := g.failureLines, g.failureBytes
	g.failureLines, g.failureBytes, g.failureWindowStart = 0, 0, now
	if g.failureFlushTimer != nil {
		g.failureFlushTimer.Stop()
	}
	g.failureFlushTimer = time.AfterFunc(emergencyWindow, g.flushWriterFailures)
	g.mu.Unlock()

	metrics.RecordLogSuppression(metrics.LogSuppressReasonWriterFailure, int(lines), bytes)
}

// flushWriterFailures publishes writer failures accumulated after the most
// recent one-second report. Logger Sync calls it so shutdown cannot lose the
// final, quiet tail of a failed output stream.
//
// Parameters: none.
//
// Return values: none.
func (g *emergencyGate) flushWriterFailures() {
	g.mu.Lock()
	lines, bytes := g.failureLines, g.failureBytes
	g.failureLines, g.failureBytes = 0, 0
	g.failureWindowStart = time.Time{}
	if g.failureFlushTimer != nil {
		g.failureFlushTimer.Stop()
		g.failureFlushTimer = nil
	}
	g.mu.Unlock()

	if lines > 0 {
		metrics.RecordLogSuppression(metrics.LogSuppressReasonWriterFailure, int(lines), bytes)
	}
}

// reset returns the gate to its idle state. It exists for tests, which must not
// inherit disk pressure engaged by an earlier case.
//
// Parameters: none.
//
// Return values: none.
func (g *emergencyGate) reset() {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.diskPressure = false
	g.activeFileCap = false
	g.windowStart = time.Time{}
	g.windowBytes = 0
	g.pendingLines = 0
	g.pendingBytes = 0
	g.totalLines = 0
	g.totalBytes = 0
	g.lastSummaryAt = time.Time{}
	g.failureWindowStart = time.Time{}
	g.failureLines = 0
	g.failureBytes = 0
	if g.failureFlushTimer != nil {
		g.failureFlushTimer.Stop()
		g.failureFlushTimer = nil
	}
	g.engaged.Store(false)
	g.now = func() time.Time { return time.Now().UTC() }
}

// emergencyCore is the zapcore.Core that applies the byte budget.
//
// It sits BELOW the sampling core so the two compose correctly: sampling
// decides which lines the process wants, and the budget decides how many of
// those the disk can afford. Reversing them would charge the budget for lines
// sampling was going to discard anyway, and the suppression counters would
// overstate the loss.
type emergencyCore struct {
	zapcore.Core
	gate *emergencyGate
}

// With implements zapcore.Core, keeping the gate shared across derived loggers.
//
// Parameters:
//   - fields: the fields to bind.
//
// Return values:
//   - zapcore.Core: a core with the fields bound and the same gate.
func (c *emergencyCore) With(fields []zapcore.Field) zapcore.Core {
	return &emergencyCore{Core: c.Core.With(fields), gate: c.gate}
}

// Check implements zapcore.Core by taking over the write, which is the only way
// to see -- and be able to discard -- the entry the inner core would emit.
//
// Parameters:
//   - entry: the entry being logged.
//   - ce: the checked entry accumulated so far.
//
// Return values:
//   - *zapcore.CheckedEntry: ce, with this core added when the level is enabled.
func (c *emergencyCore) Check(entry zapcore.Entry, ce *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	if c.Core.Enabled(entry.Level) {
		return ce.AddCore(entry, c)
	}
	return ce
}

// Write implements zapcore.Core, enforcing the emergency byte budget and
// swallowing writer failures.
//
// It always returns nil. A non-nil error would be rendered by zap onto the
// logger's error output -- which under disk pressure is the very file that just
// refused the write -- so the failure is counted instead of amplified.
//
// Parameters:
//   - entry: the entry to write.
//   - fields: the entry's fields.
//
// Return values:
//   - error: always nil, by design.
func (c *emergencyCore) Write(entry zapcore.Entry, fields []zapcore.Field) error {
	size := emergencyEntryOverheadBytes + len(entry.Message)
	if c.gate.engaged.Load() {
		size = encodedEntryBytes(entry, append(c.Core.Fields(), fields...))
		if !c.gate.admit(size) {
			return nil
		}
	}

	if err := c.Core.Write(entry, fields); err != nil {
		c.gate.recordWriterFailure(size)
	}

	return nil
}

// Sync implements zapcore.Core by syncing the wrapped writer and publishing any
// writer failures that arrived after the last periodic metric report.
//
// Parameters: none.
//
// Return values:
//   - error: the wrapped core's sync result.
func (c *emergencyCore) Sync() error {
	err := c.Core.Sync()
	c.gate.flushWriterFailures()
	return errors.WithStack(err)
}

// emergencyOption builds the zap option that installs the bounded policy.
//
// Parameters: none.
//
// Return values:
//   - zap.Option: the option to apply to the process logger.
func emergencyOption() zap.Option {
	return zap.WrapCore(func(core zapcore.Core) zapcore.Core {
		return &emergencyCore{Core: core, gate: diskEmergency}
	})
}

// estimateEntryBytes approximates the encoded size of one log line.
//
// It is an estimate, not a measurement: the exact figure is only known after
// the encoder has run, and encoding an entry twice to charge a budget would
// cost more than the budget saves. The estimate is deliberately computed only
// while the emergency is engaged, so the healthy path pays nothing for it.
//
// Parameters:
//   - entry: the entry being written.
//   - fields: the entry's fields.
//
// Return values:
//   - int: the estimated encoded size in bytes; never negative.
func estimateEntryBytes(entry zapcore.Entry, fields []zapcore.Field) int {
	size := emergencyEntryOverheadBytes + len(entry.Message) + len(entry.LoggerName)
	for i := range fields {
		size += len(fields[i].Key) + 2
		switch fields[i].Type {
		case zapcore.SkipType:
		case zapcore.StringType:
			size += len(fields[i].String)
		case zapcore.ErrorType:
			if err, ok := fields[i].Interface.(error); ok && err != nil {
				size += len(err.Error())
				continue
			}
			size += emergencyFieldGuessBytes
		default:
			size += emergencyFieldGuessBytes
		}
	}

	return size
}

// encodedEntryBytes measures the console entry shape configured by
// go-utils/log for the process logger. It includes fields bound through
// logger.With and lets zap encode every supported field type, so a large byte
// slice, object marshaler, or request URL cannot bypass the emergency budget.
//
// Parameters:
//   - entry: the entry about to be written.
//   - fields: all bound and per-entry fields.
//
// Return values:
//   - int: encoded byte count, or MaxInt when encoding fails so containment
//     remains conservative during disk pressure.
func encodedEntryBytes(entry zapcore.Entry, fields []zapcore.Field) int {
	cfg := zap.NewProductionEncoderConfig()
	cfg.EncodeCaller = zapcore.ShortCallerEncoder
	cfg.EncodeLevel = zapcore.CapitalLevelEncoder
	cfg.EncodeTime = zapcore.RFC3339TimeEncoder
	buf, err := zapcore.NewConsoleEncoder(cfg).EncodeEntry(entry, fields)
	if err != nil {
		return math.MaxInt
	}
	size := buf.Len()
	buf.Free()
	return size
}

// setClock overrides the gate's time source.
//
// It takes the mutex the gate's own methods take, so a test can install a
// deterministic clock without racing the logging path.
//
// Parameters:
//   - now: the replacement clock; must return UTC.
//
// Return values: none.
func (g *emergencyGate) setClock(now func() time.Time) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.now = now
}
