package logger

// Application log-file retention throughput tests (proposal
// docs/proposals/20260905_observability-data-tiering.md, Phase 3 / W3.3).
//
// The file sweeper is the one retention target that removes files rather than
// rows, so it carries its own target label; everything else about its
// accounting -- completed vs canceled vs failed -- follows the same rule as the
// database sweeps.

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/metrics"
)

// logRetentionSweepSample is one recorded sweep.
type logRetentionSweepSample struct {
	target     string
	result     string
	rows       float64
	durationMs float64
}

// logRetentionSweepSpy captures retention sweep metrics.
//
// It is not safe for concurrent use: every test here runs its sweep on the test
// goroutine.
type logRetentionSweepSpy struct {
	*metrics.NoOpRecorder

	sweeps []logRetentionSweepSample
}

// RecordRetentionSweep implements metrics.RetentionRecorder.
//
// Parameters:
//   - target: the swept file set.
//   - result: one of the metrics.RetentionResult* constants.
//   - rows: files removed.
//   - durationMs: how long the sweep took.
//
// Return values: none.
func (s *logRetentionSweepSpy) RecordRetentionSweep(target, result string, rows, durationMs float64) {
	s.sweeps = append(s.sweeps, logRetentionSweepSample{
		target: target, result: result, rows: rows, durationMs: durationMs,
	})
}

// only returns the single recorded sweep, failing when there is not exactly one.
//
// Parameters:
//   - t: the test, used to fail fast.
//
// Return values:
//   - logRetentionSweepSample: the recorded sweep.
func (s *logRetentionSweepSpy) only(t *testing.T) logRetentionSweepSample {
	t.Helper()
	require.Len(t, s.sweeps, 1, "one sweep must produce exactly one sample")
	return s.sweeps[0]
}

// installLogRetentionSweepSpy swaps in a recorder capturing retention sweeps.
//
// Parameters:
//   - t: the test, used to register cleanup.
//
// Return values:
//   - *logRetentionSweepSpy: the installed recorder.
func installLogRetentionSweepSpy(t *testing.T) *logRetentionSweepSpy {
	t.Helper()
	spy := &logRetentionSweepSpy{NoOpRecorder: &metrics.NoOpRecorder{}}
	prev := metrics.Recorder()
	metrics.SetRecorder(spy)
	t.Cleanup(func() { metrics.SetRecorder(prev) })
	return spy
}

// TestLogRetentionSweepReportsCompletedThroughput verifies a sweep that examined
// everything it was asked to reports the files it actually removed.
func TestLogRetentionSweepReportsCompletedThroughput(t *testing.T) {
	dir := t.TempDir()
	writeLogFile(t, dir, "oneapi-20260101.log", 10, 72*time.Hour)
	writeLogFile(t, dir, "oneapi-20260102.log", 10, 48*time.Hour)
	writeLogFile(t, dir, "oneapi-20260103.log", 10, time.Minute)

	spy := installLogRetentionSweepSpy(t)
	runLogRetentionSweep(context.Background(), Logger, 1, dir, 0)

	sample := spy.only(t)
	require.Equal(t, retentionTargetAppLogFiles, sample.target)
	require.Equal(t, metrics.RetentionResultCompleted, sample.result)
	require.Equal(t, float64(2), sample.rows,
		"only the two expired files were removed")
	require.GreaterOrEqual(t, sample.durationMs, float64(0))

	require.NoFileExists(t, filepath.Join(dir, "oneapi-20260101.log"))
	require.FileExists(t, filepath.Join(dir, "oneapi-20260103.log"))
}

// TestLogRetentionSweepReportsCanceledOnShutdown verifies a sweep the shutdown
// cut short is reported as canceled, not completed: it left files behind.
func TestLogRetentionSweepReportsCanceledOnShutdown(t *testing.T) {
	dir := t.TempDir()
	writeLogFile(t, dir, "oneapi-20260101.log", 10, 72*time.Hour)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	spy := installLogRetentionSweepSpy(t)
	runLogRetentionSweep(ctx, Logger, 1, dir, 0)

	sample := spy.only(t)
	require.Equal(t, retentionTargetAppLogFiles, sample.target)
	require.Equal(t, metrics.RetentionResultCanceled, sample.result)
	require.Zero(t, sample.rows)
	require.FileExists(t, filepath.Join(dir, "oneapi-20260101.log"),
		"the cancelled sweep must have removed nothing, so work genuinely remained")
}

// TestLogRetentionSweepReportsFailure verifies a sweep that could not read its
// directory is reported as failed rather than as a completed sweep that happened
// to delete nothing.
func TestLogRetentionSweepReportsFailure(t *testing.T) {
	notADir := filepath.Join(t.TempDir(), "oneapi.log")
	require.NoError(t, os.WriteFile(notADir, []byte("x"), 0o600))

	spy := installLogRetentionSweepSpy(t)
	runLogRetentionSweep(context.Background(), Logger, 1, notADir, 0)

	sample := spy.only(t)
	require.Equal(t, retentionTargetAppLogFiles, sample.target)
	require.Equal(t, metrics.RetentionResultFailed, sample.result)
	require.Zero(t, sample.rows)
}

// TestDeleteExpiredLogFilesStopsAtAFileBoundary verifies the expiry pass honors
// cancellation the way the database sweeps honor a chunk boundary, which is what
// makes the canceled result above a measured fact rather than a label.
func TestDeleteExpiredLogFilesStopsAtAFileBoundary(t *testing.T) {
	dir := t.TempDir()
	writeLogFile(t, dir, "oneapi-20260101.log", 10, 72*time.Hour)
	writeLogFile(t, dir, "oneapi-20260102.log", 10, 72*time.Hour)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	deleted, err := deleteExpiredLogFiles(ctx, Logger, 1, dir)
	require.ErrorIs(t, err, context.Canceled)
	require.Zero(t, deleted)
	require.FileExists(t, filepath.Join(dir, "oneapi-20260101.log"))
}
