package render

import (
	"context"

	"github.com/Laisky/errors/v2"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	"github.com/gin-gonic/gin"

	commonsse "github.com/songquanpeng/one-api/common/sse"
)

// heartbeatLineReaderLogPlan describes how a HeartbeatLineReader error should be logged.
type heartbeatLineReaderLogPlan struct {
	level       zapcore.Level
	message     string
	termination string
}

// classifyHeartbeatLineReaderLogPlan classifies a HeartbeatLineReader error.
func classifyHeartbeatLineReaderLogPlan(err error) heartbeatLineReaderLogPlan {
	switch {
	case errors.Is(err, context.Canceled):
		return heartbeatLineReaderLogPlan{
			level:       zapcore.DebugLevel,
			message:     "stream stopped after downstream cancellation",
			termination: "client_canceled",
		}
	case errors.Is(err, context.DeadlineExceeded):
		return heartbeatLineReaderLogPlan{
			level:       zapcore.DebugLevel,
			message:     "stream stopped after downstream deadline",
			termination: "client_deadline_exceeded",
		}
	case errors.Is(err, commonsse.ErrLineTooLong):
		return heartbeatLineReaderLogPlan{
			level:       zapcore.ErrorLevel,
			message:     "stream line exceeded reader limit",
			termination: "malformed_upstream_line",
		}
	default:
		return heartbeatLineReaderLogPlan{
			level:       zapcore.ErrorLevel,
			message:     "error reading stream",
			termination: "unexpected_error",
		}
	}
}

// buildHeartbeatLineReaderLogFields builds structured fields for a HeartbeatLineReader error.
func buildHeartbeatLineReaderLogFields(c *gin.Context, err error, hbr *HeartbeatLineReader, plan heartbeatLineReaderLogPlan) []zap.Field {
	fields := []zap.Field{
		zap.NamedError("stream_error", err),
		zap.String("stream_termination", plan.termination),
	}

	if hbr != nil {
		fields = append(fields, zap.Int("heartbeats_sent", hbr.HeartbeatsSent()))
		if heartbeatWriteErr := hbr.HeartbeatWriteErr(); heartbeatWriteErr != nil {
			fields = append(fields, zap.NamedError("heartbeat_write_error", heartbeatWriteErr))
		}
	}

	if c != nil && c.Request != nil {
		if clientCtxErr := c.Request.Context().Err(); clientCtxErr != nil {
			fields = append(fields, zap.String("client_context_state", clientCtxErr.Error()))
		}
	}

	return fields
}

// LogHeartbeatLineReaderError logs a HeartbeatLineReader error with the correct severity.
func LogHeartbeatLineReaderError(c *gin.Context, logger glog.Logger, err error, hbr *HeartbeatLineReader) {
	if err == nil || logger == nil {
		return
	}

	plan := classifyHeartbeatLineReaderLogPlan(err)
	fields := buildHeartbeatLineReaderLogFields(c, err, hbr, plan)

	switch plan.level {
	case zapcore.DebugLevel:
		logger.Debug(plan.message, fields...)
	default:
		logger.Error(plan.message, fields...)
	}
}
