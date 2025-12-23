package middleware

import (
	"bytes"
	"io"
	"net/http"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/songquanpeng/one-api/common/config"
	"github.com/songquanpeng/one-api/common/ctxkey"
	"github.com/songquanpeng/one-api/relay/format"
)

// APIFormatAutoDetect returns a middleware that automatically detects the API format
// of incoming requests and handles misrouted requests according to configuration.
//
// When a client sends a request in the wrong format (e.g., Response API format
// to /v1/chat/completions), this middleware can either:
// - Transparently process the request by redirecting it to the correct handler (default)
// - Return a 302 redirect to the correct endpoint
//
// Configuration:
// - AUTO_DETECT_API_FORMAT: Enable/disable this feature (default: true)
// - AUTO_DETECT_API_FORMAT_ACTION: "transparent" (default) or "redirect"
//
// This middleware should be applied to the chat-style API endpoints:
// - /v1/chat/completions
// - /v1/responses
// - /v1/messages
func APIFormatAutoDetect(engine *gin.Engine) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Skip if auto-detection is disabled
		if !config.AutoDetectAPIFormat {
			c.Next()
			return
		}

		lg := gmw.GetLogger(c)
		path := c.Request.URL.Path

		// Only apply to chat-style API endpoints
		expectedFormat := format.FormatFromPath(path)
		if expectedFormat == format.Unknown {
			c.Next()
			return
		}

		// Read the request body for format detection
		bodyBytes, err := io.ReadAll(c.Request.Body)
		if err != nil {
			lg.Warn("failed to read request body for format detection", zap.Error(err))
			c.Next()
			return
		}

		// Restore the body for subsequent handlers
		c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

		// Empty body - let the actual handler deal with it
		if len(bodyBytes) == 0 {
			c.Next()
			return
		}

		// Detect the actual format
		actualFormat, err := format.DetectFormat(bodyBytes)
		if err != nil {
			lg.Debug("failed to detect request format", zap.Error(err))
			c.Set(ctxkey.APIFormat, expectedFormat.String())
			c.Next()
			return
		}

		if actualFormat != format.Unknown {
			c.Set(ctxkey.APIFormat, actualFormat.String())
		} else {
			c.Set(ctxkey.APIFormat, expectedFormat.String())
		}

		// If format is unknown or matches expected, continue normally
		if actualFormat == format.Unknown || actualFormat == expectedFormat {
			c.Next()
			return
		}

		// Format mismatch detected
		lg.Info("detected API format mismatch",
			zap.String("path", path),
			zap.String("expected_format", expectedFormat.String()),
			zap.String("actual_format", actualFormat.String()),
			zap.String("action", config.AutoDetectAPIFormatAction),
		)

		// Handle according to configuration
		switch config.AutoDetectAPIFormatAction {
		case "redirect":
			handleRedirect(c, actualFormat)
		default:
			// Default: transparent processing
			handleTransparent(c, engine, actualFormat, bodyBytes)
		}
	}
}

// handleRedirect sends a 302 redirect to the correct endpoint.
func handleRedirect(c *gin.Context, actualFormat format.APIFormat) {
	targetPath := actualFormat.Endpoint()
	if targetPath == "" {
		c.Next()
		return
	}

	// Preserve query parameters
	if c.Request.URL.RawQuery != "" {
		targetPath += "?" + c.Request.URL.RawQuery
	}

	c.Redirect(http.StatusFound, targetPath)
	c.Abort()
}

// handleTransparent rewrites the request path and re-dispatches to the correct handler.
func handleTransparent(c *gin.Context, engine *gin.Engine, actualFormat format.APIFormat, bodyBytes []byte) {
	targetPath := actualFormat.Endpoint()
	if targetPath == "" {
		c.Next()
		return
	}

	lg := gmw.GetLogger(c)
	originalPath := c.Request.URL.Path

	// Rewrite the request path
	c.Request.URL.Path = targetPath

	// Ensure body is available for the new handler
	c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

	lg.Info("transparently routing mismatched API format",
		zap.String("original_path", originalPath),
		zap.String("target_path", targetPath),
		zap.String("format", actualFormat.String()),
	)

	// Re-dispatch to the engine to invoke the correct handler chain
	engine.HandleContext(c)

	// Stop further processing in the current chain
	c.Abort()
}
