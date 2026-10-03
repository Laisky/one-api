package controller

import (
	"context"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/relay/mcp"
)

const modernMCPStreamContextKey = "one-api.mcp.response-stream"

// forwardedModernMCPToolMeta returns a request-local metadata copy and the capabilities the tools gateway can relay.
// Optional task/subscription/App extensions and deprecated logging are not negotiated by this tools-only gateway.
func forwardedModernMCPToolMeta(meta map[string]any) map[string]any {
	forwarded := make(map[string]any)
	for key, value := range meta {
		if key == "io.modelcontextprotocol/logLevel" {
			continue
		}
		forwarded[key] = value
	}
	capabilities := make(map[string]any)
	if declared, ok := meta[mcp.MetaClientCapabilitiesKey].(map[string]any); ok {
		for _, name := range []string{"elicitation", "sampling", "roots"} {
			if value, exists := declared[name]; exists {
				capabilities[name] = value
			}
		}
	}
	forwarded[mcp.MetaClientCapabilitiesKey] = capabilities
	return forwarded
}

// acceptsModernMCPSSE reports whether the caller explicitly accepts the SSE media type with nonzero quality.
// JSON-only callers retain the buffered JSON result path without receiving unsolicited notifications.
func acceptsModernMCPSSE(headers http.Header) bool {
	for _, header := range headers.Values("Accept") {
		for _, entry := range strings.Split(header, ",") {
			mediaType, parameters, err := mime.ParseMediaType(strings.TrimSpace(entry))
			if err != nil || !strings.EqualFold(mediaType, "text/event-stream") {
				continue
			}
			if quality, exists := parameters["q"]; exists {
				number, err := strconv.ParseFloat(quality, 64)
				if err != nil || !(number > 0 && number <= 1) {
					continue
				}
			}
			return true
		}
	}
	return false
}

// forwardModernMCPProgress lazily starts the authenticated request's SSE response and flushes one notification.
// The synchronous callback provides backpressure; write/cancellation errors stop upstream consumption.
func forwardModernMCPProgress(ctx context.Context, c *gin.Context, notification json.RawMessage) error {
	if err := ctx.Err(); err != nil {
		return errors.Wrap(err, "forward MCP progress")
	}
	if !c.GetBool(modernMCPStreamContextKey) {
		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-store")
		c.Header("X-Accel-Buffering", "no")
		c.Status(http.StatusOK)
		c.Set(modernMCPStreamContextKey, true)
	}
	return writeModernMCPEvent(c.Writer, notification)
}

// writeModernMCPResponse writes either one JSON response or the terminal event of an already-started SSE response.
// A failed write is logged once and cannot cause another upstream execution or a second response encoding.
func writeModernMCPResponse(c *gin.Context, status int, envelope any) {
	if !c.GetBool(modernMCPStreamContextKey) {
		c.JSON(status, envelope)
		return
	}
	if c.Request.Context().Err() != nil {
		return
	}
	encoded, err := json.Marshal(envelope)
	if err == nil {
		err = writeModernMCPEvent(c.Writer, encoded)
	}
	if err != nil {
		logger := gmw.GetLogger(c)
		logger.Debug("MCP response stream delivery failed", zap.Error(err))
	}
}

// writeModernMCPEvent writes and flushes one compact JSON SSE event, detecting short and failed writes.
// Marshaling RawMessage compacts embedded newlines so untrusted JSON cannot inject extra SSE events.
func writeModernMCPEvent(writer http.ResponseWriter, raw json.RawMessage) error {
	encoded, err := json.Marshal(raw)
	if err != nil {
		return errors.Wrap(err, "encode MCP SSE event")
	}
	frame := append([]byte("event: message\ndata: "), encoded...)
	frame = append(frame, '\n', '\n')
	written, err := writer.Write(frame)
	if err != nil {
		return errors.Wrap(err, "write MCP SSE event")
	}
	if written != len(frame) {
		return errors.WithStack(io.ErrShortWrite)
	}
	if err := http.NewResponseController(writer).Flush(); err != nil {
		return errors.Wrap(err, "flush MCP SSE event")
	}
	return nil
}
