// Package asyncvideo persists video jobs without importing any provider adaptor.
package asyncvideo

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/ctxkey"
	dbmodel "github.com/Laisky/one-api/model"
	metalib "github.com/Laisky/one-api/relay/meta"
)

const videoTaskType = "video"

var pendingBindings sync.Map

type pendingBinding struct {
	mu     sync.Mutex
	record dbmodel.AsyncTaskBinding
}

// PersistTask binds an async video-generation task id to the channel
// that created it, so follow-up status/content requests can be pinned to the
// original upstream. The input is an internal {id} envelope, not necessarily
// the provider response. Callers retain their provider-specific wire contract.
func PersistTask(c *gin.Context, body []byte) error {
	if c == nil {
		return errors.New("async task binding request context is nil")
	}
	if len(body) == 0 {
		return errors.New("async task binding response body is empty")
	}
	var payload struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return errors.Wrap(err, "parse async task binding")
	}
	taskID := strings.TrimSpace(payload.ID)
	if taskID == "" {
		return errors.New("async task binding response has no task id")
	}
	if c.Request == nil {
		return errors.New("async task binding request is missing")
	}
	metaInfo := metalib.GetByContext(c)
	if metaInfo == nil || metaInfo.ChannelId == 0 || metaInfo.ChannelType == 0 || metaInfo.UserId == 0 {
		return errors.New("async task binding is missing user, channel, or channel type metadata")
	}

	var snapshot map[string]any
	if raw, ok := c.Get(ctxkey.AsyncTaskRequestMetadata); ok {
		if typed, ok := raw.(map[string]any); ok {
			snapshot = typed
		}
	}
	requestJSON, err := dbmodel.MarshalRequestMetadata(snapshot)
	if err != nil {
		if logger := gmw.GetLogger(c); logger != nil {
			logger.Warn("failed to marshal async task snapshot", zap.Error(err))
		}
		requestJSON = ""
	}

	requestPath := ""
	if c.Request.URL != nil {
		requestPath = c.Request.URL.Path
	}

	binding := &dbmodel.AsyncTaskBinding{
		TaskID:        taskID,
		TaskType:      videoTaskType,
		UserID:        metaInfo.UserId,
		UserUUID:      dbmodel.StringPtrIfNotEmpty(metaInfo.UserUUID),
		TokenID:       metaInfo.TokenId,
		TokenUUID:     dbmodel.StringPtrIfNotEmpty(metaInfo.TokenUUID),
		ChannelID:     metaInfo.ChannelId,
		ChannelUUID:   dbmodel.StringPtrIfNotEmpty(metaInfo.ChannelUUID),
		ChannelType:   metaInfo.ChannelType,
		OriginModel:   metaInfo.OriginModelName,
		ActualModel:   metaInfo.ActualModelName,
		RequestMethod: c.Request.Method,
		RequestPath:   requestPath,
		RequestParams: requestJSON,
	}

	if err := dbmodel.SaveAsyncTaskBinding(gmw.Ctx(c), binding); err != nil {
		if retryErr := queueBindingRetry(binding, gmw.Ctx(c)); retryErr != nil {
			return errors.Wrapf(err, "persist async task binding %s; durable recovery record failed: %v", taskID, retryErr)
		}
		return errors.Wrapf(err, "persist async task binding %s", taskID)
	}
	pendingBindings.Delete(taskID)
	return nil
}

// RetryPendingTaskBinding retries a queued or durable local task-binding write
// for its owning user. Parameters are the request context, task ID, and
// authenticated user ID. Returns whether a retry record existed and any
// persistence error.
func RetryPendingTaskBinding(ctx context.Context, taskID string, userID int) (bool, error) {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return false, nil
	}
	raw, ok := pendingBindings.Load(taskID)
	if !ok {
		return dbmodel.RecoverAsyncTaskBindingRetry(ctx, taskID, userID)
	}
	queued := raw.(*pendingBinding)
	if userID <= 0 || queued.record.UserID != userID {
		return false, nil
	}
	return true, persistPendingBinding(ctx, taskID, queued)
}

// ReplayPendingTaskBindings replays a bounded set of durable retry records during startup. Parameters: ctx bounds startup database work and limit caps the batch. Returns the number restored and any recovery error.
func ReplayPendingTaskBindings(ctx context.Context, limit int) (int, error) {
	return dbmodel.RecoverAsyncTaskBindingRetries(ctx, limit)
}

// queueBindingRetry durably records a failed binding and schedules short-lived
// in-process retries. It retries only the local routing write; it never resubmits
// the paid upstream task. Returns any durable-record error.
func queueBindingRetry(binding *dbmodel.AsyncTaskBinding, ctx context.Context) error {
	if binding == nil || strings.TrimSpace(binding.TaskID) == "" {
		return errors.New("async task binding retry requires a task id")
	}
	durableErr := dbmodel.SaveAsyncTaskBindingRetry(ctx, binding)
	queued := &pendingBinding{record: *binding}
	if _, loaded := pendingBindings.LoadOrStore(binding.TaskID, queued); loaded {
		return durableErr
	}
	detached := context.Background()
	if ctx != nil {
		detached = context.WithoutCancel(ctx)
	}
	go func() {
		for _, delay := range []time.Duration{50 * time.Millisecond, 250 * time.Millisecond, time.Second} {
			time.Sleep(delay)
			if err := persistPendingBinding(detached, binding.TaskID, queued); err == nil {
				return
			} else if logger := gmw.GetLogger(detached); logger != nil {
				logger.Warn("retry persist async task binding failed",
					zap.Error(err), zap.String("task_id", binding.TaskID))
			}
		}
	}()
	return durableErr
}

// persistPendingBinding saves one queued binding under its mutex. Parameters are the database context, task ID, and queued binding. Returns any database write error and removes the queue entry after success.
func persistPendingBinding(ctx context.Context, taskID string, queued *pendingBinding) error {
	queued.mu.Lock()
	defer queued.mu.Unlock()
	if err := dbmodel.SaveAsyncTaskBinding(ctx, &queued.record); err != nil {
		return err
	}
	if err := dbmodel.DeleteAsyncTaskBindingRetry(ctx, taskID); err != nil {
		return err
	}
	pendingBindings.CompareAndDelete(taskID, queued)
	return nil
}
