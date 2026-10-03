package logger

import (
	"testing"
	"time"

	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/config"
)

// countingCore records how many entries reach the write stage.
type countingCore struct {
	zapcore.LevelEnabler
	written *int
	fields  []zapcore.Field
}

// With implements zapcore.Core.
//
// Parameters:
//   - fields: fields to bind.
//
// Return values:
//   - zapcore.Core: a core with the fields bound.
func (c *countingCore) With(fields []zapcore.Field) zapcore.Core {
	return &countingCore{LevelEnabler: c.LevelEnabler, written: c.written, fields: append(c.fields, fields...)}
}

// Check implements zapcore.Core by adding itself when the level is enabled.
//
// Parameters:
//   - entry: the entry being logged.
//   - ce: the checked entry accumulated so far.
//
// Return values:
//   - *zapcore.CheckedEntry: ce, with this core added when enabled.
func (c *countingCore) Check(entry zapcore.Entry, ce *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	if c.Enabled(entry.Level) {
		return ce.AddCore(entry, c)
	}
	return ce
}

// Write implements zapcore.Core by counting the entry.
//
// Parameters:
//   - entry: the entry to write.
//   - fields: the entry's fields.
//
// Return values:
//   - error: always nil.
func (c *countingCore) Write(zapcore.Entry, []zapcore.Field) error {
	*c.written++
	return nil
}

// Sync implements zapcore.Core.
//
// Parameters: none.
//
// Return values:
//   - error: always nil.
func (c *countingCore) Sync() error { return nil }

// Fields implements zapcore.Core (an addition in the Laisky zap fork).
//
// Parameters: none.
//
// Return values:
//   - []zapcore.Field: the bound fields.
func (c *countingCore) Fields() []zapcore.Field { return c.fields }

// newSamplingHarness builds a logger whose core is the level-bounded sampler.
//
// Parameters:
//   - t: the test, used for helper bookkeeping.
//   - first: entries kept per (level, message) per tick.
//   - thereafter: thinning factor after the initial budget.
//
// Return values:
//   - *zap.Logger: the logger under test.
//   - *int: how many entries reached the write stage.
func newSamplingHarness(t *testing.T, first, thereafter int) (*zap.Logger, *int) {
	t.Helper()
	written := 0
	base := &countingCore{LevelEnabler: zapcore.DebugLevel, written: &written}
	core := &levelBoundedSampler{
		base:    base,
		sampled: zapcore.NewSamplerWithOptions(base, time.Hour, first, thereafter),
	}
	return zap.New(core), &written
}

// TestLevelBoundedSamplerThinsBelowWarn verifies repetitive INFO chatter is
// thinned to the configured budget.
func TestLevelBoundedSamplerThinsBelowWarn(t *testing.T) {
	logger, written := newSamplingHarness(t, 3, 0)

	for range 100 {
		logger.Info("record log")
	}

	require.Equal(t, 3, *written, "only the initial budget survives when thereafter is 0")
}

// TestLevelBoundedSamplerThereafterKeepsEveryNth verifies the thinning factor.
func TestLevelBoundedSamplerThereafterKeepsEveryNth(t *testing.T) {
	logger, written := newSamplingHarness(t, 2, 10)

	for range 42 {
		logger.Info("record log")
	}

	// 2 from the initial budget, then one in every 10 of the remaining 40.
	require.Equal(t, 6, *written)
}

// TestLevelBoundedSamplerNeverThinsWarnAndAbove is the important guarantee: a
// repeated warning or error reports the SCALE of an incident, and thinning it
// would misreport how bad things are.
func TestLevelBoundedSamplerNeverThinsWarnAndAbove(t *testing.T) {
	logger, written := newSamplingHarness(t, 1, 0)

	for range 50 {
		logger.Warn("channel suspended")
	}
	for range 50 {
		logger.Error("upstream failure")
	}

	require.Equal(t, 100, *written, "warn and error must pass through unsampled")
}

// TestLevelBoundedSamplerDistinguishesMessages verifies a rare message keeps its
// own budget instead of competing with the chatty one.
func TestLevelBoundedSamplerDistinguishesMessages(t *testing.T) {
	logger, written := newSamplingHarness(t, 2, 0)

	for range 100 {
		logger.Info("record log")
	}
	logger.Info("a rare event")

	require.Equal(t, 3, *written, "a distinct message must not be starved by a chatty one")
}

// TestLevelBoundedSamplerPropagatesFields verifies With keeps both cores in step
// so derived loggers stay sampled.
func TestLevelBoundedSamplerPropagatesFields(t *testing.T) {
	logger, written := newSamplingHarness(t, 1, 0)

	child := logger.With(zap.String("component", "relay"))
	for range 20 {
		child.Info("record log")
	}

	require.Equal(t, 1, *written, "a derived logger must remain sampled")
}

// TestSamplingOptionDisabledByDefault verifies sampling stays off unless
// configured, so the default deployment logs exactly as it does today.
func TestSamplingOptionDisabledByDefault(t *testing.T) {
	prev := config.LogSampleInitial
	config.LogSampleInitial = 0
	t.Cleanup(func() { config.LogSampleInitial = prev })

	_, ok := samplingOption()
	require.False(t, ok)
}

// TestSamplingOptionEnabled verifies a positive initial budget produces an option.
func TestSamplingOptionEnabled(t *testing.T) {
	prev := config.LogSampleInitial
	config.LogSampleInitial = 10
	t.Cleanup(func() { config.LogSampleInitial = prev })

	opt, ok := samplingOption()
	require.True(t, ok)
	require.NotNil(t, opt)
}
