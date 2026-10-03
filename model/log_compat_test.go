package model

// Backward-compatibility regression tests for the Phase-0 W0.4 log-volume work
// (proposal docs/proposals/20260905_observability-data-tiering.md).
//
// The compact "record log" INFO line drops fields an operator's log pipeline may
// parse, so it must never become active without being asked for. These tests pin
// that: the default emits the pre-proposal line, and the compact form appears
// only when selected.

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	gmw "github.com/Laisky/gin-middlewares/v7"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	glogger "gorm.io/gorm/logger"

	"github.com/Laisky/one-api/common/config"
)

// captureRecordLogLine records one consume log and returns the decoded JSON of
// the "record log" line it emitted.
//
// Parameters:
//   - t: the test, used to fail fast and register cleanup.
//   - format: the config.LogRecordLine* value to apply.
//
// Return values:
//   - map[string]any: the decoded log entry.
func captureRecordLogLine(t *testing.T, format string) map[string]any {
	t.Helper()

	prevFormat := config.LogRecordLineFormat
	config.LogRecordLineFormat = format
	t.Cleanup(func() { config.LogRecordLineFormat = prevFormat })

	dir := t.TempDir()
	db, err := gorm.Open(sqlite.Open(filepath.Join(dir, "logs.db")), &gorm.Config{Logger: glogger.Discard})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Log{}))

	prevDB := LOG_DB
	LOG_DB = db
	t.Cleanup(func() { LOG_DB = prevDB })

	logPath := filepath.Join(dir, "app.log")
	fileLogger, err := glog.New(
		glog.WithName("compat"),
		glog.WithLevel(glog.LevelInfo),
		glog.WithEncoding(glog.EncodingJSON),
		glog.WithOutputPaths([]string{logPath}),
		glog.WithErrorOutputPaths([]string{logPath}),
	)
	require.NoError(t, err)

	ctx := gmw.SetLogger(context.Background(), fileLogger)
	recordLogHelper(ctx, &Log{
		UserId:           7,
		Username:         "compat-user",
		CreatedAt:        1757030400,
		Type:             LogTypeConsume,
		Content:          "Model invocation recorded: model=gpt-4.1, quota=$0.004321",
		ModelName:        "gpt-4.1",
		Quota:            4321,
		PromptTokens:     128,
		CompletionTokens: 64,
		RequestId:        "req-compat-1",
		TraceId:          "trace-compat-1",
	})
	require.NoError(t, fileLogger.Sync())

	raw, err := os.ReadFile(logPath)
	require.NoError(t, err)

	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		if entry["message"] == "record log" {
			return entry
		}
	}
	t.Fatalf("no \"record log\" line was emitted; log contents:\n%s", raw)
	return nil
}

// TestRecordLogLineDefaultsToPreProposalShape verifies an operator who upgrades
// without changing configuration keeps the log line they had.
//
// Parameters:
//   - t: the test handle.
//
// Return values: none.
func TestRecordLogLineDefaultsToPreProposalShape(t *testing.T) {
	require.Equal(t, config.LogRecordLineFull, config.LogRecordLineFormat,
		"the standalone default must keep the pre-proposal line; changing it would break log pipelines on upgrade")

	entry := captureRecordLogLine(t, config.LogRecordLineFull)

	require.Contains(t, entry, "content", "the default line must still carry content")
	require.Contains(t, entry, "created_at", "the default line must still carry created_at")
	require.Contains(t, entry, "log_request_id")
	require.Contains(t, entry, "log_trace_id")
	require.EqualValues(t, 4321, entry["quota"])
}

// TestRecordLogLineCompactIsOptIn verifies the compact form drops exactly the
// two fields it claims to, and only when selected.
//
// Parameters:
//   - t: the test handle.
//
// Return values: none.
func TestRecordLogLineCompactIsOptIn(t *testing.T) {
	entry := captureRecordLogLine(t, config.LogRecordLineCompact)

	require.NotContains(t, entry, "content", "the compact line drops content")
	require.NotContains(t, entry, "created_at", "the compact line drops created_at")

	// Everything an operator greps for on the billing path must survive.
	require.Contains(t, entry, "log_request_id")
	require.Contains(t, entry, "log_trace_id")
	require.EqualValues(t, 4321, entry["quota"])
	require.EqualValues(t, 128, entry["prompt_tokens"])
	require.EqualValues(t, 64, entry["completion_tokens"])
	require.EqualValues(t, LogTypeConsume, entry["type"])
}
