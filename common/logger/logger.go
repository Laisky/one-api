package logger

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	errors "github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	gutils "github.com/Laisky/go-utils/v6"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	"github.com/gin-gonic/gin"

	"github.com/songquanpeng/one-api/common/config"
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
		level := glog.LevelInfo
		if config.DebugEnabled {
			level = glog.LevelDebug
		}

		Logger, err = glog.NewConsoleWithName("one-api", level)
		if err != nil {
			panic(fmt.Sprintf("failed to create logger: %+v", err))
		}
	})
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
			return 0, err
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
