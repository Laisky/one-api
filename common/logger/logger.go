package logger

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	errors "github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	gutils "github.com/Laisky/go-utils/v6"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/config"
)

var (
	// Logger is the primary structured logger shared by the entire application.
	Logger       glog.Logger
	setupLogOnce sync.Once
	initLogOnce  sync.Once
)

// init initializes the logger automatically when the package is imported.
func init() {
	initLogger()
}

// initLogger initializes the go-utils logger.
func initLogger() {
	initLogOnce.Do(func() {
		var err error
		level := defaultLevel()
		Logger, err = glog.NewConsoleWithName("one-api", level)
		if err != nil {
			panic(fmt.Sprintf("failed to create logger: %+v", err))
		}

		// Two library loggers write alongside Logger: glog.Shared, which
		// gmw.GetLogger falls back to when handed a context that carries no
		// request logger, and gmw.Logger, which gin-middlewares builds for
		// itself at info. Align both so one verbosity knob governs the process.
		alignLibraryLogger("shared", glog.Shared, level)
		alignLibraryLogger("gin-mw", gmw.Logger, level)
	})
}

// alignLibraryLogger moves a third-party logger onto the level this process resolved.
//
// Parameters:
//   - name: the logger's name, used only to report a failure.
//   - target: the logger to adjust; a nil logger is skipped.
//   - level: the level to apply.
func alignLibraryLogger(name string, target glog.Logger, level glog.Level) {
	if target == nil {
		return
	}
	// glog.Shared and gmw.Logger are interface values holding pointers, so a nil
	// logger can still compare non-nil as an interface.
	if v := reflect.ValueOf(target); v.Kind() == reflect.Pointer && v.IsNil() {
		return
	}
	if err := target.ChangeLevel(level); err != nil {
		Logger.Warn("failed to align library logger level",
			zap.String("logger", name),
			zap.String("level", level.String()),
			zap.Error(err))
	}
}

// defaultLevel resolves the verbosity of the shared logger.
//
// Precedence is LOG_LEVEL, then DEBUG, then the context default: info for a
// running server, but silent under `go test`, where the relay/billing log
// stream otherwise buries the test results in megabytes of output. Re-enable it
// for a single run with `LOG_LEVEL=info go test ./...` (or DEBUG=true).
//
// Return values:
//   - glog.Level: the level the shared logger starts at.
func defaultLevel() glog.Level {
	if level, ok := parseLevel(os.Getenv("LOG_LEVEL")); ok {
		return level
	}
	if config.DebugEnabled {
		return glog.LevelDebug
	}
	if testing.Testing() {
		return glog.LevelFatal
	}
	return glog.LevelInfo
}

// parseLevel converts a LOG_LEVEL environment value into a logger level.
//
// Parameters:
//   - raw: the raw environment value, which may be empty or unrecognized.
//
// Return values:
//   - glog.Level: the parsed level, meaningful only when ok is true.
//   - bool: whether raw named a supported level.
func parseLevel(raw string) (glog.Level, bool) {
	switch glog.Level(strings.ToLower(strings.TrimSpace(raw))) {
	case glog.LevelDebug:
		return glog.LevelDebug, true
	case glog.LevelInfo:
		return glog.LevelInfo, true
	case glog.LevelWarn:
		return glog.LevelWarn, true
	case glog.LevelError:
		return glog.LevelError, true
	case glog.LevelFatal:
		return glog.LevelFatal, true
	default:
		return glog.LevelUnspecified, false
	}
}

// QuietForTests reports whether library-level logging should stay silent because
// the process is a `go test` binary running without an explicit verbosity request.
// Subsystems with their own logger (notably GORM) consult it so a test run emits
// one coherent amount of output instead of each library deciding on its own.
//
// Return values:
//   - bool: true when third-party logging should be suppressed.
func QuietForTests() bool {
	if !testing.Testing() {
		return false
	}
	if _, ok := parseLevel(os.Getenv("LOG_LEVEL")); ok {
		return false
	}
	return !config.DebugEnabled
}

// SetupLogger configures the shared logger to write to stdout and the configured log directory with optional rotation.
func SetupLogger() {
	setupLogOnce.Do(func() {
		if strings.TrimSpace(LogDir) == "" {
			Logger.Info("log directory not configured; file logging disabled")
			return
		}

		if err := os.MkdirAll(LogDir, 0o755); err != nil {
			Logger.Error("failed to ensure log directory", zap.String("log_dir", LogDir), zap.Error(err))
			return
		}

		basePath := filepath.Join(LogDir, "oneapi.log")
		outputPaths := []string{"stdout"}
		errorPaths := []string{"stderr"}

		rotationEnabled := !config.OnlyOneLogFile
		rotationInterval := rotationIntervalDaily
		sinkPath := basePath

		if rotationEnabled {
			parsedInterval, err := parseRotationInterval(config.LogRotationInterval)
			if err != nil {
				Logger.Warn("invalid log rotation interval, defaulting to daily",
					zap.String("requested_interval", config.LogRotationInterval),
					zap.Error(err))
			} else {
				rotationInterval = parsedInterval
			}

			sinkURL, err := buildRotationSinkURL(basePath, rotationInterval, config.LogRetentionDays)
			if err != nil {
				Logger.Error("failed to configure log rotation sink",
					zap.String("log_path", basePath),
					zap.Error(err))
				rotationEnabled = false
				sinkPath = basePath
			} else {
				sinkPath = sinkURL
			}
		}

		outputPaths = append(outputPaths, sinkPath)
		errorPaths = append(errorPaths, sinkPath)

		previous := Logger
		if err := configureGlobalLogger(outputPaths, errorPaths); err != nil {
			Logger.Error("failed to attach log sinks", zap.Error(err))
			return
		}

		applyGinWriters()

		if previous != nil {
			_ = previous.Sync()
		}

		fields := []zap.Field{
			zap.String("log_dir", LogDir),
			zap.Bool("rotation_enabled", rotationEnabled),
		}
		if rotationEnabled {
			fields = append(fields,
				zap.String("rotation_interval", rotationInterval.String()),
				zap.Int("retention_days", config.LogRetentionDays),
			)
		}
		Logger.Info("log sinks configured", fields...)
	})
}

// FromContext returns a context-aware logger when available, otherwise the global logger.
func FromContext(ctx context.Context) glog.Logger {
	if ctx == nil {
		return Logger
	}
	if lg := gmw.GetLogger(ctx); lg != nil {
		return lg
	}
	return Logger
}

// configureGlobalLogger reinitializes the shared logger with the provided output paths.
func configureGlobalLogger(outputPaths, errorPaths []string) error {
	level := Logger.Level()
	newLogger, err := glog.New(
		glog.WithName("one-api"),
		glog.WithLevel(level),
		glog.WithEncoding(glog.EncodingConsole),
		glog.WithOutputPaths(outputPaths),
		glog.WithErrorOutputPaths(errorPaths),
	)
	if err != nil {
		return errors.Wrap(err, "create file logger")
	}

	Logger = newLogger
	return nil
}

// applyGinWriters routes Gin's default writers through the structured logger while retaining stdout/stderr output.
func applyGinWriters() {
	gin.DefaultWriter = &ginZapWriter{level: zapcore.InfoLevel, fallback: os.Stdout}
	gin.DefaultErrorWriter = &ginZapWriter{level: zapcore.ErrorLevel, fallback: os.Stderr}
}

// ginZapWriter forwards Gin logs to the shared logger and optional fallback writer.
type ginZapWriter struct {
	level    zapcore.Level
	fallback io.Writer
}

// Write implements io.Writer by logging the provided payload at the configured level and delegating to the fallback writer.
func (w *ginZapWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}

	message := strings.TrimSpace(string(p))
	if message != "" {
		switch {
		case w.level >= zapcore.ErrorLevel:
			Logger.Error(message)
		case w.level >= zapcore.WarnLevel:
			Logger.Warn(message)
		default:
			Logger.Info(message)
		}
	}

	if w.fallback != nil {
		if _, err := w.fallback.Write(p); err != nil {
			return 0, errors.Wrap(err, "write to fallback logger")
		}
	}

	return len(p), nil
}

// SetupEnhancedLogger sets up the logger with alertPusher integration.
func SetupEnhancedLogger(ctx context.Context) {
	opts := []zap.Option{}

	// Setup alert pusher if configured.
	if config.LogPushAPI != "" {
		ratelimiter, err := gutils.NewRateLimiter(ctx, gutils.RateLimiterArgs{
			Max:     1,
			NPerSec: 1,
		})
		if err != nil {
			Logger.Panic("create ratelimiter", zap.Error(err))
		}

		alertPusher, err := glog.NewAlert(
			ctx,
			config.LogPushAPI,
			glog.WithAlertType(config.LogPushType),
			glog.WithAlertToken(config.LogPushToken),
			glog.WithAlertHookLevel(zap.ErrorLevel),
			glog.WithRateLimiter(ratelimiter),
		)
		if err != nil {
			Logger.Panic("create AlertPusher", zap.Error(err))
		}

		opts = append(opts, zap.HooksWithFields(alertPusher.GetZapHook()))
		Logger.Info("alert pusher configured",
			zap.String("alert_api", config.LogPushAPI),
			zap.String("alert_type", config.LogPushType),
		)
	}

	// Get hostname for logger context.
	hostname, err := os.Hostname()
	if err != nil {
		Logger.Panic("get hostname", zap.Error(err))
	}

	// Apply options and add hostname context.
	logger := Logger.WithOptions(opts...).With(
		zap.String("host", hostname),
	)
	Logger = logger

	// Set log level based on debug mode.
	if config.DebugEnabled {
		_ = Logger.ChangeLevel("debug")
		Logger.Info("running in debug mode with enhanced logging")
	} else {
		_ = Logger.ChangeLevel("info")
		Logger.Info("running in production mode with enhanced logging")
	}
}
