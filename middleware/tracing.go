package middleware

import (
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/tracing"
)

// TracingMiddleware creates storage-free middleware that records request tracing information.
func TracingMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		tracing.RecordTraceStart(c)

		writer := &tracingResponseWriter{
			ResponseWriter: c.Writer,
			context:        c,
			firstWrite:     true,
		}
		c.Writer = writer

		c.Next()

		tracing.RecordTraceEnd(c)
	}
}

// tracingResponseWriter wraps gin.ResponseWriter to capture first response timing.
type tracingResponseWriter struct {
	gin.ResponseWriter
	context    *gin.Context
	firstWrite bool
}

// Write captures the first write to record when response delivery starts.
func (w *tracingResponseWriter) Write(data []byte) (int, error) {
	if w.firstWrite {
		w.firstWrite = false
		tracing.RecordTraceEvent(w.context, tracing.EventRelayStart)
	}
	return w.ResponseWriter.Write(data)
}

// WriteHeader captures the first header write.
func (w *tracingResponseWriter) WriteHeader(statusCode int) {
	if w.firstWrite {
		w.firstWrite = false
		tracing.RecordTraceEvent(w.context, tracing.EventRelayStart)
	}
	w.ResponseWriter.WriteHeader(statusCode)
}

// WriteString captures the first string write.
func (w *tracingResponseWriter) WriteString(s string) (int, error) {
	if w.firstWrite {
		w.firstWrite = false
		tracing.RecordTraceEvent(w.context, tracing.EventRelayStart)
	}
	return w.ResponseWriter.WriteString(s)
}
