package middleware

// Request tracing lifecycle (proposal
// docs/proposals/20260905_observability-data-tiering.md, Phase 1 / W1,
// "Outcome").
//
// This middleware owns the two ends of a request's trace and the one signal the
// trace pipeline cannot observe anywhere else: whether the request FAILED even
// though its HTTP status says otherwise. Once a streaming response has flushed
// its headers the status is pinned at 200, so a later upstream error, timeout,
// or client disconnect is invisible to c.Writer.Status(). Reporting it as a
// semantic failure is what keeps such a trace from being sampled out at
// TRACE_SAMPLE_RATE < 1.

import (
	"context"
	"net/http"

	"github.com/Laisky/errors/v2"
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
				tracing.RecordTraceFailure(c, tracing.FailurePanic)
				tracing.RecordTraceEndWithStatus(c, http.StatusInternalServerError)
				panic(recovered)
			}
			noteRequestContextFailure(c)
			tracing.RecordTraceEnd(c)
		}()

		// Continue processing the request
		c.Next()
	}
}

// noteRequestContextFailure reports a failure the request context observed.
//
// A client that goes away mid-stream and an upstream deadline that expires both
// leave the handler returning normally, often with a 200 already on the wire.
// The request context is the only place the difference is still visible by the
// time the trace ends.
//
// Parameters:
//   - c: the gin context of the finished request; nil or a request-less context
//     is a no-op.
//
// Return values: none.
func noteRequestContextFailure(c *gin.Context) {
	if c == nil || c.Request == nil {
		return
	}
	ctx := c.Request.Context()
	if ctx == nil {
		return
	}

	switch err := ctx.Err(); {
	case err == nil:
		return
	case errors.Is(err, context.DeadlineExceeded):
		tracing.RecordTraceFailure(c, tracing.FailureTimeout)
	case errors.Is(err, context.Canceled):
		tracing.RecordTraceFailure(c, tracing.FailureClientCanceled)
	}
}

// failureKindForStatus maps a late HTTP status onto a semantic failure kind.
//
// Parameters:
//   - status: the status the handler tried to send.
//
// Return values:
//   - tracing.FailureKind: FailureNone for any non-error status.
func failureKindForStatus(status int) tracing.FailureKind {
	switch {
	case status == http.StatusRequestTimeout || status == http.StatusGatewayTimeout:
		return tracing.FailureTimeout
	case status >= 400:
		return tracing.FailureUpstream
	default:
		return tracing.FailureNone
	}
}

// tracingResponseWriter wraps gin.ResponseWriter to capture first response timing
type tracingResponseWriter struct {
	gin.ResponseWriter
	context    *gin.Context
	firstWrite bool
}

// noteFirstWrite records the first client response instant exactly once.
//
// Parameters: none.
//
// Return values: none.
func (w *tracingResponseWriter) noteFirstWrite() {
	if !w.firstWrite {
		return
	}
	w.firstWrite = false
	// Record when we first start sending response to client
	tracing.RecordTraceTimestamp(w.context, model.TimestampFirstClientResponse)
}

// noteLateErrorStatus reports an error status the client will never receive.
//
// gin drops a WriteHeader that arrives after the response body has started: the
// status stays whatever was flushed, normally 200. That is exactly the relay
// path where a stream begins successfully and the upstream then fails, and it is
// why sampling on c.Writer.Status() alone loses those traces. Intercepting the
// call here is the only production-order place the intended status is still
// visible.
//
// Parameters:
//   - statusCode: the status the handler asked for.
//
// Return values: none.
func (w *tracingResponseWriter) noteLateErrorStatus(statusCode int) {
	if !w.ResponseWriter.Written() || statusCode == w.ResponseWriter.Status() {
		return
	}
	tracing.RecordTraceFailure(w.context, failureKindForStatus(statusCode))
}

// Write captures the first write to record when we start sending response to client
func (w *tracingResponseWriter) Write(data []byte) (int, error) {
	w.noteFirstWrite()
	return w.ResponseWriter.Write(data)
}

// WriteHeader captures the first header write, and an error status that arrives
// too late to reach the client.
func (w *tracingResponseWriter) WriteHeader(statusCode int) {
	w.noteLateErrorStatus(statusCode)
	w.noteFirstWrite()
	w.ResponseWriter.WriteHeader(statusCode)
}

// WriteString captures the first string write
func (w *tracingResponseWriter) WriteString(s string) (int, error) {
	w.noteFirstWrite()
	return w.ResponseWriter.WriteString(s)
}
