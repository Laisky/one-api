package middleware

import (
	"net/http"

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

		// Deferred so a panicking handler still produces a complete trace.
		// gin.Recovery() is registered before this middleware, so its recover
		// runs after ours and the panic still reaches it. At this point Recovery
		// has not written its 500 yet, so record that status explicitly before
		// re-panicking for the outer middleware to handle.
		defer func() {
			if recovered := recover(); recovered != nil {
				tracing.RecordTraceEndWithStatus(c, http.StatusInternalServerError)
				panic(recovered)
			}
			tracing.RecordTraceEnd(c)
		}()

		// Continue processing the request
		c.Next()
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
