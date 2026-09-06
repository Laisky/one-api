package controller

import (
	"context"
	"net/http"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/helper"
	"github.com/Laisky/one-api/common/identity"
	"github.com/Laisky/one-api/model"
)

// traceAvailabilityNotRetainedLocally identifies a trace that is intentionally
// absent from this node's SQL store because sampling or an external sink owns
// its retention. It is an API contract consumed by Modern's log details UI.
const traceAvailabilityNotRetainedLocally = "not_retained_locally"

// lookupTrace fetches a trace from local SQL without flushing the process-wide
// asynchronous writer.
//
// Under TRACE_WRITE_MODE=batched, a just-finished trace can be temporarily
// unavailable while its normal batch is being written. A lookup must not force
// every queued trace to flush: untrusted callers could otherwise defeat
// batching and create process-wide write amplification. Callers that already
// own the related log receive a clear local-retention state instead.
//
// Parameters:
//   - ctx: request scope for the lookup.
//   - traceId: the trace identifier to fetch.
//
// Return values:
//   - *model.Trace: the trace row.
//   - error: wrapped failure when the trace does not exist.
func lookupTrace(ctx context.Context, traceId string) (*model.Trace, error) {
	return model.GetTraceByTraceId(ctx, traceId)
}

// traceReadAllowed reports whether the authenticated caller can inspect a
// trace. Administrators and root users may inspect all traces; ordinary users
// must own a billing log correlated to the trace.
// Parameters:
//   - c: Gin context containing authenticated role and user id.
//   - ctx: cancellation scope for the ownership query.
//   - traceID: trace correlation identifier to authorize.
//
// Return values:
//   - bool: true when the caller may read the trace.
//   - error: wrapped database failure while checking ordinary-user ownership.
func traceReadAllowed(c *gin.Context, ctx context.Context, traceID string) (bool, error) {
	if c.GetInt(ctxkey.Role) >= model.RoleAdminUser {
		return true, nil
	}

	return model.TraceBelongsToUser(ctx, traceID, c.GetInt(ctxkey.Id))
}

// logReadAllowed reports whether the authenticated caller can inspect a log's
// correlated trace. Administrators and root users may inspect all logs;
// ordinary users may inspect only their own log.
// Parameters:
//   - c: Gin context containing authenticated role and user id.
//   - log: billing log whose trace is being requested.
//
// Return values:
//   - bool: true when the caller may read the log correlation.
func logReadAllowed(c *gin.Context, log *model.Log) bool {
	if log == nil {
		return false
	}
	return c.GetInt(ctxkey.Role) >= model.RoleAdminUser || (c.GetInt(ctxkey.Id) > 0 && c.GetInt(ctxkey.Id) == log.UserId)
}

// localTraceUnavailableResponse builds the successful, explicit response for
// an owned log whose trace is not retained in local SQL. This is expected when
// sampling discards a normal trace or an external trace sink is selected.
// Parameters:
//   - traceID: correlation identifier recorded on the billing log.
//
// Return values:
//   - gin.H: API response payload with the availability contract.
func localTraceUnavailableResponse(traceID string) gin.H {
	return gin.H{
		"success": true,
		"data": gin.H{
			"availability": traceAvailabilityNotRetainedLocally,
			"trace_id":     traceID,
		},
	}
}

// GetTraceByTraceId retrieves tracing information for a specific trace ID
func GetTraceByTraceId(c *gin.Context) {
	lg := gmw.GetLogger(c)
	traceId := c.Param("trace_id")
	if traceId == "" {
		helper.RespondErrorWithStatus(c, http.StatusBadRequest, errors.New("trace_id parameter is required"))
		return
	}

	ctx := gmw.Ctx(c)
	if ctx == nil && c.Request != nil {
		ctx = c.Request.Context()
	}
	allowed, accessErr := traceReadAllowed(c, ctx, traceId)
	if accessErr != nil {
		lg.Error("failed to verify trace ownership", zap.Error(accessErr), zap.String("trace_id", traceId))
		helper.RespondErrorWithStatus(c, http.StatusInternalServerError, errors.New("failed to verify trace access"))
		return
	}
	if !allowed {
		helper.RespondErrorWithStatus(c, http.StatusNotFound, errors.New("trace not found"))
		return
	}

	trace, err := lookupTrace(ctx, traceId)
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			lg.Error("failed to get trace by trace ID", zap.Error(err), zap.String("trace_id", traceId))
		}
		helper.RespondErrorWithStatus(c, http.StatusNotFound, errors.New("trace not found"))
		return
	}

	// Parse timestamps for easier frontend consumption
	timestamps, err := trace.GetTraceTimestamps()
	if err != nil {
		lg.Error("failed to parse trace timestamps",
			zap.Error(err),
			zap.String("trace_id", traceId))
		helper.RespondErrorWithStatus(c, http.StatusInternalServerError, errors.New("failed to parse trace timestamps"))
		return
	}

	// Create response with parsed timestamps
	response := gin.H{
		"success": true,
		"data": gin.H{
			"uuid":       trace.UUID,
			"trace_id":   trace.TraceId,
			"url":        trace.URL,
			"method":     trace.Method,
			"body_size":  trace.BodySize,
			"status":     trace.Status,
			"created_at": trace.CreatedAt,
			"updated_at": trace.UpdatedAt,
			"timestamps": timestamps,
		},
	}

	c.JSON(http.StatusOK, response)
}

// GetTraceByLogId retrieves tracing information for a log entry
func GetTraceByLogId(c *gin.Context) {
	lg := gmw.GetLogger(c)
	logIdStr := c.Param("log_id")
	if logIdStr == "" {
		helper.RespondErrorWithStatus(c, http.StatusBadRequest, errors.New("log_id parameter is required"))
		return
	}

	logId, err := resolveLogRef(logIdStr)
	if err != nil {
		helper.RespondErrorWithStatus(c, http.StatusBadRequest, errors.New("invalid log_id parameter"))
		return
	}

	// Get the log entry to find the trace_id
	log, err := model.GetLogById(logId)
	if err != nil {
		lg.Error("failed to get log by ID",
			zap.Error(err),
			zap.Int("log_id", logId))
		helper.RespondErrorWithStatus(c, http.StatusNotFound, errors.New("log not found"))
		return
	}
	if !logReadAllowed(c, log) {
		helper.RespondErrorWithStatus(c, http.StatusNotFound, errors.New("log not found"))
		return
	}

	if log.TraceId == "" {
		helper.RespondErrorWithStatus(c, http.StatusNotFound, errors.New("no trace information available for this log entry"))
		return
	}

	// Get the trace information
	ctx := gmw.Ctx(c)
	if ctx == nil && c.Request != nil {
		ctx = c.Request.Context()
	}

	trace, err := lookupTrace(ctx, log.TraceId)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusOK, localTraceUnavailableResponse(log.TraceId))
			return
		}
		lg.Error("failed to get trace by trace ID from log", traceLogFields(log, zap.Error(err), zap.String("trace_id", log.TraceId))...)
		helper.RespondErrorWithStatus(c, http.StatusInternalServerError, errors.New("failed to retrieve trace information"))
		return
	}

	// Parse timestamps for easier frontend consumption
	timestamps, err := trace.GetTraceTimestamps()
	if err != nil {
		lg.Error("failed to parse trace timestamps from log",
			traceLogFields(log, zap.Error(err), zap.String("trace_id", log.TraceId))...)
		helper.RespondErrorWithStatus(c, http.StatusInternalServerError, errors.New("failed to parse trace timestamps"))
		return
	}

	// Calculate durations for better UX
	durations := calculateTraceDurations(timestamps)

	// Create response with parsed timestamps and durations
	response := gin.H{
		"success": true,
		"data": gin.H{
			"uuid":       trace.UUID,
			"trace_id":   trace.TraceId,
			"url":        trace.URL,
			"method":     trace.Method,
			"body_size":  trace.BodySize,
			"status":     trace.Status,
			"created_at": trace.CreatedAt,
			"updated_at": trace.UpdatedAt,
			"timestamps": timestamps,
			"durations":  durations,
			"log": gin.H{
				"uuid":         log.UUID,
				"user_uuid":    log.UserUUID,
				"channel_uuid": log.ChannelUUID,
				"username":     log.Username,
				"content":      log.Content,
				"type":         log.Type,
			},
		},
	}

	c.JSON(http.StatusOK, response)
}

// traceLogFields renders the identity of a consume-log row for a request-scoped
// logger. Only the log row's own reference and the channel it billed are emitted:
// the request logger is already bound to the CALLER's user and token identity, and
// zap does not de-duplicate keys, so emitting the log row's user_id/username here
// would double those keys — and silently disagree with the bound values whenever an
// admin inspects another account's log. The log owner is still returned to the
// client in the response payload.
//
// Parameters:
//   - log: the consume-log row being inspected; may be nil.
//   - extra: additional fields appended after the identity fields.
//
// Return values:
//   - []zap.Field: ready-to-log field slice.
func traceLogFields(log *model.Log, extra ...zap.Field) []zap.Field {
	if log == nil {
		return extra
	}

	fields := identity.NewLogRef(log.Id, log.UUID).Zap()
	fields = log.Refs().Channel.AppendZap(fields)
	return append(fields, extra...)
}

// calculateTraceDurations calculates durations between key timestamps
func calculateTraceDurations(timestamps *model.TraceTimestamps) gin.H {
	durations := gin.H{}

	if timestamps.RequestReceived != nil && timestamps.RequestForwarded != nil {
		durations["processing_time"] = *timestamps.RequestForwarded - *timestamps.RequestReceived
	}

	if timestamps.RequestForwarded != nil && timestamps.FirstUpstreamResponse != nil {
		durations["upstream_response_time"] = *timestamps.FirstUpstreamResponse - *timestamps.RequestForwarded
	}

	if timestamps.FirstUpstreamResponse != nil && timestamps.FirstClientResponse != nil {
		durations["response_processing_time"] = *timestamps.FirstClientResponse - *timestamps.FirstUpstreamResponse
	}

	if timestamps.FirstClientResponse != nil && timestamps.UpstreamCompleted != nil {
		durations["streaming_time"] = *timestamps.UpstreamCompleted - *timestamps.FirstClientResponse
	}

	if timestamps.RequestReceived != nil && timestamps.RequestCompleted != nil {
		durations["total_time"] = *timestamps.RequestCompleted - *timestamps.RequestReceived
	}

	return durations
}
