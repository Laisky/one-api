package controller

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/middleware"
	dbmodel "github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay"
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/asyncvideo"
	"github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

// asyncVideoTaskResponse is a stable public DTO. No database row, credentials,
// input payload, upstream ID, channel identity or provider cost is serialized.
type asyncVideoTaskResponse struct {
	ID            string             `json:"id"`
	Object        string             `json:"object"`
	Model         string             `json:"model"`
	Status        string             `json:"status"`
	CreatedAt     int64              `json:"created_at"`
	BillingStatus string             `json:"billing_status"`
	Result        *asyncvideo.Result `json:"result"`
	Error         *asyncVideoError   `json:"error"`
}

// asyncVideoError is a safe, provider-independent public error.
type asyncVideoError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	TaskID  string `json:"task_id,omitempty"`
}

// ReplayAsyncVideoTask reattaches idempotent requests before channel selection
// or balance admission. A spent token may only reattach, never create more work.
// Identical parameters may use either sync or async delivery without resubmission.
func ReplayAsyncVideoTask(c *gin.Context) {
	if c.Request.Method != http.MethodPost || (c.Request.URL.Path != "/v1/async/videos" && c.Request.URL.Path != "/v1/videos/generations") || c.GetHeader("Idempotency-Key") == "" {
		c.Next()
		return
	}
	rawKey := c.GetHeader("Idempotency-Key")
	if len(rawKey) > 256 || strings.TrimSpace(rawKey) != rawKey {
		writeAsyncVideoError(c, http.StatusBadRequest, "invalid_async_video_request", "Invalid Idempotency-Key.", "")
		c.Abort()
		return
	}
	key := dbmodel.AsyncTaskDedupKey(c.GetInt(ctxkey.Id), c.GetString(ctxkey.UserUUID), rawKey)
	task, err := dbmodel.LookupOwnedAsyncTaskReceipt(gmw.Ctx(c), key, c.GetInt(ctxkey.Id), c.GetString(ctxkey.UserUUID))
	if err != nil {
		writeAsyncVideoError(c, http.StatusServiceUnavailable, "async_task_store_unavailable", "Cannot reattach this asynchronous task.", "")
		c.Abort()
		return
	}
	if task != nil {
		// Only an existing durable receipt owns the JSON/hash contract. A new
		// traditional video request must retain its original payload semantics.
		_, hash, err := asyncVideoRequestIdentity(c)
		if err != nil {
			writeAsyncVideoError(c, http.StatusBadRequest, "invalid_async_video_request", "Invalid asynchronous video request.", "")
			c.Abort()
			return
		}
		if subtle.ConstantTimeCompare([]byte(task.RequestHash), []byte(hash)) != 1 {
			writeAsyncVideoError(c, http.StatusConflict, "idempotency_conflict", "Cannot reattach this asynchronous task.", "")
			c.Abort()
			return
		}
		if !asyncVideoTaskAllowed(c, task) {
			writeAsyncVideoError(c, http.StatusForbidden, "model_not_allowed", "Token does not allow this task model.", "")
			c.Abort()
			return
		}
		c.Set(asyncvideo.DurableTaskKey, task.ID)
		deliverAsyncVideoTask(c, task, c.Request.URL.Path == "/v1/videos/generations", time.Duration(config.AsyncVideoWaitSeconds)*time.Second)
		c.Abort()
		return
	}
	if c.GetBool("async_task_reattach_only") {
		writeAsyncVideoError(c, http.StatusForbidden, "insufficient_quota", "A spent token can only retrieve an existing task.", "")
		c.Abort()
		return
	}
	c.Next()
}

// RelayAsyncVideoHelper validates, quotes and atomically reserves a task before
// any paid provider call. After that boundary all outcomes are rendered here:
// a relay failure/HTTP retry must never start another job. syncWait selects final
// result delivery rather than the explicit 202 task API.
func RelayAsyncVideoHelper(c *gin.Context, syncWait bool) *relaymodel.ErrorWithStatusCode {
	if c.Request.Method != http.MethodPost {
		return openai.ErrorWrapper(errors.New("async video creation requires POST"), "invalid_async_video_method", http.StatusMethodNotAllowed)
	}
	originalBody, bodyErr := common.GetRequestBody(c)
	if bodyErr != nil {
		return openai.ErrorWrapper(bodyErr, "invalid_async_video_request", http.StatusBadRequest)
	}
	// A quote may fail before reservation. Preserve the caller's original model
	// and fields for safe selection of another channel; preparation is per attempt.
	defer func() {
		c.Set(ctxkey.KeyRequestBody, originalBody)
		c.Request.Body = io.NopCloser(bytes.NewReader(originalBody))
	}()
	key, hash, err := asyncVideoRequestIdentity(c)
	if err != nil {
		return openai.ErrorWrapper(err, "invalid_async_video_request", http.StatusBadRequest)
	}
	video := &relaymodel.VideoRequest{}
	if err := common.UnmarshalBodyReusable(c, video); err != nil || strings.TrimSpace(video.Model) == "" {
		return openai.ErrorWrapper(errors.New("async video requires a JSON object with model"), "invalid_async_video_request", http.StatusBadRequest)
	}
	info := meta.GetByContext(c)
	info.Mode = relaymode.AsyncVideos
	info.OriginModelName = video.Model
	info.ActualModelName = meta.GetMappedModelName(video.Model, info.ModelMapping)
	video.Model = info.ActualModelName
	ad := relay.GetAdaptor(info.APIType)
	if ad == nil {
		return openai.ErrorWrapper(errors.New("async video adaptor unavailable"), "unsupported_async_video_provider", http.StatusBadRequest)
	}
	if _, ok := ad.(asyncvideo.Provider); !ok {
		return openai.ErrorWrapper(errors.New("channel does not implement durable async video"), "unsupported_async_video_provider", http.StatusBadRequest)
	}
	ad.Init(info)
	meta.Set2Context(c, info)
	images := 0
	if preparer, ok := ad.(adaptor.VideoRequestPreparer); ok {
		images, err = preparer.PrepareVideoRequest(c, video)
		if err != nil {
			return openai.ErrorWrapper(err, "invalid_async_video_request", http.StatusBadRequest)
		}
	}
	quota, costFactor, err := asyncVideoQuotedQuota(c, info, video, images)
	if err != nil {
		return openai.ErrorWrapper(err, "video_pricing_unavailable", http.StatusBadGateway)
	}
	body, err := common.GetRequestBody(c)
	if err != nil {
		return openai.ErrorWrapper(err, "invalid_async_video_request", http.StatusBadRequest)
	}
	record := &dbmodel.AsyncTask{DedupKey: key, RequestHash: hash, UserID: info.UserId, UserUUID: info.UserUUID,
		TokenID: info.TokenId, TokenUUID: info.TokenUUID, ChannelID: info.ChannelId, ChannelUUID: info.ChannelUUID,
		ChannelType: info.ChannelType, BaseURL: info.BaseURL, OriginModel: info.OriginModelName, ActualModel: info.ActualModelName,
		RequestBody: string(body), RequestID: c.GetString(ctxkey.RequestId), Quota: quota, CostQuotaPerUSD: costFactor}
	if traceID, traceErr := gmw.TraceID(c); traceErr == nil {
		record.TraceID = traceID.String()
	}
	task, _, err := dbmodel.ReserveAsyncTask(gmw.Ctx(c), record)
	if err != nil {
		// An uncertain database COMMIT may already own a reservation. Never allow
		// the cross-channel relay retry loop to try a second task in this request.
		status, code := http.StatusServiceUnavailable, "async_task_store_unavailable"
		if errors.Is(err, dbmodel.ErrAsyncIdempotencyConflict) {
			status, code = http.StatusConflict, "idempotency_conflict"
		}
		if errors.Is(err, dbmodel.ErrAsyncQuota) {
			status, code = http.StatusForbidden, "insufficient_quota"
		}
		// ReserveAsyncTask may assign an ID before its transaction rolls back.
		// Only its successful return proves a durable task (including a lost
		// COMMIT acknowledgement recovered by the reservation lookup).
		writeAsyncVideoError(c, status, code, "The asynchronous task could not be admitted; retry only with the same Idempotency-Key.", "")
		return nil
	}
	c.Set(asyncvideo.DurableTaskKey, task.ID)
	asyncvideo.Notify()
	deliverAsyncVideoTask(c, task, syncWait, time.Duration(config.AsyncVideoWaitSeconds)*time.Second)
	return nil
}

// asyncVideoRequestIdentity validates a bounded idempotency key and hashes the
// ORIGINAL client body before provider normalization removes or maps fields.
func asyncVideoRequestIdentity(c *gin.Context) (string, string, error) {
	key := c.GetHeader("Idempotency-Key")
	if len(key) > 256 || strings.TrimSpace(key) != key {
		return "", "", errors.New("invalid Idempotency-Key")
	}
	body, err := common.GetRequestBody(c)
	if err != nil {
		return "", "", errors.Wrap(err, "read async video body")
	}
	hash, err := dbmodel.AsyncTaskRequestHash(body)
	return dbmodel.AsyncTaskDedupKey(c.GetInt(ctxkey.Id), c.GetString(ctxkey.UserUUID), key), hash, err
}

// GetAsyncVideoTask serves owner-scoped durable state without contacting the
// provider. A disabled channel or a spent token must not hide prepaid outputs.
func GetAsyncVideoTask(c *gin.Context) {
	task, err := dbmodel.GetOwnedAsyncTask(gmw.Ctx(c), c.Param("video_id"), c.GetInt(ctxkey.Id), c.GetString(ctxkey.UserUUID))
	if err != nil {
		status, code := http.StatusServiceUnavailable, "async_task_store_unavailable"
		if errors.Is(err, gorm.ErrRecordNotFound) {
			status, code = http.StatusNotFound, "async_task_not_found"
		}
		writeAsyncVideoError(c, status, code, "Cannot retrieve this asynchronous task.", "")
		return
	}
	if !asyncVideoTaskAllowed(c, task) {
		writeAsyncVideoError(c, http.StatusForbidden, "model_not_allowed", "Token does not allow this task model.", "")
		return
	}
	response, err := publicAsyncVideoTask(task)
	if err != nil {
		writeAsyncVideoError(c, http.StatusServiceUnavailable, "async_result_unavailable", "Stored result is unavailable; retry retrieval, not generation.", task.ID)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, response)
}

// asyncVideoTaskAllowed enforces current-token model permissions on an existing
// task. Ownership is independently checked by the database query.
func asyncVideoTaskAllowed(c *gin.Context, task *dbmodel.AsyncTask) bool {
	allowed := c.GetString(ctxkey.AvailableModels)
	return allowed == "" || middleware.IsModelInList(task.OriginModel, allowed)
}

// publicAsyncVideoTask converts private storage into the stable task DTO.
func publicAsyncVideoTask(task *dbmodel.AsyncTask) (asyncVideoTaskResponse, error) {
	status := task.State
	if status == dbmodel.AsyncTaskReserved || status == dbmodel.AsyncTaskSubmitting {
		status = dbmodel.AsyncTaskQueued
	}
	response := asyncVideoTaskResponse{ID: task.ID, Object: "video.task", Model: task.OriginModel, Status: status, CreatedAt: task.CreatedAt / 1000, BillingStatus: task.BillingState}
	if task.State == dbmodel.AsyncTaskCompleted && task.BillingState == dbmodel.AsyncBillingSettled && task.ResultJSON != "" {
		response.Result = &asyncvideo.Result{}
		if err := json.Unmarshal([]byte(task.ResultJSON), response.Result); err != nil {
			return response, errors.Wrap(err, "decode stored video result")
		}
	}
	if task.ErrorCode != "" && task.ErrorCode != "upstream_poll_unavailable" {
		response.Error = &asyncVideoError{Code: task.ErrorCode, Message: "The video task did not complete normally. Reuse this task ID; do not submit it again."}
	}
	return response, nil
}

// deliverAsyncVideoTask waits only for delivery, never owns provider execution.
// Timeout returns 504 + a resumable task ID; disconnect stops the waiter alone.
// Success returns the same data-array shape used by synchronous media generation.
func deliverAsyncVideoTask(c *gin.Context, task *dbmodel.AsyncTask, syncWait bool, wait time.Duration) {
	c.Header("Location", "/v1/async/videos/"+task.ID)
	c.Header("X-Async-Task-Id", task.ID)
	c.Header("Cache-Control", "no-store")
	if !syncWait {
		response, err := publicAsyncVideoTask(task)
		if err != nil {
			writeAsyncVideoError(c, http.StatusServiceUnavailable, "async_result_unavailable", "Stored result is unavailable.", task.ID)
			return
		}
		c.JSON(http.StatusAccepted, response)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), wait)
	defer cancel()
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		switch task.State {
		case dbmodel.AsyncTaskCompleted:
			response, err := publicAsyncVideoTask(task)
			if err != nil || response.Result == nil {
				writeAsyncVideoError(c, http.StatusServiceUnavailable, "async_result_unavailable", "Stored result is unavailable.", task.ID)
				return
			}
			c.JSON(http.StatusOK, gin.H{"created": task.CreatedAt / 1000, "data": response.Result.Videos})
			return
		case dbmodel.AsyncTaskFailed, dbmodel.AsyncTaskCancelled:
			writeAsyncVideoError(c, http.StatusBadGateway, "video_generation_failed", "The upstream video generation did not succeed.", task.ID)
			return
		case dbmodel.AsyncTaskUnknown, dbmodel.AsyncTaskReconciliation:
			writeAsyncVideoError(c, http.StatusBadGateway, "async_task_reconciliation_required", "The existing task requires reconciliation; do not submit a replacement.", task.ID)
			return
		}
		select {
		case <-ctx.Done():
			if c.Request.Context().Err() == nil {
				writeAsyncVideoError(c, http.StatusGatewayTimeout, "async_video_wait_timeout", "The task is still tracked. Retrieve its status or retry with the same Idempotency-Key.", task.ID)
			}
			return
		case <-ticker.C:
			next, err := dbmodel.GetOwnedAsyncTask(ctx, task.ID, task.UserID, task.UserUUID)
			if err != nil {
				if ctx.Err() != nil {
					continue
				}
				writeAsyncVideoError(c, http.StatusServiceUnavailable, "async_task_store_unavailable", "Task state is temporarily unavailable; generation must not be resubmitted.", task.ID)
				return
			}
			task = next
		}
	}
}

// writeAsyncVideoError renders a stable error without exposing provider bodies.
func writeAsyncVideoError(c *gin.Context, status int, code, message, taskID string) {
	c.Header("Cache-Control", "no-store")
	c.JSON(status, gin.H{"error": asyncVideoError{Code: code, Message: message, TaskID: taskID}})
}
