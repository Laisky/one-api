package middleware

import (
	"net/http"
	"os"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel/attribute"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/helper"
	"github.com/Laisky/one-api/common/tracing"
)

var (
	relayAccessLoggerOnce sync.Once
	relayAccessLogger     *zap.Logger
)

const relayAccessPromptEdgeRunes = 100

// RelayAccessLog records one concise access log for each relay request when enabled.
func RelayAccessLog() gin.HandlerFunc {
	return func(c *gin.Context) {
		startedAt := time.Now().UTC()
		c.Next()

		path := ""
		if c.Request.URL != nil {
			path = c.Request.URL.Path
		}
		status := c.Writer.Status()
		if status == 0 {
			status = http.StatusOK
		}

		fields := []zap.Field{
			zap.String("method", c.Request.Method),
			zap.String("path", path),
			zap.Int("status", status),
			zap.Int64("duration_ms", time.Since(startedAt).Milliseconds()),
			zap.Int("response_bytes", c.Writer.Size()),
			zap.String(tracing.GenAIOperationNameAttr, relayAccessOperationName(path)),
			zap.String(tracing.GenAIProviderNameAttr, "oneapi"),
		}
		requestID := c.GetString(helper.RequestIdKey)
		if requestID == "" {
			requestID = c.GetString(ctxkey.RequestId)
		}
		if requestID != "" {
			fields = append(fields, zap.String("request_id", requestID))
		}
		if traceID := tracing.GetOpenTelemetryTraceID(c); traceID != "" {
			fields = append(fields, zap.String("otel_trace_id", traceID))
		}
		if spanID := tracing.GetOpenTelemetrySpanID(c); spanID != "" {
			fields = append(fields, zap.String("otel_span_id", spanID))
		}
		if requestModel := c.GetString(ctxkey.RequestModel); requestModel != "" {
			fields = append(fields, zap.String(tracing.GenAIRequestModelAttr, requestModel))
		}
		if relayMode := c.GetInt(ctxkey.RelayMode); relayMode != 0 {
			fields = append(fields, zap.Int("oneapi.relay_mode", relayMode))
		}
		if channelID := c.GetInt(ctxkey.ChannelId); channelID != 0 {
			fields = append(fields, zap.Int(tracing.OneAPIChannelIDAttr, channelID))
		}
		if channelUUID := c.GetString(ctxkey.ChannelUUID); channelUUID != "" {
			fields = append(fields, zap.String("oneapi.channel_uuid", channelUUID))
		}
		if channelName := c.GetString(ctxkey.ChannelName); channelName != "" {
			fields = append(fields, zap.String("oneapi.channel_name", channelName))
		}
		if baseURL := c.GetString(ctxkey.BaseURL); baseURL != "" {
			fields = append(fields, zap.String(tracing.OneAPIUpstreamAddressAttr, baseURL))
		}
		if promptExcerpt, truncated := relayAccessPromptExcerpt(c); promptExcerpt != "" {
			fields = append(fields,
				zap.String("gen_ai.input.messages", promptExcerpt),
				zap.Bool("prompt_truncated", truncated),
			)
		}
		if len(c.Errors) > 0 {
			fields = append(fields, zap.String("errors", c.Errors.String()))
		}

		recordRelayAccessSpanAttributes(c, status)
		getRelayAccessLogger().Info("relay access", fields...)
	}
}

// relayAccessPromptExcerpt returns a bounded first/last excerpt of the cached request body.
func relayAccessPromptExcerpt(c *gin.Context) (string, bool) {
	if c == nil {
		return "", false
	}
	raw, ok := c.Get(ctxkey.KeyRequestBody)
	if !ok {
		return "", false
	}
	body, ok := raw.([]byte)
	if !ok || len(body) == 0 {
		return "", false
	}

	text := string(body)
	runeCount := utf8.RuneCountInString(text)
	if runeCount <= relayAccessPromptEdgeRunes*2 {
		return text, false
	}

	runes := []rune(text)
	return string(runes[:relayAccessPromptEdgeRunes]) + "..." + string(runes[runeCount-relayAccessPromptEdgeRunes:]), true
}

// recordRelayAccessSpanAttributes adds non-prompt GenAI and OneAPI attributes to the active span.
func recordRelayAccessSpanAttributes(c *gin.Context, status int) {
	if c == nil || c.Request == nil {
		return
	}

	attrs := []attribute.KeyValue{
		attribute.String(tracing.GenAIOperationNameAttr, relayAccessOperationName(requestPath(c))),
		attribute.String(tracing.GenAIProviderNameAttr, "oneapi"),
		attribute.Int("http.response.status_code", status),
	}
	if requestModel := c.GetString(ctxkey.RequestModel); requestModel != "" {
		attrs = append(attrs, attribute.String(tracing.GenAIRequestModelAttr, requestModel))
	}
	if channelID := c.GetInt(ctxkey.ChannelId); channelID != 0 {
		attrs = append(attrs, attribute.Int(tracing.OneAPIChannelIDAttr, channelID))
	}
	if relayMode := c.GetInt(ctxkey.RelayMode); relayMode != 0 {
		attrs = append(attrs, attribute.Int("oneapi.relay_mode", relayMode))
	}
	if baseURL := c.GetString(ctxkey.BaseURL); baseURL != "" {
		attrs = append(attrs, attribute.String(tracing.OneAPIUpstreamAddressAttr, baseURL))
	}

	tracing.SetSpanAttributes(c.Request.Context(), attrs...)
}

// relayAccessOperationName maps relay paths to the GenAI operation name.
func relayAccessOperationName(path string) string {
	switch path {
	case "/v1/chat/completions", "/v1/messages":
		return "chat"
	case "/v1/completions":
		return "text_completion"
	case "/v1/responses":
		return "response"
	case "/v1/embeddings":
		return "embeddings"
	default:
		return "chat"
	}
}

// requestPath returns the request URL path when it is available.
func requestPath(c *gin.Context) string {
	if c == nil || c.Request == nil || c.Request.URL == nil {
		return ""
	}
	return c.Request.URL.Path
}

// getRelayAccessLogger returns the dedicated JSON logger for relay access logs.
func getRelayAccessLogger() *zap.Logger {
	relayAccessLoggerOnce.Do(func() {
		encoderConfig := zap.NewProductionEncoderConfig()
		encoderConfig.TimeKey = "timestamp"
		encoderConfig.LevelKey = "severity_text"
		encoderConfig.MessageKey = "message"
		encoderConfig.EncodeTime = zapcore.RFC3339TimeEncoder
		encoderConfig.EncodeLevel = zapcore.CapitalLevelEncoder
		core := zapcore.NewCore(
			zapcore.NewJSONEncoder(encoderConfig),
			zapcore.AddSync(os.Stdout),
			zapcore.InfoLevel,
		)
		relayAccessLogger = zap.New(core).Named("one-api.relay_access")
	})
	return relayAccessLogger
}
