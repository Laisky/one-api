package model

import (
	"context"
	"math"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const AsyncTaskLease = time.Minute

// ClaimAsyncTask claims one bounded, due job using a fenced database lease. A
// submitting lease is NEVER retried: if its worker died, acceptance is unknown.
// Polling leases may safely be reclaimed because they perform GETs only.
func ClaimAsyncTask(ctx context.Context, now time.Time) (*AsyncTask, error) {
	if DB == nil {
		return nil, errors.New("async task database unavailable")
	}
	nowMS := now.UTC().UnixMilli()
	db := DB.WithContext(ctx)
	var expired []string
	if err := db.Model(&AsyncTask{}).Where("state = ? AND lease_until <= ?", AsyncTaskSubmitting, nowMS).Order("lease_until ASC").Limit(100).Pluck("id", &expired).Error; err != nil {
		return nil, errors.Wrap(err, "find expired submitting leases")
	}
	if len(expired) > 0 {
		// Evidence is committed before accounting. A crash during supplemental
		// collection must not erase the accepted ID or resubmit the paid POST.
		if err := db.Model(&AsyncTask{}).Where("id IN ? AND state = ? AND lease_until <= ? AND upstream_id <> ?", expired, AsyncTaskSubmitting, nowMS, "").
			Updates(map[string]any{"state": AsyncTaskQueued, "error_code": "receipt_recovered", "lease_owner": "", "lease_until": int64(0), "next_poll_at": int64(0)}).Error; err != nil {
			return nil, errors.Wrap(err, "recover accepted async submissions")
		}
		if err := db.Model(&AsyncTask{}).Where("id IN ? AND state = ? AND lease_until <= ? AND (upstream_id = ? OR upstream_id IS NULL)", expired, AsyncTaskSubmitting, nowMS, "").
			Updates(map[string]any{"state": AsyncTaskUnknown, "error_code": "submission_unknown", "lease_owner": "", "lease_until": int64(0), "next_poll_at": int64(0)}).Error; err != nil {
			return nil, errors.Wrap(err, "recover uncertain async submissions")
		}
	}
	var tasks []AsyncTask
	if err := db.Where("(state IN ? OR (state = ? AND evidence_pending = ?)) AND billing_state = ? AND next_poll_at <= ? AND lease_until <= ?", []string{AsyncTaskReserved, AsyncTaskQueued, AsyncTaskRunning, AsyncTaskFailed, AsyncTaskCancelled}, AsyncTaskUnknown, true, AsyncBillingHeld, nowMS, nowMS).
		Order("next_poll_at ASC").Limit(16).Find(&tasks).Error; err != nil {
		return nil, errors.Wrap(err, "find due async tasks")
	}
	for i := range tasks {
		task := &tasks[i]
		owner := uuid.NewString()
		state := task.State
		if state == AsyncTaskReserved {
			state = AsyncTaskSubmitting
		}
		result := db.Model(&AsyncTask{}).Where("id = ? AND state = ? AND billing_state = ? AND lease_until <= ?", task.ID, task.State, AsyncBillingHeld, nowMS).
			Updates(map[string]any{"state": state, "lease_owner": owner, "lease_until": now.Add(AsyncTaskLease).UnixMilli()})
		if result.Error != nil {
			return nil, errors.Wrap(result.Error, "claim async task lease")
		}
		if result.RowsAffected == 1 {
			task.State, task.LeaseOwner, task.LeaseUntil = state, owner, now.Add(AsyncTaskLease).UnixMilli()
			return task, nil
		}
	}
	return nil, nil
}

// AsyncTaskUpdate is a provider-independent observation. Refund is authoritative
// only for rejected submission or explicit upstream refund confirmation.
type AsyncTaskUpdate struct {
	State        string
	UpstreamID   string
	ResultJSON   string
	ErrorCode    string
	Refund       bool
	NextPollAt   int64
	PollFailures int
	CostUSD      string
}

// ApplyAsyncTaskUpdate persists a fenced observation and atomically closes the
// quota hold when successful or explicitly refundable. Duplicate/stale workers
// cannot settle or refund again. Failures without refund evidence retain a hold.
func ApplyAsyncTaskUpdate(ctx context.Context, task *AsyncTask, update AsyncTaskUpdate) error {
	if DB == nil || task == nil || task.ID == "" || task.LeaseOwner == "" {
		return errors.New("invalid async task update")
	}
	switch update.State {
	case AsyncTaskQueued, AsyncTaskRunning, AsyncTaskCompleted, AsyncTaskFailed, AsyncTaskCancelled, AsyncTaskUnknown, AsyncTaskReconciliation:
	default:
		return errors.New("invalid async task state")
	}
	if update.Refund && update.State != AsyncTaskFailed && update.State != AsyncTaskCancelled {
		return errors.New("refund requires a definitive terminal failure")
	}
	if update.State == AsyncTaskCompleted && update.ResultJSON == "" {
		return errors.New("completed async task requires a result")
	}
	if len(update.UpstreamID) > 191 || len(update.ResultJSON) > MaxAsyncTaskBody || len(update.ErrorCode) > 64 {
		return errors.New("async task observation exceeds limits")
	}
	if (task.State == AsyncTaskFailed || task.State == AsyncTaskCancelled) && update.State != task.State && update.State != AsyncTaskReconciliation {
		return errors.New("terminal async task cannot change outcome")
	}
	leaseID, leaseState, leaseOwner := task.ID, task.State, task.LeaseOwner
	now := time.Now().UTC().UnixMilli()
	closed := update.State == AsyncTaskCompleted || update.Refund
	billingState := AsyncBillingHeld
	if update.State == AsyncTaskCompleted {
		billingState = AsyncBillingSettled
	} else if update.Refund {
		billingState = AsyncBillingRefunded
	}
	values := map[string]any{"state": update.State, "error_code": update.ErrorCode, "lease_owner": "", "lease_until": int64(0),
		"next_poll_at": update.NextPollAt, "poll_failures": update.PollFailures, "evidence_pending": false}
	if update.UpstreamID != "" {
		values["upstream_id"] = update.UpstreamID
		values["request_body"] = ""
	}
	if update.ResultJSON != "" {
		values["result_json"] = update.ResultJSON
	}
	if closed {
		values["billing_state"] = billingState
		values["completed_at"] = now
		values["request_body"] = ""
	}
	var token Token
	costChanged := false
	err := runWithSQLiteBusyRetryForDB(ctx, DB, func() error {
		return DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if update.UpstreamID != "" {
				// Lock and compare in Go: SQL equality may be case-insensitive or
				// ignore trailing spaces, but provider task IDs are byte-exact.
				// SQLite's dialect omits FOR UPDATE; its enclosing transaction and
				// busy retry still prevent a successful stale read/write pair.
				var accepted AsyncTask
				if err := tx.Select("upstream_id").Clauses(clause.Locking{Strength: "UPDATE"}).
					Where("id = ? AND state = ? AND lease_owner = ? AND lease_until > ? AND billing_state = ?", leaseID, leaseState, leaseOwner, now, AsyncBillingHeld).
					Take(&accepted).Error; err != nil {
					if errors.Is(err, gorm.ErrRecordNotFound) {
						return ErrAsyncLeaseLost
					}
					return errors.Wrap(err, "lock accepted async identity")
				}
				if accepted.UpstreamID != "" && accepted.UpstreamID != update.UpstreamID {
					return errors.New("async task observation conflicts with accepted identity")
				}
			}
			result := tx.Model(&AsyncTask{}).Where("id = ? AND state = ? AND lease_owner = ? AND lease_until > ? AND billing_state = ?", leaseID, leaseState, leaseOwner, now, AsyncBillingHeld).Updates(values)
			if result.Error != nil {
				return errors.Wrap(result.Error, "persist async task observation")
			}
			if result.RowsAffected != 1 {
				return ErrAsyncLeaseLost
			}
			var persisted AsyncTask
			if err := tx.Where("id = ?", task.ID).Take(&persisted).Error; err != nil {
				return errors.Wrap(err, "read locked async financial receipt")
			}
			// The persisted row, not a mutable worker argument, owns all money.
			task = &persisted
			changedCost, err := collectAsyncTaskCost(tx, task, update.CostUSD)
			if err != nil {
				return err
			}
			costChanged = changedCost
			if closed || changedCost {
				if err := tx.Model(&AsyncTask{}).Where("id = ?", task.ID).Updates(map[string]any{
					"billing_revision": gorm.Expr("billing_revision + 1"), "log_recorded": false,
					"log_next_attempt_at": int64(0), "log_failures": 0,
				}).Error; err != nil {
					return errors.Wrap(err, "schedule async financial receipt")
				}
			}
			if !closed && !changedCost {
				return nil
			}
			// Select owners in the same transaction. Never credit a different account
			// that reused a deleted numeric ID, or silently forget a failed refund.
			if err := tx.Where("id = ? AND uuid = ?", task.UserID, task.UserUUID).Take(&User{}).Error; err != nil {
				return errors.Wrap(err, "validate async task billing owner")
			}
			token = Token{UserId: task.UserID}
			if err := tx.Where("id = ? AND user_id = ? AND uuid = ?", task.TokenID, task.UserID, task.TokenUUID).Take(&token).Error; err != nil {
				if !errors.Is(err, gorm.ErrRecordNotFound) {
					return errors.Wrap(err, "validate async task billing token")
				}
				// A deleted token cannot invalidate already prepaid work. Preserve the
				// owner wallet but never mutate a replacement token with the same ID.
				token = Token{UserId: task.UserID}
			}
			token.UnlimitedQuota = task.TokenUnlimited
			if !closed {
				return nil
			}
			if update.Refund && task.Quota > 0 {
				if err := adjustAsyncTaskQuota(tx, task, -task.Quota, false); err != nil {
					return errors.Wrap(err, "refund async task reservation")
				}
			}
			if billingState == AsyncBillingSettled {
				result := tx.Model(&User{}).Where("id = ? AND uuid = ? AND used_quota <= ?", task.UserID, task.UserUUID, int64(math.MaxInt64)-task.Quota).
					Updates(map[string]any{"used_quota": gorm.Expr("used_quota + ?", task.Quota), "request_count": gorm.Expr("request_count + 1")})
				if result.Error != nil {
					return errors.Wrap(result.Error, "record async task usage")
				}
				if result.RowsAffected != 1 {
					return errors.New("async task user usage overflow")
				}
				// Channel removal must not prevent owner settlement. Fence ID reuse; a
				// removed channel's historical cost is still in the durable task receipt.
				channelWrite := tx.Model(&Channel{}).Where("id = ? AND uuid = ? AND used_quota <= ?", task.ChannelID, task.ChannelUUID, int64(math.MaxInt64)-task.Quota).
					Update("used_quota", gorm.Expr("used_quota + ?", task.Quota))
				if channelWrite.Error != nil {
					return errors.Wrap(channelWrite.Error, "record async task channel usage")
				}
				if channelWrite.RowsAffected == 0 && task.Quota > 0 {
					var present int64
					if err := tx.Model(&Channel{}).Where("id = ? AND uuid = ?", task.ChannelID, task.ChannelUUID).Count(&present).Error; err != nil {
						return errors.Wrap(err, "check async channel usage overflow")
					}
					if present != 0 {
						return errors.New("async task channel usage overflow")
					}
				}
			}
			return nil
		})
	})
	if err != nil {
		return errors.Wrap(err, "commit async task observation")
	}
	if closed || costChanged {
		refreshAsyncTaskQuotaCache(ctx, &token)
	}
	return nil
}

// CleanSettledAsyncTasks prunes a bounded batch of fully logged receipts after
// the 30-day idempotency window. Active, uncertain and financially held jobs are
// deliberately excluded so retention can never lose a paid outstanding task.
func CleanSettledAsyncTasks(ctx context.Context, now time.Time) error {
	if DB == nil {
		return errors.New("async task database unavailable")
	}
	var ids []string
	db := DB.WithContext(ctx)
	if err := db.Model(&AsyncTask{}).Where("completed_at > 0 AND completed_at < ? AND billing_state IN ? AND log_recorded = ?", now.Add(-30*24*time.Hour).UnixMilli(), []string{AsyncBillingSettled, AsyncBillingRefunded}, true).
		Order("completed_at ASC").Limit(100).Pluck("id", &ids).Error; err != nil {
		return errors.Wrap(err, "select expired async task receipts")
	}
	if len(ids) == 0 {
		return nil
	}
	return errors.Wrap(db.Where("id IN ?", ids).Delete(&AsyncTask{}).Error, "prune expired async task receipts")
}
