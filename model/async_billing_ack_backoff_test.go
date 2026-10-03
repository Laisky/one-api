package model

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// TestAsyncBillingAckBatchBackoff verifies that an entire batch of failed log
// acknowledgements backs off, lets later receipts progress, and recovers once.
func TestAsyncBillingAckBatchBackoff(t *testing.T) {
	_, input := durableJobTestStore(t)
	verifyAsyncBillingAckBatchBackoff(t, input)
}

// TestAsyncBillingAckBackoffLiveDatabases runs the same physical outbox and
// wallet assertions on disposable MySQL and PostgreSQL primary/log databases.
func TestAsyncBillingAckBackoffLiveDatabases(t *testing.T) {
	for _, backend := range []string{"mysql", "postgres"} {
		t.Run(backend, func(t *testing.T) {
			verifyAsyncBillingAckBatchBackoff(t, asyncBillingLiveStore(t, backend))
		})
	}
}

// verifyAsyncBillingAckBatchBackoff exercises a full failing acknowledgement
// batch and a healthy tail item. It asserts actual persisted backoff, revision
// fencing, unchanged debits, and exactly one log per task after retry recovery.
func verifyAsyncBillingAckBatchBackoff(t *testing.T, input *AsyncTask) {
	t.Helper()
	ctx := context.Background()
	const balance = 100000000
	require.NoError(t, DB.Model(&User{}).Where("id = ?", input.UserID).Update("quota", int64(balance)).Error)
	require.NoError(t, DB.Model(&Token{}).Where("id = ?", input.TokenID).Update("remain_quota", int64(balance)).Error)
	ids := make([]string, 0, 33)
	for i := range 33 {
		item := *input
		item.DedupKey = AsyncTaskDedupKey(item.UserID, item.UserUUID, fmt.Sprintf("ack-%d", i))
		item.RequestID = fmt.Sprintf("ack-receipt-%d", i)
		task := reserveDurableJob(t, &item)
		ids = append(ids, task.ID)
		// These are due retries. A 16-second fourth-attempt delay avoids a
		// timing race between the first and second pass on overloaded runners.
		require.NoError(t, DB.Model(&AsyncTask{}).Where("id = ?", task.ID).
			Updates(map[string]any{"updated_at": int64(i + 1), "log_failures": 3}).Error)
	}
	const callback = "async_ack_backoff_failure"
	require.NoError(t, DB.Callback().Update().Before("gorm:update").Register(callback, func(tx *gorm.DB) {
		fields, ok := tx.Statement.Dest.(map[string]any)
		if ok && tx.Statement.Table == "async_tasks" && fields["log_recorded"] == true &&
			!strings.Contains(fmt.Sprint(tx.Statement.Clauses["WHERE"]), ids[32]) {
			tx.AddError(errors.New("injected acknowledgement failure"))
		}
	}))
	removed := false
	t.Cleanup(func() {
		if !removed {
			require.NoError(t, DB.Callback().Update().Remove(callback))
		}
	})
	before := time.Now().UTC()
	require.Error(t, FlushAsyncTaskLogs(ctx))
	var blocked []AsyncTask
	require.NoError(t, DB.Where("id IN ?", ids[:32]).Find(&blocked).Error)
	require.Len(t, blocked, 32)
	for _, item := range blocked {
		require.False(t, item.LogRecorded)
		require.Equal(t, 4, item.LogFailures)
		require.GreaterOrEqual(t, item.LogNextAttemptAt, before.Add(16*time.Second).UnixMilli())
	}
	require.NoError(t, FlushAsyncTaskLogs(ctx), "failed acknowledgements must not starve the next batch")
	var healthy AsyncTask
	require.NoError(t, DB.Where("id = ?", ids[32]).Take(&healthy).Error)
	require.True(t, healthy.LogRecorded)
	require.Zero(t, healthy.LogFailures)
	require.Zero(t, healthy.LogNextAttemptAt)
	require.NoError(t, DB.Callback().Update().Remove(callback))
	removed = true
	// Make stored retries due without sleeping; do not change their receipts.
	require.NoError(t, DB.Model(&AsyncTask{}).Where("id IN ?", ids[:32]).Update("log_next_attempt_at", int64(0)).Error)
	require.NoError(t, FlushAsyncTaskLogs(ctx))
	require.NoError(t, FlushAsyncTaskLogs(ctx))
	var pending int64
	require.NoError(t, DB.Model(&AsyncTask{}).Where("log_recorded = ?", false).Count(&pending).Error)
	require.Zero(t, pending)
	var logs []Log
	require.NoError(t, LOG_DB.Find(&logs).Error)
	require.Len(t, logs, 33, "lost acknowledgements must not duplicate consumption logs")
	var costs []UserRequestCost
	require.NoError(t, DB.Find(&costs).Error)
	require.Len(t, costs, 33)
	for _, entry := range logs {
		require.EqualValues(t, input.Quota, entry.Quota)
	}
	for _, cost := range costs {
		require.Equal(t, input.Quota, cost.Quota)
	}
	user, token, channel := asyncBillingBalances(t, input)
	require.EqualValues(t, balance-33*input.Quota, user.Quota)
	require.EqualValues(t, balance-33*input.Quota, token.RemainQuota)
	require.EqualValues(t, 33*input.Quota, token.UsedQuota)
	require.Zero(t, channel.UsedQuota, "logging cannot settle still-held work")
}
