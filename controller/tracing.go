package controller

import (
	"net/http"
	"strconv"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/songquanpeng/one-api/model"
)

// GetTraceByTraceId retrieves tracing information for a specific trace ID
func GetTraceByTraceId(c *gin.Context) {
	lg := gmw.GetLogger(c)
	traceId := c.Param("trace_id")
	if traceId == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "trace_id parameter is required",
		})
		return
	}

	ctx := gmw.Ctx(c)
	if ctx == nil && c.Request != nil {
		ctx = c.Request.Context()
	}

	trace, err := model.GetTraceByTraceId(ctx, traceId)
	if err != nil {
		lg.Error("failed to get trace by trace ID",
			zap.Error(err),
			zap.String("trace_id", traceId))
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"message": "trace not found",
		})
		return
	}

	// Parse timestamps for easier frontend consumption
	timestamps, err := trace.GetTraceTimestamps()
	if err != nil {
		lg.Error("failed to parse trace timestamps",
			zap.Error(err),
			zap.String("trace_id", traceId))
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "failed to parse trace timestamps",
		})
		return
	}

	// Create response with parsed timestamps
	response := gin.H{
		"success": true,
		"data": gin.H{
			"id":         trace.Id,
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
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "log_id parameter is required",
		})
		return
	}

	logId, err := strconv.Atoi(logIdStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "invalid log_id parameter",
		})
		return
	}

	// Get the log entry to find the trace_id
	log, err := model.GetLogById(logId)
	if err != nil {
		lg.Error("failed to get log by ID",
			zap.Error(err),
			zap.Int("log_id", logId))
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"message": "log not found",
		})
		return
	}

	if log.TraceId == "" {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"message": "no trace information available for this log entry",
		})
		return
	}

	// Get the trace information
	ctx := gmw.Ctx(c)
	if ctx == nil && c.Request != nil {
		ctx = c.Request.Context()
	}

	trace, err := model.GetTraceByTraceId(ctx, log.TraceId)
	if err != nil {
		lg.Error("failed to get trace by trace ID from log",
			zap.Error(err),
			zap.String("trace_id", log.TraceId),
			zap.Int("log_id", logId))
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"message": "trace information not found",
		})
		return
	}

	// Parse timestamps for easier frontend consumption
	timestamps, err := trace.GetTraceTimestamps()
	if err != nil {
		lg.Error("failed to parse trace timestamps from log",
			zap.Error(err),
			zap.String("trace_id", log.TraceId),
			zap.Int("log_id", logId))
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "failed to parse trace timestamps",
		})
		return
	}

	// Calculate durations for better UX
	durations := calculateTraceDurations(timestamps)

	// Create response with parsed timestamps and durations
	response := gin.H{
		"success": true,
		"data": gin.H{
			"id":         trace.Id,
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
				"id":       log.Id,
				"user_id":  log.UserId,
				"username": log.Username,
				"content":  log.Content,
				"type":     log.Type,
			},
		},
	}

	c.JSON(http.StatusOK, response)
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
