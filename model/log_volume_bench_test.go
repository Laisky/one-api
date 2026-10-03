package model

// Application log volume benchmark (proposal
// docs/proposals/20260905_observability-data-tiering.md, Phase 0 / W0.4).
//
// This file is written to compile UNCHANGED in both the pre-work tree
// (commit 397781e1) and the current tree, so the baseline is the real
// historical recordLogHelper rather than a reconstruction. It therefore uses
// only symbols that exist in both: recordLogHelper, Log, LOG_DB, logger.Logger,
// and the glog constructor options.
//
// It measures BYTES WRITTEN to a file sink for one billed request's "record
// log" line. That is the quantity W0.4 claims to reduce; measuring anything
// else (whole-process log volume, for example) would attribute savings to this
// change that it does not produce.

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	gmw "github.com/Laisky/gin-middlewares/v7"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	glogger "gorm.io/gorm/logger"

	"github.com/Laisky/one-api/common/logger"
)

// logVolumeContent enumerates realistic rendered content strings, so the report
// shows how the saving scales rather than only its most flattering point.
var logVolumeContent = []struct {
	name  string
	value string
}{
	{"content_short", "Model invocation recorded: model=gpt-4.1-mini, quota=$0.000012"},
	{"content_typical", "Model invocation recorded: model=gpt-4.1, channel=OpenAI Primary(17), quota=$0.0043210, tokens=1024 prompt/512 completion."},
	{"content_long", "Model invocation recorded: model=claude-sonnet-4-5-20260514, channel=Anthropic EU Pooled Gateway(214), quota=$0.1284310, tokens=131072 prompt/8192 completion, cache_read=98304, cache_write_5m=4096, cache_write_1h=1024, tools=web_search:3,code_interpreter:1, stream=true, system_prompt_reset=false."},
}

// setupLogVolumeBench points LOG_DB at a throwaway SQLite database and installs
// a file-backed logger whose output can be measured.
//
// Parameters:
//   - tb: the test or benchmark, used to fail fast and to register cleanup.
//   - encoding: the log encoding to measure. Production uses console encoding
//     (common/logger/logger.go configureGlobalLogger); JSON is measured too
//     because a shipping pipeline usually re-encodes to JSON downstream.
//
// Return values:
//   - string: path of the log file whose growth is measured.
func setupLogVolumeBench(tb testing.TB, encoding glog.Encoding) string {
	tb.Helper()

	dir := tb.TempDir()

	db, err := gorm.Open(sqlite.Open(filepath.Join(dir, "logs.db")), &gorm.Config{Logger: glogger.Discard})
	require.NoError(tb, err)
	require.NoError(tb, db.AutoMigrate(&Log{}))

	prevDB := LOG_DB
	LOG_DB = db

	// The compact INFO line is opt-in (LOG_RECORD_LINE_FORMAT), so the
	// benchmark must select it explicitly; otherwise it would measure the
	// pre-change line in both trees and report no saving at all. The constant
	// is referenced by value so this file still compiles in the pre-change
	// tree, where the knob does not exist.
	setRecordLineFormatForBench("compact")

	logPath := filepath.Join(dir, "app.log")
	fileLogger, err := glog.New(
		glog.WithName("bench"),
		glog.WithLevel(glog.LevelInfo),
		glog.WithEncoding(encoding),
		glog.WithOutputPaths([]string{logPath}),
		glog.WithErrorOutputPaths([]string{logPath}),
	)
	require.NoError(tb, err)

	prevLogger := logger.Logger
	logger.Logger = fileLogger

	tb.Cleanup(func() {
		logger.Logger = prevLogger
		LOG_DB = prevDB
	})

	return logPath
}

// logFileSize returns the current size of the measured log file.
//
// Parameters:
//   - tb: the test or benchmark, used to fail fast.
//   - path: the log file path.
//
// Return values:
//   - int64: the file size in bytes, or 0 when it does not exist yet.
func logFileSize(tb testing.TB, path string) int64 {
	tb.Helper()
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0
		}
		tb.Fatalf("stat log file: %v", err)
	}
	return info.Size()
}

// newLogVolumeRow builds one consume log row of the shape the billing pipeline
// produces.
//
// Parameters:
//   - content: the rendered content string for this row.
//
// Return values:
//   - *Log: the row to record.
func newLogVolumeRow(content string) *Log {
	return &Log{
		UserId:           4213,
		Username:         "acme-production",
		CreatedAt:        1757030400,
		Type:             LogTypeConsume,
		Content:          content,
		TokenName:        "prod-gateway-key",
		ModelName:        "gpt-4.1",
		Quota:            43210,
		PromptTokens:     1024,
		CompletionTokens: 512,
		ChannelId:        17,
		RequestId:        "01J9ZR8H4Q9N2V6XK3M7B5T0FA",
		TraceId:          "4bf92f3577b34da6a3ce929d0e0e4736",
		ElapsedTime:      1843,
	}
}

// BenchmarkRecordLogLineBytes measures the bytes one billed request's
// "record log" line writes to the application log at INFO level.
//
// In the pre-work tree this is the fat line carrying the whole rendered content
// string; in the current tree it is the compact line. The delta between the two
// trees is exactly what the W0.4 demotion saves, and nothing more.
//
// Parameters:
//   - b: the benchmark handle.
//
// Return values: none.
func BenchmarkRecordLogLineBytes(b *testing.B) {
	encodings := []struct {
		name  string
		value glog.Encoding
	}{
		{"console", glog.EncodingConsole},
		{"json", glog.EncodingJSON},
	}

	for _, enc := range encodings {
		for _, tc := range logVolumeContent {
			b.Run(enc.name+"/"+tc.name, func(b *testing.B) {
				logPath := setupLogVolumeBench(b, enc.value)
				row := newLogVolumeRow(tc.value)
				// recordLogHelper resolves its logger with logger.FromContext,
				// which prefers the request-scoped logger and falls back to
				// glog.Shared (silenced under `go test`) rather than to
				// logger.Logger. The measured logger must therefore be attached
				// to the context, exactly as middleware attaches it in
				// production.
				ctx := gmw.SetLogger(context.Background(), logger.Logger)

				// One warm-up write so file creation is not attributed to the loop.
				warm := *row
				recordLogHelper(ctx, &warm)
				require.NoError(b, logger.Logger.Sync())

				before := logFileSize(b, logPath)

				b.ResetTimer()
				for range b.N {
					entry := *row
					recordLogHelper(ctx, &entry)
				}
				b.StopTimer()

				require.NoError(b, logger.Logger.Sync())
				after := logFileSize(b, logPath)

				require.Greater(b, after, before, "the benchmark must actually write log bytes")
				b.ReportMetric(float64(after-before)/float64(b.N), "log_bytes/op")
			})
		}
	}
}

// setRecordLineFormatForBench selects the record-log line shape under
// measurement, where the running tree supports selecting one.
//
// The pre-change tree has no such knob and always emits the full line, so this
// is a no-op there; the build tag-free shim keeps one benchmark file valid in
// both trees.
//
// Parameters:
//   - format: "full" or "compact".
//
// Return values: none.
func setRecordLineFormatForBench(format string) {
	applyRecordLineFormat(format)
}
