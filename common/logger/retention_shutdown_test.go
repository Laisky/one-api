package logger

// Log retention worker shutdown (proposal
// docs/proposals/20260905_observability-data-tiering.md, Phase 1 / W1, row
// "Shutdown": "deadlines report unfinished work").
//
// The workers already honored ctx.Done() and already tracked themselves in
// retentionWorkerGroup; what was missing was a production way for the shutdown
// sequence to JOIN them within its deadline. These tests pin that contract.

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestWaitForRetentionWorkersJoinsStartedWorkers verifies the join blocks until
// the log retention workers have actually returned after their context is
// cancelled, rather than reporting success while they still run.
func TestWaitForRetentionWorkersJoinsStartedWorkers(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "oneapi.log"), []byte("fresh"), 0o644))

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	StartLogRetentionCleaner(ctx, 1, dir)
	require.Positive(t, retentionWorkersActive.Load(),
		"the retention sweep must register itself so a shutdown can join it")

	cancel()

	waitCtx, waitCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer waitCancel()
	require.NoError(t, WaitForRetentionWorkers(waitCtx))
	require.Zero(t, retentionWorkersActive.Load(),
		"every worker must have returned before the join reports success")
}

// TestWaitForRetentionWorkersReportsDeadline verifies a worker still running at
// the shutdown deadline is reported as unfinished work.
func TestWaitForRetentionWorkersReportsDeadline(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "oneapi.log"), []byte("fresh"), 0o644))

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() {
		cancel()
		joinCtx, joinCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer joinCancel()
		require.NoError(t, WaitForRetentionWorkers(joinCtx))
	})

	StartLogRetentionCleaner(ctx, 1, dir)

	// The workers' context is deliberately still live: this is the case where
	// the shutdown budget runs out while a worker is running.
	deadlineCtx, deadlineCancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer deadlineCancel()

	err := WaitForRetentionWorkers(deadlineCtx)
	require.Error(t, err)
	require.ErrorIs(t, err, context.DeadlineExceeded)
}

// TestWaitForRetentionWorkersReturnsWithoutWorkers verifies the zero-configuration
// deployment, which starts no log workers at all, is never named in the shutdown
// deadline report.
func TestWaitForRetentionWorkersReturnsWithoutWorkers(t *testing.T) {
	require.Eventually(t, func() bool { return retentionWorkersActive.Load() == 0 },
		10*time.Second, 10*time.Millisecond, "a log retention worker is still running")

	expired, cancel := context.WithDeadline(context.Background(), time.Now().UTC().Add(-time.Second))
	defer cancel()

	require.NoError(t, WaitForRetentionWorkers(expired))
}
