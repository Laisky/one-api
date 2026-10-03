package model

import (
	"context"
	"fmt"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// asyncBillingBalances reads physical wallet/usage state instead of cache or logs.
func asyncBillingBalances(t *testing.T, task *AsyncTask) (User, Token, Channel) {
	t.Helper()
	var user User
	var token Token
	var channel Channel
	require.NoError(t, DB.Where("id = ?", task.UserID).Take(&user).Error)
	require.NoError(t, DB.Where("id = ?", task.TokenID).Take(&token).Error)
	require.NoError(t, DB.Where("id = ?", task.ChannelID).Take(&channel).Error)
	return user, token, channel
}

// TestAsyncBillingStateMatrix checks the balance-conservation rule for every
// supported outcome, including unknown work and late explicit refunds.
func TestAsyncBillingStateMatrix(t *testing.T) {
	for _, unlimited := range []bool{false, true} {
		for _, state := range []string{AsyncTaskQueued, AsyncTaskRunning, AsyncTaskUnknown, AsyncTaskReconciliation, AsyncTaskFailed, AsyncTaskCancelled, AsyncTaskCompleted, "refunded"} {
			t.Run(fmt.Sprintf("unlimited=%t/%s", unlimited, state), func(t *testing.T) {
				_, input := durableJobTestStore(t)
				require.NoError(t, DB.Model(&Token{}).Where("id = ?", input.TokenID).Update("unlimited_quota", unlimited).Error)
				task := reserveDurableJob(t, input)
				require.NoError(t, FlushAsyncTaskLogs(context.Background()))
				lease, err := ClaimAsyncTask(context.Background(), time.Now())
				require.NoError(t, err)
				require.NotNil(t, lease)
				update := AsyncTaskUpdate{State: state, UpstreamID: "matrix-job"}
				refund := state == "refunded"
				if refund {
					update.State = AsyncTaskFailed
					update.Refund = true
				}
				if state == AsyncTaskCompleted {
					update.ResultJSON = `{"videos":[{"url":"https://media.example/v.mp4"}]}`
				}
				require.NoError(t, ApplyAsyncTaskUpdate(context.Background(), lease, update))
				require.ErrorIs(t, ApplyAsyncTaskUpdate(context.Background(), lease, update), ErrAsyncLeaseLost)
				require.NoError(t, FlushAsyncTaskLogs(context.Background()))
				user, token, channel := asyncBillingBalances(t, task)
				expectedDebit := task.Quota
				if refund {
					expectedDebit = 0
				}
				require.EqualValues(t, 1000000-expectedDebit, user.Quota)
				if unlimited {
					require.EqualValues(t, 1000000, token.RemainQuota)
				} else {
					require.EqualValues(t, 1000000-expectedDebit, token.RemainQuota)
					require.Equal(t, expectedDebit, token.UsedQuota)
				}
				if state == AsyncTaskCompleted {
					require.Equal(t, task.Quota, user.UsedQuota)
					require.EqualValues(t, 1, user.RequestCount)
					require.Equal(t, task.Quota, channel.UsedQuota)
				} else {
					require.Zero(t, user.UsedQuota)
					require.Zero(t, user.RequestCount)
					require.Zero(t, channel.UsedQuota)
				}
				var logs []Log
				require.NoError(t, LOG_DB.Where("request_id = ?", task.RequestID).Find(&logs).Error)
				require.Len(t, logs, 1)
				require.EqualValues(t, expectedDebit, logs[0].Quota)
				var cost UserRequestCost
				require.NoError(t, DB.Where("request_id = ?", task.RequestID).Take(&cost).Error)
				require.Equal(t, expectedDebit, cost.Quota)
			})
		}
	}
}

// TestAsyncBillingConcurrentDifferentRequestsCannotOverspend releases many
// distinct requests together; only fully funded transactions may be admitted.
func TestAsyncBillingConcurrentDifferentRequestsCannotOverspend(t *testing.T) {
	_, template := durableJobTestStore(t)
	const parallel = 32
	results := make(chan *AsyncTask, parallel)
	errs := make(chan error, parallel)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range parallel {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			input := *template
			input.DedupKey = AsyncTaskDedupKey(input.UserID, input.UserUUID, fmt.Sprint(i))
			input.RequestID = fmt.Sprintf("concurrent-%d", i)
			task, _, err := ReserveAsyncTask(context.Background(), &input)
			if err != nil {
				errs <- err
			} else {
				results <- task
			}
		}(i)
	}
	close(start)
	wg.Wait()
	close(results)
	close(errs)
	require.Len(t, results, 5)
	require.Len(t, errs, parallel-5)
	for err := range errs {
		require.ErrorIs(t, err, ErrAsyncQuota)
	}
	user, token, _ := asyncBillingBalances(t, template)
	require.Zero(t, user.Quota)
	require.Zero(t, token.RemainQuota)
	require.EqualValues(t, 1000000, token.UsedQuota)
	var count int64
	require.NoError(t, DB.Model(&AsyncTask{}).Count(&count).Error)
	require.EqualValues(t, 5, count)
}

// TestAsyncBillingConcurrentSettlementAndRefundHasSingleWinner ensures two
// processes sharing a valid lease cannot perform two financial transitions.
func TestAsyncBillingConcurrentSettlementAndRefundHasSingleWinner(t *testing.T) {
	_, input := durableJobTestStore(t)
	task := reserveDurableJob(t, input)
	lease, err := ClaimAsyncTask(context.Background(), time.Now())
	require.NoError(t, err)
	start := make(chan struct{})
	results := make(chan error, 20)
	var wg sync.WaitGroup
	for i := range 20 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			copy := *lease
			update := AsyncTaskUpdate{State: AsyncTaskCompleted, ResultJSON: `{"videos":[{"url":"https://media.example/v.mp4"}]}`}
			if i%2 == 1 {
				update = AsyncTaskUpdate{State: AsyncTaskFailed, Refund: true}
			}
			results <- ApplyAsyncTaskUpdate(context.Background(), &copy, update)
		}(i)
	}
	close(start)
	wg.Wait()
	close(results)
	winners := 0
	for err := range results {
		if err == nil {
			winners++
		} else {
			require.ErrorIs(t, err, ErrAsyncLeaseLost)
		}
	}
	require.Equal(t, 1, winners)
	stored, err := GetOwnedAsyncTask(context.Background(), task.ID, task.UserID, task.UserUUID)
	require.NoError(t, err)
	user, token, channel := asyncBillingBalances(t, task)
	if stored.BillingState == AsyncBillingSettled {
		require.EqualValues(t, 800000, user.Quota)
		require.EqualValues(t, 200000, user.UsedQuota)
		require.EqualValues(t, 1, user.RequestCount)
		require.EqualValues(t, 200000, channel.UsedQuota)
		require.EqualValues(t, 800000, token.RemainQuota)
	} else {
		require.Equal(t, AsyncBillingRefunded, stored.BillingState)
		require.EqualValues(t, 1000000, user.Quota)
		require.EqualValues(t, 1000000, token.RemainQuota)
		require.Zero(t, user.UsedQuota)
	}
}

// TestAsyncBillingCostAdjustmentsAreMonotonic covers quoted/actual differences,
// fractional quota, fixed tariffs, debt, missing cost and immutable multipliers.
func TestAsyncBillingCostAdjustmentsAreMonotonic(t *testing.T) {
	for _, test := range []struct {
		name, cost, factor string
		want               int64
	}{
		{"higher", "0.8", "500000", 400000}, {"lower", "0.1", "500000", 200000}, {"equal", "0.4", "500000", 200000},
		{"rounded up", "0.4000001", "500000", 200001}, {"zero actual", "0", "500000", 200000}, {"no actual", "", "500000", 200000},
		{"fixed admin tariff", "8", "", 200000}, {"captured markup", "0.8", "750000", 600000},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, input := durableJobTestStore(t)
			input.CostQuotaPerUSD = test.factor
			require.NoError(t, DB.Model(&User{}).Where("id = ?", input.UserID).Update("quota", 200000).Error)
			require.NoError(t, DB.Model(&Token{}).Where("id = ?", input.TokenID).Update("remain_quota", 200000).Error)
			task := reserveDurableJob(t, input)
			lease, err := ClaimAsyncTask(context.Background(), time.Now())
			require.NoError(t, err)
			lease.CostQuotaPerUSD = "0"
			lease.Quota = 0 // never trust a mutable worker price
			require.NoError(t, ApplyAsyncTaskUpdate(context.Background(), lease, AsyncTaskUpdate{State: AsyncTaskQueued, UpstreamID: "charge-job", CostUSD: test.cost}))
			lease, err = ClaimAsyncTask(context.Background(), time.Now())
			require.NoError(t, err)
			require.NoError(t, ApplyAsyncTaskUpdate(context.Background(), lease, AsyncTaskUpdate{State: AsyncTaskCompleted, CostUSD: "0", ResultJSON: `{"videos":[{"url":"https://media.example/v.mp4"}]}`}))
			user, token, channel := asyncBillingBalances(t, task)
			require.EqualValues(t, 200000-test.want, user.Quota)
			require.EqualValues(t, 200000-test.want, token.RemainQuota)
			require.Equal(t, test.want, token.UsedQuota)
			require.Equal(t, test.want, user.UsedQuota)
			require.Equal(t, test.want, channel.UsedQuota)
		})
	}
}

// TestAsyncBillingSettlementRollbackMatrix fails each write of final settlement
// or refund. The result and all monetary effects must commit together or not at all.
func TestAsyncBillingSettlementRollbackMatrix(t *testing.T) {
	for _, refund := range []bool{false, true} {
		for _, table := range []string{"async_tasks", "users", "tokens", "channels"} {
			if refund && table == "channels" {
				continue
			}
			t.Run(fmt.Sprintf("refund=%t/%s", refund, table), func(t *testing.T) {
				_, input := durableJobTestStore(t)
				input.CostQuotaPerUSD = "500000"
				task := reserveDurableJob(t, input)
				lease, err := ClaimAsyncTask(context.Background(), time.Now())
				require.NoError(t, err)
				require.NoError(t, DB.Callback().Update().Before("gorm:update").Register("billing_write_failure", func(tx *gorm.DB) {
					if tx.Statement.Table == table {
						tx.AddError(errors.New("injected financial write failure"))
					}
				}))
				update := AsyncTaskUpdate{State: AsyncTaskCompleted, CostUSD: "0.8", ResultJSON: `{"videos":[{"url":"https://media.example/v.mp4"}]}`}
				if refund {
					update = AsyncTaskUpdate{State: AsyncTaskFailed, Refund: true}
				}
				require.Error(t, ApplyAsyncTaskUpdate(context.Background(), lease, update))
				require.NoError(t, DB.Callback().Update().Remove("billing_write_failure"))
				saved, err := GetOwnedAsyncTask(context.Background(), task.ID, task.UserID, task.UserUUID)
				require.NoError(t, err)
				require.Equal(t, AsyncBillingHeld, saved.BillingState)
				require.Empty(t, saved.ResultJSON)
				user, token, channel := asyncBillingBalances(t, task)
				require.EqualValues(t, 800000, user.Quota)
				require.EqualValues(t, 800000, token.RemainQuota)
				require.Zero(t, user.UsedQuota)
				require.Zero(t, channel.UsedQuota)
				require.NoError(t, ApplyAsyncTaskUpdate(context.Background(), lease, update))
			})
		}
	}
}

// TestAsyncBillingChannelCounterOverflowDoesNotHideCost keeps the prepaid hold
// and withholds success when channel accounting would overflow instead of skipping it.
func TestAsyncBillingChannelCounterOverflowDoesNotHideCost(t *testing.T) {
	_, input := durableJobTestStore(t)
	task := reserveDurableJob(t, input)
	lease, err := ClaimAsyncTask(context.Background(), time.Now())
	require.NoError(t, err)
	require.NoError(t, DB.Model(&Channel{}).Where("id = ?", task.ChannelID).Update("used_quota", int64(math.MaxInt64)).Error)
	require.Error(t, ApplyAsyncTaskUpdate(context.Background(), lease, AsyncTaskUpdate{State: AsyncTaskCompleted, ResultJSON: `{"videos":[{"url":"https://media.example/v.mp4"}]}`}))
	user, _, _ := asyncBillingBalances(t, task)
	require.EqualValues(t, 800000, user.Quota)
	require.Zero(t, user.UsedQuota)
}
