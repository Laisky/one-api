package logger

// Optional OTLP application-log sink wiring (proposal
// docs/proposals/20260905_observability-data-tiering.md, Phase 3 / W3.2:
// "Keep the bridge isolated and optional, preserve file/stdout defaults").
//
// The bridge itself lives in common/logger/otelbridge, which knows nothing
// about this package's sinks, rotation, disk guard or sampling. This file is
// the only place the two meet, and it does exactly one thing: fan the process
// logger's core out to the bridge in addition to whatever local sinks are
// configured.
//
// WHERE IT SITS IN THE CORE STACK, AND WHY
//
// SetupEnhancedLogger layers cores with zap.WrapCore, each option wrapping the
// previous result, so the order of the option slice is the order of the stack
// from the inside out:
//
//	base (file / stdout writers)
//	  emergencyCore        <- disk byte budget
//	    NewTee(emergency, otlpCore)   <- this file
//	      levelBoundedSampler         <- sampling
//
// The tee goes ABOVE the emergency gate and BELOW sampling, and both halves of
// that are deliberate:
//
//   - Above the emergency gate, because that gate exists to stop a log storm
//     from filling the LOCAL DISK. A full disk is not a reason to stop
//     exporting to a collector that has room, and charging the disk budget for
//     records that never touch the disk would make its suppression counters
//     describe something that did not happen.
//   - Below sampling, because sampling decides which lines the process wants at
//     all. An operator who enabled LOG_SAMPLE_INITIAL to survive a storm did
//     not ask for the unsampled stream to be shipped off-box instead, and the
//     collector would be the next thing to fall over.
//
// For the same reason the bridge sits below sampling, its severity floor can
// only narrow what the process already logs; see effectiveBridgeLevel.

import (
	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/logger/otelbridge"
)

// otlpBridgeOption builds the zap option that fans the logger out to the OTLP
// application-log bridge.
//
// The returned core is usable immediately even though no logger provider exists
// yet: main.go configures logging before it initializes OpenTelemetry, because
// telemetry initialization logs. Records written in that window are counted as
// dropped_not_ready and still reach every local sink.
//
// Parameters: none.
//
// Return values:
//   - zap.Option: the option to apply; meaningful only when ok is true.
//   - bool: whether APP_LOG_SINK asked for the bridge at all.
func otlpBridgeOption() (zap.Option, bool) {
	if !config.AppLogOTLPEnabled {
		return nil, false
	}

	level := effectiveBridgeLevel()
	return zap.WrapCore(func(core zapcore.Core) zapcore.Core {
		return zapcore.NewTee(core, otelbridge.NewCore(otelbridge.Shared, otelbridge.DefaultScopeName, level))
	}), true
}

// effectiveBridgeLevel resolves the level the bridge actually exports at.
//
// LOG_OTLP_MIN_LEVEL can only ever RAISE the threshold, never lower it, and that
// asymmetry is deliberate. zapcore.NewTee reports an entry as enabled when ANY
// of its children accepts it, so a bridge core configured below the process log
// level would not merely export more -- it would make the whole process
// construct and evaluate entries it is configured not to log, paying the field
// cost at every debug call site in the relay path, and producing an OTLP stream
// containing lines that appear in no local log file.
//
// Measured before this clamp existed: with the process at info and the floor at
// debug, a debug line was exported while the local sink recorded nothing.
//
// The process level after SetupEnhancedLogger is deterministic -- debug when
// DEBUG is set, info otherwise -- so the clamp needs no coordination with the
// logger's runtime level.
//
// Parameters: none.
//
// Return values:
//   - zapcore.Level: the greater of the configured floor and the process level.
func effectiveBridgeLevel() zapcore.Level {
	floor := otlpBridgeLevel(config.AppLogOTLPMinLevel)

	process := zapcore.InfoLevel
	if config.DebugEnabled {
		process = zapcore.DebugLevel
	}
	if floor < process {
		return process
	}
	return floor
}

// otlpBridgeLevel maps a configured severity floor onto a zap level.
//
// The floor is independent of the process log level on purpose: a gateway
// running at debug for local diagnosis must not start shipping debug volume to
// a shared collector because someone set DEBUG=true.
//
// Parameters:
//   - name: one of the config.AppLogOTLPLevel* constants.
//
// Return values:
//   - zapcore.Level: the level the bridge exports at and above; info for any
//     unrecognized name, matching the configuration normalizer.
func otlpBridgeLevel(name string) zapcore.Level {
	switch name {
	case config.AppLogOTLPLevelDebug:
		return zapcore.DebugLevel
	case config.AppLogOTLPLevelWarn:
		return zapcore.WarnLevel
	case config.AppLogOTLPLevelError:
		return zapcore.ErrorLevel
	default:
		return zapcore.InfoLevel
	}
}
