package model

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestAsyncBillingReceiptRetentionSkipsLivePrefix proves a full oldest batch of
// unresolved paid tasks cannot permanently starve cleanup of later orphan logs.
func TestAsyncBillingReceiptRetentionSkipsLivePrefix(t *testing.T) {
	_, input := durableJobTestStore(t)
	verifyAsyncReceiptRetentionProgress(t, input)
}

// TestAsyncBillingRetentionLiveDatabases runs the bounded retention progress
// assertions on real MySQL and PostgreSQL primary/log databases.
func TestAsyncBillingRetentionLiveDatabases(t *testing.T) {
	for _, backend := range []string{"mysql", "postgres"} {
		t.Run(backend, func(t *testing.T) { verifyAsyncReceiptRetentionProgress(t, asyncBillingLiveStore(t, backend)) })
	}
}

// verifyAsyncReceiptRetentionProgress creates real prepaid receipts, prunes one
// completed task, and checks two bounded sweeps pass a live 100-receipt prefix.
func verifyAsyncReceiptRetentionProgress(t *testing.T, input *AsyncTask) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()
	finished := reserveDurableJob(t, input)
	completeDurableJob(t, finished)
	require.NoError(t, FlushAsyncTaskLogs(ctx))
	require.NoError(t, DB.Model(&AsyncTask{}).Where("id = ?", finished.ID).Update("completed_at", now.Add(-32*24*time.Hour).UnixMilli()).Error)
	require.NoError(t, LOG_DB.Model(&AsyncTaskLogReceipt{}).Where("task_id = ?", finished.ID).Update("created_at", now.Add(-32*24*time.Hour).UnixMilli()).Error)
	require.NoError(t, CleanSettledAsyncTasks(ctx, now))
	for i := range 100 {
		item := *input
		item.Quota = 1
		item.DedupKey = AsyncTaskDedupKey(item.UserID, item.UserUUID, fmt.Sprintf("retention-%d", i))
		item.RequestID = fmt.Sprintf("retention-%d", i)
		reserveDurableJob(t, &item)
	}
	for range 4 {
		require.NoError(t, FlushAsyncTaskLogs(ctx))
	}
	require.NoError(t, LOG_DB.Model(&AsyncTaskLogReceipt{}).Where("task_id <> ?", finished.ID).Update("created_at", now.Add(-33*24*time.Hour).UnixMilli()).Error)
	for range 2 {
		require.NoError(t, CleanAsyncTaskLogReceipts(ctx, now))
	}
	var count int64
	require.NoError(t, LOG_DB.Model(&AsyncTaskLogReceipt{}).Where("task_id = ?", finished.ID).Count(&count).Error)
	require.Zero(t, count, "held receipt prefix must not monopolize every cleanup batch")
	require.NoError(t, LOG_DB.Model(&AsyncTaskLogReceipt{}).Count(&count).Error)
	require.EqualValues(t, 100, count, "unresolved receipts must remain")
	require.NoError(t, DB.Model(&AsyncTask{}).Where("billing_state = ?", AsyncBillingHeld).Count(&count).Error)
	require.EqualValues(t, 100, count)
}
