package logger

// Application log sampling (proposal
// docs/proposals/20260905_observability-data-tiering.md, Phase 0 / W0.4).
//
// At a few thousand requests per second the process emits several INFO lines
// per request, and the same handful of message strings dominate the volume.
// zap's sampler is the right tool: it keeps the first N entries of each
// (level, message) pair per tick and then one in every M, so rare messages are
// never thinned while repetitive per-request chatter is.
//
// Sampling is bounded to levels BELOW warn. A repeated warning or error is a
// signal about scale — how many channels are failing, how many requests are
// timing out — and thinning it would misreport an incident.

import (
	"time"

	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"

	"github.com/Laisky/one-api/common/config"
)

// samplingThreshold is the first level that is never sampled.
const samplingThreshold = zapcore.WarnLevel

// levelBoundedSampler routes entries below samplingThreshold through a sampling
// core and everything at or above it through the unsampled core.
type levelBoundedSampler struct {
	base    zapcore.Core
	sampled zapcore.Core
}

// Enabled implements zapcore.Core by deferring to the unsampled core.
//
// Parameters:
//   - level: the level being tested.
//
// Return values:
//   - bool: whether the level is enabled at all.
func (c *levelBoundedSampler) Enabled(level zapcore.Level) bool {
	return c.base.Enabled(level)
}

// With implements zapcore.Core, propagating fields to both cores so the
// sampler keeps sharing its counters across derived loggers.
//
// Parameters:
//   - fields: the fields to bind.
//
// Return values:
//   - zapcore.Core: a core with the fields bound.
func (c *levelBoundedSampler) With(fields []zapcore.Field) zapcore.Core {
	return &levelBoundedSampler{
		base:    c.base.With(fields),
		sampled: c.sampled.With(fields),
	}
}

// Check implements zapcore.Core by routing on level.
//
// Parameters:
//   - entry: the entry being logged.
//   - ce: the checked entry accumulated so far.
//
// Return values:
//   - *zapcore.CheckedEntry: ce, with a writing core added when the entry survives.
func (c *levelBoundedSampler) Check(entry zapcore.Entry, ce *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	if entry.Level >= samplingThreshold {
		return c.base.Check(entry, ce)
	}
	return c.sampled.Check(entry, ce)
}

// Write implements zapcore.Core. Check always adds an inner core rather than
// this wrapper, so this method exists to satisfy the interface.
//
// Parameters:
//   - entry: the entry to write.
//   - fields: the entry's fields.
//
// Return values:
//   - error: whatever the unsampled core reports.
func (c *levelBoundedSampler) Write(entry zapcore.Entry, fields []zapcore.Field) error {
	return c.base.Write(entry, fields)
}

// Fields implements zapcore.Core (an addition in the Laisky zap fork) by
// reporting the fields bound to the unsampled core.
//
// Parameters: none.
//
// Return values:
//   - []zapcore.Field: the fields bound so far.
func (c *levelBoundedSampler) Fields() []zapcore.Field {
	return c.base.Fields()
}

// Sync implements zapcore.Core by flushing the unsampled core.
//
// Parameters: none.
//
// Return values:
//   - error: whatever the unsampled core reports.
func (c *levelBoundedSampler) Sync() error {
	return c.base.Sync()
}

// samplingOption builds the zap option that installs the level-bounded sampler.
//
// Parameters: none.
//
// Return values:
//   - zap.Option: the option to apply; meaningful only when ok is true.
//   - bool: whether sampling is configured at all.
func samplingOption() (zap.Option, bool) {
	if config.LogSampleInitial <= 0 {
		return nil, false
	}

	tick := time.Duration(config.LogSampleTickMs) * time.Millisecond
	first := config.LogSampleInitial
	thereafter := config.LogSampleThereafter

	return zap.WrapCore(func(core zapcore.Core) zapcore.Core {
		return &levelBoundedSampler{
			base:    core,
			sampled: zapcore.NewSamplerWithOptions(core, tick, first, thereafter),
		}
	}), true
}
