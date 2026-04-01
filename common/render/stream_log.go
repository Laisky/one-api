package render

import (
	"bufio"
	"context"

	"github.com/Laisky/errors/v2"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	"github.com/gin-gonic/gin"
)

// heartbeatScannerLogPlan describes how a HeartbeatScanner read error should be logged.
// It captures the log level, message, termination category, and whether the scanner limit
// should be included in the emitted fields.
type heartbeatScannerLogPlan struct {
	level                      zapcore.Level
	message                    string
	termination                string
	includeScannerMaxTokenSize bool
}

// classifyHeartbeatScannerLogPlan classifies a HeartbeatScanner read error.
// It accepts the scanner error and returns the log plan that preserves DEBUG logging for
// client-originated cancellations while keeping real scanner failures at ERROR.
func classifyHeartbeatScannerLogPlan(err error) heartbeatScannerLogPlan {
	switch {
	case errors.Is(err, context.Canceled):
		return heartbeatScannerLogPlan{
			level:       zapcore.DebugLevel,
			message:     "stream stopped after downstream cancellation",
			termination: "client_canceled",
		}
	case errors.Is(err, context.DeadlineExceeded):
		return heartbeatScannerLogPlan{
			level:       zapcore.DebugLevel,
			message:     "stream stopped after downstream deadline",
			termination: "client_deadline_exceeded",
		}
	case errors.Is(err, bufio.ErrTooLong):
		return heartbeatScannerLogPlan{
			level:                      zapcore.ErrorLevel,
			message:                    "stream token exceeded scanner limit",
			termination:                "scanner_token_too_long",
			includeScannerMaxTokenSize: true,
		}
	default:
		return heartbeatScannerLogPlan{
			level:                      zapcore.ErrorLevel,
			message:                    "error reading stream",
			termination:                "unexpected_error",
			includeScannerMaxTokenSize: true,
		}
	}
}

// buildHeartbeatScannerLogFields builds structured fields for a HeartbeatScanner read error.
// It accepts the gin context, scanner error, heartbeat scanner instance, scanner limit, and
// log plan, then returns the fields that should accompany the emitted log entry.
func buildHeartbeatScannerLogFields(c *gin.Context, err error, hbs *HeartbeatScanner, scannerMaxTokenSize int, plan heartbeatScannerLogPlan) []zap.Field {
	fields := []zap.Field{
		zap.NamedError("stream_error", err),
		zap.String("stream_termination", plan.termination),
	}

	if hbs != nil {
		fields = append(fields, zap.Int("heartbeats_sent", hbs.HeartbeatsSent()))
		if heartbeatWriteErr := hbs.HeartbeatWriteErr(); heartbeatWriteErr != nil {
			fields = append(fields, zap.NamedError("heartbeat_write_error", heartbeatWriteErr))
		}
	}

	if c != nil && c.Request != nil {
		if clientCtxErr := c.Request.Context().Err(); clientCtxErr != nil {
			fields = append(fields, zap.String("client_context_state", clientCtxErr.Error()))
		}
	}

	if plan.includeScannerMaxTokenSize {
		fields = append(fields, zap.Int("scanner_max_token_size", scannerMaxTokenSize))
	}

	return fields
}

// LogHeartbeatScannerError logs a HeartbeatScanner read error with the correct severity.
// It accepts the request context, request-scoped logger, scanner error, configured scanner
// limit, and scanner instance, and writes either a DEBUG or ERROR log entry.
func LogHeartbeatScannerError(c *gin.Context, logger glog.Logger, err error, scannerMaxTokenSize int, hbs *HeartbeatScanner) {
	if err == nil || logger == nil {
		return
	}

	plan := classifyHeartbeatScannerLogPlan(err)
	fields := buildHeartbeatScannerLogFields(c, err, hbs, scannerMaxTokenSize, plan)

	switch plan.level {
	case zapcore.DebugLevel:
		logger.Debug(plan.message, fields...)
	default:
		logger.Error(plan.message, fields...)
	}
}
