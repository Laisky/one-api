package model

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"gorm.io/gorm"
)

// AsyncTaskBinding persists the relationship between a long-running upstream task (e.g. video render job)
// and the channel/user metadata required to resume or poll that task later.
// Fields capture routing identifiers plus a trimmed request snapshot for diagnostics.
type AsyncTaskBinding struct {
	Id             int     `json:"id" gorm:"primaryKey;autoIncrement"`
	UUID           string  `json:"uuid" gorm:"type:char(36);index;column:uuid"`
	TaskID         string  `json:"task_id" gorm:"size:191;uniqueIndex;not null"`
	TaskType       string  `json:"task_type" gorm:"size:32;index;not null"`
	UserID         int     `json:"user_id" gorm:"index;not null"`
	UserUUID       *string `json:"user_uuid" gorm:"type:char(36);column:user_uuid;index"`
	TokenID        int     `json:"token_id" gorm:"index"`
	TokenUUID      *string `json:"token_uuid" gorm:"type:char(36);column:token_uuid;index"`
	ChannelID      int     `json:"channel_id" gorm:"index;not null"`
	ChannelUUID    *string `json:"channel_uuid" gorm:"type:char(36);column:channel_uuid;index"`
	ChannelType    int     `json:"channel_type" gorm:"index;not null"`
	OriginModel    string  `json:"origin_model" gorm:"size:128"`
	ActualModel    string  `json:"actual_model" gorm:"size:128"`
	RequestMethod  string  `json:"request_method" gorm:"size:16"`
	RequestPath    string  `json:"request_path" gorm:"size:255"`
	RequestParams  string  `json:"request_params" gorm:"type:text"`
	CreatedAt      int64   `json:"created_at" gorm:"autoCreateTime:milli;index"`
	UpdatedAt      int64   `json:"updated_at" gorm:"autoUpdateTime:milli"`
	LastAccessedAt int64   `json:"last_accessed_at" gorm:"index"`
}

// SaveAsyncTaskBinding creates or updates an async task binding for the provided task id.
// Existing rows are refreshed with the latest routing information and request snapshot.
func SaveAsyncTaskBinding(ctx context.Context, binding *AsyncTaskBinding) error {
	if binding == nil {
		return errors.New("async task binding cannot be nil")
	}
	binding.TaskID = strings.TrimSpace(binding.TaskID)
	binding.TaskType = strings.TrimSpace(binding.TaskType)
	if binding.TaskID == "" {
		return errors.New("async task binding requires task id")
	}
	if binding.TaskType == "" {
		return errors.New("async task binding requires task type")
	}
	if binding.ChannelID <= 0 {
		return errors.New("async task binding requires channel id")
	}
	if binding.ChannelType <= 0 {
		return errors.New("async task binding requires channel type")
	}
	if binding.UserID <= 0 {
		return errors.New("async task binding requires user id")
	}

	now := time.Now().UTC().UnixMilli()
	if binding.CreatedAt == 0 {
		binding.CreatedAt = now
	}
	binding.UpdatedAt = now
	if binding.LastAccessedAt == 0 {
		binding.LastAccessedAt = now
	}

	detached := context.Background()
	if ctx != nil {
		detached = context.WithoutCancel(ctx)
	}
	db := DB.WithContext(detached)

	existing := AsyncTaskBinding{}
	err := db.Where("task_id = ?", binding.TaskID).First(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		if err := db.Create(binding).Error; err != nil {
			return errors.Wrap(err, "create async task binding")
		}
		if lg := gmw.GetLogger(detached); lg != nil {
			lg.Debug("created async task binding",
				zap.String("task_id", binding.TaskID),
				zap.String("task_type", binding.TaskType),
				zap.Int("channel_id", binding.ChannelID))
		}
		return nil
	}
	if err != nil {
		return errors.Wrap(err, "query async task binding")
	}

	updates := map[string]any{
		"task_type":        binding.TaskType,
		"user_id":          binding.UserID,
		"user_uuid":        binding.UserUUID,
		"token_id":         binding.TokenID,
		"token_uuid":       binding.TokenUUID,
		"channel_id":       binding.ChannelID,
		"channel_uuid":     binding.ChannelUUID,
		"channel_type":     binding.ChannelType,
		"origin_model":     binding.OriginModel,
		"actual_model":     binding.ActualModel,
		"request_method":   binding.RequestMethod,
		"request_path":     binding.RequestPath,
		"request_params":   binding.RequestParams,
		"last_accessed_at": binding.LastAccessedAt,
		"updated_at":       binding.UpdatedAt,
	}

	if err := db.Model(&existing).Updates(updates).Error; err != nil {
		return errors.Wrap(err, "update async task binding")
	}
	if lg := gmw.GetLogger(detached); lg != nil {
		lg.Debug("updated async task binding",
			zap.String("task_id", binding.TaskID),
			zap.String("task_type", binding.TaskType),
			zap.Int("channel_id", binding.ChannelID))
	}
	return nil
}

// GetAsyncTaskBindingByTaskID fetches an async task binding by task id.
func GetAsyncTaskBindingByTaskID(ctx context.Context, taskID string) (*AsyncTaskBinding, error) {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return nil, errors.New("async task binding lookup requires task id")
	}

	detached := context.Background()
	if ctx != nil {
		detached = context.WithoutCancel(ctx)
	}

	binding := &AsyncTaskBinding{}
	if err := DB.WithContext(detached).Where("task_id = ?", taskID).First(binding).Error; err != nil {
		return nil, errors.Wrapf(err, "fetch async task binding: %s", taskID)
	}
	return binding, nil
}

// TouchAsyncTaskBinding updates the last accessed timestamp for a task binding when it is reused.
func TouchAsyncTaskBinding(ctx context.Context, taskID string) error {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return errors.New("async task binding touch requires task id")
	}

	detached := context.Background()
	if ctx != nil {
		detached = context.WithoutCancel(ctx)
	}

	now := time.Now().UTC().UnixMilli()
	tx := DB.WithContext(detached).
		Model(&AsyncTaskBinding{}).
		Where("task_id = ?", taskID).
		Updates(map[string]any{
			"last_accessed_at": now,
			"updated_at":       now,
		})
	if tx.Error != nil {
		return errors.Wrap(tx.Error, "touch async task binding")
	}
	if tx.RowsAffected == 0 {
		return errors.Wrap(gorm.ErrRecordNotFound, "touch async task binding")
	}
	return nil
}

// MarshalRequestMetadata safely serializes request metadata maps for storage.
func MarshalRequestMetadata(metadata map[string]any) (string, error) {
	if len(metadata) == 0 {
		return "", nil
	}
	encoded, err := json.Marshal(metadata)
	if err != nil {
		return "", errors.Wrap(err, "marshal async task request metadata")
	}
	return string(encoded), nil
}
