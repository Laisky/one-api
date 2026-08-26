package middleware

import (
	"net/http"
	"time"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/helper"
	"github.com/Laisky/one-api/common/tracing"
)

// RelayAccessLog records one concise access log for each relay request when enabled.
func RelayAccessLog() gin.HandlerFunc {
	return func(c *gin.Context) {
		startedAt := time.Now().UTC()
		c.Next()

		lg := gmw.GetLogger(c)
		status := c.Writer.Status()
		if status == 0 {
			status = http.StatusOK
		}

		fields := []zap.Field{
			zap.String("method", c.Request.Method),
			zap.String("path", c.Request.URL.Path),
			zap.Int("status", status),
			zap.Int64("duration_ms", time.Since(startedAt).Milliseconds()),
			zap.Int("response_bytes", c.Writer.Size()),
		}
		if requestID := c.GetString(helper.RequestIdKey); requestID != "" {
			fields = append(fields, zap.String("request_id", requestID))
		}
		if traceID := tracing.GetOpenTelemetryTraceID(c); traceID != "" {
			fields = append(fields, zap.String("trace_id", traceID))
		}
		if spanID := tracing.GetOpenTelemetrySpanID(c); spanID != "" {
			fields = append(fields, zap.String("span_id", spanID))
		}
		if requestModel := c.GetString(ctxkey.RequestModel); requestModel != "" {
			fields = append(fields, zap.String("request_model", requestModel))
		}
		if relayMode := c.GetInt(ctxkey.RelayMode); relayMode != 0 {
			fields = append(fields, zap.Int("relay_mode", relayMode))
		}
		if baseURL := c.GetString(ctxkey.BaseURL); baseURL != "" {
			fields = append(fields, zap.String("upstream_address", baseURL))
		}
		if len(c.Errors) > 0 {
			fields = append(fields, zap.String("errors", c.Errors.String()))
		}

		lg.Info("relay access", fields...)
	}
}
