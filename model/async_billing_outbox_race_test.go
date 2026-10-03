package model

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestAsyncBillingStaleOutboxCannotOverwriteFinalChargeOrRefund sends an old
// held receipt after a newer final receipt and checks both physical databases.
func TestAsyncBillingStaleOutboxCannotOverwriteFinalChargeOrRefund(t *testing.T) {
	for _, refund := range []bool{false, true} {
		t.Run(fmt.Sprint(refund), func(t *testing.T) {
			_, input := durableJobTestStore(t)
			input.CostQuotaPerUSD = "500000"
			task := reserveDurableJob(t, input)
			stale := *task
			require.NoError(t, FlushAsyncTaskLogs(context.Background()))
			lease, err := ClaimAsyncTask(context.Background(), time.Now())
			require.NoError(t, err)
			update := AsyncTaskUpdate{State: AsyncTaskCompleted, CostUSD: "0.8", ResultJSON: `{"videos":[{"url":"https://media.example/v.mp4"}]}`}
			if refund {
				update = AsyncTaskUpdate{State: AsyncTaskFailed, Refund: true}
			}
			require.NoError(t, ApplyAsyncTaskUpdate(context.Background(), lease, update))
			require.NoError(t, FlushAsyncTaskLogs(context.Background()))
			require.NoError(t, writeAsyncTaskRequestCost(context.Background(), &stale))
			require.NoError(t, writeAsyncTaskLog(context.Background(), &stale))
			var cost UserRequestCost
			var logs []Log
			require.NoError(t, DB.Where("request_id = ?", task.RequestID).Take(&cost).Error)
			require.NoError(t, LOG_DB.Where("request_id = ?", task.RequestID).Find(&logs).Error)
			require.Len(t, logs, 1)
			expected := int64(400000)
			if refund {
				expected = 0
			}
			require.Equal(t, expected, cost.Quota)
			require.EqualValues(t, expected, logs[0].Quota)
		})
	}
}

// TestAsyncBillingWholePoisonBatchBacksOff exercises more poison rows than the
// batch limit: later valid work must be selected on the next pass, not starved.
func TestAsyncBillingWholePoisonBatchBacksOff(t *testing.T) {
	_, input := durableJobTestStore(t)
	for i := range 32 {
		item := *input
		item.ID = NewAsyncTaskID()
		item.DedupKey = AsyncTaskDedupKey(item.UserID, item.UserUUID, fmt.Sprint(i))
		item.State = AsyncTaskFailed
		item.BillingState = AsyncBillingRefunded
		item.BillingRevision = 1
		item.RequestID = fmt.Sprintf("poison-%d", i)
		item.UpdatedAt = 1
		require.NoError(t, DB.Create(&item).Error)
		require.NoError(t, DB.Create(&UserRequestCost{UserID: item.UserID + 1, RequestID: item.RequestID, Quota: 77}).Error)
	}
	valid := reserveDurableJob(t, input)
	require.Error(t, FlushAsyncTaskLogs(context.Background()))
	require.NoError(t, FlushAsyncTaskLogs(context.Background()))
	var saved AsyncTask
	require.NoError(t, DB.Where("id = ?", valid.ID).Take(&saved).Error)
	require.True(t, saved.LogRecorded)
	var pending int64
	require.NoError(t, DB.Model(&AsyncTask{}).Where("log_recorded = ? AND log_next_attempt_at > ?", false, time.Now().UnixMilli()).Count(&pending).Error)
	require.EqualValues(t, 32, pending)
}

// TestAsyncBillingNumericBounds rejects malformed, negative, extreme-exponent
// and overflowing charges. Valid small positive amounts round up, not to free.
func TestAsyncBillingNumericBounds(t *testing.T) {
	for _, bad := range []string{"-1", "NaN", "Infinity", "1e309", "1e-9999", "1e999999999", "1/2", "0x1p2", "null", "999999999999999999999999999999"} {
		_, err := AsyncUpstreamCostQuota("500000", bad)
		require.Error(t, err, bad)
	}
	for _, tiny := range []string{"0.0000001", "1e-300"} {
		quota, err := AsyncUpstreamCostQuota("500000", tiny)
		require.NoError(t, err)
		require.EqualValues(t, 1, quota)
	}
}
