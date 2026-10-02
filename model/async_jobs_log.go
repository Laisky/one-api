package model

import (
	"context"
	"time"

	"github.com/Laisky/errors/v2"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common/config"
)

// AsyncTaskLogReceipt is the idempotency key for the log outbox. It lives in
// LOG_DB, in the SAME transaction as Log, including split-database deployments.
// Log.UUID is not unique and must never be used as an exactly-once constraint.
type AsyncTaskLogReceipt struct {
	TaskID    string `gorm:"primaryKey;size:64"`
	CreatedAt int64  `gorm:"autoCreateTime:milli;index"`
}

// FlushAsyncTaskLogs delivers a bounded batch of terminal financial receipts.
// The main task is acknowledged only after the log transaction commits; a crash
// between databases replays the receipt, not the charge and not a duplicate log.
func FlushAsyncTaskLogs(ctx context.Context) error {
	if DB == nil || LOG_DB == nil {
		return errors.New("async task log databases unavailable")
	}
	var tasks []AsyncTask
	if err := DB.WithContext(ctx).Where("log_recorded = ? AND billing_state IN ?", false, []string{AsyncBillingSettled, AsyncBillingRefunded}).Order("updated_at ASC").Limit(32).Find(&tasks).Error; err != nil {
		return errors.Wrap(err, "load async task log outbox")
	}
	for i := range tasks {
		task := &tasks[i]
		if err := writeAsyncTaskRequestCost(ctx, task); err != nil {
			return err
		}
		if config.IsLogConsumeEnabled() {
			if err := writeAsyncTaskLog(ctx, task); err != nil {
				return err
			}
		}
		if err := DB.WithContext(ctx).Model(&AsyncTask{}).Where("id = ? AND billing_state = ?", task.ID, task.BillingState).Update("log_recorded", true).Error; err != nil {
			return errors.Wrap(err, "acknowledge async task log")
		}
	}
	// Receipt cleanup separately checks that the primary task is absent.
	return nil
}

// writeAsyncTaskLog inserts a unique outbox receipt and its public usage log in
// one LOG_DB transaction. A duplicate commit acknowledgement is treated as success.
func writeAsyncTaskLog(ctx context.Context, task *AsyncTask) error {
	db := LOG_DB.WithContext(ctx)
	var existing AsyncTaskLogReceipt
	err := db.Where("task_id = ?", task.ID).Take(&existing).Error
	if err == nil {
		return nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return errors.Wrap(err, "find async task log receipt")
	}
	err = db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&AsyncTaskLogReceipt{TaskID: task.ID}).Error; err != nil {
			return errors.Wrap(err, "create async task log receipt")
		}
		quota := int64(0)
		if task.BillingState == AsyncBillingSettled {
			quota = task.Quota
		}
		entry := &Log{UserId: task.UserID, UserUUID: StringPtrIfNotEmpty(task.UserUUID),
			ChannelId: task.ChannelID, ChannelUUID: StringPtrIfNotEmpty(task.ChannelUUID),
			TokenName: task.TokenName, TokenUUID: StringPtrIfNotEmpty(task.TokenUUID),
			Type: LogTypeConsume, CreatedAt: time.Now().UTC().Unix(), ModelName: task.OriginModel, OriginModelName: task.OriginModel,
			Quota: int(quota), RequestId: task.RequestID, TraceId: task.TraceID,
			Content: "async video " + task.BillingState, ElapsedTime: task.CompletedAt - task.CreatedAt,
			Metadata: LogMetadata{"async_task_id": task.ID, "task_status": task.State, "billing_status": task.BillingState}}
		return errors.Wrap(tx.Create(entry).Error, "write async task consume log")
	})
	if err != nil {
		if lookup := db.Where("task_id = ?", task.ID).Take(&existing).Error; lookup == nil {
			return nil
		}
		return errors.Wrap(err, "commit async task log receipt")
	}
	return nil
}

// writeAsyncTaskRequestCost mirrors the settled receipt into the existing request
// cost surface. The original request ID is server-generated. Owner fencing avoids
// overwriting another user's receipt when an integration reuses an identifier.
func writeAsyncTaskRequestCost(ctx context.Context, task *AsyncTask) error {
	if task.RequestID == "" || len(task.RequestID) > RequestIDMaxLen {
		return nil
	}
	quota := int64(0)
	if task.BillingState == AsyncBillingSettled {
		quota = task.Quota
	}
	db := DB.WithContext(ctx)
	var existing UserRequestCost
	lookup := db.Where("request_id = ?", task.RequestID).Take(&existing).Error
	if errors.Is(lookup, gorm.ErrRecordNotFound) {
		entry := &UserRequestCost{UserID: task.UserID, UserUUID: StringPtrIfNotEmpty(task.UserUUID), RequestID: task.RequestID, Quota: quota, CreatedTime: time.Now().UTC().Unix()}
		if err := db.Create(entry).Error; err == nil {
			return nil
		}
		lookup = db.Where("request_id = ?", task.RequestID).Take(&existing).Error
	}
	if lookup != nil {
		return errors.Wrap(lookup, "load async request cost")
	}
	if existing.UserID != task.UserID || (existing.UserUUID != nil && *existing.UserUUID != task.UserUUID) {
		return errors.New("async request cost owner mismatch")
	}
	return errors.Wrap(db.Model(&UserRequestCost{}).Where("id = ? AND user_id = ?", existing.Id, task.UserID).Update("quota", quota).Error, "update async request cost")
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
