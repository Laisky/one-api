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

// TestAsyncBillingPoisonOutboxDoesNotBlockLaterReceipts injects individual
// failures in cost writing, log insertion and primary acknowledgement.
func TestAsyncBillingPoisonOutboxDoesNotBlockLaterReceipts(t *testing.T) {
	for _, stage := range []string{"cost", "log", "ack"} {
		t.Run(stage, func(t *testing.T) {
			_, template := durableJobTestStore(t)
			first := reserveDurableJob(t, template)
			completeDurableJob(t, first)
			secondInput := *template
			secondInput.DedupKey = AsyncTaskDedupKey(template.UserID, template.UserUUID, "second")
			secondInput.RequestID = "second-receipt"
			second := reserveDurableJob(t, &secondInput)
			completeDurableJob(t, second)
			require.NoError(t, DB.Model(&AsyncTask{}).Where("id = ?", first.ID).Update("updated_at", int64(1)).Error)
			if stage == "cost" {
				require.NoError(t, DB.Create(&UserRequestCost{RequestID: first.RequestID, UserID: first.UserID + 1, Quota: 77}).Error)
			}
			if stage == "log" {
				require.NoError(t, LOG_DB.Callback().Create().Before("gorm:create").Register("billing_poison", func(tx *gorm.DB) {
					if entry, ok := tx.Statement.Dest.(*Log); ok && entry.RequestId == first.RequestID {
						tx.AddError(errors.New("poisoned log"))
					}
				}))
				defer LOG_DB.Callback().Create().Remove("billing_poison")
			}
			if stage == "ack" {
				require.NoError(t, DB.Callback().Update().Before("gorm:update").Register("billing_poison", func(tx *gorm.DB) {
					if tx.Statement.Table == "async_tasks" {
						fields, ok := tx.Statement.Dest.(map[string]any)
						if ok && fields["log_recorded"] == true && strings.Contains(fmt.Sprint(tx.Statement.Clauses["WHERE"]), first.ID) {
							tx.AddError(errors.New("poisoned ack"))
						}
					}
				}))
				defer DB.Callback().Update().Remove("billing_poison")
			}
			beforeFlush := time.Now().UTC()
			err := FlushAsyncTaskLogs(context.Background())
			require.Error(t, err)
			var saved AsyncTask
			require.NoError(t, DB.Where("id = ?", second.ID).Take(&saved).Error)
			require.True(t, saved.LogRecorded, "a broken older %s must not starve a financially settled task", stage)
			var failed AsyncTask
			require.NoError(t, DB.Where("id = ?", first.ID).Take(&failed).Error)
			require.False(t, failed.LogRecorded)
			require.Equal(t, 1, failed.LogFailures, "the failed receipt must count its own attempt")
			require.Greater(t, failed.LogNextAttemptAt, beforeFlush.UnixMilli(), "a failed %s must back off, not occupy every batch", stage)
		})
	}
}

// TestAsyncBillingSettlementUsesPersistedFinancialSnapshot injects accidental
// mutation of a worker snapshot: only the durable reservation may move balances.
func TestAsyncBillingSettlementUsesPersistedFinancialSnapshot(t *testing.T) {
	_, template := durableJobTestStore(t)
	task := reserveDurableJob(t, template)
	lease, err := ClaimAsyncTask(context.Background(), time.Now())
	require.NoError(t, err)
	require.NotNil(t, lease)
	lease.Quota = 1
	lease.TokenUnlimited = true
	require.NoError(t, ApplyAsyncTaskUpdate(context.Background(), lease, AsyncTaskUpdate{State: AsyncTaskFailed, Refund: true}))
	var user User
	var token Token
	require.NoError(t, DB.Where("id = ?", task.UserID).Take(&user).Error)
	require.NoError(t, DB.Where("id = ?", task.TokenID).Take(&token).Error)
	require.EqualValues(t, 1000000, user.Quota)
	require.EqualValues(t, 1000000, token.RemainQuota)
	require.Zero(t, token.UsedQuota)
}

// TestAsyncBillingHeldTaskHasDurableCost proves users cannot hide paid work by
// disconnecting before it settles: the debited reservation is visible to audit.
func TestAsyncBillingHeldTaskHasDurableCost(t *testing.T) {
	_, template := durableJobTestStore(t)
	task := reserveDurableJob(t, template)
	require.NoError(t, FlushAsyncTaskLogs(context.Background()))
	var cost UserRequestCost
	require.NoError(t, DB.Where("request_id = ?", task.RequestID).Take(&cost).Error)
	require.Equal(t, task.Quota, cost.Quota)
	var entry Log
	require.NoError(t, LOG_DB.Where("request_id = ?", task.RequestID).Take(&entry).Error)
	require.EqualValues(t, task.Quota, entry.Quota)
}
