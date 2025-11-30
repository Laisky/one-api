package logger

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/songquanpeng/one-api/common/config"
)

func TestSetupEnhancedLogger(t *testing.T) {
	ctx := context.Background()

	// Test without alert pusher configuration
	t.Run("without_alert_pusher", func(t *testing.T) {
		// Ensure no alert pusher config
		config.LogPushAPI = ""
		config.LogPushType = ""
		config.LogPushToken = ""

		// This should not panic and should work normally
		SetupEnhancedLogger(ctx)

		// Test that logger is working
		Logger.Info("test log message without alert pusher")
	})

	// Test with alert pusher configuration (but invalid URL to avoid actual network calls)
	t.Run("with_alert_pusher_config", func(t *testing.T) {
		// Set alert pusher config
		config.LogPushAPI = "http://invalid-test-url.example.com/api/push"
		config.LogPushType = "test"
		config.LogPushToken = "test-token"

		// This should not panic even with invalid URL during setup
		SetupEnhancedLogger(ctx)

		// Test that logger is working
		Logger.Info("test log message with alert pusher config")
	})
}

func TestSetupEnhancedLoggerWithEnvironmentVariables(t *testing.T) {
	ctx := context.Background()

	// Test with environment variables
	t.Run("with_env_vars", func(t *testing.T) {
		// Set environment variables
		os.Setenv("LOG_PUSH_API", "http://test-api.example.com/push")
		os.Setenv("LOG_PUSH_TYPE", "webhook")
		os.Setenv("LOG_PUSH_TOKEN", "test-env-token")

		// Reload config to pick up env vars
		config.LogPushAPI = os.Getenv("LOG_PUSH_API")
		config.LogPushType = os.Getenv("LOG_PUSH_TYPE")
		config.LogPushToken = os.Getenv("LOG_PUSH_TOKEN")

		// This should not panic
		SetupEnhancedLogger(ctx)

		// Test that logger is working
		Logger.Info("test log message with environment variables")

		// Clean up
		os.Unsetenv("LOG_PUSH_API")
		os.Unsetenv("LOG_PUSH_TYPE")
		os.Unsetenv("LOG_PUSH_TOKEN")
	})
}

func TestLoggerErrorLevelWithAlertPusher(t *testing.T) {
	ctx := context.Background()

	// Test that error level logs would trigger alert pusher (if configured)
	t.Run("error_level_logging", func(t *testing.T) {
		// Set up with mock alert pusher config
		config.LogPushAPI = "http://mock-alert-api.example.com/push"
		config.LogPushType = "mock"
		config.LogPushToken = "mock-token"

		SetupEnhancedLogger(ctx)

		// Test error level logging (this would trigger alert pusher if URL was valid)
		Logger.Error("test error message for alert pusher",
			zap.String("component", "test"),
			zap.String("error_type", "test_error"))

		// Give a small delay to allow any async processing
		time.Sleep(100 * time.Millisecond)
	})
}

func TestLoggerDebugMode(t *testing.T) {
	ctx := context.Background()

	t.Run("debug_mode_enabled", func(t *testing.T) {
		// Enable debug mode
		originalDebugEnabled := config.DebugEnabled
		config.DebugEnabled = true

		SetupEnhancedLogger(ctx)

		// Test debug logging
		Logger.Debug("test debug message")
		Logger.Info("test info message in debug mode")

		// Restore original setting
		config.DebugEnabled = originalDebugEnabled
	})

	t.Run("debug_mode_disabled", func(t *testing.T) {
		// Disable debug mode
		originalDebugEnabled := config.DebugEnabled
		config.DebugEnabled = false

		SetupEnhancedLogger(ctx)

		// Test logging in production mode
		Logger.Info("test info message in production mode")

		// Restore original setting
		config.DebugEnabled = originalDebugEnabled
	})
}

func TestSetupLoggerWritesToFile(t *testing.T) {
	dir := t.TempDir()

	originalLogger := Logger
	originalLogDir := LogDir
	originalOnlyOne := config.OnlyOneLogFile
	originalDefaultWriter := gin.DefaultWriter
	originalDefaultErrorWriter := gin.DefaultErrorWriter

	t.Cleanup(func() {
		Logger = originalLogger
		LogDir = originalLogDir
		config.OnlyOneLogFile = originalOnlyOne
		gin.DefaultWriter = originalDefaultWriter
		gin.DefaultErrorWriter = originalDefaultErrorWriter
		ResetSetupLogOnceForTests()
	})

	LogDir = dir
	config.OnlyOneLogFile = true
	ResetSetupLogOnceForTests()

	SetupLogger()

	Logger.Info("file logging test entry")
	_ = Logger.Sync()

	logPath := filepath.Join(dir, "oneapi.log")
	content, err := os.ReadFile(logPath)
	require.NoError(t, err, "failed to read log file")
	require.Contains(t, string(content), "file logging test entry", "log file %s does not contain expected log entry", logPath)
}

func TestResetSetupLogOnceForTestsAllowsReconfiguration(t *testing.T) {
	originalLogger := Logger
	originalLogDir := LogDir
	originalOnlyOne := config.OnlyOneLogFile
	originalDefaultWriter := gin.DefaultWriter
	originalDefaultErrorWriter := gin.DefaultErrorWriter

	t.Cleanup(func() {
		Logger = originalLogger
		LogDir = originalLogDir
		config.OnlyOneLogFile = originalOnlyOne
		gin.DefaultWriter = originalDefaultWriter
		gin.DefaultErrorWriter = originalDefaultErrorWriter
		ResetSetupLogOnceForTests()
	})

	config.OnlyOneLogFile = true
	firstDir := t.TempDir()
	secondDir := t.TempDir()

	LogDir = firstDir
	ResetSetupLogOnceForTests()
	SetupLogger()
	Logger.Info("first directory setup complete")
	_ = Logger.Sync()

	firstLogPath := filepath.Join(firstDir, "oneapi.log")
	_, err := os.Stat(firstLogPath)
	require.NoError(t, err, "expected log file in first dir")

	LogDir = secondDir
	SetupLogger()
	secondLogPath := filepath.Join(secondDir, "oneapi.log")
	_, err = os.Stat(secondLogPath)
	require.True(t, os.IsNotExist(err), "log file %s should not exist before reset", secondLogPath)

	ResetSetupLogOnceForTests()
	SetupLogger()
	Logger.Info("second directory setup complete after reset")
	_ = Logger.Sync()

	_, err = os.Stat(secondLogPath)
	require.NoError(t, err, "expected log file after reset")
}

func TestStartLogRetentionCleaner(t *testing.T) {
	dir := t.TempDir()
	oldLog := filepath.Join(dir, "oneapi-20200101.log")
	err := os.WriteFile(oldLog, []byte("old"), 0o644)
	require.NoError(t, err, "failed to create old log file")
	cutoff := time.Now().Add(-48 * time.Hour)
	err = os.Chtimes(oldLog, cutoff, cutoff)
	require.NoError(t, err, "failed to set old log file times")

	freshLog := filepath.Join(dir, "oneapi.log")
	err = os.WriteFile(freshLog, []byte("fresh"), 0o644)
	require.NoError(t, err, "failed to create fresh log file")

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() {
		cancel()
		WaitForLogRetentionCleanerForTests()
	})

	StartLogRetentionCleaner(ctx, 1, dir)

	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, err := os.Stat(oldLog); os.IsNotExist(err) {
			break
		}
		require.True(t, time.Now().Before(deadline), "expired log file %s was not removed", oldLog)
		time.Sleep(10 * time.Millisecond)
	}

	_, err = os.Stat(freshLog)
	require.NoError(t, err, "fresh log file %s should not be removed", freshLog)
}

func TestDailyLogRotationCreatesNewFiles(t *testing.T) {
	dir := t.TempDir()
	originalLogger := Logger
	originalLogDir := LogDir
	originalOnlyOne := config.OnlyOneLogFile
	originalRotationInterval := config.LogRotationInterval
	originalDefaultWriter := gin.DefaultWriter
	originalDefaultErrorWriter := gin.DefaultErrorWriter

	t.Cleanup(func() {
		Logger = originalLogger
		LogDir = originalLogDir
		config.OnlyOneLogFile = originalOnlyOne
		config.LogRotationInterval = originalRotationInterval
		gin.DefaultWriter = originalDefaultWriter
		gin.DefaultErrorWriter = originalDefaultErrorWriter
		ResetSetupLogOnceForTests()
		ResetRotationNowFuncForTests()
	})

	ResetSetupLogOnceForTests()

	baseTime := time.Date(2025, 1, 2, 10, 0, 0, 0, time.UTC)
	currentTime := baseTime
	SetRotationNowFuncForTests(func() time.Time {
		return currentTime
	})

	LogDir = dir
	config.OnlyOneLogFile = false
	config.LogRotationInterval = "daily"

	SetupLogger()

	Logger.Info("first day entry")
	_ = Logger.Sync()

	firstDayPath := filepath.Join(dir, "oneapi-20250102.log")
	activeContent, err := os.ReadFile(firstDayPath)
	require.NoError(t, err)
	require.Contains(t, string(activeContent), "first day entry")

	currentTime = currentTime.Add(25 * time.Hour)

	Logger.Info("second day entry")
	_ = Logger.Sync()

	rotatedContent, err := os.ReadFile(firstDayPath)
	require.NoError(t, err)
	require.Contains(t, string(rotatedContent), "first day entry")

	secondDayPath := filepath.Join(dir, "oneapi-20250103.log")
	newActiveContent, err := os.ReadFile(secondDayPath)
	require.NoError(t, err)
	require.Contains(t, string(newActiveContent), "second day entry")
}

func TestLogRetentionCleanerAllowsLoggerReconfigure(t *testing.T) {
	dir := t.TempDir()

	originalLogger := Logger
	originalLogDir := LogDir
	originalOnlyOne := config.OnlyOneLogFile
	originalRotationInterval := config.LogRotationInterval
	originalDefaultWriter := gin.DefaultWriter
	originalDefaultErrorWriter := gin.DefaultErrorWriter

	t.Cleanup(func() {
		Logger = originalLogger
		LogDir = originalLogDir
		config.OnlyOneLogFile = originalOnlyOne
		config.LogRotationInterval = originalRotationInterval
		gin.DefaultWriter = originalDefaultWriter
		gin.DefaultErrorWriter = originalDefaultErrorWriter
		ResetSetupLogOnceForTests()
		WaitForLogRetentionCleanerForTests()
		ResetRotationNowFuncForTests()
	})

	ResetSetupLogOnceForTests()
	LogDir = dir
	config.OnlyOneLogFile = true
	SetupLogger()
	Logger.Info("first configuration active")
	_ = Logger.Sync()

	ctx, cancel := context.WithCancel(context.Background())
	StartLogRetentionCleaner(ctx, 1, dir)

	ResetSetupLogOnceForTests()
	config.OnlyOneLogFile = false
	config.LogRotationInterval = "daily"
	fixedTime := time.Date(2025, time.January, 5, 9, 0, 0, 0, time.UTC)
	SetRotationNowFuncForTests(func() time.Time { return fixedTime })
	SetupLogger()
	Logger.Info("second configuration active")
	_ = Logger.Sync()

	cancel()
	WaitForLogRetentionCleanerForTests()

	activeLogPath := filepath.Join(dir, fmt.Sprintf("oneapi-%s.log", fixedTime.Format("20060102")))
	activeContent, err := os.ReadFile(activeLogPath)
	require.NoError(t, err)
	require.Contains(t, string(activeContent), "second configuration active")
}
