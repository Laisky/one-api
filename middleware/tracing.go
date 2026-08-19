package middleware

import (
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/tracing"
	"github.com/Laisky/one-api/model"
)

// TracingMiddleware creates a middleware that records request tracing information
func TracingMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Record the start of the request
		tracing.RecordTraceStart(c)

		// Use a custom response writer to capture when we start writing the response
		writer := &tracingResponseWriter{
			ResponseWriter: c.Writer,
			context:        c,
			firstWrite:     true,
		}
		c.Writer = writer

		// Continue processing the request
		c.Next()

		// Record the end of the request
		tracing.RecordTraceEnd(c)
	}
}

// tracingResponseWriter wraps gin.ResponseWriter to capture first response timing
type tracingResponseWriter struct {
	gin.ResponseWriter
	context    *gin.Context
	firstWrite bool
}

// Write captures the first write to record when we start sending response to client
func (w *tracingResponseWriter) Write(data []byte) (int, error) {
	if w.firstWrite {
		w.firstWrite = false
		// Record when we first start sending response to client
		tracing.RecordTraceTimestamp(w.context, model.TimestampFirstClientResponse)
	}
	return w.ResponseWriter.Write(data)
}

// WriteHeader captures the first header write
func (w *tracingResponseWriter) WriteHeader(statusCode int) {
	if w.firstWrite {
		w.firstWrite = false
		// Record when we first start sending response to client
		tracing.RecordTraceTimestamp(w.context, model.TimestampFirstClientResponse)
	}
	w.ResponseWriter.WriteHeader(statusCode)
}

// WriteString captures the first string write
func (w *tracingResponseWriter) WriteString(s string) (int, error) {
	if w.firstWrite {
		w.firstWrite = false
		// Record when we first start sending response to client
		tracing.RecordTraceTimestamp(w.context, model.TimestampFirstClientResponse)
	}
	return w.ResponseWriter.WriteString(s)
}
