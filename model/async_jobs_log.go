package model

import (
	"context"
	"time"

	"github.com/Laisky/errors/v2"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common/config"
)

// AsyncTaskLogReceipt serializes versioned log delivery inside LOG_DB. One task
// owns one log row, updated from held to settled/refunded, never duplicate charges.
// Revision fencing prevents a delayed hold writer from overwriting a final bill.
type AsyncTaskLogReceipt struct {
	TaskID    string `gorm:"primaryKey;size:64"`
	Revision  int64  `gorm:"not null;default:0"`
	LogID     int    `gorm:"not null;default:0"`
	CreatedAt int64  `gorm:"autoCreateTime:milli;index"`
}

// FlushAsyncTaskLogs delivers up to 32 due financial receipts, including held
// funds. Independent per-row backoff prevents poison records from blocking the
// batch or occupying every slot forever. Returned errors preserve retry evidence.
func FlushAsyncTaskLogs(ctx context.Context) error {
	if DB == nil || (config.IsLogConsumeEnabled() && LOG_DB == nil) {
		return errors.New("async task log databases unavailable")
	}
	var tasks []AsyncTask
	now := time.Now().UTC()
	if err := DB.WithContext(ctx).Where("log_recorded = ? AND log_next_attempt_at <= ?", false, now.UnixMilli()).Order("log_next_attempt_at ASC, updated_at ASC").Limit(32).Find(&tasks).Error; err != nil {
		return errors.Wrap(err, "load async task log outbox")
	}
	var firstErr error
	for i := range tasks {
		task := &tasks[i]
		err := writeAsyncTaskRequestCost(ctx, task)
		if err == nil && config.IsLogConsumeEnabled() {
			err = writeAsyncTaskLog(ctx, task)
		}
		current := DB.WithContext(ctx).Model(&AsyncTask{}).Where("id = ? AND billing_revision = ? AND billing_state = ? AND quota = ?", task.ID, task.BillingRevision, task.BillingState, task.Quota)
		if err == nil {
			err = current.Updates(map[string]any{"log_recorded": true, "log_failures": 0, "log_next_attempt_at": int64(0)}).Error
		}
		if err == nil {
			continue
		}
		if firstErr == nil {
			firstErr = errors.Wrapf(err, "deliver async financial receipt %s", task.ID)
		}
		failures := min(task.LogFailures+1, 8)
		next := now.Add(time.Duration(min(300, 1<<failures)) * time.Second).UnixMilli()
		if backoffErr := current.Updates(map[string]any{"log_failures": failures, "log_next_attempt_at": next}).Error; backoffErr != nil {
			firstErr = errors.Wrap(errors.Join(firstErr, backoffErr), "persist async receipt backoff")
		}
	}
	return firstErr
}

// asyncTaskFinancialQuota returns the total already removed from the wallet.
// Uncertain/held work is NOT a zero-cost failure; only a committed refund is zero.
func asyncTaskFinancialQuota(task *AsyncTask) int64 {
	if task.BillingState == AsyncBillingRefunded {
		return 0
	}
	return task.Quota
}

// writeAsyncTaskLog atomically fences a newer financial revision and writes its
// single log row. Lost commits and stale/concurrent writers are repeat-safe.
func writeAsyncTaskLog(ctx context.Context, task *AsyncTask) error {
	db := LOG_DB.WithContext(ctx)
	err := runWithSQLiteBusyRetryForDB(ctx, LOG_DB, func() error {
		return db.Transaction(func(tx *gorm.DB) error {
			var receipt AsyncTaskLogReceipt
			lookup := tx.Where("task_id = ?", task.ID).Take(&receipt).Error
			switch {
			case errors.Is(lookup, gorm.ErrRecordNotFound):
				receipt = AsyncTaskLogReceipt{TaskID: task.ID, Revision: task.BillingRevision}
				if err := tx.Create(&receipt).Error; err != nil {
					return errors.Wrap(err, "create async log receipt")
				}
			case lookup != nil:
				return errors.Wrap(lookup, "read async log receipt")
			case receipt.Revision >= task.BillingRevision:
				return nil
			default:
				result := tx.Model(&AsyncTaskLogReceipt{}).Where("task_id = ? AND revision = ?", task.ID, receipt.Revision).Update("revision", task.BillingRevision)
				if result.Error != nil {
					return errors.Wrap(result.Error, "fence async log revision")
				}
				if result.RowsAffected == 0 {
					return nil
				}
			}
			entry := &Log{UserId: task.UserID, UserUUID: StringPtrIfNotEmpty(task.UserUUID),
				ChannelId: task.ChannelID, ChannelUUID: StringPtrIfNotEmpty(task.ChannelUUID),
				TokenName: task.TokenName, TokenUUID: StringPtrIfNotEmpty(task.TokenUUID),
				Type: LogTypeConsume, CreatedAt: task.CreatedAt / 1000, ModelName: task.OriginModel, OriginModelName: task.OriginModel,
				Quota: int(asyncTaskFinancialQuota(task)), RequestId: task.RequestID, TraceId: task.TraceID,
				Content: "async video " + task.BillingState, ElapsedTime: max(int64(0), task.CompletedAt-task.CreatedAt),
				Metadata: LogMetadata{"async_task_id": task.ID, "task_status": task.State, "billing_status": task.BillingState}}
			if receipt.LogID > 0 {
				// A retention sweep may have removed an old held log; recreate it only if
				// absent. The primary financial receipt remains authoritative throughout.
				var existing Log
				lookup := tx.Where("id = ?", receipt.LogID).Take(&existing).Error
				if lookup == nil {
					result := tx.Model(&existing).Updates(map[string]any{"quota": entry.Quota, "content": entry.Content, "elapsed_time": entry.ElapsedTime, "metadata": entry.Metadata})
					return errors.Wrap(result.Error, "update async consume log")
				}
				if !errors.Is(lookup, gorm.ErrRecordNotFound) {
					return errors.Wrap(lookup, "read async consume log")
				}
			}
			if err := tx.Create(entry).Error; err != nil {
				return errors.Wrap(err, "write async consume log")
			}
			return errors.Wrap(tx.Model(&AsyncTaskLogReceipt{}).Where("task_id = ?", task.ID).Update("log_id", entry.Id).Error, "link async consume log")
		})
	})
	if err != nil {
		var existing AsyncTaskLogReceipt
		if lookup := db.Where("task_id = ?", task.ID).Take(&existing).Error; lookup == nil && existing.Revision >= task.BillingRevision {
			return nil
		}
		return errors.Wrap(err, "commit async log receipt")
	}
	return nil
}

// writeAsyncTaskRequestCost locks the original task while mirroring its CURRENT
// financial state into the existing cost surface. Old outbox snapshots can never
// overwrite a newer refund/charge. This lock is released before contacting LOG_DB.
func writeAsyncTaskRequestCost(ctx context.Context, task *AsyncTask) error {
	if task.RequestID == "" || len(task.RequestID) > RequestIDMaxLen {
		return nil
	}
	return runWithSQLiteBusyRetryForDB(ctx, DB, func() error {
		return DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			// An UPDATE also locks the matched row on MySQL when its value is unchanged.
			if err := tx.Model(&AsyncTask{}).Where("id = ?", task.ID).UpdateColumn("billing_revision", gorm.Expr("billing_revision")).Error; err != nil {
				return errors.Wrap(err, "lock async cost receipt")
			}
			var current AsyncTask
			if err := tx.Where("id = ?", task.ID).Take(&current).Error; err != nil {
				return errors.Wrap(err, "read current async cost receipt")
			}
			var existing UserRequestCost
			lookup := tx.Where("request_id = ?", current.RequestID).Take(&existing).Error
			if errors.Is(lookup, gorm.ErrRecordNotFound) {
				entry := &UserRequestCost{UserID: current.UserID, UserUUID: StringPtrIfNotEmpty(current.UserUUID), RequestID: current.RequestID, Quota: asyncTaskFinancialQuota(&current), CreatedTime: current.CreatedAt / 1000}
				return errors.Wrap(tx.Create(entry).Error, "create async request cost")
			}
			if lookup != nil {
				return errors.Wrap(lookup, "load async request cost")
			}
			if existing.UserID != current.UserID || (existing.UserUUID != nil && *existing.UserUUID != current.UserUUID) {
				return errors.New("async request cost owner mismatch")
			}
			return errors.Wrap(tx.Model(&UserRequestCost{}).Where("id = ? AND user_id = ?", existing.Id, current.UserID).Update("quota", asyncTaskFinancialQuota(&current)).Error, "update async request cost")
		})
	})
}

// CleanAsyncTaskLogReceipts deletes only bounded, old receipts whose original
// task no longer exists in the primary database. This ordering is safe with split
// LOG_DB: a crash before acknowledgement leaves the task and therefore its receipt.
func CleanAsyncTaskLogReceipts(ctx context.Context, now time.Time) error {
	if DB == nil || LOG_DB == nil {
		return errors.New("async task log databases unavailable")
	}
	var receipts []AsyncTaskLogReceipt
	if err := LOG_DB.WithContext(ctx).Where("created_at < ?", now.Add(-31*24*time.Hour).UnixMilli()).Order("created_at ASC").Limit(100).Find(&receipts).Error; err != nil {
		return errors.Wrap(err, "find expired async log receipts")
	}
	for _, receipt := range receipts {
		var count int64
		if err := DB.WithContext(ctx).Model(&AsyncTask{}).Where("id = ?", receipt.TaskID).Count(&count).Error; err != nil {
			return errors.Wrap(err, "check retained async task")
		}
		if count == 0 {
			if err := LOG_DB.WithContext(ctx).Where("task_id = ?", receipt.TaskID).Delete(&AsyncTaskLogReceipt{}).Error; err != nil {
				return errors.Wrap(err, "prune async log receipt")
			}
		}
	}
	return nil
}
